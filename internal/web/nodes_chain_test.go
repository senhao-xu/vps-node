package web_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func seedSSNode(t *testing.T, e *testEnv, cookie *http.Cookie, serverID int64, name string, port int, chainNodeID any) int64 {
	t.Helper()
	body := map[string]any{
		"server_id": serverID, "address": name + ".example.com", "name": name,
		"protocol": "shadowsocks", "port": port,
		"settings": map[string]any{"cipher": "2022-blake3-aes-128-gcm"},
	}
	if chainNodeID != nil {
		body["chain_node_id"] = chainNodeID
	}
	resp, raw := e.do(t, "POST", "/api/nodes", body, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create node %s: %d %s", name, resp.StatusCode, raw)
	}
	return int64(jsonMap(t, raw)["id"].(float64))
}

func TestNodeChainLifecycle(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverA := e.seedServer(t, "entry-server")
	serverB := e.seedServer(t, "exit-server")

	exitID := seedSSNode(t, e, cookie, serverB, "exit", 20001, nil)
	entryID := seedSSNode(t, e, cookie, serverA, "entry", 10001, exitID)

	revEntry1 := e.revision(t, serverA)
	revExit1 := e.revision(t, serverB)

	// Node detail echoes the chain with display names.
	resp, body := e.do(t, "GET", fmt.Sprintf("/api/nodes/%d", entryID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get node: %d %s", resp.StatusCode, body)
	}
	detail := jsonMap(t, body)
	if int64(detail["chain_node_id"].(float64)) != exitID {
		t.Fatalf("chain_node_id must echo, got %s", body)
	}
	chain := detail["chain_node"].(map[string]any)
	if chain["name"] != "exit" || chain["server_name"] != "exit-server" {
		t.Fatalf("chain_node must carry exit display names, got %s", body)
	}

	// The node list carries the same chain ref.
	resp, body = e.do(t, "GET", "/api/nodes", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list nodes: %d %s", resp.StatusCode, body)
	}
	items := jsonMap(t, body)["items"].([]any)
	found := false
	for _, raw := range items {
		item := raw.(map[string]any)
		if int64(item["id"].(float64)) == entryID {
			found = true
			chain := item["chain_node"].(map[string]any)
			if chain["name"] != "exit" || chain["server_name"] != "exit-server" {
				t.Fatalf("list chain_node mismatch: %v", item)
			}
		}
	}
	if !found {
		t.Fatalf("entry node missing from list: %s", body)
	}

	// Cycle: exit → entry must be rejected.
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", exitID), map[string]any{
		"chain_node_id": entryID,
	}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
		t.Fatalf("cycle must be 422 validation, got %d %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "cycle") {
		t.Fatalf("cycle error must be readable, got %s", body)
	}

	// Missing and disabled targets are rejected.
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", entryID), map[string]any{
		"chain_node_id": 99999,
	}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("missing target must be 422, got %d %s", resp.StatusCode, body)
	}
	disabledID := seedSSNode(t, e, cookie, serverB, "disabled-exit", 20002, nil)
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", disabledID), map[string]any{
		"status": "disabled",
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disable node: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", entryID), map[string]any{
		"chain_node_id": disabledID,
	}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("disabled target must be 422, got %d %s", resp.StatusCode, body)
	}

	// Deleting a referenced exit is blocked with the referencing names.
	resp, body = e.do(t, "DELETE", fmt.Sprintf("/api/nodes/%d", exitID), nil, cookie)
	if resp.StatusCode != http.StatusConflict || errorCode(t, body) != "conflict" {
		t.Fatalf("referenced exit delete must be 409, got %d %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "entry") {
		t.Fatalf("conflict message must name the referencing node, got %s", body)
	}

	// Unlinking (JSON null) bumps both servers and clears the chain.
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", entryID), map[string]any{
		"chain_node_id": nil,
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unlink: %d %s", resp.StatusCode, body)
	}
	if jsonMap(t, body)["chain_node_id"] != nil {
		t.Fatalf("chain must be cleared, got %s", body)
	}
	if e.revision(t, serverA) == revEntry1 || e.revision(t, serverB) == revExit1 {
		t.Fatal("unlinking must bump both server revisions")
	}

	// Now the exit can be deleted.
	resp, body = e.do(t, "DELETE", fmt.Sprintf("/api/nodes/%d", exitID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete unlinked exit: %d %s", resp.StatusCode, body)
	}

	// A partial update without chain_node_id keeps the (now cleared) link as-is.
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", entryID), map[string]any{
		"name": "entry-renamed",
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("rename: %d %s", resp.StatusCode, body)
	}
	if jsonMap(t, body)["chain_node_id"] != nil {
		t.Fatalf("partial update must preserve chain state, got %s", body)
	}
}

