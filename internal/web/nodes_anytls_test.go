package web_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
	"vps-node/internal/repo"
	"vps-node/internal/secrets"
)

func TestAnyTLSCreateValidation(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "anytls")
	now := time.Now()
	cert, key := testTLSMaterial(t, "anytls.example.com", now.Add(-time.Hour), now.Add(time.Hour))
	_, otherKey := testTLSMaterial(t, "anytls.example.com", now.Add(-time.Hour), now.Add(time.Hour))
	expiredCert, expiredKey := testTLSMaterial(t, "anytls.example.com", now.Add(-2*time.Hour), now.Add(-time.Hour))

	tlsSettings := func(name string) map[string]any { return map[string]any{"server_name": name} }
	cases := []struct {
		name     string
		settings map[string]any
	}{
		{"missing all", map[string]any{}},
		{"missing certificate", map[string]any{"tls": tlsSettings("anytls.example.com"), "private_key": key}},
		{"missing private key", map[string]any{"tls": tlsSettings("anytls.example.com"), "certificate": cert}},
		{"missing server name", map[string]any{"certificate": cert, "private_key": key}},
		{"bad pem", map[string]any{"tls": tlsSettings("anytls.example.com"), "certificate": "bad", "private_key": "bad"}},
		{"mismatched pair", map[string]any{"tls": tlsSettings("anytls.example.com"), "certificate": cert, "private_key": otherKey}},
		{"cert does not cover sni", map[string]any{"tls": tlsSettings("wrong.example.com"), "certificate": cert, "private_key": key}},
		{"expired cert", map[string]any{"tls": tlsSettings("anytls.example.com"), "certificate": expiredCert, "private_key": expiredKey}},
		{"unknown field", map[string]any{"tls": tlsSettings("anytls.example.com"), "certificate": cert, "private_key": key, "up_mbps": 100}},
		{"no obfs", map[string]any{"tls": tlsSettings("anytls.example.com"), "certificate": cert, "private_key": key, "obfs_password": "x"}},
		{"no hop ports", map[string]any{"tls": tlsSettings("anytls.example.com"), "certificate": cert, "private_key": key, "hop_ports": "30000-40000"}},
	}
	for i, tc := range cases {
		resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
			"server_id": serverID, "address": "node.example.com", "name": fmt.Sprintf("bad-%d", i), "protocol": "anytls",
			"port": 30000 + i, "settings": tc.settings,
		}, cookie)
		if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
			t.Fatalf("%s: expected 422 validation, got %d %s", tc.name, resp.StatusCode, body)
		}
	}

	resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "node.example.com", "name": "good", "protocol": "anytls", "port": 30100,
		"settings": map[string]any{"tls": map[string]any{"server_name": "anytls.example.com"}, "certificate": cert, "private_key": key},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	created := jsonMap(t, body)
	if created["protocol"] != "anytls" {
		t.Fatalf("unexpected protocol: %s", body)
	}
	assertNoSecrets(t, body, cert, key)
}

