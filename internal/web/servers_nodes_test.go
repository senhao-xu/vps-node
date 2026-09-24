package web_test

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"vps-node/internal/adminauth"
	"vps-node/internal/repo"
	"vps-node/internal/secrets"
)

func TestNodeCRUDOwnershipAndSecretExposure(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": 99999, "address": "node.example.com", "name": "n", "protocol": "vless", "port": 443,
	}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
		t.Fatalf("unknown server must be 422 validation, got %d %s", resp.StatusCode, body)
	}

	serverID := e.seedServer(t, "s1")
	resp, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "node.example.com", "name": "hk-ss", "protocol": "shadowsocks", "port": 8388,
		"settings": map[string]any{"cipher": "2022-blake3-aes-128-gcm", "password": "secret-password"},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create node: %d %s", resp.StatusCode, body)
	}
	created := jsonMap(t, body)
	nodeID := int64(created["id"].(float64))
	if created["status"] != "active" || created["protocol"] != "shadowsocks" || created["port"].(float64) != 8388 {
		t.Fatalf("unexpected node payload: %s", body)
	}
	assertNoSecrets(t, body, "secret-password")

	resp, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "node.example.com", "name": "dup-port", "protocol": "vless", "port": 8388,
		"settings": map[string]any{"private_key": testRealityPrivateKey(t), "reality_settings": map[string]any{"server_name": "example.com"}},
	}, cookie)
	if resp.StatusCode != http.StatusConflict || errorCode(t, body) != "conflict" {
		t.Fatalf("port conflict must be 409, got %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "node.example.com", "name": "hk-ss", "protocol": "vless", "port": 9000,
		"settings": map[string]any{"private_key": testRealityPrivateKey(t), "reality_settings": map[string]any{"server_name": "example.com"}},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("duplicate name must be allowed (node copy), got %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "node.example.com", "name": "bad", "protocol": "snell", "port": 9001,
	}, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad protocol must be 400, got %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "node.example.com", "name": "bad", "protocol": "vless", "port": 70000,
	}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad port must be 422, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/nodes?server_id=%d", serverID), nil, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["total"].(float64) != 2 {
		t.Fatalf("node list: %d %s", resp.StatusCode, body)
	}
	assertNoSecrets(t, body, "secret-password")

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/nodes/%d", nodeID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("node detail: %d %s", resp.StatusCode, body)
	}
	detail := jsonMap(t, body)
	if detail["user_count"].(float64) != 0 || detail["online_users"].(float64) != 0 {
		t.Fatalf("unexpected counts: %s", body)
	}
	serverRef := detail["server"].(map[string]any)
	if serverRef["id"].(float64) != float64(serverID) || serverRef["name"] != "s1" {
		t.Fatalf("unexpected server ref: %s", body)
	}
	assertNoSecrets(t, body, "secret-password")

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{
		"name": "renamed", "port": 8389,
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update node: %d %s", resp.StatusCode, body)
	}
	updated := jsonMap(t, body)
	if updated["name"] != "renamed" || updated["port"].(float64) != 8389 {
		t.Fatalf("unexpected update payload: %s", body)
	}
	assertNoSecrets(t, body, "secret-password")

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{
		"settings": map[string]any{"password": "new-password", "evil": "field"},
	}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
		t.Fatalf("unknown settings field must be 422, got %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{
		"settings": map[string]any{"cipher": "not-a-cipher"},
	}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad cipher must be 422, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{"status": "disabled"}, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["status"] != "disabled" {
		t.Fatalf("node disable: %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "DELETE", fmt.Sprintf("/api/nodes/%d", nodeID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete node: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/nodes/%d", nodeID), nil, cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d %s", resp.StatusCode, body)
	}
}

func TestNodeListFilters(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	server1 := e.seedServer(t, "s1")
	server2 := e.seedServer(t, "s2")
	ctx := context.Background()
	mustNode := func(serverID int64, name, protocol string, port int, status string) {
		t.Helper()
		if _, err := e.repo.CreateNode(ctx, repo.NewNode{ServerID: serverID, Name: name, Protocol: protocol, Port: port, Status: status}); err != nil {
			t.Fatalf("seed node: %v", err)
		}
	}
	mustNode(server1, "hk-ss", repo.ProtocolShadowsocks, 8388, repo.NodeStatusActive)
	mustNode(server1, "hk-vless", repo.ProtocolVLESS, 443, repo.NodeStatusDisabled)
	mustNode(server2, "jp-hy2", repo.ProtocolHysteria2, 8443, repo.NodeStatusActive)

	resp, body := e.do(t, "GET", "/api/nodes", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list nodes: %d %s", resp.StatusCode, body)
	}
	list := jsonMap(t, body)
	if list["total"].(float64) != 3 {
		t.Fatalf("expected 3 nodes, got %s", body)
	}
	for _, item := range list["items"].([]any) {
		node := item.(map[string]any)
		serverRef, ok := node["server"].(map[string]any)
		if !ok || serverRef["name"] == "" || serverRef["id"].(float64) != node["server_id"].(float64) {
			t.Fatalf("every node DTO must carry its server ref, got %s", body)
		}
	}

	cases := []struct {
		query string
		total float64
	}{
		{"?protocol=vless", 1},
		{"?status=active", 2},
		{"?q=hk", 2},
		{"?q=HK", 2},
		{"?q=%25", 0},
		{fmt.Sprintf("?server_id=%d&protocol=shadowsocks&status=active&q=hk", server1), 1},
		{"?page=2&page_size=2", 3},
	}
	for _, tc := range cases {
		resp, body = e.do(t, "GET", "/api/nodes"+tc.query, nil, cookie)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /api/nodes%s: %d %s", tc.query, resp.StatusCode, body)
		}
		if got := jsonMap(t, body)["total"].(float64); got != tc.total {
			t.Fatalf("GET /api/nodes%s: total=%v want %v", tc.query, got, tc.total)
		}
	}

	badCases := []string{
		"?protocol=snell",
		"?status=paused",
		"?server_id=abc",
	}
	for _, query := range badCases {
		resp, body = e.do(t, "GET", "/api/nodes"+query, nil, cookie)
		if resp.StatusCode != http.StatusBadRequest || errorCode(t, body) != "invalid_request" {
			t.Fatalf("GET /api/nodes%s must be 400 invalid_request, got %d %s", query, resp.StatusCode, body)
		}
	}
}

