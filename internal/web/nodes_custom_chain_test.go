package web_test

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

const testExternalSSLink = "ss://aes-128-gcm:ext-secret@203.0.113.9:8388#EXT-SS"

func createLinksCustomNode(t *testing.T, e *testEnv, cookie *http.Cookie, name, content string) int64 {
	t.Helper()
	resp, body := e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": name, "source_type": "links", "content": content,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create custom node %s: %d %s", name, resp.StatusCode, body)
	}
	return int64(jsonMap(t, body)["id"].(float64))
}

func customNodeEntry(t *testing.T, e *testEnv, cookie *http.Cookie, id int64, name string) map[string]any {
	t.Helper()
	resp, body := e.do(t, "GET", "/api/custom-nodes/"+formatID(id)+"/nodes", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("custom entries: %d %s", resp.StatusCode, body)
	}
	for _, raw := range jsonMap(t, body)["entries"].([]any) {
		entry := raw.(map[string]any)
		if entry["name"] == name {
			return entry
		}
	}
	t.Fatalf("entry %q not found: %s", name, body)
	return nil
}

// configOutboundByTag returns the rendered outbound with the given tag, or nil.
func configOutboundByTag(t *testing.T, body, tag string) map[string]any {
	t.Helper()
	config := jsonMap(t, body)["config"].(map[string]any)["singbox"].(map[string]any)
	for _, raw := range config["outbounds"].([]any) {
		ob := raw.(map[string]any)
		if ob["tag"] == tag {
			return ob
		}
	}
	return nil
}

func TestAgentConfigExternalChainRendering(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "external-entry")

	sourceID := createLinksCustomNode(t, e, cookie, "external-links", testExternalSSLink+"\n"+
		"trojan://tpass@203.0.113.10:443?sni=t.example.com#EXT-TROJAN")
	ssEntry := customNodeEntry(t, e, cookie, sourceID, "EXT-SS")
	if ssEntry["chain_supported"] != true {
		t.Fatalf("ss entry must be chain_supported: %v", ssEntry)
	}
	key := ssEntry["key"].(string)

	resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "entry.example.com", "name": "entry-ext",
		"protocol": "shadowsocks", "port": 10001,
		"settings":               map[string]any{"cipher": "2022-blake3-aes-128-gcm"},
		"chain_custom_node_id":   sourceID,
		"chain_custom_entry_key": key,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create entry: %d %s", resp.StatusCode, body)
	}
	created := jsonMap(t, body)
	entryID := int64(created["id"].(float64))
	if int64(created["chain_custom_node_id"].(float64)) != sourceID || created["chain_custom_entry_key"] != key {
		t.Fatalf("create response must echo the external target: %s", body)
	}
	if created["chain_custom_node_name"] != "external-links" {
		t.Fatalf("create response must echo the source name: %s", body)
	}
	if created["chain_node_id"] != nil {
		t.Fatalf("managed target must stay empty: %s", body)
	}

	agentKey := e.agentKey(t, cookie, serverID)
	resp, body = e.doAgent(t, "GET", "/api/agent/config?version=0", nil, agentKey)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agent config: %d %s", resp.StatusCode, body)
	}
	chain := configOutboundByTag(t, body, fmt.Sprintf("chain-%d", entryID))
	if chain == nil {
		t.Fatalf("entry config must render the external chain outbound: %s", body)
	}
	if chain["type"] != "shadowsocks" || chain["server"] != "203.0.113.9" || int64(chain["server_port"].(float64)) != 8388 {
		t.Fatalf("external outbound must dial the entry server/port, got %v", chain)
	}
	if chain["method"] != "aes-128-gcm" || chain["password"] != "ext-secret" {
		t.Fatalf("external outbound must carry the entry credentials, got %v", chain)
	}
	config := jsonMap(t, body)["config"].(map[string]any)["singbox"].(map[string]any)
	rules := config["route"].(map[string]any)["rules"].([]any)
	if len(rules) != 1 || rules[0].(map[string]any)["outbound"] != fmt.Sprintf("chain-%d", entryID) {
		t.Fatalf("route rule must point at the external chain outbound: %s", body)
	}

	// External exits are not panel agents: no relay pseudo user is injected and
	// the entry node still serves direct users normally.
	inbound := config["inbounds"].([]any)[0].(map[string]any)
	if users := inbound["users"].([]any); len(users) != 0 {
		t.Fatalf("external exit must not inject relay users, got %s", body)
	}
}

