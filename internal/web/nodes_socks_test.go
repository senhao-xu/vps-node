package web_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func createSocksNode(t *testing.T, e *testEnv, cookie *http.Cookie, serverID int64, name string, port int, chainNodeID any) int64 {
	t.Helper()
	body := map[string]any{
		"server_id": serverID, "address": name + ".example.com", "name": name,
		"protocol": "socks", "port": port, "settings": map[string]any{},
	}
	if chainNodeID != nil {
		body["chain_node_id"] = chainNodeID
	}
	resp, raw := e.do(t, "POST", "/api/nodes", body, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create socks node %s: %d %s", name, resp.StatusCode, raw)
	}
	return int64(jsonMap(t, raw)["id"].(float64))
}

func TestSocksCreateValidation(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "socks")

	resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "node.example.com", "name": "bad", "protocol": "socks", "port": 1080,
		"settings": map[string]any{"cipher": "none"},
	}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
		t.Fatalf("unknown socks settings must be 422 validation, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "node.example.com", "name": "socks", "protocol": "socks", "port": 1080,
		"settings": map[string]any{},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	nodeID := int64(jsonMap(t, body)["id"].(float64))

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/nodes/%d", nodeID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get node: %d %s", resp.StatusCode, body)
	}
	settings := jsonMap(t, body)["settings"].(map[string]any)
	if len(settings) != 0 {
		t.Fatalf("socks settings must stay empty, got %v", settings)
	}
}

func TestSocksAgentConfigAndCredential(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()
	serverID := e.seedServer(t, "socks")
	nodeID := createSocksNode(t, e, cookie, serverID, "socks", 1080, nil)

	userID := e.seedUser(t, "socks-user")
	if err := e.repo.AuthorizeUserNode(ctx, userID, nodeID); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	token := e.agentKey(t, cookie, serverID)
	resp, body := e.doAgent(t, "GET", "/api/agent/config?version=0", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agent config: %d %s", resp.StatusCode, body)
	}
	m := jsonMap(t, body)
	inbound := m["config"].(map[string]any)["singbox"].(map[string]any)["inbounds"].([]any)[0].(map[string]any)
	if inbound["type"] != "socks" || inbound["tag"] != fmt.Sprintf("socks-%d", nodeID) || inbound["listen"] != "::" {
		t.Fatalf("unexpected socks inbound: %s", body)
	}
	users := inbound["users"].([]any)
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %s", body)
	}
	socksUser := users[0].(map[string]any)
	u, _ := e.repo.GetUser(ctx, userID)
	if socksUser["username"] != fmt.Sprintf("u-%d", userID) || socksUser["password"] != u.UUID {
		t.Fatalf("socks user must authenticate with u-<id>/uuid, got %v", socksUser)
	}

	userDTO := agentUserByUUID(t, m, u.UUID)
	if userDTO == nil {
		t.Fatalf("expected user in payload: %s", body)
	}
	credential := userDTO["nodes"].([]any)[0].(map[string]any)["credential"].(map[string]any)
	if credential["contract"] != "socks-v1" || credential["username"] != fmt.Sprintf("u-%d", userID) || credential["password"] != u.UUID {
		t.Fatalf("socks credential must follow socks-v1, got %v", credential)
	}
}

func TestSocksSubscriptionOutput(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()
	serverID := e.seedServer(t, "sub")
	nodeID := createSocksNode(t, e, cookie, serverID, "socks", 1080, nil)

	userID := e.seedUser(t, "sub-user")
	if err := e.repo.AuthorizeUserNode(ctx, userID, nodeID); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	u, _ := e.repo.GetUser(ctx, userID)

	resp, body := e.do(t, "POST", fmt.Sprintf("/api/users/%d/subscription", userID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("subscription: %d %s", resp.StatusCode, body)
	}
	link := subscriptionURL(t, body)

	resp, body = e.do(t, "GET", extractPath(link)+"?flag=general", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("general subscription: %d %s", resp.StatusCode, body)
	}
	decoded, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		t.Fatalf("general subscription is not base64: %v", err)
	}
	credential := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf("u-%d:%s", userID, u.UUID)))
	wantLink := "socks://" + credential + "@socks.example.com:1080#socks"
	if !strings.Contains(string(decoded), wantLink) {
		t.Fatalf("general subscription must carry %q, got %q", wantLink, string(decoded))
	}

	resp, body = e.do(t, "GET", extractPath(link)+"?flag=clash-meta", nil, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clash subscription: %d %s", resp.StatusCode, body)
	}
	var clash map[string]any
	if err := yaml.Unmarshal([]byte(body), &clash); err != nil {
		t.Fatalf("clash output is not yaml: %v", err)
	}
	proxies, _ := clash["proxies"].([]any)
	if len(proxies) != 1 {
		t.Fatalf("expected exactly the socks proxy, got %v", proxies)
	}
	proxy := proxies[0].(map[string]any)
	if proxy["type"] != "socks5" || proxy["username"] != fmt.Sprintf("u-%d", userID) ||
		proxy["password"] != u.UUID || proxy["udp"] != true {
		t.Fatalf("unexpected socks clash proxy: %v", proxy)
	}
}

func TestSocksChainExitRendersOutbound(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverA := e.seedServer(t, "entry-server")
	serverB := e.seedServer(t, "exit-server")

	exitID := createSocksNode(t, e, cookie, serverB, "socks-exit", 20001, nil)
	entryID := seedSSNode(t, e, cookie, serverA, "entry", 10001, exitID)

	keyA := e.agentKey(t, cookie, serverA)
	resp, body := e.doAgent(t, "GET", "/api/agent/config?version=0", nil, keyA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("entry config: %d %s", resp.StatusCode, body)
	}
	config := jsonMap(t, body)["config"].(map[string]any)["singbox"].(map[string]any)
	var chain map[string]any
	for _, raw := range config["outbounds"].([]any) {
		ob := raw.(map[string]any)
		if ob["tag"] == fmt.Sprintf("chain-%d", entryID) {
			chain = ob
		}
	}
	if chain == nil {
		t.Fatalf("entry config must render a socks chain outbound: %s", body)
	}
	if chain["type"] != "socks" || chain["server"] != "socks-exit.example.com" ||
		int64(chain["server_port"].(float64)) != 20001 || chain["username"] != fmt.Sprintf("relay-%d", serverA) {
		t.Fatalf("unexpected socks chain outbound: %v", chain)
	}
	if _, ok := chain["password"].(string); !ok {
		t.Fatalf("socks chain outbound must carry a derived relay password: %v", chain)
	}
}
