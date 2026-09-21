package web_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"vps-node/internal/repo"
	"vps-node/internal/singbox"
)

func (e *testEnv) doAgent(t *testing.T, method, path string, body any, token string) (*http.Response, string) {
	t.Helper()
	return e.doWithHeader(t, method, path, body, "Authorization", "Bearer "+token)
}

func (e *testEnv) registerAgent(t *testing.T, cookie *http.Cookie, serverID int64, version string) string {
	t.Helper()
	resp, body := e.do(t, "POST", fmt.Sprintf("/api/servers/%d/register-token", serverID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register-token: %d %s", resp.StatusCode, body)
	}
	regToken := jsonMap(t, body)["register_token"].(string)

	resp, body = e.doAgent(t, "POST", "/api/agent/register",
		map[string]any{"register_token": regToken, "version": version}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agent register: %d %s", resp.StatusCode, body)
	}
	payload := jsonMap(t, body)
	token, _ := payload["agent_token"].(string)
	if token == "" {
		t.Fatalf("expected agent_token, got %s", body)
	}
	if int64(payload["server_id"].(float64)) != serverID {
		t.Fatalf("expected server_id %d, got %s", serverID, body)
	}
	for _, key := range []string{"agent_id", "heartbeat_interval_seconds", "sync_interval_seconds", "traffic_interval_seconds"} {
		if _, ok := payload[key].(float64); !ok {
			t.Fatalf("expected %s in register response, got %s", key, body)
		}
	}
	return token
}

func TestAgentRegisterSingleUse(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "s1")

	resp, body := e.doAgent(t, "POST", "/api/agent/heartbeat",
		map[string]any{"version": "1.0.0", "cpu_percent": 1.0, "memory_percent": 1.0, "disk_percent": 1.0, "uptime_seconds": 1}, "")
	if resp.StatusCode != http.StatusUnauthorized || errorCode(t, body) != "unauthorized" {
		t.Fatalf("missing token must be 401, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "POST", "/api/agent/register", map[string]any{"register_token": "bogus"}, nil)
	if resp.StatusCode != http.StatusUnauthorized || errorCode(t, body) != "unauthorized" {
		t.Fatalf("bogus register token must be 401, got %d %s", resp.StatusCode, body)
	}

	token := e.registerAgent(t, cookie, serverID, "1.0.0")

	resp, body = e.doAgent(t, "POST", "/api/agent/heartbeat", validHB(), token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat with registered token must work: %d %s", resp.StatusCode, body)
	}

	regResp, regBody := e.do(t, "POST", fmt.Sprintf("/api/servers/%d/register-token", serverID), nil, cookie)
	if regResp.StatusCode != http.StatusOK {
		t.Fatalf("second register-token: %d %s", regResp.StatusCode, regBody)
	}
	resp, body = e.doAgent(t, "POST", "/api/agent/register",
		map[string]any{"register_token": jsonMap(t, regBody)["register_token"], "version": "1.0.0"}, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fresh register token must work: %d %s", resp.StatusCode, body)
	}
	newToken := jsonMap(t, body)["agent_token"].(string)
	if newToken == "" {
		t.Fatalf("re-registration must return a new agent token, got %s", body)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/heartbeat", validHB(), token)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("re-registration must rotate the agent token (old token 401), got %d %s", resp.StatusCode, body)
	}
	resp, body = e.doAgent(t, "POST", "/api/agent/heartbeat", validHB(), newToken)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat with rotated token must work: %d %s", resp.StatusCode, body)
	}
}

func TestAgentRegisterTokenCannotBeReused(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "s1")

	resp, body := e.do(t, "POST", fmt.Sprintf("/api/servers/%d/register-token", serverID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("register-token: %d %s", resp.StatusCode, body)
	}
	regToken := jsonMap(t, body)["register_token"].(string)

	register := func(tok string) int {
		r, _ := e.doAgent(t, "POST", "/api/agent/register",
			map[string]any{"register_token": tok, "version": "1.0.0"}, "")
		return r.StatusCode
	}
	if code := register(regToken); code != http.StatusOK {
		t.Fatalf("first use of register token must succeed, got %d", code)
	}
	if code := register(regToken); code != http.StatusUnauthorized {
		t.Fatalf("second use of the same register token must be 401, got %d", code)
	}
}

func TestAgentTokenRotationInvalidatesOld(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "s1")
	token := e.registerAgent(t, cookie, serverID, "1.0.0")

	resp, body := e.do(t, "POST", fmt.Sprintf("/api/servers/%d/agent-token", serverID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agent-token rotation: %d %s", resp.StatusCode, body)
	}
	newToken := jsonMap(t, body)["agent_token"].(string)

	hb := func(tok string) int {
		resp, _ := e.doAgent(t, "POST", "/api/agent/heartbeat",
			map[string]any{"version": "1.0.0", "cpu_percent": 1.0, "memory_percent": 1.0, "disk_percent": 1.0, "uptime_seconds": 1}, tok)
		return resp.StatusCode
	}
	if code := hb(token); code != http.StatusUnauthorized {
		t.Fatalf("old agent token must be 401 after rotation, got %d", code)
	}
	if code := hb(newToken); code != http.StatusOK {
		t.Fatalf("new agent token must work, got %d", code)
	}
}

func TestAgentServerIDDerivedFromCredentialNotBody(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()

	serverA := e.seedServer(t, "a")
	serverB := e.seedServer(t, "b")
	nodeA := e.seedNode(t, serverA, "a1", 443)
	nodeB := e.seedNode(t, serverB, "b1", 8443)
	tokenA := e.registerAgent(t, cookie, serverA, "1.0.0")

	u1 := e.seedUser(t, "u1")
	if err := e.repo.AuthorizeUserNode(ctx, u1, nodeA); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)

	traffic := map[string]any{
		"batch_seq": 1,
		"server_id": serverB,
		"records": []map[string]any{
			{"user_id": u1, "node_id": nodeA, "server_id": serverB,
				"upload_bytes": 10, "download_bytes": 20, "recorded_at": now},
		},
	}
	resp, body := e.doAgent(t, "POST", "/api/agent/traffic", traffic, tokenA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("traffic with spoofed server_id must be accepted: %d %s", resp.StatusCode, body)
	}
	u, _ := e.repo.GetUser(ctx, u1)
	if u.UsedBytes != 30 {
		t.Fatalf("expected used_bytes 30, got %d", u.UsedBytes)
	}
	var recServerID int64
	if err := e.db.QueryRow(`SELECT server_id FROM traffic_records WHERE node_id = ?`, nodeA).Scan(&recServerID); err != nil {
		t.Fatalf("query traffic record: %v", err)
	}
	if recServerID != serverA {
		t.Fatalf("traffic record must be attributed to credential server %d, got %d", serverA, recServerID)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/traffic", map[string]any{
		"batch_seq": 2,
		"server_id": serverB,
		"records": []map[string]any{
			{"user_id": u1, "node_id": nodeB, "upload_bytes": 5, "download_bytes": 5, "recorded_at": now},
		},
	}, tokenA)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("foreign node must be 422 regardless of body server_id, got %d %s", resp.StatusCode, body)
	}
	u, _ = e.repo.GetUser(ctx, u1)
	if u.UsedBytes != 30 {
		t.Fatalf("rejected batch must not mutate totals, got %d", u.UsedBytes)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/sessions", map[string]any{
		"reported_at": now,
		"server_id":   serverB,
		"sessions": []map[string]any{
			{"user_id": u1, "node_id": nodeA, "ip": "1.2.3.4", "upload_bytes": 1, "download_bytes": 1,
				"connected_at": now, "last_seen_at": now},
		},
	}, tokenA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sessions with spoofed server_id must be accepted: %d %s", resp.StatusCode, body)
	}
	var sessServerID int64
	if err := e.db.QueryRow(`SELECT server_id FROM sessions WHERE node_id = ?`, nodeA).Scan(&sessServerID); err != nil {
		t.Fatalf("query session: %v", err)
	}
	if sessServerID != serverA {
		t.Fatalf("session must be attributed to credential server %d, got %d", serverA, sessServerID)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/sessions", map[string]any{
		"reported_at": now,
		"server_id":   serverB,
		"sessions": []map[string]any{
			{"user_id": u1, "node_id": nodeB, "ip": "1.2.3.4", "upload_bytes": 1, "download_bytes": 1,
				"connected_at": now, "last_seen_at": now},
		},
	}, tokenA)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("foreign node session must be 422 regardless of body server_id, got %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/sessions", u1), nil, cookie)
	if items := jsonMap(t, body)["items"].([]any); len(items) != 1 {
		t.Fatalf("rejected report must not replace snapshot, got %s", body)
	}
}