func TestNodeProtocolSettingsAllowlists(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "s1")

	cases := []struct {
		name     string
		protocol string
		settings map[string]any
		wantCode int
	}{
		{"vless ok", "vless", map[string]any{"private_key": testRealityPrivateKey(t), "reality_settings": map[string]any{"server_name": "a.com", "short_id": "0123abcd"}}, http.StatusCreated},
		{"vless bad names", "vless", map[string]any{"reality_settings": "not-an-object"}, http.StatusUnprocessableEntity},
		{"hysteria2 incomplete tls", "hysteria2", map[string]any{"password": "pw", "bandwidth": map[string]any{"up": 100, "down": 200.0}}, http.StatusUnprocessableEntity},
		{"hysteria2 bad bw", "hysteria2", map[string]any{"bandwidth": map[string]any{"up": 12.5}}, http.StatusUnprocessableEntity},
		{"hysteria2 negative", "hysteria2", map[string]any{"bandwidth": map[string]any{"down": -1}}, http.StatusUnprocessableEntity},
		{"ss empty settings", "shadowsocks", nil, http.StatusUnprocessableEntity},
	}
	for i, tc := range cases {
		resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
			"server_id": serverID, "address": "node.example.com", "name": fmt.Sprintf("n-%d", i), "protocol": tc.protocol,
			"port": 10000 + i, "settings": tc.settings,
		}, cookie)
		if resp.StatusCode != tc.wantCode {
			t.Fatalf("%s: expected %d, got %d %s", tc.name, tc.wantCode, resp.StatusCode, body)
		}
	}
}

func TestNodeProtocolParamsValidation(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "s1")
	now := time.Now()
	cert, key := testTLSMaterial(t, "hy2.example.com", now.Add(-time.Hour), now.Add(time.Hour))

	hy2Base := func() map[string]any {
		return map[string]any{"tls": map[string]any{"server_name": "hy2.example.com"}, "certificate": cert, "private_key": key}
	}

	cases := []struct {
		name     string
		mutate   func(settings map[string]any)
		wantCode int
	}{
		{"hy2 obfs ok", func(s map[string]any) {
			s["obfs"] = map[string]any{"open": true, "type": "salamander", "password": "obfs-secret"}
		}, http.StatusCreated},
		{"hy2 obfs too long", func(s map[string]any) {
			s["obfs"] = map[string]any{"password": strings.Repeat("a", 65)}
		}, http.StatusUnprocessableEntity},
		{"hy2 hop ok", func(s map[string]any) { s["hop_interval"] = "30000-40000" }, http.StatusCreated},
		{"hy2 hop empty ok", func(s map[string]any) { s["hop_interval"] = "" }, http.StatusCreated},
		{"hy2 hop bad format", func(s map[string]any) { s["hop_interval"] = "abc" }, http.StatusUnprocessableEntity},
		{"hy2 hop missing end", func(s map[string]any) { s["hop_interval"] = "1-" }, http.StatusUnprocessableEntity},
		{"hy2 hop reversed", func(s map[string]any) { s["hop_interval"] = "40000-30000" }, http.StatusUnprocessableEntity},
		{"hy2 hop out of range", func(s map[string]any) { s["hop_interval"] = "0-40000" }, http.StatusUnprocessableEntity},
		{"hy2 hop end too large", func(s map[string]any) { s["hop_interval"] = "1-70000" }, http.StatusUnprocessableEntity},
	}
	for i, tc := range cases {
		settings := hy2Base()
		tc.mutate(settings)
		resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
			"server_id": serverID, "address": "node.example.com", "name": fmt.Sprintf("param-%d", i), "protocol": "hysteria2",
			"port": 20000 + i, "settings": settings,
		}, cookie)
		if resp.StatusCode != tc.wantCode {
			t.Fatalf("%s: expected %d, got %d %s", tc.name, tc.wantCode, resp.StatusCode, body)
		}
		if tc.wantCode != http.StatusCreated && errorCode(t, body) != "validation" {
			t.Fatalf("%s: expected validation error code, got %s", tc.name, body)
		}
	}
}

func TestNodeProtocolParamsRoundTrip(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()
	serverID := e.seedServer(t, "s1")
	now := time.Now()
	cert, key := testTLSMaterial(t, "hy2.example.com", now.Add(-time.Hour), now.Add(time.Hour))

	// New plain settings fields are stored verbatim and survive partial updates.
	resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "node.example.com", "name": "hy2-params", "protocol": "hysteria2", "port": 21443,
		"settings": map[string]any{
			"tls":         map[string]any{"server_name": "hy2.example.com"},
			"certificate": cert, "private_key": key,
			"obfs":         map[string]any{"open": true, "type": "salamander", "password": "obfs-secret"},
			"hop_interval": "30000-40000",
		},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	nodeID := int64(jsonMap(t, body)["id"].(float64))
	node, err := e.repo.GetNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	for _, want := range []string{`"password":"obfs-secret"`, `"hop_interval":"30000-40000"`} {
		if !strings.Contains(node.ProtocolSettings, want) {
			t.Fatalf("settings must contain %s, got %s", want, node.ProtocolSettings)
		}
	}

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{
		"settings": map[string]any{"bandwidth": map[string]any{"up": 100}},
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("partial update: %d %s", resp.StatusCode, body)
	}
	node, err = e.repo.GetNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if !strings.Contains(node.ProtocolSettings, `"password":"obfs-secret"`) || !strings.Contains(node.ProtocolSettings, `"hop_interval":"30000-40000"`) {
		t.Fatalf("partial update must preserve new fields, got %s", node.ProtocolSettings)
	}

	// The agent config must render obfs in the inbound and never leak hop_ports.
	userID := e.seedUser(t, "params-user")
	if err := e.repo.AuthorizeUserNode(ctx, userID, nodeID); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	token := e.agentKey(t, cookie, serverID)
	resp, body = e.doAgent(t, "GET", "/api/agent/config?version=0", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agent config: %d %s", resp.StatusCode, body)
	}
	m := jsonMap(t, body)
	if agentUserByUUID(t, m, "params-user") == nil {
		t.Fatalf("expected user in payload: %s", body)
	}
	inbound := m["config"].(map[string]any)["singbox"].(map[string]any)["inbounds"].([]any)[0].(map[string]any)
	obfs, ok := inbound["obfs"].(map[string]any)
	if !ok || obfs["type"] != "salamander" || obfs["password"] != "obfs-secret" {
		t.Fatalf("inbound must carry salamander obfs: %s", body)
	}
	for _, leak := range []string{"hop_interval", "hop_ports", "mport", "ports"} {
		if _, has := inbound[leak]; has {
			t.Fatalf("hop_interval must not leak into the server inbound: %s", body)
		}
	}
}

