package web_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func createHTTPNode(t *testing.T, e *testEnv, cookie *http.Cookie, serverID int64, name string, port int, settings map[string]any, chainNodeID any) int64 {
	t.Helper()
	body := map[string]any{
		"server_id": serverID, "address": name + ".example.com", "name": name,
		"protocol": "http", "port": port, "settings": settings,
	}
	if chainNodeID != nil {
		body["chain_node_id"] = chainNodeID
	}
	resp, raw := e.do(t, "POST", "/api/nodes", body, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create http node %s: %d %s", name, resp.StatusCode, raw)
	}
	return int64(jsonMap(t, raw)["id"].(float64))
}

func TestHTTPCreateValidation(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "http")
	now := time.Now()
	cert, key := testTLSMaterial(t, "http.example.com", now.Add(-time.Hour), now.Add(time.Hour))
	_, otherKey := testTLSMaterial(t, "http.example.com", now.Add(-time.Hour), now.Add(time.Hour))
	tlsOK := map[string]any{"tls": map[string]any{"server_name": "http.example.com"}, "certificate": cert, "private_key": key}

	cases := []struct {
		name     string
		settings map[string]any
	}{
		{"unknown field", map[string]any{"cipher": "none"}},
		{"tls without material", map[string]any{"tls": map[string]any{"server_name": "http.example.com"}}},
		{"certificate without key", map[string]any{"tls": map[string]any{"server_name": "http.example.com"}, "certificate": cert}},
		{"mismatched pair", map[string]any{"tls": map[string]any{"server_name": "http.example.com"}, "certificate": cert, "private_key": otherKey}},
		{"cert does not cover sni", map[string]any{"tls": map[string]any{"server_name": "wrong.example.com"}, "certificate": cert, "private_key": key}},
	}
	for i, tc := range cases {
		resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
			"server_id": serverID, "address": "node.example.com", "name": fmt.Sprintf("bad-%d", i),
			"protocol": "http", "port": 30000 + i, "settings": tc.settings,
		}, cookie)
		if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
			t.Fatalf("%s: expected 422 validation, got %d %s", tc.name, resp.StatusCode, body)
		}
	}

	// A plaintext HTTP proxy accepts an empty settings object and keeps {}.
	plainID := createHTTPNode(t, e, cookie, serverID, "http-plain", 8080, map[string]any{}, nil)
	resp, body := e.do(t, "GET", fmt.Sprintf("/api/nodes/%d", plainID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get plain node: %d %s", resp.StatusCode, body)
	}
	if settings := jsonMap(t, body)["settings"].(map[string]any); len(settings) != 0 {
		t.Fatalf("plaintext http settings must stay empty, got %v", settings)
	}

	// A TLS HTTP proxy is accepted and never echoes its secret material.
	resp, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "tls.example.com", "name": "http-tls", "protocol": "http", "port": 8443,
		"settings": tlsOK,
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create tls node: %d %s", resp.StatusCode, body)
	}
	assertNoSecrets(t, body, cert, key)
}

func TestHTTPAgentConfigAndCredential(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()
	serverID := e.seedServer(t, "http")
	now := time.Now()
	cert, key := testTLSMaterial(t, "http.example.com", now.Add(-time.Hour), now.Add(time.Hour))

	plainID := createHTTPNode(t, e, cookie, serverID, "http-plain", 8080, map[string]any{}, nil)
	tlsID := createHTTPNode(t, e, cookie, serverID, "http-tls", 8443, map[string]any{
		"tls": map[string]any{"server_name": "http.example.com"}, "certificate": cert, "private_key": key,
	}, nil)

	userID := e.seedUser(t, "http-user")
	if err := e.repo.AuthorizeUserNode(ctx, userID, plainID); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	token := e.agentKey(t, cookie, serverID)
	resp, body := e.doAgent(t, "GET", "/api/agent/config?version=0", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agent config: %d %s", resp.StatusCode, body)
	}
	m := jsonMap(t, body)
	inbounds := m["config"].(map[string]any)["singbox"].(map[string]any)["inbounds"].([]any)
	var plainInbound map[string]any
	for _, raw := range inbounds {
		inbound := raw.(map[string]any)
		if inbound["tag"] == fmt.Sprintf("http-%d", plainID) {
			plainInbound = inbound
		}
	}
	if plainInbound == nil {
		t.Fatalf("missing plaintext http inbound: %s", body)
	}
	if plainInbound["type"] != "http" || plainInbound["listen"] != "::" || int64(plainInbound["listen_port"].(float64)) != 8080 {
		t.Fatalf("unexpected http inbound: %+v", plainInbound)
	}
	if _, has := plainInbound["tls"]; has {
		t.Fatalf("plaintext http inbound must not carry tls: %+v", plainInbound)
	}
	u, _ := e.repo.GetUser(ctx, userID)
	users := plainInbound["users"].([]any)
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %s", body)
	}
	httpUser := users[0].(map[string]any)
	if httpUser["username"] != fmt.Sprintf("u-%d", userID) || httpUser["password"] != u.UUID {
		t.Fatalf("http user must authenticate with u-<id>/uuid, got %v", httpUser)
	}

	// The TLS node renders an inline-PEM tls block.
	tlsInbound := findInboundByTag(t, inbounds, fmt.Sprintf("http-%d", tlsID))
	if tlsInbound == nil {
		t.Fatalf("missing tls http inbound: %s", body)
	}
	tlsMap, ok := tlsInbound["tls"].(map[string]any)
	if !ok || tlsMap["enabled"] != true || tlsMap["server_name"] != "http.example.com" {
		t.Fatalf("tls http inbound must carry inline tls: %+v", tlsInbound)
	}
	if certLines, ok := tlsMap["certificate"].([]any); !ok || len(certLines) < 2 {
		t.Fatalf("http certificate must be inlined as lines: %+v", tlsMap)
	}

	userDTO := agentUserByUUID(t, m, u.UUID)
	if userDTO == nil {
		t.Fatalf("expected user in payload: %s", body)
	}
	credential := userDTO["nodes"].([]any)[0].(map[string]any)["credential"].(map[string]any)
	if credential["contract"] != "http-v1" || credential["username"] != fmt.Sprintf("u-%d", userID) || credential["password"] != u.UUID {
		t.Fatalf("http credential must follow http-v1, got %v", credential)
	}
}