func TestAgentHeartbeatUpdatesServerMetrics(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "s1")
	token := e.registerAgent(t, cookie, serverID, "1.0.0")

	resp, body := e.doAgent(t, "POST", "/api/agent/heartbeat", map[string]any{
		"version": "2.0.0", "cpu_percent": 23.5, "memory_percent": 52.0, "disk_percent": 41.0, "uptime_seconds": 123456,
	}, token)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat: %d %s", resp.StatusCode, body)
	}
	m := jsonMap(t, body)
	if m["ok"] != true {
		t.Fatalf("expected ok, got %s", body)
	}
	if rev, _ := m["server_revision"].(float64); int64(rev) != e.revision(t, serverID) {
		t.Fatalf("heartbeat must report current revision, got %s", body)
	}
	if m["heartbeat_interval_seconds"].(float64) != 30 {
		t.Fatalf("expected default heartbeat interval, got %s", body)
	}

	s, err := e.repo.GetServer(context.Background(), serverID)
	if err != nil {
		t.Fatalf("get server: %v", err)
	}
	if s.CPUPercent != 23.5 || s.MemoryPercent != 52.0 || s.DiskPercent != 41.0 || s.UptimeSeconds != 123456 {
		t.Fatalf("server metrics not updated: %+v", s)
	}
	if s.AgentVersion != "2.0.0" || s.LastSeenAt == nil {
		t.Fatalf("agent version/last_seen not updated: %+v", s)
	}
	agent, err := e.repo.GetAgentByServerID(context.Background(), serverID)
	if err != nil || agent.Version != "2.0.0" || agent.LastSeenAt == nil {
		t.Fatalf("agent heartbeat not stored: %+v %v", agent, err)
	}
	if rev := e.revision(t, serverID); m["server_revision"].(float64) != float64(rev) {
		t.Fatalf("heartbeat must not bump revision: %d vs %s", rev, body)
	}

	for _, tc := range []map[string]any{
		{"version": "1.0", "cpu_percent": 101.0, "memory_percent": 1.0, "disk_percent": 1.0, "uptime_seconds": 1},
		{"version": "1.0", "cpu_percent": -1.0, "memory_percent": 1.0, "disk_percent": 1.0, "uptime_seconds": 1},
		{"version": "1.0", "cpu_percent": 1.0, "memory_percent": 1.0, "disk_percent": 1.0, "uptime_seconds": -5},
		{"cpu_percent": 1.0, "memory_percent": 1.0, "disk_percent": 1.0, "uptime_seconds": 1},
	} {
		resp, body = e.doAgent(t, "POST", "/api/agent/heartbeat", tc, token)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("invalid heartbeat %+v must be 422, got %d %s", tc, resp.StatusCode, body)
		}
	}
}