func TestHysteria2TLSValidationAndUpdate(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "hy2")
	now := time.Now()
	cert, key := testTLSMaterial(t, "hy2.example.com", now.Add(-time.Hour), now.Add(time.Hour))
	_, otherKey := testTLSMaterial(t, "hy2.example.com", now.Add(-time.Hour), now.Add(time.Hour))
	expiredCert, expiredKey := testTLSMaterial(t, "hy2.example.com", now.Add(-2*time.Hour), now.Add(-time.Hour))
	cases := []map[string]any{
		{"tls": map[string]any{"server_name": "hy2.example.com"}, "certificate": "bad", "private_key": "bad"},
		{"tls": map[string]any{"server_name": "hy2.example.com"}, "certificate": cert, "private_key": otherKey},
		{"tls": map[string]any{"server_name": "wrong.example.com"}, "certificate": cert, "private_key": key},
		{"tls": map[string]any{"server_name": "hy2.example.com"}, "certificate": expiredCert, "private_key": expiredKey},
	}
	for i, settings := range cases {
		resp, body := e.do(t, "POST", "/api/nodes", map[string]any{"server_id": serverID, "address": "node.example.com", "name": fmt.Sprintf("bad-%d", i), "protocol": "hysteria2", "port": 9000 + i, "settings": settings}, cookie)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("case %d: %d %s", i, resp.StatusCode, body)
		}
	}
	resp, body := e.do(t, "POST", "/api/nodes", map[string]any{"server_id": serverID, "address": "node.example.com", "name": "good", "protocol": "hysteria2", "port": 9100, "settings": map[string]any{"tls": map[string]any{"server_name": "hy2.example.com"}, "certificate": cert, "private_key": key}}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	id := int64(jsonMap(t, body)["id"].(float64))
	assertNoSecrets(t, body, cert, key)
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", id), map[string]any{"settings": map[string]any{}}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("retain: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", id), map[string]any{"settings": map[string]any{"certificate": cert}}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("unpaired certificate replacement: %d %s", resp.StatusCode, body)
	}
	newCert, newKey := testTLSMaterial(t, "hy2.example.com", now.Add(-time.Hour), now.Add(2*time.Hour))
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", id), map[string]any{"settings": map[string]any{"certificate": newCert, "private_key": newKey}}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("replace: %d %s", resp.StatusCode, body)
	}
}

func TestRealityKeypairGeneration(t *testing.T) {
	e := newTestEnv(t)
	serverID := e.seedServer(t, "s1")
	revision := e.revision(t, serverID)

	resp, body := e.do(t, "POST", "/api/nodes/reality-keypair", nil, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("generation without admin must be 401, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "POST", "/api/nodes/reality-keypair", nil, e.login(t))
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("generate reality keypair: %d %s", resp.StatusCode, body)
	}
	generated := jsonMap(t, body)
	privateBytes, err := base64.RawURLEncoding.DecodeString(generated["private_key"].(string))
	if err != nil {
		t.Fatalf("decode private key: %v", err)
	}
	privateKey, err := ecdh.X25519().NewPrivateKey(privateBytes)
	if err != nil {
		t.Fatalf("parse private key: %v", err)
	}
	publicKey := base64.RawURLEncoding.EncodeToString(privateKey.PublicKey().Bytes())
	if generated["public_key"] != publicKey {
		t.Fatalf("public key does not match private key: %s", body)
	}
	shortID, err := hex.DecodeString(generated["short_id"].(string))
	if err != nil || len(shortID) != 8 {
		t.Fatalf("short_id must be eight bytes of hex: %s", body)
	}
	if got := e.revision(t, serverID); got != revision {
		t.Fatalf("key generation must not bump revision: got %d want %d", got, revision)
	}
}

func TestNodeSettingsUpdatePreservesRealitySecret(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "s1")
	privateKey := testRealityPrivateKey(t)
	resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "node.example.com", "name": "vless", "protocol": "vless", "port": 443,
		"settings": map[string]any{"private_key": privateKey, "reality_settings": map[string]any{"server_name": "a.com", "short_id": "0123abcd"}},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create vless: %d %s", resp.StatusCode, body)
	}
	nodeID := int64(jsonMap(t, body)["id"].(float64))

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{
		"settings": map[string]any{"reality_settings": map[string]any{"short_id": "abcdef12"}},
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("partial update: %d %s", resp.StatusCode, body)
	}

	node, err := e.repo.GetNode(context.Background(), nodeID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	plain, err := secrets.Decrypt(e.appKey, node.SecretEnc)
	if err != nil {
		t.Fatalf("decrypt node secret: %v", err)
	}
	if !strings.Contains(string(plain), privateKey) {
		t.Fatal("partial update must preserve the existing private key")
	}
	if !strings.Contains(node.ProtocolSettings, `"server_name":"a.com"`) || !strings.Contains(node.ProtocolSettings, `"short_id":"abcdef12"`) {
		t.Fatalf("partial update must merge public settings, got %s", node.ProtocolSettings)
	}
}

