package web_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"gopkg.in/yaml.v3"
	"vps-node/internal/subscription"
)

func TestCustomNodeCRUDAPI(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	resp, body := e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "ext", "source_type": "links",
		"content": "ss://aes-128-gcm:secret@1.2.3.4:8388#ok\nbad line",
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	created := jsonMap(t, body)
	id := int64(created["id"].(float64))
	if warnings, ok := created["warnings"].([]any); !ok || len(warnings) != 1 {
		t.Fatalf("expected one parse warning for the bad line: %s", body)
	}
	if strings.Contains(body, "secret") || strings.Contains(body, "1.2.3.4") {
		t.Fatalf("content must not be echoed: %s", body)
	}

	resp, body = e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "ext", "source_type": "links", "content": "ss://a:b@2.2.2.2:1",
	}, cookie)
	if resp.StatusCode != http.StatusConflict || errorCode(t, body) != "conflict" {
		t.Fatalf("duplicate name: %d %s", resp.StatusCode, body)
	}

	for name, payload := range map[string]map[string]any{
		"bad source_type": {"name": "x", "source_type": "file", "content": "ss://a:b@2.2.2.2:1"},
		"empty content":   {"name": "x", "source_type": "links", "content": "  "},
		"ftp url":         {"name": "x", "source_type": "subscription", "content": "ftp://example.com/sub"},
		"name too long":   {"name": strings.Repeat("n", 129), "source_type": "links", "content": "ss://a:b@2.2.2.2:1"},
	} {
		resp, body = e.do(t, "POST", "/api/custom-nodes", payload, cookie)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("%s: %d %s", name, resp.StatusCode, body)
		}
	}

	resp, body = e.do(t, "GET", "/api/custom-nodes", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: %d %s", resp.StatusCode, body)
	}
	items := jsonMap(t, body)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("list items: %s", body)
	}
	if strings.Contains(body, "secret") {
		t.Fatalf("list must not echo content: %s", body)
	}

	resp, body = e.do(t, "PUT", "/api/custom-nodes/"+formatID(id), map[string]any{
		"name": "ext-renamed", "status": "disabled",
	}, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["name"] != "ext-renamed" || jsonMap(t, body)["status"] != "disabled" {
		t.Fatalf("update: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "PUT", "/api/custom-nodes/"+formatID(id), map[string]any{"status": "bogus"}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad status: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "PUT", "/api/custom-nodes/9999", map[string]any{"name": "x"}, cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("update missing: %d %s", resp.StatusCode, body)
	}

	resp, _ = e.do(t, "GET", "/api/custom-nodes", nil, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated: %d", resp.StatusCode)
	}

	resp, _ = e.do(t, "DELETE", "/api/custom-nodes/"+formatID(id), nil, cookie)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	resp, _ = e.do(t, "DELETE", "/api/custom-nodes/"+formatID(id), nil, cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("delete missing: %d", resp.StatusCode)
	}
}

func TestCustomNodeSubscriptionMerge(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	user := e.seedUser(t, "cn-merge")
	resp, body := e.do(t, "POST", "/api/users/"+formatID(user)+"/subscription", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatal(body)
	}
	link := extractPath(subscriptionURL(t, body))

	const customLink = "ss://aes-128-gcm:secret@ext.example.com:8388#ext-node"
	resp, body = e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "ext", "source_type": "links", "content": customLink,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatal(body)
	}
	customID := int64(jsonMap(t, body)["id"].(float64))

	fetchGeneral := func() string {
		t.Helper()
		resp, body := e.do(t, "GET", link+"?flag=general", nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("general: %d %s", resp.StatusCode, body)
		}
		decoded, err := base64.StdEncoding.DecodeString(body)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		return string(decoded)
	}
	fetchClash := func() map[string]any {
		t.Helper()
		resp, body := e.do(t, "GET", link+"?flag=clash-meta", nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("clash: %d %s", resp.StatusCode, body)
		}
		var config map[string]any
		if err := yaml.Unmarshal([]byte(body), &config); err != nil {
			t.Fatalf("clash yaml: %v", err)
		}
		return config
	}
	clashHasProxy := func(name string) bool {
		for _, p := range fetchClash()["proxies"].([]any) {
			if p.(map[string]any)["name"] == name {
				return true
			}
		}
		return false
	}

	// Default: not authorized → absent.
	if strings.Contains(fetchGeneral(), "ext-node") || clashHasProxy("ext-node") {
		t.Fatal("unauthorized custom node must not render")
	}

	// Unknown id rejected.
	resp, body = e.do(t, "PUT", "/api/users/"+formatID(user)+"/custom-nodes", map[string]any{"custom_node_ids": []int64{9999}}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unknown custom_node_id: %d %s", resp.StatusCode, body)
	}

	// Authorize → visible in both formats.
	resp, body = e.do(t, "PUT", "/api/users/"+formatID(user)+"/custom-nodes", map[string]any{"custom_node_ids": []int64{customID}}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("authorize: %d %s", resp.StatusCode, body)
	}
	got := jsonMap(t, body)
	if ids := got["custom_node_ids"].([]any); len(ids) != 1 {
		t.Fatalf("authorize response: %s", body)
	}
	if !strings.Contains(fetchGeneral(), customLink) {
		t.Fatalf("general subscription must contain the link verbatim: %s", fetchGeneral())
	}
	if !clashHasProxy("ext-node") {
		t.Fatal("clash subscription must contain the parsed proxy")
	}
	// The proxy joined __ALL_PROXIES__.
	groups := fetchClash()["proxy-groups"].([]any)
	found := false
	for _, g := range groups {
		for _, p := range g.(map[string]any)["proxies"].([]any) {
			if p == "ext-node" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("custom proxy must be referenced by proxy groups")
	}

	// Disable → absent.
	resp, body = e.do(t, "PUT", "/api/custom-nodes/"+formatID(customID), map[string]any{"status": "disabled"}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatal(body)
	}
	if strings.Contains(fetchGeneral(), "ext-node") || clashHasProxy("ext-node") {
		t.Fatal("disabled custom node must not render")
	}

	// Re-enable, then revoke → absent.
	resp, _ = e.do(t, "PUT", "/api/custom-nodes/"+formatID(customID), map[string]any{"status": "active"}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatal("re-enable")
	}
	resp, body = e.do(t, "PUT", "/api/users/"+formatID(user)+"/custom-nodes", map[string]any{"custom_node_ids": []int64{}}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatal(body)
	}
	if strings.Contains(fetchGeneral(), "ext-node") || clashHasProxy("ext-node") {
		t.Fatal("revoked custom node must not render")
	}

	// GET mirrors the authorized set.
	resp, body = e.do(t, "PUT", "/api/users/"+formatID(user)+"/custom-nodes", map[string]any{"custom_node_ids": []int64{customID}}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatal(body)
	}
	resp, body = e.do(t, "GET", "/api/users/"+formatID(user)+"/custom-nodes", nil, cookie)
	if resp.StatusCode != http.StatusOK || len(jsonMap(t, body)["custom_node_ids"].([]any)) != 1 {
		t.Fatalf("get authorizations: %d %s", resp.StatusCode, body)
	}
	// Deleting the custom node cascades the authorization.
	resp, _ = e.do(t, "DELETE", "/api/custom-nodes/"+formatID(customID), nil, cookie)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatal("delete custom node")
	}
	resp, body = e.do(t, "GET", "/api/users/"+formatID(user)+"/custom-nodes", nil, cookie)
	if len(jsonMap(t, body)["custom_node_ids"].([]any)) != 0 {
		t.Fatalf("cascade: %s", body)
	}
}