func TestAgentConfigCurrentAndPayload(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()
	realityPrivateKey := testRealityPrivateKey(t)

	serverA := e.seedServer(t, "a")
	serverB := e.seedServer(t, "b")

	_, body := e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverA, "name": "a-vless", "protocol": "vless", "port": 443,
		"settings": map[string]any{"private_key": realityPrivateKey, "short_id": "abcd1234", "server_names": []string{"example.com"}},
	}, cookie)
	nodeA1 := int64(jsonMap(t, body)["id"].(float64))

	_, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverA, "name": "a-ss", "protocol": "shadowsocks", "port": 8388,
		"settings": map[string]any{"method": "2022-blake3-aes-128-gcm", "password": "server-secret-pw"},
	}, cookie)
	nodeA2 := int64(jsonMap(t, body)["id"].(float64))

	_, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverB, "name": "b-ss", "protocol": "shadowsocks", "port": 8443,
		"settings": map[string]any{"method": "2022-blake3-aes-128-gcm", "password": "ss-pw"},
	}, cookie)
	nodeB := int64(jsonMap(t, body)["id"].(float64))

	tokenA := e.registerAgent(t, cookie, serverA, "1.0.0")
	tokenB := e.registerAgent(t, cookie, serverB, "1.0.0")

	u1 := e.seedUser(t, "u1")
	u2 := e.seedUser(t, "u2")
	u3 := e.seedUser(t, "u3")
	u4 := e.seedUser(t, "u4")
	if err := e.repo.AuthorizeUserNode(ctx, u1, nodeA1); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if err := e.repo.AuthorizeUserNode(ctx, u1, nodeA2); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if err := e.repo.AuthorizeUserNode(ctx, u2, nodeA2); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if err := e.repo.AuthorizeUserNode(ctx, u2, nodeB); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if err := e.repo.AuthorizeUserNode(ctx, u3, nodeA1); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if err := e.repo.SetUserStatus(ctx, u3, repo.UserStatusDisabled); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if err := e.repo.AuthorizeUserNode(ctx, u4, nodeA1); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if err := e.repo.AddUserUsedBytes(ctx, u4, 5000, 0); err != nil {
		t.Fatalf("over-quota: %v", err)
	}

	revA := e.revision(t, serverA)

	resp, body := e.doAgent(t, "GET", "/api/agent/config?version=0", nil, tokenA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("config fetch: %d %s", resp.StatusCode, body)
	}
	m := jsonMap(t, body)
	if m["status"] != "updated" {
		t.Fatalf("expected updated, got %s", body)
	}
	if int64(m["revision"].(float64)) != revA {
		t.Fatalf("expected revision %d, got %s", revA, body)
	}
	if m["renderer_version"] != singbox.ContractVersion {
		t.Fatalf("expected renderer_version, got %s", body)
	}

	singboxCfg := m["config"].(map[string]any)["singbox"].(map[string]any)
	inbounds := singboxCfg["inbounds"].([]any)
	if len(inbounds) != 2 {
		t.Fatalf("expected 2 inbounds for server A, got %d: %s", len(inbounds), body)
	}
	byTag := map[string]map[string]any{}
	for _, raw := range inbounds {
		inb := raw.(map[string]any)
		byTag[inb["tag"].(string)] = inb
	}
	vlessInbound := byTag[fmt.Sprintf("vless-%d", nodeA1)]
	if vlessInbound == nil {
		t.Fatalf("missing vless inbound: %s", body)
	}
	reality := vlessInbound["tls"].(map[string]any)["reality"].(map[string]any)
	if reality["private_key"] != realityPrivateKey {
		t.Fatalf("secret not decrypted into payload: %v", reality["private_key"])
	}
	if got := len(vlessInbound["users"].([]any)); got != 1 {
		t.Fatalf("vless inbound must only carry the eligible authorized user, got %d", got)
	}

	ssInbound := byTag[fmt.Sprintf("shadowsocks-%d", nodeA2)]
	if ssInbound == nil {
		t.Fatalf("missing ss inbound: %s", body)
	}
	if ssInbound["password"] != "server-secret-pw" {
		t.Fatalf("unexpected ss server password: %v", ssInbound["password"])
	}
	u1DTO, u2DTO := agentUserByUUID(t, m, "u1"), agentUserByUUID(t, m, "u2")
	if u1DTO == nil || u2DTO == nil {
		t.Fatalf("expected eligible users u1 and u2 in payload: %s", body)
	}
	if agentUserByUUID(t, m, "u3") != nil || agentUserByUUID(t, m, "u4") != nil {
		t.Fatalf("disabled or over-quota user must be absent: %s", body)
	}
	if len(u1DTO["nodes"].([]any)) != 2 {
		t.Fatalf("u1 must have 2 nodes, got %s", body)
	}
	var ssCred map[string]any
	for _, raw := range u1DTO["nodes"].([]any) {
		n := raw.(map[string]any)
		if n["id"].(float64) == float64(nodeA2) {
			ssCred = n["credential"].(map[string]any)
		}
	}
	if ssCred == nil || ssCred["contract"] != "ss-cred-v1" {
		t.Fatalf("missing ss credential: %s", body)
	}
	u1rec, _ := e.repo.GetUser(ctx, u1)
	wantPW, err := singbox.DeriveSSPassword(e.appKey, nodeA2, u1rec.UUID, singbox.SSMethod2022Aes128Gcm)
	if err != nil {
		t.Fatalf("derive: %v", err)
	}
	if ssCred["password"] != wantPW {
		t.Fatalf("ss credential derivation mismatch: %v != %s", ssCred["password"], wantPW)
	}
	if len(u2DTO["nodes"].([]any)) != 1 {
		t.Fatalf("u2 must only see node A2 on this server, got %s", body)
	}

	resp, body = e.doAgent(t, "GET", fmt.Sprintf("/api/agent/config?version=%d", revA), nil, tokenA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("config current fetch: %d %s", resp.StatusCode, body)
	}
	m = jsonMap(t, body)
	if m["status"] != "current" || int64(m["revision"].(float64)) != revA {
		t.Fatalf("expected current at revision %d, got %s", revA, body)
	}
	if _, has := m["config"]; has {
		t.Fatalf("current response must not carry config payload: %s", body)
	}

	resp, _ = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d", nodeA2), map[string]any{"port": 8389}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("node bump: %d %s", resp.StatusCode, body)
	}
	revA2 := e.revision(t, serverA)
	if revA2 <= revA {
		t.Fatalf("revision must increase, got %d -> %d", revA, revA2)
	}
	resp, body = e.doAgent(t, "GET", fmt.Sprintf("/api/agent/config?version=%d", revA), nil, tokenA)
	m = jsonMap(t, body)
	if m["status"] != "updated" || int64(m["revision"].(float64)) != revA2 {
		t.Fatalf("revision bump must be detected, got %s", body)
	}

	resp, body = e.doAgent(t, "GET", "/api/agent/config?version=0", nil, tokenB)
	m = jsonMap(t, body)
	users := m["users"].([]any)
	if len(users) != 1 {
		t.Fatalf("server B payload must contain only u2, got %s", body)
	}
	inbounds = m["config"].(map[string]any)["singbox"].(map[string]any)["inbounds"].([]any)
	if len(inbounds) != 1 {
		t.Fatalf("server B payload must contain only its node, got %s", body)
	}

	resp, _ = e.do(t, "PUT", fmt.Sprintf("/api/servers/%d", serverA), map[string]any{"status": "disabled"}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disable server: %d %s", resp.StatusCode, body)
	}
	resp, body = e.doAgent(t, "GET", "/api/agent/config?version=0", nil, tokenA)
	if resp.StatusCode != http.StatusForbidden || errorCode(t, body) != "forbidden" {
		t.Fatalf("disabled server must stop config sync with 403, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.doAgent(t, "GET", "/api/agent/config?version=abc", nil, tokenA)
	if resp.StatusCode != http.StatusBadRequest || errorCode(t, body) != "invalid_request" {
		t.Fatalf("invalid version param must be 400, got %d %s", resp.StatusCode, body)
	}
}

func agentUserByUUID(t *testing.T, m map[string]any, uuid string) map[string]any {
	t.Helper()
	users, _ := m["users"].([]any)
	for _, raw := range users {
		u := raw.(map[string]any)
		if u["uuid"] == uuid {
			return u
		}
	}
	return nil
}

func validHB() map[string]any {
	return map[string]any{"version": "1.0.0", "cpu_percent": 1.0, "memory_percent": 1.0, "disk_percent": 1.0, "uptime_seconds": 1}
}

func TestAgentTrafficIngestion(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()

	serverA := e.seedServer(t, "a")
	serverB := e.seedServer(t, "b")
	nodeA := e.seedNode(t, serverA, "a1", 443)
	nodeB := e.seedNode(t, serverB, "b1", 443)
	tokenA := e.registerAgent(t, cookie, serverA, "1.0.0")
	tokenB := e.registerAgent(t, cookie, serverB, "1.0.0")

	u1 := e.seedUser(t, "u1")
	u2 := e.seedUser(t, "u2")
	u3 := e.seedUser(t, "u3")
	if err := e.repo.AuthorizeUserNode(ctx, u1, nodeA); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if err := e.repo.AuthorizeUserNode(ctx, u2, nodeB); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if err := e.repo.AuthorizeUserNode(ctx, u3, nodeB); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second).Format(time.RFC3339)
	batch := map[string]any{
		"batch_seq": 1,
		"records": []map[string]any{
			{"user_id": u1, "node_id": nodeA, "upload_bytes": 100, "download_bytes": 200, "recorded_at": now},
		},
	}
	resp, body := e.doAgent(t, "POST", "/api/agent/traffic", batch, tokenA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("traffic ingest: %d %s", resp.StatusCode, body)
	}
	m := jsonMap(t, body)
	if m["accepted"] != true || m["records"].(float64) != 1 {
		t.Fatalf("unexpected response: %s", body)
	}
	u, _ := e.repo.GetUser(ctx, u1)
	if u.UsedBytes != 300 {
		t.Fatalf("expected used_bytes 300, got %d", u.UsedBytes)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/traffic", batch, tokenA)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["accepted"] != true {
		t.Fatalf("duplicate batch must return accepted: %d %s", resp.StatusCode, body)
	}
	u, _ = e.repo.GetUser(ctx, u1)
	if u.UsedBytes != 300 {
		t.Fatalf("duplicate batch must not double count, got %d", u.UsedBytes)
	}
	upload, download, err := e.repo.SumTraffic(ctx, repo.TrafficFilter{UserID: u1})
	if err != nil || upload != 100 || download != 200 {
		t.Fatalf("traffic records must be counted once: %d %d %v", upload, download, err)
	}

	crossBatch := map[string]any{
		"batch_seq": 1,
		"records": []map[string]any{
			{"user_id": u2, "node_id": nodeB, "upload_bytes": 30, "download_bytes": 20, "recorded_at": now},
		},
	}
	resp, body = e.doAgent(t, "POST", "/api/agent/traffic", crossBatch, tokenB)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("same seq from another agent must be accepted: %d %s", resp.StatusCode, body)
	}
	u2rec, _ := e.repo.GetUser(ctx, u2)
	if u2rec.UsedBytes != 50 {
		t.Fatalf("expected u2 used 50, got %d", u2rec.UsedBytes)
	}

	reject := func(name string, payload map[string]any, wantCode int, wantErr string) {
		t.Helper()
		resp, body := e.doAgent(t, "POST", "/api/agent/traffic", payload, tokenA)
		if resp.StatusCode != wantCode || (wantErr != "" && errorCode(t, body) != wantErr) {
			t.Fatalf("%s: expected %d %s, got %d %s", name, wantCode, wantErr, resp.StatusCode, body)
		}
	}

	reject("foreign node", map[string]any{
		"batch_seq": 2,
		"records": []map[string]any{
			{"user_id": u2, "node_id": nodeB, "upload_bytes": 1, "download_bytes": 1, "recorded_at": now},
		},
	}, http.StatusUnprocessableEntity, "validation")
	reject("unauthorized user", map[string]any{
		"batch_seq": 2,
		"records": []map[string]any{
			{"user_id": u3, "node_id": nodeA, "upload_bytes": 1, "download_bytes": 1, "recorded_at": now},
		},
	}, http.StatusUnprocessableEntity, "validation")
	reject("negative counter", map[string]any{
		"batch_seq": 2,
		"records": []map[string]any{
			{"user_id": u1, "node_id": nodeA, "upload_bytes": -1, "download_bytes": 1, "recorded_at": now},
		},
	}, http.StatusUnprocessableEntity, "validation")
	reject("old timestamp", map[string]any{
		"batch_seq": 2,
		"records": []map[string]any{
			{"user_id": u1, "node_id": nodeA, "upload_bytes": 1, "download_bytes": 1,
				"recorded_at": time.Now().Add(-25 * time.Hour).UTC().Format(time.RFC3339)},
		},
	}, http.StatusUnprocessableEntity, "validation")
	reject("bad timestamp", map[string]any{
		"batch_seq": 2,
		"records": []map[string]any{
			{"user_id": u1, "node_id": nodeA, "upload_bytes": 1, "download_bytes": 1, "recorded_at": "not-a-time"},
		},
	}, http.StatusUnprocessableEntity, "validation")
	reject("bad batch_seq", map[string]any{
		"batch_seq": 0,
		"records": []map[string]any{
			{"user_id": u1, "node_id": nodeA, "upload_bytes": 1, "download_bytes": 1, "recorded_at": now},
		},
	}, http.StatusUnprocessableEntity, "validation")

	oversized := map[string]any{"batch_seq": 2, "records": []map[string]any{}}
	for i := 0; i < 1001; i++ {
		oversized["records"] = append(oversized["records"].([]map[string]any), map[string]any{
			"user_id": u1, "node_id": nodeA, "upload_bytes": 1, "download_bytes": 1, "recorded_at": now,
		})
	}
	resp, body = e.doAgent(t, "POST", "/api/agent/traffic", oversized, tokenA)
	if resp.StatusCode != http.StatusRequestEntityTooLarge || errorCode(t, body) != "payload_too_large" {
		t.Fatalf("oversized batch must be 413, got %d %s", resp.StatusCode, body)
	}

	u, _ = e.repo.GetUser(ctx, u1)
	if u.UsedBytes != 300 {
		t.Fatalf("rejected batches must not mutate totals, got %d", u.UsedBytes)
	}
}