func TestServerAgentKeyLifecycle(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	serverID := e.seedServer(t, "s1")

	resp, body := e.do(t, "GET", fmt.Sprintf("/api/servers/%d/agent-key", serverID), nil, cookie)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("key read before generation must be 409, got %d %s", resp.StatusCode, body)
	}

	key1 := e.agentKey(t, cookie, serverID)
	if key1 == "" {
		t.Fatal("expected a generated agent key")
	}

	var storedHash string
	var storedEnc []byte
	if err := e.db.QueryRowContext(context.Background(), `SELECT key_hash, key_enc FROM agents WHERE server_id = ?`, serverID).Scan(&storedHash, &storedEnc); err != nil {
		t.Fatalf("agent key must be stored: %v", err)
	}
	if storedHash == key1 || storedHash != adminauth.HashToken(key1) {
		t.Fatal("agent key must be stored as sha256 hash")
	}
	plain, err := secrets.Decrypt(e.appKey, storedEnc)
	if err != nil || string(plain) != key1 {
		t.Fatalf("agent key must be encrypted with the panel app key: %v", err)
	}

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/servers/%d/agent-key", serverID), nil, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["agent_key"] != key1 {
		t.Fatalf("stored key must be readable: %d %s", resp.StatusCode, body)
	}

	key2 := e.agentKey(t, cookie, serverID)
	if key2 == key1 {
		t.Fatal("reset must issue a new key")
	}
	if resp, body = e.doAgent(t, "POST", "/api/agent/heartbeat", validHB(), key1); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old key must be invalid after reset, got %d %s", resp.StatusCode, body)
	}
	if resp, body = e.doAgent(t, "POST", "/api/agent/heartbeat", validHB(), key2); resp.StatusCode != http.StatusOK {
		t.Fatalf("new key must authenticate, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "GET", "/api/servers/99999/agent-key", nil, cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("key read for unknown server must be 404, got %d %s", resp.StatusCode, body)
	}
}

func TestServerCreateGeneratesAgentKey(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	resp, body := e.do(t, "POST", "/api/servers", map[string]any{"name": "s1"}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create server: %d %s", resp.StatusCode, body)
	}
	created := jsonMap(t, body)
	agentKey, _ := created["agent_key"].(string)
	if agentKey == "" {
		t.Fatalf("create server must return a non-empty agent_key, got %s", body)
	}
	assertNoSecrets(t, body)
	serverID := int64(created["id"].(float64))

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/servers/%d/agent-key", serverID), nil, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["agent_key"] != agentKey {
		t.Fatalf("issued key must be readable right after creation: %d %s", resp.StatusCode, body)
	}

	if resp, body = e.doAgent(t, "POST", "/api/agent/heartbeat", validHB(), agentKey); resp.StatusCode != http.StatusOK {
		t.Fatalf("issued key must authenticate: %d %s", resp.StatusCode, body)
	}

	if rev := e.revision(t, serverID); rev != 0 {
		t.Fatalf("creating a server must not bump its revision, got %d", rev)
	}
}