func findInboundByTag(t *testing.T, inbounds []any, tag string) map[string]any {
	t.Helper()
	for _, raw := range inbounds {
		inbound := raw.(map[string]any)
		if inbound["tag"] == tag {
			return inbound
		}
	}
	return nil
}

func TestHTTPSubscriptionOutput(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()
	serverID := e.seedServer(t, "sub")
	now := time.Now()
	cert, key := testTLSMaterial(t, "http.example.com", now.Add(-time.Hour), now.Add(time.Hour))

	plainID := createHTTPNode(t, e, cookie, serverID, "http-plain", 8080, map[string]any{}, nil)
	tlsID := createHTTPNode(t, e, cookie, serverID, "http-tls", 8443, map[string]any{
		"tls": map[string]any{"server_name": "http.example.com"}, "certificate": cert, "private_key": key,
	}, nil)

	userID := e.seedUser(t, "sub-http-user")
	for _, id := range []int64{plainID, tlsID} {
		if err := e.repo.AuthorizeUserNode(ctx, userID, id); err != nil {
			t.Fatalf("authorize: %v", err)
		}
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
	text := string(decoded)
	plainLink := fmt.Sprintf("http://u-%d:%s@http-plain.example.com:8080#http-plain", userID, u.UUID)
	if !strings.Contains(text, plainLink) {
		t.Fatalf("general subscription must carry %q, got %q", plainLink, text)
	}
	if !strings.Contains(text, "tls=1") || !strings.Contains(text, "sni=http.example.com") {
		t.Fatalf("tls http link must carry tls=1 and sni, got %q", text)
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
	if len(proxies) != 2 {
		t.Fatalf("expected both http proxies, got %v", proxies)
	}
	plainProxy := proxies[0].(map[string]any)
	if plainProxy["type"] != "http" || plainProxy["username"] != fmt.Sprintf("u-%d", userID) ||
		plainProxy["password"] != u.UUID || plainProxy["udp"] != true {
		t.Fatalf("unexpected plaintext http clash proxy: %v", plainProxy)
	}
	tlsProxy := proxies[1].(map[string]any)
	if tlsProxy["tls"] != true || tlsProxy["sni"] != "http.example.com" || tlsProxy["skip-cert-verify"] != false {
		t.Fatalf("unexpected tls http clash proxy: %v", tlsProxy)
	}
}

func TestHTTPChainExitRendersOutbound(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverA := e.seedServer(t, "entry-server")
	serverB := e.seedServer(t, "exit-server")

	exitID := createHTTPNode(t, e, cookie, serverB, "http-exit", 20001, map[string]any{}, nil)
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
		t.Fatalf("entry config must render an http chain outbound: %s", body)
	}
	if chain["type"] != "http" || chain["server"] != "http-exit.example.com" ||
		int64(chain["server_port"].(float64)) != 20001 || chain["username"] != fmt.Sprintf("relay-%d", serverA) {
		t.Fatalf("unexpected http chain outbound: %v", chain)
	}
	if _, has := chain["tls"]; has {
		t.Fatalf("plaintext http exit must not attach tls: %v", chain)
	}
	if _, ok := chain["password"].(string); !ok {
		t.Fatalf("http chain outbound must carry a derived relay password: %v", chain)
	}
}

func TestHTTPNodeShare(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "share")
	nodeID := createHTTPNode(t, e, cookie, serverID, "http-share", 8080, map[string]any{}, nil)
	userID := e.seedUser(t, "share-http-user")
	u, _ := e.repo.GetUser(context.Background(), userID)

	resp, body := e.do(t, "GET", fmt.Sprintf("/api/nodes/%d/share?user_id=%d", nodeID, userID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("share: %d %s", resp.StatusCode, body)
	}
	links, _ := jsonMap(t, body)["links"].([]any)
	if len(links) != 1 {
		t.Fatalf("expected 1 http share link, got %s", body)
	}
	want := fmt.Sprintf("http://u-%d:%s@http-share.example.com:8080#http-share", userID, u.UUID)
	if links[0].(string) != want {
		t.Fatalf("share link = %q, want %q", links[0].(string), want)
	}
}