func TestAgentSessionsSnapshot(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()

	serverA := e.seedServer(t, "a")
	serverB := e.seedServer(t, "b")
	nodeA := e.seedNode(t, serverA, "a1", 443)
	nodeB := e.seedNode(t, serverB, "b1", 443)
	tokenA := e.registerAgent(t, cookie, serverA, "1.0.0")

	u1 := e.seedUser(t, "u1")
	if err := e.repo.AuthorizeUserNode(ctx, u1, nodeA); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	report := func(ip string, lastSeen time.Time) map[string]any {
		return map[string]any{
			"reported_at": now.Format(time.RFC3339),
			"sessions": []map[string]any{
				{"user_id": u1, "node_id": nodeA, "ip": ip, "upload_bytes": 10, "download_bytes": 20,
					"connected_at": lastSeen.Add(-5 * time.Minute).Format(time.RFC3339),
					"last_seen_at": lastSeen.Format(time.RFC3339)},
			},
		}
	}

	resp, body := e.doAgent(t, "POST", "/api/agent/sessions", report("1.2.3.4", now), tokenA)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["sessions"].(float64) != 1 {
		t.Fatalf("session report: %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/sessions", u1), nil, cookie)
	items := jsonMap(t, body)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["ip"] != "1.2.3.4" {
		t.Fatalf("expected fresh session, got %s", body)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/sessions", report("5.6.7.8", now), tokenA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second report: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/sessions", u1), nil, cookie)
	items = jsonMap(t, body)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["ip"] != "5.6.7.8" {
		t.Fatalf("snapshot must be fully replaced, got %s", body)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/sessions", report("9.9.9.9", now.Add(-time.Hour)), tokenA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("stale report: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/sessions", u1), nil, cookie)
	if items := jsonMap(t, body)["items"].([]any); len(items) != 0 {
		t.Fatalf("stale session must not be shown as online, got %s", body)
	}
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/sessions?include_stale=true", u1), nil, cookie)
	if items := jsonMap(t, body)["items"].([]any); len(items) != 1 {
		t.Fatalf("stale session must appear with include_stale, got %s", body)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/sessions", map[string]any{
		"reported_at": now.Format(time.RFC3339),
		"sessions": []map[string]any{
			{"user_id": u1, "node_id": nodeB, "ip": "1.2.3.4", "upload_bytes": 1, "download_bytes": 1,
				"connected_at": now.Format(time.RFC3339), "last_seen_at": now.Format(time.RFC3339)},
		},
	}, tokenA)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("foreign node session must be 422, got %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/sessions", u1), nil, cookie)
	if items := jsonMap(t, body)["items"].([]any); len(items) != 0 {
		t.Fatalf("failed report must not replace snapshot, got %s", body)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/sessions", map[string]any{
		"reported_at": now.Format(time.RFC3339),
		"sessions": []map[string]any{
			{"user_id": u1, "node_id": nodeA, "ip": "1.2.3.4", "upload_bytes": 1, "download_bytes": 1,
				"connected_at": now.Add(time.Minute).Format(time.RFC3339), "last_seen_at": now.Format(time.RFC3339)},
		},
	}, tokenA)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("connected_at after last_seen_at must be 422, got %d %s", resp.StatusCode, body)
	}
}