func TestServerDetailShowsAgentAndRevision(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	serverID := e.seedServer(t, "s1")
	e.seedNode(t, serverID, "n1", 443)
	agentID, err := e.repo.CreateAgent(context.Background(), serverID, "agent-hash", "2.1.0")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := e.repo.BumpServerRevision(context.Background(), serverID); err != nil {
		t.Fatalf("bump: %v", err)
	}

	resp, body := e.do(t, "GET", fmt.Sprintf("/api/servers/%d", serverID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("server detail: %d %s", resp.StatusCode, body)
	}
	detail := jsonMap(t, body)
	if detail["revision"].(float64) != 1 {
		t.Fatalf("expected revision 1, got %s", body)
	}
	agent := detail["agent"].(map[string]any)
	if agent["id"].(float64) != float64(agentID) || agent["version"] != "2.1.0" {
		t.Fatalf("unexpected agent info: %s", body)
	}
	if len(detail["nodes"].([]any)) != 1 {
		t.Fatalf("expected 1 node, got %s", body)
	}
	assertNoSecrets(t, body)
}

func TestServerDeleteCascade(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()

	serverID := e.seedServer(t, "s1")
	nodeID := e.seedNode(t, serverID, "n1", 443)
	userID := e.seedUser(t, "u1")
	if err := e.repo.AuthorizeUserNode(ctx, userID, nodeID); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	agentID, err := e.repo.CreateAgent(ctx, serverID, "agent-hash", "1.0.0")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	now := time.Now()
	if _, _, err := e.repo.IngestDeviceBatch(ctx, agentID, 1, serverID, []repo.NewOnlineDevice{
		{UserID: userID, NodeID: nodeID, IP: "1.1.1.1", Online: 1},
	}, map[int64]int{userID: 1}, now.Unix()); err != nil {
		t.Fatalf("devices: %v", err)
	}
	if err := e.repo.InsertTrafficRecords(ctx, []repo.NewTrafficRecord{
		{UserID: userID, NodeID: nodeID, ServerID: serverID, U: 5, D: 6, CreatedAt: now},
	}); err != nil {
		t.Fatalf("traffic: %v", err)
	}

	resp, body := e.do(t, "DELETE", fmt.Sprintf("/api/servers/%d", serverID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete server: %d %s", resp.StatusCode, body)
	}

	if _, err := e.repo.GetServer(ctx, serverID); err != repo.ErrNotFound {
		t.Fatalf("expected server gone, got %v", err)
	}
	if _, err := e.repo.GetNode(ctx, nodeID); err != repo.ErrNotFound {
		t.Fatalf("expected node cascade deleted, got %v", err)
	}
	if _, err := e.repo.GetAgentByServerID(ctx, serverID); err != repo.ErrNotFound {
		t.Fatalf("expected agent cascade deleted, got %v", err)
	}
	ids, err := e.repo.ListNodeIDsByUser(ctx, userID)
	if err != nil || len(ids) != 0 {
		t.Fatalf("expected authorizations removed, got %v %v", ids, err)
	}

	upload, download, err := e.repo.SumTraffic(ctx, repo.TrafficFilter{UserID: userID})
	if err != nil || upload != 5 || download != 6 {
		t.Fatalf("traffic must survive server deletion, got %d %d %v", upload, download, err)
	}
	if devices, err := e.repo.ListDevicesByUser(ctx, userID); err != nil || len(devices) != 0 {
		t.Fatalf("online devices must cascade with the server, got %v %v", devices, err)
	}

	if _, err := e.repo.GetUser(ctx, userID); err != nil {
		t.Fatalf("user must survive: %v", err)
	}
}

func TestServerStatusComputedFromHeartbeat(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()

	serverID := e.seedServer(t, "fresh")
	disabledID := e.seedServer(t, "disabled")
	e.seedServer(t, "no-heartbeat")

	if err := e.repo.UpdateServerMetrics(ctx, serverID, 20, 50, 40, 1000, time.Now()); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if err := e.repo.UpdateServerMetrics(ctx, disabledID, 20, 50, 40, 1000, time.Now()); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if err := e.repo.SetServerStatus(ctx, disabledID, repo.ServerStatusDisabled); err != nil {
		t.Fatalf("disable: %v", err)
	}

	_, body := e.do(t, "GET", "/api/servers", nil, cookie)
	items := jsonMap(t, body)["items"].([]any)
	statuses := map[string]string{}
	for _, raw := range items {
		item := raw.(map[string]any)
		statuses[item["name"].(string)] = item["status"].(string)
	}
	if statuses["fresh"] != repo.ServerStatusActive {
		t.Fatalf("fresh heartbeat must be active, got %s", statuses["fresh"])
	}
	if statuses["disabled"] != repo.ServerStatusDisabled {
		t.Fatalf("disabled server must stay disabled, got %s", statuses["disabled"])
	}
	if statuses["no-heartbeat"] != repo.ServerStatusOffline {
		t.Fatalf("server without heartbeat must be offline, got %s", statuses["no-heartbeat"])
	}
}

func TestServerUpdateValidation(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	serverID := e.seedServer(t, "s1")

	resp, body := e.do(t, "PUT", fmt.Sprintf("/api/servers/%d", serverID), map[string]any{"status": "offline"}, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("admin cannot set offline status, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/servers/%d", serverID), map[string]any{
		"name": "renamed", "status": "disabled",
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("server update: %d %s", resp.StatusCode, body)
	}
	updated := jsonMap(t, body)
	if updated["name"] != "renamed" || updated["status"] != "disabled" {
		t.Fatalf("unexpected update: %s", body)
	}
}

func TestRevisionBumpsOnMutations(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	server1 := e.seedServer(t, "s1")
	server2 := e.seedServer(t, "s2")
	node1 := e.seedNode(t, server1, "n1", 443)
	node2 := e.seedNode(t, server2, "n2", 443)

	_, body := e.do(t, "POST", "/api/users", map[string]any{
		"username": "revuser", "node_ids": []int64{node1, node2},
	}, cookie)
	userID := int64(jsonMap(t, body)["id"].(float64))

	rev11 := e.revision(t, server1)
	rev21 := e.revision(t, server2)
	if rev11 != 1 || rev21 != 1 {
		t.Fatalf("user creation with nodes must bump both revisions, got %d %d", rev11, rev21)
	}

	_, _ = e.do(t, "PUT", fmt.Sprintf("/api/users/%d", userID), map[string]any{"status": "disabled"}, cookie)
	if rev := e.revision(t, server1); rev != 2 {
		t.Fatalf("user status change must bump revision, got %d", rev)
	}
	if rev := e.revision(t, server2); rev != 2 {
		t.Fatalf("user status change must bump all affected servers, got %d", rev)
	}

	_, _ = e.do(t, "POST", fmt.Sprintf("/api/users/%d/reset-traffic", userID), nil, cookie)
	if rev := e.revision(t, server1); rev != 3 {
		t.Fatalf("reset-traffic must bump revision, got %d", rev)
	}

	_, _ = e.do(t, "POST", fmt.Sprintf("/api/users/%d/expire-now", userID), nil, cookie)
	if rev := e.revision(t, server1); rev != 4 {
		t.Fatalf("expire-now must bump revision, got %d", rev)
	}

	_, _ = e.do(t, "POST", fmt.Sprintf("/api/users/%d/reset-token", userID), nil, cookie)
	if rev := e.revision(t, server1); rev != 4 {
		t.Fatalf("token reset must not bump revision (not runtime config), got %d", rev)
	}

	resp, body := e.do(t, "PUT", fmt.Sprintf("/api/users/%d", userID), map[string]any{"status": "bogus"}, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected invalid status rejection, got %d %s", resp.StatusCode, body)
	}
	if rev := e.revision(t, server1); rev != 4 {
		t.Fatalf("failed mutation must not bump revision, got %d", rev)
	}

	_, _ = e.do(t, "PUT", fmt.Sprintf("/api/users/%d/nodes", userID), map[string]any{"node_ids": []int64{node1}}, cookie)
	if rev := e.revision(t, server1); rev != 5 {
		t.Fatalf("authorization change must bump kept server, got %d", rev)
	}
	if rev := e.revision(t, server2); rev != 5 {
		t.Fatalf("authorization change must bump removed server, got %d", rev)
	}

	resp, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": server1, "address": "node.example.com", "name": "n3", "protocol": "vless", "port": 9443,
		"settings": map[string]any{"private_key": testRealityPrivateKey(t), "reality_settings": map[string]any{"server_name": "example.com"}},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("node create: %d %s", resp.StatusCode, body)
	}
	node3 := int64(jsonMap(t, body)["id"].(float64))
	if rev := e.revision(t, server1); rev != 6 {
		t.Fatalf("node create must bump revision, got %d", rev)
	}

	_, _ = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", node3), map[string]any{"name": "n3b"}, cookie)
	if rev := e.revision(t, server1); rev != 7 {
		t.Fatalf("node update must bump revision, got %d", rev)
	}

	_, _ = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", node3), map[string]any{"status": "disabled"}, cookie)
	if rev := e.revision(t, server1); rev != 8 {
		t.Fatalf("node status change must bump revision, got %d", rev)
	}

	_, _ = e.do(t, "DELETE", fmt.Sprintf("/api/nodes/%d", node3), nil, cookie)
	if rev := e.revision(t, server1); rev != 9 {
		t.Fatalf("node delete must bump revision, got %d", rev)
	}

	_, _ = e.do(t, "PUT", fmt.Sprintf("/api/servers/%d", server1), map[string]any{"status": "disabled"}, cookie)
	if rev := e.revision(t, server1); rev != 10 {
		t.Fatalf("server status change must bump own revision, got %d", rev)
	}

	_, _ = e.do(t, "DELETE", fmt.Sprintf("/api/users/%d", userID), nil, cookie)
	if rev := e.revision(t, server1); rev != 11 {
		t.Fatalf("user delete must bump authorized server revision, got %d", rev)
	}
}

func TestNodeRateAndTags(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "meta")

	resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "node.example.com", "name": "meta-ss", "protocol": "shadowsocks", "port": 8388,
		"rate": 2.5, "tags": []string{"hk", "premium"},
		"settings": map[string]any{"cipher": "2022-blake3-aes-128-gcm"},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create with rate/tags: %d %s", resp.StatusCode, body)
	}
	created := jsonMap(t, body)
	if created["rate"].(float64) != 2.5 {
		t.Fatalf("rate must round-trip, got %s", body)
	}
	tags := created["tags"].([]any)
	if len(tags) != 2 || tags[0] != "hk" || tags[1] != "premium" {
		t.Fatalf("tags must round-trip, got %s", body)
	}
	nodeID := int64(created["id"].(float64))

	resp, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "node.example.com", "name": "meta-default", "protocol": "shadowsocks", "port": 8389,
		"settings": map[string]any{"cipher": "2022-blake3-aes-128-gcm"},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create with defaults: %d %s", resp.StatusCode, body)
	}
	defaults := jsonMap(t, body)
	if defaults["rate"].(float64) != 1 || len(defaults["tags"].([]any)) != 0 {
		t.Fatalf("omitted rate/tags must default to 1/[], got %s", body)
	}

	manyTags := make([]string, 21)
	for i := range manyTags {
		manyTags[i] = "t"
	}
	badCases := []map[string]any{
		{"rate": 0},
		{"rate": -1},
		{"tags": []string{""}},
		{"tags": []string{strings.Repeat("a", 33)}},
		{"tags": manyTags},
	}
	for i, extra := range badCases {
		payload := map[string]any{
			"server_id": serverID, "address": "node.example.com", "name": fmt.Sprintf("meta-bad-%d", i), "protocol": "shadowsocks",
			"port": 8500 + i, "settings": map[string]any{"cipher": "2022-blake3-aes-128-gcm"},
		}
		for key, value := range extra {
			payload[key] = value
		}
		resp, body := e.do(t, "POST", "/api/nodes", payload, cookie)
		if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
			t.Fatalf("invalid rate/tags %v must be 422 validation, got %d %s", extra, resp.StatusCode, body)
		}
	}

	rev := e.revision(t, serverID)
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{
		"rate": 3.0, "tags": []string{"jp"},
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update rate/tags: %d %s", resp.StatusCode, body)
	}
	updated := jsonMap(t, body)
	if updated["rate"].(float64) != 3 || len(updated["tags"].([]any)) != 1 {
		t.Fatalf("rate/tags must update, got %s", body)
	}
	if e.revision(t, serverID) <= rev {
		t.Fatal("rate/tags update must bump the server revision")
	}

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{"rate": 4.0}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("partial rate update: %d %s", resp.StatusCode, body)
	}
	updated = jsonMap(t, body)
	if updated["rate"].(float64) != 4 || len(updated["tags"].([]any)) != 1 {
		t.Fatalf("omitted tags must be retained, got %s", body)
	}
}

