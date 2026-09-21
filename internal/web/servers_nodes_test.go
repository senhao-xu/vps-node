package web_test

import (
	"context"
	"crypto/ecdh"
	"encoding/base64"
	"encoding/hex"
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
		"server_id": 99999, "name": "n", "protocol": "vless", "port": 443,
	}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
		t.Fatalf("unknown server must be 422 validation, got %d %s", resp.StatusCode, body)
	}

	serverID := e.seedServer(t, "s1")
	resp, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "name": "hk-ss", "protocol": "shadowsocks", "port": 8388,
		"settings": map[string]any{"method": "2022-blake3-aes-128-gcm", "password": "secret-password"},
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
		"server_id": serverID, "name": "dup-port", "protocol": "vless", "port": 8388,
		"settings": map[string]any{"private_key": testRealityPrivateKey(t), "server_names": []string{"example.com"}},
	}, cookie)
	if resp.StatusCode != http.StatusConflict || errorCode(t, body) != "conflict" {
		t.Fatalf("port conflict must be 409, got %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "name": "hk-ss", "protocol": "vless", "port": 9000,
		"settings": map[string]any{"private_key": testRealityPrivateKey(t), "server_names": []string{"example.com"}},
	}, cookie)
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("name conflict must be 409, got %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "name": "bad", "protocol": "snell", "port": 9001,
	}, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("bad protocol must be 400, got %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverID, "name": "bad", "protocol": "vless", "port": 70000,
	}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("bad port must be 422, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/nodes?server_id=%d", serverID), nil, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["total"].(float64) != 1 {
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
		"settings": map[string]any{"method": "not-a-cipher"},
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
		{"vless ok", "vless", map[string]any{"private_key": testRealityPrivateKey(t), "short_id": "0123abcd", "server_names": []string{"a.com", "b.com"}}, http.StatusCreated},
		{"vless bad names", "vless", map[string]any{"server_names": "not-an-array"}, http.StatusUnprocessableEntity},
		{"hysteria2 incomplete tls", "hysteria2", map[string]any{"password": "pw", "up_mbps": 100, "down_mbps": 200.0}, http.StatusUnprocessableEntity},
		{"hysteria2 bad bw", "hysteria2", map[string]any{"up_mbps": 12.5}, http.StatusUnprocessableEntity},
		{"hysteria2 negative", "hysteria2", map[string]any{"down_mbps": -1}, http.StatusUnprocessableEntity},
		{"ss empty settings", "shadowsocks", nil, http.StatusUnprocessableEntity},
	}
	for i, tc := range cases {
		resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
			"server_id": serverID, "name": fmt.Sprintf("n-%d", i), "protocol": tc.protocol,
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
		return map[string]any{"server_name": "hy2.example.com", "certificate": cert, "private_key": key}
	}

	cases := []struct {
		name     string
		mutate   func(settings map[string]any)
		wantCode int
	}{
		{"hy2 obfs ok", func(s map[string]any) { s["obfs_password"] = "obfs-secret" }, http.StatusCreated},
		{"hy2 obfs too long", func(s map[string]any) { s["obfs_password"] = strings.Repeat("a", 65) }, http.StatusUnprocessableEntity},
		{"hy2 hop ok", func(s map[string]any) { s["hop_ports"] = "30000-40000" }, http.StatusCreated},
		{"hy2 hop empty ok", func(s map[string]any) { s["hop_ports"] = "" }, http.StatusCreated},
		{"hy2 hop bad format", func(s map[string]any) { s["hop_ports"] = "abc" }, http.StatusUnprocessableEntity},
		{"hy2 hop missing end", func(s map[string]any) { s["hop_ports"] = "1-" }, http.StatusUnprocessableEntity},
		{"hy2 hop reversed", func(s map[string]any) { s["hop_ports"] = "40000-30000" }, http.StatusUnprocessableEntity},
		{"hy2 hop out of range", func(s map[string]any) { s["hop_ports"] = "0-40000" }, http.StatusUnprocessableEntity},
		{"hy2 hop end too large", func(s map[string]any) { s["hop_ports"] = "1-70000" }, http.StatusUnprocessableEntity},
	}
	for i, tc := range cases {
		settings := hy2Base()
		tc.mutate(settings)
		resp, body := e.do(t, "POST", "/api/nodes", map[string]any{
			"server_id": serverID, "name": fmt.Sprintf("param-%d", i), "protocol": "hysteria2",
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
		"server_id": serverID, "name": "hy2-params", "protocol": "hysteria2", "port": 21443,
		"settings": map[string]any{
			"server_name": "hy2.example.com", "certificate": cert, "private_key": key,
			"obfs_password": "obfs-secret", "hop_ports": "30000-40000",
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
	for _, want := range []string{`"obfs_password":"obfs-secret"`, `"hop_ports":"30000-40000"`} {
		if !strings.Contains(node.Settings, want) {
			t.Fatalf("settings must contain %s, got %s", want, node.Settings)
		}
	}

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{
		"settings": map[string]any{"up_mbps": 100},
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("partial update: %d %s", resp.StatusCode, body)
	}
	node, err = e.repo.GetNode(ctx, nodeID)
	if err != nil {
		t.Fatalf("get node: %v", err)
	}
	if !strings.Contains(node.Settings, `"obfs_password":"obfs-secret"`) || !strings.Contains(node.Settings, `"hop_ports":"30000-40000"`) {
		t.Fatalf("partial update must preserve new fields, got %s", node.Settings)
	}

	// The agent config must render obfs in the inbound and never leak hop_ports.
	userID := e.seedUser(t, "params-user")
	if err := e.repo.AuthorizeUserNode(ctx, userID, nodeID); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	token := e.registerAgent(t, cookie, serverID, "1.0.0")
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
	for _, leak := range []string{"hop_ports", "mport", "ports"} {
		if _, has := inbound[leak]; has {
			t.Fatalf("hop_ports must not leak into the server inbound: %s", body)
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
		{"server_name": "hy2.example.com", "certificate": "bad", "private_key": "bad"},
		{"server_name": "hy2.example.com", "certificate": cert, "private_key": otherKey},
		{"server_name": "wrong.example.com", "certificate": cert, "private_key": key},
		{"server_name": "hy2.example.com", "certificate": expiredCert, "private_key": expiredKey},
	}
	for i, settings := range cases {
		resp, body := e.do(t, "POST", "/api/nodes", map[string]any{"server_id": serverID, "name": fmt.Sprintf("bad-%d", i), "protocol": "hysteria2", "port": 9000 + i, "settings": settings}, cookie)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("case %d: %d %s", i, resp.StatusCode, body)
		}
	}
	resp, body := e.do(t, "POST", "/api/nodes", map[string]any{"server_id": serverID, "name": "good", "protocol": "hysteria2", "port": 9100, "settings": map[string]any{"server_name": "hy2.example.com", "certificate": cert, "private_key": key}}, cookie)
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
		"server_id": serverID, "name": "vless", "protocol": "vless", "port": 443,
		"settings": map[string]any{"private_key": privateKey, "short_id": "0123abcd", "server_names": []string{"a.com"}},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create vless: %d %s", resp.StatusCode, body)
	}
	nodeID := int64(jsonMap(t, body)["id"].(float64))

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeID), map[string]any{
		"settings": map[string]any{"short_id": "abcdef12"},
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
	if !strings.Contains(node.Settings, `"server_names":["a.com"]`) || !strings.Contains(node.Settings, `"short_id":"abcdef12"`) {
		t.Fatalf("partial update must merge public settings, got %s", node.Settings)
	}
}

func TestServerRegisterAndAgentTokenLifecycle(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	serverID := e.seedServer(t, "s1")

	resp, body := e.do(t, "POST", fmt.Sprintf("/api/servers/%d/agent-token", serverID), nil, cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("agent rotation without agent must be 404, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "POST", fmt.Sprintf("/api/servers/%d/register-token", serverID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register-token: %d %s", resp.StatusCode, body)
	}
	first := jsonMap(t, body)
	regToken1, _ := first["register_token"].(string)
	if regToken1 == "" {
		t.Fatalf("expected register token, got %s", body)
	}
	if _, err := time.Parse(time.RFC3339, first["expires_at"].(string)); err != nil {
		t.Fatalf("expected expires_at, got %s", body)
	}

	var storedHash string
	if err := e.db.QueryRowContext(context.Background(), `SELECT register_token_hash FROM servers WHERE id = ?`, serverID).Scan(&storedHash); err != nil || storedHash == "" {
		t.Fatalf("register token hash must be stored: %v", err)
	}
	if storedHash == regToken1 {
		t.Fatal("register token must be stored as hash")
	}

	resp, body = e.do(t, "POST", fmt.Sprintf("/api/servers/%d/register-token", serverID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register-token 2: %d %s", resp.StatusCode, body)
	}
	regToken2 := jsonMap(t, body)["register_token"].(string)
	if found, err := e.repo.GetServerByRegisterTokenHash(context.Background(), adminauth.HashToken(regToken1)); err == nil && found.ID == serverID {
		t.Fatal("old register token must be invalidated")
	}
	if _, err := e.repo.GetServerByRegisterTokenHash(context.Background(), adminauth.HashToken(regToken2)); err != nil {
		t.Fatalf("new register token must resolve: %v", err)
	}

	if _, err := e.repo.CreateAgent(context.Background(), serverID, "agent-hash", "1.0.0"); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	resp, body = e.do(t, "POST", fmt.Sprintf("/api/servers/%d/agent-token", serverID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agent rotation: %d %s", resp.StatusCode, body)
	}
	rotated := jsonMap(t, body)
	if rotated["agent_token"] == "" {
		t.Fatalf("expected agent_token, got %s", body)
	}
	if _, ok := rotated["expires_hint"]; !ok {
		t.Fatalf("expected expires_hint key, got %s", body)
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
	if _, err := e.repo.CreateAgent(ctx, serverID, "agent-hash", "1.0.0"); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	now := time.Now()
	if err := e.repo.ReplaceServerSessions(ctx, serverID, []repo.NewSession{
		{UserID: userID, NodeID: nodeID, ServerID: serverID, IP: "1.1.1.1", ConnectedAt: now, LastSeenAt: now},
	}); err != nil {
		t.Fatalf("sessions: %v", err)
	}
	if err := e.repo.InsertTrafficRecords(ctx, []repo.NewTrafficRecord{
		{UserID: userID, NodeID: nodeID, ServerID: serverID, UploadBytes: 5, DownloadBytes: 6, CreatedAt: now},
	}); err != nil {
		t.Fatalf("traffic: %v", err)
	}
	if _, err := e.repo.InsertConnectionLogs(ctx, []repo.NewConnectionLog{
		{UserID: userID, NodeID: nodeID, ServerID: serverID, IP: "1.1.1.1", ConnectedAt: now, Status: "closed"},
	}); err != nil {
		t.Fatalf("logs: %v", err)
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
	sessions, err := e.repo.ListSessionsByUser(ctx, userID, time.Time{})
	if err != nil || len(sessions) != 0 {
		t.Fatalf("expected sessions removed, got %v %v", sessions, err)
	}

	upload, download, err := e.repo.SumTraffic(ctx, repo.TrafficFilter{UserID: userID})
	if err != nil || upload != 5 || download != 6 {
		t.Fatalf("traffic must survive server deletion, got %d %d %v", upload, download, err)
	}
	logs, total, err := e.repo.ListConnectionLogs(ctx, repo.LogFilter{UserID: userID})
	if err != nil || total != 1 || len(logs) != 1 {
		t.Fatalf("connection logs must survive server deletion, got %d %v", total, err)
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
	if updated["address"] != "s1.example.com" {
		t.Fatalf("partial update must keep address: %s", body)
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
		"server_id": server1, "name": "n3", "protocol": "vless", "port": 9443,
		"settings": map[string]any{"private_key": testRealityPrivateKey(t), "server_names": []string{"example.com"}},
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