func TestAgentConfigChainRendering(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverA := e.seedServer(t, "entry-server")
	serverB := e.seedServer(t, "exit-server")

	exitID := seedSSNode(t, e, cookie, serverB, "exit", 20001, nil)
	entryID := seedSSNode(t, e, cookie, serverA, "entry", 10001, exitID)

	// Entry server config: chain outbound + route rule.
	keyA := e.agentKey(t, cookie, serverA)
	resp, body := e.doAgent(t, "GET", "/api/agent/config?version=0", nil, keyA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("entry config: %d %s", resp.StatusCode, body)
	}
	config := jsonMap(t, body)["config"].(map[string]any)["singbox"].(map[string]any)
	outbounds := config["outbounds"].([]any)
	var chain map[string]any
	for _, raw := range outbounds {
		ob := raw.(map[string]any)
		if ob["tag"] == fmt.Sprintf("chain-%d", entryID) {
			chain = ob
		}
	}
	if chain == nil {
		t.Fatalf("entry config must render a chain outbound: %s", body)
	}
	if chain["server"] != "exit.example.com" || int64(chain["server_port"].(float64)) != 20001 {
		t.Fatalf("chain outbound must dial the exit node, got %v", chain)
	}
	rules := config["route"].(map[string]any)["rules"].([]any)
	rule := rules[0].(map[string]any)
	if rule["outbound"] != fmt.Sprintf("chain-%d", entryID) {
		t.Fatalf("route rule must point at the chain outbound, got %v", rule)
	}

	// Exit server config: relay pseudo user on the inbound.
	keyB := e.agentKey(t, cookie, serverB)
	resp, body = e.doAgent(t, "GET", "/api/agent/config?version=0", nil, keyB)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("exit config: %d %s", resp.StatusCode, body)
	}
	config = jsonMap(t, body)["config"].(map[string]any)["singbox"].(map[string]any)
	users := config["inbounds"].([]any)[0].(map[string]any)["users"].([]any)
	if len(users) != 1 {
		t.Fatalf("exit inbound must carry exactly the relay user, got %s", body)
	}
	relayUser := users[0].(map[string]any)
	wantName := fmt.Sprintf("relay-%d", serverA)
	if relayUser["name"] != wantName {
		t.Fatalf("relay user must be named %s, got %v", wantName, relayUser)
	}

	// Both sides derive the same relay credential.
	inboundPassword := relayUser["password"].(string)
	outboundPassword := chain["password"].(string)
	if !strings.HasSuffix(outboundPassword, ":"+inboundPassword) {
		t.Fatalf("entry outbound password must combine server key with the relay credential, got %q vs %q", outboundPassword, inboundPassword)
	}

	// The relay user never appears in the panel's user payload.
	if strings.Contains(body, "relay-") && agentUserByUUID(t, jsonMap(t, body), "relay-"+fmt.Sprint(serverA)) != nil {
		t.Fatal("relay pseudo users must not leak into the user payload")
	}

	// Unlink → both configs return to direct-only.
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", entryID), map[string]any{
		"chain_node_id": nil,
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unlink: %d %s", resp.StatusCode, body)
	}
	resp, body = e.doAgent(t, "GET", "/api/agent/config?version=0", nil, keyA)
	config = jsonMap(t, body)["config"].(map[string]any)["singbox"].(map[string]any)
	if len(config["outbounds"].([]any)) != 1 {
		t.Fatalf("unlinked entry server must render only the direct outbound: %s", body)
	}
	resp, body = e.doAgent(t, "GET", "/api/agent/config?version=0", nil, keyB)
	config = jsonMap(t, body)["config"].(map[string]any)["singbox"].(map[string]any)
	if users := config["inbounds"].([]any)[0].(map[string]any)["users"].([]any); len(users) != 0 {
		t.Fatalf("unlinked exit server must drop relay users: %s", body)
	}
}

func TestNodeChainExitStillServesDirectUsers(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverA := e.seedServer(t, "entry-server")
	serverB := e.seedServer(t, "exit-server")

	exitID := seedSSNode(t, e, cookie, serverB, "exit", 20001, nil)
	seedSSNode(t, e, cookie, serverA, "entry", 10001, exitID)

	// A direct user on the exit node still gets normal credentials alongside
	// the relay pseudo user.
	userID := e.seedUser(t, "direct-user")
	if err := e.repo.AuthorizeUserNode(t.Context(), userID, exitID); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	keyB := e.agentKey(t, cookie, serverB)
	resp, body := e.doAgent(t, "GET", "/api/agent/config?version=0", nil, keyB)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("exit config: %d %s", resp.StatusCode, body)
	}
	config := jsonMap(t, body)["config"].(map[string]any)["singbox"].(map[string]any)
	users := config["inbounds"].([]any)[0].(map[string]any)["users"].([]any)
	if len(users) != 2 {
		t.Fatalf("exit inbound must serve the direct user plus the relay user, got %s", body)
	}
	names := []string{users[0].(map[string]any)["name"].(string), users[1].(map[string]any)["name"].(string)}
	joined := strings.Join(names, ",")
	if !strings.Contains(joined, fmt.Sprintf("u-%d", userID)) || !strings.Contains(joined, fmt.Sprintf("relay-%d", serverA)) {
		t.Fatalf("unexpected inbound users: %v", names)
	}
}

// Guard: deleting a node that nothing chains to stays allowed.
func TestNodeChainErrorMapping(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "s1")
	nodeID := seedSSNode(t, e, cookie, serverID, "n1", 10001, nil)

	resp, body := e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{
		"chain_node_id": nodeID,
	}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
		t.Fatalf("self chain must be 422 validation, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{
		"chain_node_id": "not-a-number",
	}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("malformed chain_node_id must be 422, got %d %s", resp.StatusCode, body)
	}

	// The exit FK still protects against dangling references at the DB level.
	if err := e.repo.DeleteNode(t.Context(), nodeID); err != nil {
		t.Fatalf("delete node: %v", err)
	}
}