func TestNodeDTOIncludesRateAndTagsOnEveryPath(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()
	serverID := e.seedServer(t, "paths")

	resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "node.example.com", "name": "paths-ss", "protocol": "shadowsocks", "port": 8388,
		"rate": 1.5, "tags": []string{"edge"},
		"settings": map[string]any{"cipher": "2022-blake3-aes-128-gcm"},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	nodeID := int64(jsonMap(t, body)["id"].(float64))

	assertRateTags := func(label, body string, node map[string]any) {
		t.Helper()
		if node["rate"].(float64) != 1.5 {
			t.Fatalf("%s: rate missing, got %s", label, body)
		}
		tags, ok := node["tags"].([]any)
		if !ok || len(tags) != 1 || tags[0] != "edge" {
			t.Fatalf("%s: tags missing, got %s", label, body)
		}
	}

	resp, body = e.do(t, "GET", "/api/nodes", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: %d %s", resp.StatusCode, body)
	}
	assertRateTags("list", body, jsonMap(t, body)["items"].([]any)[0].(map[string]any))

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/nodes/%d", nodeID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("detail: %d %s", resp.StatusCode, body)
	}
	assertRateTags("detail", body, jsonMap(t, body))

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/servers/%d", serverID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("server detail: %d %s", resp.StatusCode, body)
	}
	assertRateTags("server", body, jsonMap(t, body)["nodes"].([]any)[0].(map[string]any))

	userID := e.seedUser(t, "paths-user")
	if err := e.repo.AuthorizeUserNode(ctx, userID, nodeID); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/nodes", userID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("user nodes: %d %s", resp.StatusCode, body)
	}
	assertRateTags("user nodes", body, jsonMap(t, body)["nodes"].([]any)[0].(map[string]any))
}