func TestUserCustomNodeEntryAuthorization(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	user := e.seedUser(t, "cn-entries")
	resp, body := e.do(t, "POST", "/api/users/"+formatID(user)+"/subscription", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatal(body)
	}
	link := extractPath(subscriptionURL(t, body))

	const linkA = "ss://aes-128-gcm:secret@a.example.com:8388#A"
	const linkC = "ss://aes-128-gcm:secret@c.example.com:8388#C"
	resp, body = e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "entries-ext", "source_type": "links",
		"content": linkA + "\n" + linkC + "\nbad line",
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatal(body)
	}
	customID := int64(jsonMap(t, body)["id"].(float64))

	// The entries view attaches a stable key to every parsed entry.
	resp, body = e.do(t, "GET", "/api/custom-nodes/"+formatID(customID)+"/nodes", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("entries view: %d %s", resp.StatusCode, body)
	}
	keyA := ""
	for _, raw := range jsonMap(t, body)["entries"].([]any) {
		entry := raw.(map[string]any)
		if entry["name"] == "A" {
			keyA, _ = entry["key"].(string)
		}
	}
	if len(keyA) != 64 {
		t.Fatalf("entry A must carry a 64-char key: %s", body)
	}
	wantKeyA, ok := subscription.LinkEntryKey(e.appKey, linkA)
	if !ok || keyA != wantKeyA {
		t.Fatalf("entry key = %q, want %q", keyA, wantKeyA)
	}
	// Renaming the connection parameters must not change the key.
	renamedKey, _ := subscription.LinkEntryKey(e.appKey, "ss://aes-128-gcm:secret@a.example.com:8388#A-renamed")
	if renamedKey != keyA {
		t.Fatalf("key must not depend on the name: %q != %q", renamedKey, keyA)
	}
	keyC, _ := subscription.LinkEntryKey(e.appKey, linkC)
	if keyC == keyA || len(keyC) != 64 {
		t.Fatalf("distinct entries need distinct keys: A=%q C=%q", keyA, keyC)
	}

	fetchGeneral := func() string {
		t.Helper()
		resp, body := e.do(t, "GET", link+"?flag=general", nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("general: %d %s", resp.StatusCode, body)
		}
		decoded, err := base64.StdEncoding.DecodeString(body)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		return string(decoded)
	}
	clashNames := func() []string {
		t.Helper()
		resp, body := e.do(t, "GET", link+"?flag=clash-meta", nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("clash: %d %s", resp.StatusCode, body)
		}
		var config map[string]any
		if err := yaml.Unmarshal([]byte(body), &config); err != nil {
			t.Fatalf("clash yaml: %v", err)
		}
		names := []string{}
		for _, p := range config["proxies"].([]any) {
			names = append(names, p.(map[string]any)["name"].(string))
		}
		return names
	}
	hasName := func(names []string, want string) bool {
		for _, name := range names {
			if name == want {
				return true
			}
		}
		return false
	}

	// A source with no whitelist allows every entry (legacy behavior).
	resp, body = e.do(t, "PUT", "/api/users/"+formatID(user)+"/custom-nodes", map[string]any{
		"custom_node_ids": []int64{customID},
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("authorize: %d %s", resp.StatusCode, body)
	}
	general := fetchGeneral()
	if !strings.Contains(general, linkA) || !strings.Contains(general, linkC) || !strings.Contains(general, "bad line") {
		t.Fatalf("empty whitelist must keep every line: %s", general)
	}
	if names := clashNames(); !hasName(names, "A") || !hasName(names, "C") {
		t.Fatalf("empty whitelist must keep every proxy: %v", names)
	}
	if got := jsonMap(t, body)["custom_node_entries"].([]any); len(got) != 0 {
		t.Fatalf("empty whitelist must be omitted from the response: %s", body)
	}

	// A whitelist keeps only A in both formats; the unparseable line is gone.
	resp, body = e.do(t, "PUT", "/api/users/"+formatID(user)+"/custom-nodes", map[string]any{
		"custom_node_ids":     []int64{customID},
		"custom_node_entries": []any{map[string]any{"custom_node_id": customID, "entry_keys": []string{keyA}}},
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("whitelist: %d %s", resp.StatusCode, body)
	}
	general = fetchGeneral()
	if !strings.Contains(general, linkA) || strings.Contains(general, "c.example.com") || strings.Contains(general, "bad line") {
		t.Fatalf("whitelist general output: %s", general)
	}
	if names := clashNames(); !hasName(names, "A") || hasName(names, "C") {
		t.Fatalf("whitelist clash output: %v", names)
	}

	// GET echoes the non-empty whitelist.
	resp, body = e.do(t, "GET", "/api/users/"+formatID(user)+"/custom-nodes", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: %d %s", resp.StatusCode, body)
	}
	got := jsonMap(t, body)["custom_node_entries"].([]any)
	if len(got) != 1 {
		t.Fatalf("get echo: %s", body)
	}
	selection := got[0].(map[string]any)
	if int64(selection["custom_node_id"].(float64)) != customID {
		t.Fatalf("get echo source: %s", body)
	}
	keys := selection["entry_keys"].([]any)
	if len(keys) != 1 || keys[0] != keyA {
		t.Fatalf("get echo keys: %s", body)
	}

	// A rename of the connection (same parameters) stays authorized by key.
	resp, body = e.do(t, "PUT", "/api/custom-nodes/"+formatID(customID), map[string]any{
		"content": "ss://aes-128-gcm:secret@a.example.com:8388#A-renamed\n" + linkC,
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rename: %d %s", resp.StatusCode, body)
	}
	general = fetchGeneral()
	if !strings.Contains(general, "A-renamed") || strings.Contains(general, "c.example.com") {
		t.Fatalf("renamed entry must stay authorized: %s", general)
	}

	// An empty entry_keys list is accepted and means "all entries".
	resp, body = e.do(t, "PUT", "/api/users/"+formatID(user)+"/custom-nodes", map[string]any{
		"custom_node_ids":     []int64{customID},
		"custom_node_entries": []any{map[string]any{"custom_node_id": customID, "entry_keys": []string{}}},
	}, cookie)
	if resp.StatusCode != http.StatusOK || len(jsonMap(t, body)["custom_node_entries"].([]any)) != 0 {
		t.Fatalf("empty entry_keys must mean all entries: %d %s", resp.StatusCode, body)
	}
	if general := fetchGeneral(); !strings.Contains(general, "c.example.com") {
		t.Fatalf("empty entry_keys must restore all entries: %s", general)
	}

	// Validation matrix.
	for name, entries := range map[string]any{
		"unknown source":  []any{map[string]any{"custom_node_id": 9999, "entry_keys": []string{keyA}}},
		"short key":       []any{map[string]any{"custom_node_id": customID, "entry_keys": []string{"abc"}}},
		"non-hex key":     []any{map[string]any{"custom_node_id": customID, "entry_keys": []string{strings.Repeat("z", 64)}}},
		"uppercase key":   []any{map[string]any{"custom_node_id": customID, "entry_keys": []string{strings.ToUpper(keyA)}}},
		"unauthorized id": []any{map[string]any{"custom_node_id": customID, "entry_keys": []string{keyA}}},
	} {
		ids := []int64{customID}
		if name == "unauthorized id" {
			ids = []int64{}
		}
		resp, body = e.do(t, "PUT", "/api/users/"+formatID(user)+"/custom-nodes", map[string]any{
			"custom_node_ids": ids, "custom_node_entries": entries,
		}, cookie)
		if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
			t.Fatalf("%s: %d %s", name, resp.StatusCode, body)
		}
	}

	// A full-set PUT that omits custom_node_entries clears the whitelist, the
	// legacy-client contract (each listed source then means "all entries").
	resp, body = e.do(t, "PUT", "/api/users/"+formatID(user)+"/custom-nodes", map[string]any{
		"custom_node_ids":     []int64{customID},
		"custom_node_entries": []any{map[string]any{"custom_node_id": customID, "entry_keys": []string{keyA}}},
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("re-whitelist: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "PUT", "/api/users/"+formatID(user)+"/custom-nodes", map[string]any{
		"custom_node_ids": []int64{customID},
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("omit entries: %d %s", resp.StatusCode, body)
	}
	if got := jsonMap(t, body)["custom_node_entries"].([]any); len(got) != 0 {
		t.Fatalf("omitting custom_node_entries must clear whitelists: %s", body)
	}
}

func TestCustomNodeSubscriptionEntryWhitelist(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	user := e.seedUser(t, "cn-sub-entries")
	resp, body := e.do(t, "POST", "/api/users/"+formatID(user)+"/subscription", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatal(body)
	}
	link := extractPath(subscriptionURL(t, body))

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("proxies:\n" +
			"  - {name: UP-A, type: ss, server: up-a.example.com, port: 443, cipher: aes-128-gcm, password: pw, udp: true}\n" +
			"  - {name: UP-B, type: ss, server: up-b.example.com, port: 443, cipher: aes-128-gcm, password: pw, udp: true}\n"))
	}))
	defer upstream.Close()

	resp, body = e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "sub-entries", "source_type": "subscription", "content": upstream.URL,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatal(body)
	}
	customID := int64(jsonMap(t, body)["id"].(float64))
	resp, body = e.do(t, "POST", "/api/custom-nodes/"+formatID(customID)+"/refresh", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("refresh: %d %s", resp.StatusCode, body)
	}

	keyA := ""
	for _, raw := range jsonMap(t, body)["entries"].([]any) {
		entry := raw.(map[string]any)
		if entry["name"] == "UP-A" {
			keyA, _ = entry["key"].(string)
		}
	}
	if len(keyA) != 64 {
		t.Fatalf("upstream entry must carry a key: %s", body)
	}

	resp, body = e.do(t, "PUT", "/api/users/"+formatID(user)+"/custom-nodes", map[string]any{
		"custom_node_ids":     []int64{customID},
		"custom_node_entries": []any{map[string]any{"custom_node_id": customID, "entry_keys": []string{keyA}}},
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("whitelist: %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "GET", link+"?flag=clash-meta", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clash: %d %s", resp.StatusCode, body)
	}
	var config map[string]any
	if err := yaml.Unmarshal([]byte(body), &config); err != nil {
		t.Fatalf("clash yaml: %v", err)
	}
	names := []string{}
	for _, p := range config["proxies"].([]any) {
		names = append(names, p.(map[string]any)["name"].(string))
	}
	found := false
	for _, name := range names {
		if name == "UP-A" {
			found = true
		}
		if name == "UP-B" {
			t.Fatalf("unauthorized upstream proxy leaked: %v", names)
		}
	}
	if !found {
		t.Fatalf("authorized upstream proxy missing: %v", names)
	}
}