func TestAgentConfigExternalChainFallbacks(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "fallback-entry")

	// A subscription source without a fetch cache contributes no entries.
	var hits atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(testExternalSSLink + "\n"))))
	}))
	defer upstream.Close()
	resp, body := e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "no-cache", "source_type": "subscription", "content": upstream.URL,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create subscription source: %d %s", resp.StatusCode, body)
	}
	noCacheID := int64(jsonMap(t, body)["id"].(float64))

	resp, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "a.example.com", "name": "no-cache-entry",
		"protocol": "shadowsocks", "port": 11001,
		"settings":             map[string]any{"cipher": "2022-blake3-aes-128-gcm"},
		"chain_custom_node_id": noCacheID, "chain_custom_entry_key": strings.Repeat("a", 64),
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create no-cache entry: %d %s", resp.StatusCode, body)
	}
	noCacheEntryID := int64(jsonMap(t, body)["id"].(float64))

	agentKey := e.agentKey(t, cookie, serverID)
	resp, body = e.doAgent(t, "GET", "/api/agent/config?version=0", nil, agentKey)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agent config: %d %s", resp.StatusCode, body)
	}
	if chain := configOutboundByTag(t, body, fmt.Sprintf("chain-%d", noCacheEntryID)); chain != nil {
		t.Fatalf("no-cache source must fall back to direct: %s", body)
	}
	if hits.Load() != 0 {
		t.Fatalf("config build must not fetch upstream, hits=%d", hits.Load())
	}

	// A links source with a resolvable entry renders the chain.
	sourceID := createLinksCustomNode(t, e, cookie, "live-links", testExternalSSLink)
	key := customNodeEntry(t, e, cookie, sourceID, "EXT-SS")["key"].(string)
	resp, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "b.example.com", "name": "live-entry",
		"protocol": "shadowsocks", "port": 11002,
		"settings":               map[string]any{"cipher": "2022-blake3-aes-128-gcm"},
		"chain_custom_node_id":   sourceID,
		"chain_custom_entry_key": key,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create live entry: %d %s", resp.StatusCode, body)
	}
	liveEntryID := int64(jsonMap(t, body)["id"].(float64))

	chainTag := fmt.Sprintf("chain-%d", liveEntryID)
	resp, body = e.doAgent(t, "GET", "/api/agent/config?version=0", nil, agentKey)
	if chain := configOutboundByTag(t, body, chainTag); chain == nil {
		t.Fatalf("resolvable entry must render the chain: %s", body)
	}

	// A changed key no longer matches → direct fallback.
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", liveEntryID), map[string]any{
		"chain_custom_node_id": sourceID, "chain_custom_entry_key": strings.Repeat("0", 64),
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("retarget key: %d %s", resp.StatusCode, body)
	}
	resp, body = e.doAgent(t, "GET", "/api/agent/config?version=0", nil, agentKey)
	if chain := configOutboundByTag(t, body, chainTag); chain != nil {
		t.Fatalf("mismatched key must fall back to direct: %s", body)
	}

	// Restore the key, then disable the source → fallback.
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", liveEntryID), map[string]any{
		"chain_custom_node_id": sourceID, "chain_custom_entry_key": key,
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restore key: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "PUT", "/api/custom-nodes/"+formatID(sourceID), map[string]any{"status": "disabled"}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disable source: %d %s", resp.StatusCode, body)
	}
	resp, body = e.doAgent(t, "GET", "/api/agent/config?version=0", nil, agentKey)
	if chain := configOutboundByTag(t, body, chainTag); chain != nil {
		t.Fatalf("disabled source must fall back to direct: %s", body)
	}

	// Re-enabling the source restores the chain, but removing the line by
	// changing the content changes its key → fallback again.
	resp, body = e.do(t, "PUT", "/api/custom-nodes/"+formatID(sourceID), map[string]any{"status": "active"}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("enable source: %d %s", resp.StatusCode, body)
	}
	resp, body = e.doAgent(t, "GET", "/api/agent/config?version=0", nil, agentKey)
	if chain := configOutboundByTag(t, body, chainTag); chain == nil {
		t.Fatalf("re-enabled source must render the chain: %s", body)
	}
	resp, body = e.do(t, "PUT", "/api/custom-nodes/"+formatID(sourceID), map[string]any{
		"content": "ss://aes-128-gcm:other-secret@203.0.113.11:8388#EXT-SS-MOVED",
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("replace content: %d %s", resp.StatusCode, body)
	}
	resp, body = e.doAgent(t, "GET", "/api/agent/config?version=0", nil, agentKey)
	if chain := configOutboundByTag(t, body, chainTag); chain != nil {
		t.Fatalf("removed line must fall back to direct: %s", body)
	}
}