func TestVLESSNestedSettingsShape(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()
	serverID := e.seedServer(t, "nested")
	privateKey := testRealityPrivateKey(t)

	resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "node.example.com", "name": "nested-vless", "protocol": "vless", "port": 443,
		"settings": map[string]any{
			"private_key": privateKey,
			"tls":         2,
			"reality_settings": map[string]any{
				"server_name": "nested.example.com",
				"short_id":    "0123abcd",
			},
		},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create nested vless: %d %s", resp.StatusCode, body)
	}
	nodeID := int64(jsonMap(t, body)["id"].(float64))

	node, err := e.repo.GetNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	settings := map[string]any{}
	if err := json.Unmarshal([]byte(node.ProtocolSettings), &settings); err != nil {
		t.Fatalf("decode settings: %v", err)
	}
	if _, has := settings["server_names"]; has {
		t.Fatalf("flat server_names must not be stored: %s", node.ProtocolSettings)
	}
	reality, ok := settings["reality_settings"].(map[string]any)
	if !ok || reality["server_name"] != "nested.example.com" || reality["short_id"] != "0123abcd" {
		t.Fatalf("unexpected reality_settings: %s", node.ProtocolSettings)
	}
	if publicKey, _ := reality["public_key"].(string); publicKey == "" {
		t.Fatalf("public_key must be derived into reality_settings: %s", node.ProtocolSettings)
	}

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{
		"settings": map[string]any{"reality_settings": map[string]any{"short_id": "abcdef12"}},
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("nested partial update: %d %s", resp.StatusCode, body)
	}
	node, _ = e.repo.GetNode(ctx, nodeID)
	if !strings.Contains(node.ProtocolSettings, `"server_name":"nested.example.com"`) || !strings.Contains(node.ProtocolSettings, `"short_id":"abcdef12"`) {
		t.Fatalf("nested partial update must merge reality_settings, got %s", node.ProtocolSettings)
	}

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{
		"settings": map[string]any{"reality_settings": map[string]any{"bogus": "field"}},
	}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
		t.Fatalf("unknown nested field must be 422 validation, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{
		"settings": map[string]any{"reality_settings": map[string]any{"public_key": "not-the-derived-key"}},
	}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
		t.Fatalf("mismatched public_key must be 422 validation, got %d %s", resp.StatusCode, body)
	}

	rotated := testRealityPrivateKey(t)
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{
		"settings": map[string]any{"private_key": rotated},
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("private key rotation must succeed, got %d %s", resp.StatusCode, body)
	}
	node, _ = e.repo.GetNode(ctx, nodeID)
	plain, err := secrets.Decrypt(e.appKey, node.SecretEnc)
	if err != nil {
		t.Fatalf("decrypt secret: %v", err)
	}
	if !strings.Contains(string(plain), rotated) {
		t.Fatalf("rotated private key must be stored, got %s", plain)
	}
	if !strings.Contains(node.ProtocolSettings, `"server_name":"nested.example.com"`) {
		t.Fatalf("partial private key update must retain public settings, got %s", node.ProtocolSettings)
	}
}

// Reserved Xboard sections (tls_settings/network_settings/multiplex/utls,
// shadowsocks obfs_settings) are extension placeholders: accepted as opaque
// objects, stored verbatim, and never rendered. Validated sections reject
// unknown keys at any depth.
func TestNodeReservedSettingsSectionsAreOpaqueAndInert(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()
	serverID := e.seedServer(t, "reserved")

	resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "node.example.com", "name": "reserved-vless", "protocol": "vless", "port": 443,
		"settings": map[string]any{
			"private_key":      testRealityPrivateKey(t),
			"reality_settings": map[string]any{"server_name": "reserved.example.com"},
			"tls_settings":     map[string]any{"server_name": "reserved.example.com"},
			"network_settings": map[string]any{"anything": "goes"},
			"multiplex":        map[string]any{"enabled": true, "brutal": map[string]any{"up_mbps": 5}},
			"utls":             map[string]any{"enabled": true, "fingerprint": "chrome"},
		},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("reserved sections must be accepted: %d %s", resp.StatusCode, body)
	}
	nodeID := int64(jsonMap(t, body)["id"].(float64))
	node, err := e.repo.GetNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	for _, want := range []string{`"network_settings":{"anything":"goes"}`, `"utls":{"enabled":true,"fingerprint":"chrome"}`} {
		if !strings.Contains(node.ProtocolSettings, want) {
			t.Fatalf("reserved settings must be stored verbatim, want %s in %s", want, node.ProtocolSettings)
		}
	}

	userID := e.seedUser(t, "reserved-user")
	if err := e.repo.AuthorizeUserNode(ctx, userID, nodeID); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	token := e.agentKey(t, cookie, serverID)
	resp, body = e.doAgent(t, "GET", "/api/agent/config?version=0", nil, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reserved sections must not break rendering: %d %s", resp.StatusCode, body)
	}
	inbound := jsonMap(t, body)["config"].(map[string]any)["singbox"].(map[string]any)["inbounds"].([]any)[0].(map[string]any)
	for _, leak := range []string{"tls_settings", "network_settings", "multiplex", "utls"} {
		if _, has := inbound[leak]; has {
			t.Fatalf("reserved section %q must never reach the rendered inbound: %s", leak, body)
		}
	}

	// A supplied reserved section replaces the stored one as a whole (no deep merge).
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{
		"settings": map[string]any{"multiplex": map[string]any{"enabled": false}},
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reserved section patch: %d %s", resp.StatusCode, body)
	}
	node, err = e.repo.GetNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if !strings.Contains(node.ProtocolSettings, `"multiplex":{"enabled":false}`) || strings.Contains(node.ProtocolSettings, `"brutal"`) {
		t.Fatalf("reserved section must be replaced as a whole, got %s", node.ProtocolSettings)
	}
	if !strings.Contains(node.ProtocolSettings, `"network_settings":{"anything":"goes"}`) {
		t.Fatalf("unrelated reserved sections must be retained, got %s", node.ProtocolSettings)
	}

	now := time.Now()
	cert, key := testTLSMaterial(t, "hy2.example.com", now.Add(-time.Hour), now.Add(time.Hour))
	badNested := []struct {
		name     string
		protocol string
		settings map[string]any
	}{
		{"bandwidth unknown", "hysteria2", map[string]any{
			"tls": map[string]any{"server_name": "hy2.example.com"}, "certificate": cert, "private_key": key,
			"bandwidth": map[string]any{"up": 1, "bogus": 2},
		}},
		{"tls unknown", "hysteria2", map[string]any{
			"tls": map[string]any{"server_name": "hy2.example.com", "bogus": true}, "certificate": cert, "private_key": key,
		}},
		{"obfs unknown", "hysteria2", map[string]any{
			"tls": map[string]any{"server_name": "hy2.example.com"}, "certificate": cert, "private_key": key,
			"obfs": map[string]any{"open": true, "bogus": "x"},
		}},
		{"tls_settings not an object", "vless", map[string]any{
			"private_key":      testRealityPrivateKey(t),
			"reality_settings": map[string]any{"server_name": "a.com"},
			"tls_settings":     "not-an-object",
		}},
	}
	for i, tc := range badNested {
		resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
			"server_id": serverID, "address": "node.example.com", "name": fmt.Sprintf("bad-reserved-%d", i), "protocol": tc.protocol,
			"port": 26000 + i, "settings": tc.settings,
		}, cookie)
		if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
			t.Fatalf("%s must be 422 validation, got %d %s", tc.name, resp.StatusCode, body)
		}
	}
}