func TestCustomNodeUpstreamFetchAndCache(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	user := e.seedUser(t, "cn-upstream")
	resp, body := e.do(t, "POST", "/api/users/"+formatID(user)+"/subscription", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatal(body)
	}
	link := extractPath(subscriptionURL(t, body))

	var hits atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		payload := base64.StdEncoding.EncodeToString([]byte("ss://aes-128-gcm:secret@up.example.com:443#up-node\n"))
		_, _ = w.Write([]byte(payload))
	}))

	resp, body = e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "upstream-sub", "source_type": "subscription", "content": upstream.URL,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatal(body)
	}
	customID := int64(jsonMap(t, body)["id"].(float64))
	resp, body = e.do(t, "PUT", "/api/users/"+formatID(user)+"/custom-nodes", map[string]any{"custom_node_ids": []int64{customID}}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatal(body)
	}

	fetchGeneral := func() string {
		t.Helper()
		resp, body := e.do(t, "GET", link+"?flag=general", nil, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("general: %d %s", resp.StatusCode, body)
		}
		decoded, err := base64.StdEncoding.DecodeString(body)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		return string(decoded)
	}

	first := fetchGeneral()
	if !strings.Contains(first, "up-node") {
		t.Fatalf("upstream content must merge: %s", first)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("expected one upstream fetch, got %d", got)
	}
	// Within the TTL the cache is served without refetching.
	if second := fetchGeneral(); !strings.Contains(second, "up-node") || hits.Load() != 1 {
		t.Fatalf("cache not used: %s (hits %d)", second, hits.Load())
	}

	// Upstream down → stale cache still served.
	upstream.Close()
	if third := fetchGeneral(); !strings.Contains(third, "up-node") {
		t.Fatalf("stale cache must be served when upstream is down: %s", third)
	}

	// A fresh subscription-type node with an unreachable upstream and no
	// cache is skipped without failing the render. A closed httptest server
	// refuses the connection immediately (no 5s timeout burn).
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	resp, body = e.do(t, "POST", "/api/custom-nodes", map[string]any{
		"name": "broken-sub", "source_type": "subscription", "content": deadURL,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatal(body)
	}
	brokenID := int64(jsonMap(t, body)["id"].(float64))
	resp, body = e.do(t, "PUT", "/api/users/"+formatID(user)+"/custom-nodes", map[string]any{"custom_node_ids": []int64{customID, brokenID}}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatal(body)
	}
	resp, body = e.do(t, "GET", link+"?flag=general", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("broken upstream must not fail the subscription: %d %s", resp.StatusCode, body)
	}
}