func TestAnyTLSUpdatePreservesSecretAndAgentConfig(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()
	serverID := e.seedServer(t, "anytls")
	now := time.Now()
	cert, key := testTLSMaterial(t, "anytls.example.com", now.Add(-time.Hour), now.Add(time.Hour))

	resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "node.example.com", "name": "anytls", "protocol": "anytls", "port": 9443,
		"settings": map[string]any{"tls": map[string]any{"server_name": "anytls.example.com"}, "certificate": cert, "private_key": key},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	nodeID := int64(jsonMap(t, body)["id"].(float64))

	// Empty patch retains the encrypted TLS material and plain settings.
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{"settings": map[string]any{}}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("retain: %d %s", resp.StatusCode, body)
	}
	node, err := e.repo.GetNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	plain, err := secrets.Decrypt(e.appKey, node.SecretEnc)
	if err != nil {
		t.Fatalf("decrypt secret: %v", err)
	}
	storedSecret := map[string]any{}
	if err := json.Unmarshal(plain, &storedSecret); err != nil {
		t.Fatalf("decode secret: %v", err)
	}
	if storedSecret["certificate"] != cert || storedSecret["private_key"] != key {
		t.Fatal("partial update must preserve the encrypted TLS material")
	}
	if !strings.Contains(node.ProtocolSettings, `"server_name":"anytls.example.com"`) {
		t.Fatalf("partial update must preserve public settings, got %s", node.ProtocolSettings)
	}

	// Certificate and private key must be replaced as a pair.
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{"settings": map[string]any{"certificate": cert}}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unpaired certificate replacement: %d %s", resp.StatusCode, body)
	}
	newCert, newKey := testTLSMaterial(t, "anytls.example.com", now.Add(-time.Hour), now.Add(2*time.Hour))
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{"settings": map[string]any{"certificate": newCert, "private_key": newKey}}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("replace pair: %d %s", resp.StatusCode, body)
	}

	// Agent config renders the anytls inbound with uuid passwords and inline TLS.
	userID := e.seedUser(t, "anytls-user")
	if err := e.repo.AuthorizeUserNode(ctx, userID, nodeID); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	token := e.registerAgent(t, cookie, serverID, "1.0.0")
	resp, body = e.doAgent(t, "GET", "/api/agent/config?version=0", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agent config: %d %s", resp.StatusCode, body)
	}
	m := jsonMap(t, body)
	inbound := m["config"].(map[string]any)["singbox"].(map[string]any)["inbounds"].([]any)[0].(map[string]any)
	if inbound["type"] != "anytls" || inbound["tag"] != fmt.Sprintf("anytls-%d", nodeID) {
		t.Fatalf("unexpected inbound: %s", body)
	}
	users := inbound["users"].([]any)
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %s", body)
	}
	anytlsUser := users[0].(map[string]any)
	u, _ := e.repo.GetUser(ctx, userID)
	if anytlsUser["name"] != fmt.Sprintf("u-%d", userID) || anytlsUser["password"] != u.UUID {
		t.Fatalf("anytls user must authenticate with the panel uuid, got %v", anytlsUser)
	}
	tlsMap := inbound["tls"].(map[string]any)
	if tlsMap["enabled"] != true || tlsMap["server_name"] != "anytls.example.com" {
		t.Fatalf("unexpected tls block: %s", body)
	}
	certLines, ok := tlsMap["certificate"].([]any)
	if !ok || len(certLines) < 2 {
		t.Fatalf("certificate must be inlined as lines: %s", body)
	}

	userDTO := agentUserByUUID(t, m, u.UUID)
	if userDTO == nil {
		t.Fatalf("expected user in payload: %s", body)
	}
	credential := userDTO["nodes"].([]any)[0].(map[string]any)["credential"].(map[string]any)
	if credential["contract"] != "uuid-v1" || credential["password"] != u.UUID {
		t.Fatalf("anytls credential must follow uuid-v1 with the uuid as password, got %v", credential)
	}
}

func TestAnyTLSSubscriptionOutput(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()
	serverID := e.seedServer(t, "sub")
	now := time.Now()
	cert, key := testTLSMaterial(t, "anytls.example.com", now.Add(-time.Hour), now.Add(time.Hour))

	resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "node.example.com", "name": "anytls", "protocol": "anytls", "port": 9443,
		"settings": map[string]any{"tls": map[string]any{"server_name": "anytls.example.com"}, "certificate": cert, "private_key": key},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	nodeID := int64(jsonMap(t, body)["id"].(float64))

	// A legacy/incomplete node without TLS material is skipped from subscriptions.
	bareID, err := e.repo.CreateNode(ctx, repo.NewNode{ServerID: serverID, Name: "bare-anytls", Protocol: repo.ProtocolAnyTLS, Port: 9444})
	if err != nil {
		t.Fatalf("seed bare node: %v", err)
	}

	userID := e.seedUser(t, "sub-user")
	for _, id := range []int64{nodeID, bareID} {
		if err := e.repo.AuthorizeUserNode(ctx, userID, id); err != nil {
			t.Fatalf("authorize: %v", err)
		}
	}
	resp, body = e.do(t, "POST", fmt.Sprintf("/api/users/%d/subscription", userID), nil, cookie)
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
	if !strings.Contains(text, "anytls://") || !strings.Contains(text, "sni=anytls.example.com") {
		t.Fatalf("general subscription must carry the anytls link with sni, got %q", text)
	}
	if strings.Contains(text, "bare-anytls") {
		t.Fatalf("node without TLS material must be skipped, got %q", text)
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
		t.Fatalf("expected exactly the healthy anytls proxy, got %v", proxies)
	}
	proxy := proxies[0].(map[string]any)
	if proxy["type"] != "anytls" || proxy["sni"] != "anytls.example.com" || proxy["skip-cert-verify"] != false || proxy["udp"] != true {
		t.Fatalf("unexpected anytls clash proxy: %v", proxy)
	}
	u, _ := e.repo.GetUser(ctx, userID)
	if proxy["password"] != u.UUID {
		t.Fatalf("clash proxy password must be the user uuid, got %v", proxy["password"])
	}
}