func TestNodeCopyReusesSourceAndStartsDisabled(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()
	serverID := e.seedServer(t, "copy")

	resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "copy.example.com", "name": "copy-ss", "protocol": "shadowsocks", "port": 8388,
		"rate": 2.5, "tags": []string{"hk"},
		"settings": map[string]any{"cipher": "2022-blake3-aes-128-gcm", "password": "secret-password"},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create source: %d %s", resp.StatusCode, body)
	}
	sourceID := int64(jsonMap(t, body)["id"].(float64))
	revBefore := e.revision(t, serverID)

	resp, body = e.do(t, "POST", fmt.Sprintf("/api/nodes/%d/copy", sourceID), nil, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("copy: %d %s", resp.StatusCode, body)
	}
	copied := jsonMap(t, body)
	if copied["name"] != "copy-ss" || copied["address"] != "copy.example.com" || copied["port"].(float64) != 8388 {
		t.Fatalf("copy must reuse name/address/port: %s", body)
	}
	if copied["status"] != "disabled" {
		t.Fatalf("copy must start disabled: %s", body)
	}
	if copied["rate"].(float64) != 2.5 || len(copied["tags"].([]any)) != 1 {
		t.Fatalf("copy must reuse rate/tags: %s", body)
	}
	copyID := int64(copied["id"].(float64))
	if copyID == sourceID {
		t.Fatal("copy must be a new node")
	}
	if e.revision(t, serverID) <= revBefore {
		t.Fatal("copy must bump the server revision")
	}

	source, err := e.repo.GetNode(ctx, sourceID)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	duplicate, err := e.repo.GetNode(ctx, copyID)
	if err != nil {
		t.Fatalf("get copy: %v", err)
	}
	if len(duplicate.SecretEnc) == 0 || string(duplicate.SecretEnc) != string(source.SecretEnc) {
		t.Fatal("copy must reuse the encrypted secret verbatim")
	}

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", copyID), map[string]any{"status": "active"}, cookie)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("enabling a duplicate port must conflict: %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", copyID), map[string]any{"port": 8389, "status": "active"}, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["status"] != "active" {
		t.Fatalf("enable after freeing the port: %d %s", resp.StatusCode, body)
	}
}

func TestNodeIPv6Entry(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "ipv6")

	resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "hk01.example.com", "name": "hk-ss", "protocol": "shadowsocks", "port": 8388,
		"ipv6_enabled": true, "ipv6_address": "2001:db8::1",
		"settings": map[string]any{"cipher": "2022-blake3-aes-128-gcm"},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create with ipv6: %d %s", resp.StatusCode, body)
	}
	created := jsonMap(t, body)
	nodeID := int64(created["id"].(float64))
	if created["ipv6_enabled"] != true || created["ipv6_address"] != "2001:db8::1" {
		t.Fatalf("ipv6 fields must be echoed: %s", body)
	}

	resp, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "address": "hk01.example.com", "name": "enabled-without-address", "protocol": "shadowsocks", "port": 8389,
		"ipv6_enabled": true, "settings": map[string]any{"cipher": "2022-blake3-aes-128-gcm"},
	}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
		t.Fatalf("enabled without address must be 422 validation, got %d %s", resp.StatusCode, body)
	}

	for _, tc := range []struct {
		name string
		port int
		ipv6 string
	}{
		{"hostname", 8390, "v6.example.com"},
		{"ipv4-literal", 8391, "1.2.3.4"},
	} {
		resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
			"server_id": serverID, "address": "hk01.example.com", "name": tc.name, "protocol": "shadowsocks", "port": tc.port,
			"ipv6_enabled": true, "ipv6_address": tc.ipv6, "settings": map[string]any{"cipher": "2022-blake3-aes-128-gcm"},
		}, cookie)
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("%s must be accepted, got %d %s", tc.name, resp.StatusCode, body)
		}
		if got := jsonMap(t, body)["ipv6_address"]; got != tc.ipv6 {
			t.Fatalf("%s ipv6_address: got %v", tc.name, got)
		}
	}

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{"ipv6_address": "2001:db8::2"}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update ipv6 address: %d %s", resp.StatusCode, body)
	}
	updated := jsonMap(t, body)
	if updated["ipv6_enabled"] != true || updated["ipv6_address"] != "2001:db8::2" {
		t.Fatalf("partial update must keep ipv6_enabled: %s", body)
	}

	resp, body = e.do(t, "POST", fmt.Sprintf("/api/nodes/%d/copy", nodeID), nil, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("copy: %d %s", resp.StatusCode, body)
	}
	copied := jsonMap(t, body)
	if copied["ipv6_enabled"] != true || copied["ipv6_address"] != "2001:db8::2" {
		t.Fatalf("copy must reuse ipv6 fields: %s", body)
	}

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{"ipv6_enabled": false}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disable ipv6: %d %s", resp.StatusCode, body)
	}
	disabled := jsonMap(t, body)
	if disabled["ipv6_enabled"] != false || disabled["ipv6_address"] != "2001:db8::2" {
		t.Fatalf("disabling must keep the stored address: %s", body)
	}
}