func TestNodeExternalChainValidationAndTriState(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "validation-entry")
	managedExitID := seedSSNode(t, e, cookie, serverID, "managed-exit", 12001, nil)

	sourceID := createLinksCustomNode(t, e, cookie, "validation-links", testExternalSSLink)
	key := customNodeEntry(t, e, cookie, sourceID, "EXT-SS")["key"].(string)

	disabledID := createLinksCustomNode(t, e, cookie, "disabled-links", testExternalSSLink)
	resp, body := e.do(t, "PUT", "/api/custom-nodes/"+formatID(disabledID), map[string]any{"status": "disabled"}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disable source: %d %s", resp.StatusCode, body)
	}

	base := func(extra map[string]any) map[string]any {
		payload := map[string]any{
			"server_id": serverID, "address": "v.example.com", "name": "v-node",
			"protocol": "shadowsocks", "port": 13000,
			"settings": map[string]any{"cipher": "2022-blake3-aes-128-gcm"},
		}
		for k, v := range extra {
			payload[k] = v
		}
		return payload
	}

	badCreate := []map[string]any{
		{"chain_node_id": managedExitID, "chain_custom_node_id": sourceID, "chain_custom_entry_key": key},
		{"chain_custom_node_id": 99999, "chain_custom_entry_key": key},
		{"chain_custom_node_id": disabledID, "chain_custom_entry_key": key},
		{"chain_custom_node_id": sourceID, "chain_custom_entry_key": "not-a-key"},
		{"chain_custom_node_id": sourceID},
	}
	for i, extra := range badCreate {
		resp, body := e.do(t, "POST", "/api/nodes", base(extra), cookie)
		if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
			t.Fatalf("create case %d must be 422 validation, got %d %s", i, resp.StatusCode, body)
		}
	}

	resp, body = e.do(t, "POST", "/api/nodes", base(map[string]any{"chain_custom_node_id": sourceID, "chain_custom_entry_key": key}), cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("valid external chain: %d %s", resp.StatusCode, body)
	}
	nodeID := int64(jsonMap(t, body)["id"].(float64))

	// Setting both targets on update is a conflict.
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{
		"chain_node_id": managedExitID, "chain_custom_node_id": sourceID, "chain_custom_entry_key": key,
	}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("dual targets must be 422, got %d %s", resp.StatusCode, body)
	}

	// Clearing only the custom target.
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{"chain_custom_node_id": nil}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear custom: %d %s", resp.StatusCode, body)
	}
	cleared := jsonMap(t, body)
	if cleared["chain_custom_node_id"] != nil || cleared["chain_custom_entry_key"] != "" {
		t.Fatalf("custom target must be cleared: %s", body)
	}

	// Switching to a managed exit clears the external target.
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{"chain_node_id": managedExitID}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set managed: %d %s", resp.StatusCode, body)
	}
	managed := jsonMap(t, body)
	if int64(managed["chain_node_id"].(float64)) != managedExitID || managed["chain_custom_node_id"] != nil {
		t.Fatalf("managed target must replace the external one: %s", body)
	}

	// Switching back to an external target clears the managed one and echoes
	// the source name.
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{
		"chain_custom_node_id": sourceID, "chain_custom_entry_key": key,
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set external: %d %s", resp.StatusCode, body)
	}
	external := jsonMap(t, body)
	if int64(external["chain_custom_node_id"].(float64)) != sourceID || external["chain_node_id"] != nil {
		t.Fatalf("external target must replace the managed one: %s", body)
	}
	if external["chain_custom_entry_key"] != key || external["chain_custom_node_name"] != "validation-links" {
		t.Fatalf("external target must echo key/name: %s", body)
	}

	// An unrelated partial update keeps the external target.
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{"name": "renamed"}, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["chain_custom_node_id"] == nil {
		t.Fatalf("partial update must retain the external target: %d %s", resp.StatusCode, body)
	}

	// Explicit null on both targets clears everything.
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{
		"chain_node_id": nil, "chain_custom_node_id": nil,
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear both: %d %s", resp.StatusCode, body)
	}
	if m := jsonMap(t, body); m["chain_node_id"] != nil || m["chain_custom_node_id"] != nil {
		t.Fatalf("both targets must be cleared: %s", body)
	}
}