func TestAgentConnectionLogsIngestion(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()

	serverA := e.seedServer(t, "a")
	nodeA := e.seedNode(t, serverA, "a1", 443)
	tokenA := e.registerAgent(t, cookie, serverA, "1.0.0")

	u1 := e.seedUser(t, "u1")
	if err := e.repo.AuthorizeUserNode(ctx, u1, nodeA); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	logBatch := map[string]any{
		"batch_seq": 5,
		"logs": []map[string]any{
			{"user_id": u1, "node_id": nodeA, "ip": "1.2.3.4", "protocol": "vless",
				"upload_bytes": 100, "download_bytes": 200,
				"connected_at": now.Add(-5 * time.Minute).Format(time.RFC3339),
				"closed_at":    now.Format(time.RFC3339), "status": "closed"},
		},
	}
	resp, body := e.doAgent(t, "POST", "/api/agent/connection-logs", logBatch, tokenA)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["logs"].(float64) != 1 {
		t.Fatalf("log ingest: %d %s", resp.StatusCode, body)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/connection-logs", logBatch, tokenA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("duplicate log batch: %d %s", resp.StatusCode, body)
	}
	_, total, err := e.repo.ListConnectionLogs(ctx, repo.LogFilter{UserID: u1})
	if err != nil || total != 1 {
		t.Fatalf("duplicate log batch must not double insert, got %d %v", total, err)
	}

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/connection-logs", u1), nil, cookie)
	list := jsonMap(t, body)
	if list["total"].(float64) != 1 {
		t.Fatalf("expected 1 log, got %s", body)
	}
	entry := list["items"].([]any)[0].(map[string]any)
	for _, forbidden := range []string{"destination", "target", "host", "payload"} {
		if _, has := entry[forbidden]; has {
			t.Fatalf("log entries must not contain %s field: %s", forbidden, body)
		}
	}

	withDestination := map[string]any{
		"batch_seq": 6,
		"logs": []map[string]any{
			{"user_id": u1, "node_id": nodeA, "ip": "1.2.3.4", "protocol": "vless",
				"upload_bytes": 1, "download_bytes": 1,
				"connected_at": now.Format(time.RFC3339), "status": "active",
				"destination_host": "evil.com"},
		},
	}
	resp, body = e.doAgent(t, "POST", "/api/agent/connection-logs", withDestination, tokenA)
	if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
		t.Fatalf("destination field must be rejected with 422, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/connection-logs", map[string]any{
		"batch_seq": 6,
		"logs": []map[string]any{
			{"user_id": u1, "node_id": nodeA, "ip": "1.2.3.4", "protocol": "vless",
				"upload_bytes": 1, "download_bytes": 1,
				"connected_at": now.Format(time.RFC3339), "status": "open"},
		},
	}, tokenA)
	if resp.StatusCode != http.StatusBadRequest || errorCode(t, body) != "invalid_request" {
		t.Fatalf("unknown status enum must be 400, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/connection-logs", map[string]any{
		"batch_seq": 6,
		"logs": []map[string]any{
			{"user_id": u1, "node_id": nodeA, "ip": "1.2.3.4", "protocol": "vless",
				"upload_bytes": -5, "download_bytes": 1,
				"connected_at": now.Format(time.RFC3339), "status": "active"},
		},
	}, tokenA)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("negative counters must be 422, got %d %s", resp.StatusCode, body)
	}

	bigBatch := map[string]any{"batch_seq": 7, "logs": []map[string]any{}}
	for i := 0; i < 1001; i++ {
		bigBatch["logs"] = append(bigBatch["logs"].([]map[string]any), map[string]any{
			"user_id": u1, "node_id": nodeA, "ip": "1.2.3.4", "protocol": "vless",
			"upload_bytes": 1, "download_bytes": 1,
			"connected_at": now.Format(time.RFC3339), "status": "closed",
		})
	}
	resp, body = e.doAgent(t, "POST", "/api/agent/connection-logs", bigBatch, tokenA)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized log batch must be 413, got %d %s", resp.StatusCode, body)
	}

	_, total, _ = e.repo.ListConnectionLogs(ctx, repo.LogFilter{UserID: u1})
	if total != 1 {
		t.Fatalf("rejected log batches must not insert, got %d", total)
	}
}