func TestCustomNodeDeleteProtectionAndRevisionBumps(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "ref-server")

	sourceID := createLinksCustomNode(t, e, cookie, "referenced", testExternalSSLink)
	key := customNodeEntry(t, e, cookie, sourceID, "EXT-SS")["key"].(string)
	resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "r.example.com", "name": "referencing-entry",
		"protocol": "shadowsocks", "port": 14001,
		"settings":               map[string]any{"cipher": "2022-blake3-aes-128-gcm"},
		"chain_custom_node_id":   sourceID,
		"chain_custom_entry_key": key,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create referencing node: %d %s", resp.StatusCode, body)
	}
	refNodeID := int64(jsonMap(t, body)["id"].(float64))

	// Deleting the referenced source is blocked and names the entry node.
	resp, body = e.do(t, "DELETE", "/api/custom-nodes/"+formatID(sourceID), nil, cookie)
	if resp.StatusCode != http.StatusConflict || errorCode(t, body) != "conflict" {
		t.Fatalf("referenced source delete must be 409, got %d %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "referencing-entry") {
		t.Fatalf("conflict must name the entry node: %s", body)
	}

	// Updating the referenced source bumps the entry node's server.
	revBefore := e.revision(t, serverID)
	resp, body = e.do(t, "PUT", "/api/custom-nodes/"+formatID(sourceID), map[string]any{"name": "referenced-renamed"}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update source: %d %s", resp.StatusCode, body)
	}
	if e.revision(t, serverID) <= revBefore {
		t.Fatal("updating a referenced source must bump the entry server revision")
	}

	// Unlink, then the source deletes cleanly (204).
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", refNodeID), map[string]any{"chain_custom_node_id": nil}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unlink: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "DELETE", "/api/custom-nodes/"+formatID(sourceID), nil, cookie)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("unreferenced source delete must be 204, got %d %s", resp.StatusCode, body)
	}

	// A refresh of a referenced subscription source bumps the entry server.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(testExternalSSLink + "\n"))))
	}))
	defer upstream.Close()
	resp, body = e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "sub-bump", "source_type": "subscription", "content": upstream.URL,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create subscription: %d %s", resp.StatusCode, body)
	}
	subID := int64(jsonMap(t, body)["id"].(float64))
	resp, body = e.do(t, "POST", "/api/custom-nodes/"+formatID(subID)+"/refresh", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh: %d %s", resp.StatusCode, body)
	}
	subKey := customNodeEntry(t, e, cookie, subID, "EXT-SS")["key"].(string)
	resp, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "s.example.com", "name": "sub-entry",
		"protocol": "shadowsocks", "port": 14002,
		"settings":               map[string]any{"cipher": "2022-blake3-aes-128-gcm"},
		"chain_custom_node_id":   subID,
		"chain_custom_entry_key": subKey,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create sub entry: %d %s", resp.StatusCode, body)
	}
	revBefore = e.revision(t, serverID)
	resp, body = e.do(t, "POST", "/api/custom-nodes/"+formatID(subID)+"/refresh", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second refresh: %d %s", resp.StatusCode, body)
	}
	if e.revision(t, serverID) <= revBefore {
		t.Fatal("refreshing a referenced source must bump the entry server revision")
	}

	// Deleting the referenced subscription source is blocked too.
	resp, body = e.do(t, "DELETE", "/api/custom-nodes/"+formatID(subID), nil, cookie)
	if resp.StatusCode != http.StatusConflict || !strings.Contains(body, "sub-entry") {
		t.Fatalf("referenced subscription delete must be 409 naming the entry, got %d %s", resp.StatusCode, body)
	}
}

func TestCustomNodeEntriesChainSupported(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	// A supported link type.
	linksID := createLinksCustomNode(t, e, cookie, "supported", testExternalSSLink)
	if got := customNodeEntry(t, e, cookie, linksID, "EXT-SS")["chain_supported"]; got != true {
		t.Fatalf("ss must be chain_supported, got %v", got)
	}

	// An upstream Clash proxy of a type the converter cannot render.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("proxies:\n  - name: plain-http\n    type: http\n    server: 198.51.100.7\n    port: 8080\n"))
	}))
	defer upstream.Close()
	resp, body := e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "upstream-http", "source_type": "subscription", "content": upstream.URL,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create subscription: %d %s", resp.StatusCode, body)
	}
	id := int64(jsonMap(t, body)["id"].(float64))
	resp, body = e.do(t, "POST", "/api/custom-nodes/"+formatID(id)+"/refresh", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh: %d %s", resp.StatusCode, body)
	}
	entry := customNodeEntry(t, e, cookie, id, "plain-http")
	if entry["type"] != "http" || entry["chain_supported"] != false {
		t.Fatalf("http must not be chain_supported: %v", entry)
	}
}
