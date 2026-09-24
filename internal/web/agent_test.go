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

func (e *testEnv) agentKey(t *testing.T, cookie *http.Cookie, serverID int64) string {
	t.Helper()
	resp, body := e.do(t, "POST", fmt.Sprintf("/api/servers/%d/agent-key", serverID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agent-key generate: %d %s", resp.StatusCode, body)
	}
	key, _ := jsonMap(t, body)["agent_key"].(string)
	if key == "" {
		t.Fatalf("expected agent_key, got %s", body)
	}
	return key
}

func TestAgentKeyLifecycle(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "s1")

	resp, body := e.doAgent(t, "POST", "/api/agent/heartbeat", validHB(), "")
	if resp.StatusCode != http.StatusUnauthorized || errorCode(t, body) != "unauthorized" {
		t.Fatalf("missing key must be 401, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/heartbeat", validHB(), "bogus")
	if resp.StatusCode != http.StatusUnauthorized || errorCode(t, body) != "unauthorized" {
		t.Fatalf("bogus key must be 401, got %d %s", resp.StatusCode, body)
	}

	key := e.agentKey(t, cookie, serverID)

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/servers/%d/agent-key", serverID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("agent-key read: %d %s", resp.StatusCode, body)
	}
	if got := jsonMap(t, body)["agent_key"]; got != key {
		t.Fatalf("stored key must be retrievable, got %v want %s", got, key)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/heartbeat", validHB(), key)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat with agent key must work: %d %s", resp.StatusCode, body)
	}

	newKey := e.agentKey(t, cookie, serverID)
	if newKey == key {
		t.Fatal("reset must issue a new key")
	}
	resp, body = e.doAgent(t, "POST", "/api/agent/heartbeat", validHB(), key)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("old key must be 401 after reset, got %d %s", resp.StatusCode, body)
	}
	resp, body = e.doAgent(t, "POST", "/api/agent/heartbeat", validHB(), newKey)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("new key must work after reset, got %d %s", resp.StatusCode, body)
	}
}

func TestAgentKeyGetConflictsWithoutStoredKey(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "s1")
	if _, err := e.repo.CreateAgent(context.Background(), serverID, "legacy-hash", "1.0.0"); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	resp, body := e.do(t, "GET", fmt.Sprintf("/api/servers/%d/agent-key", serverID), nil, cookie)
	if resp.StatusCode != http.StatusConflict || errorCode(t, body) != "conflict" {
		t.Fatalf("key without key_enc must be 409, got %d %s", resp.StatusCode, body)
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
	tokenA := e.agentKey(t, cookie, serverA)

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
				"u": 10, "d": 20, "recorded_at": now},
		},
	}
	resp, body := e.doAgent(t, "POST", "/api/agent/traffic", traffic, tokenA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("traffic with spoofed server_id must be accepted: %d %s", resp.StatusCode, body)
	}
	u, _ := e.repo.GetUser(ctx, u1)
	if u.UsedBytes() != 30 {
		t.Fatalf("expected used_bytes 30, got %d", u.UsedBytes())
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
			{"user_id": u1, "node_id": nodeB, "u": 5, "d": 5, "recorded_at": now},
		},
	}, tokenA)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("foreign node must be 422 regardless of body server_id, got %d %s", resp.StatusCode, body)
	}
	u, _ = e.repo.GetUser(ctx, u1)
	if u.UsedBytes() != 30 {
		t.Fatalf("rejected batch must not mutate totals, got %d", u.UsedBytes())
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/devices", map[string]any{
		"batch_seq":   1,
		"recorded_at": now,
		"server_id":   serverB,
		"devices": []map[string]any{
			{"user_id": u1, "node_id": nodeA, "ips": []string{"1.2.3.4"}, "online": 1},
		},
	}, tokenA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("devices with spoofed server_id must be accepted: %d %s", resp.StatusCode, body)
	}
	var deviceServerID int64
	if err := e.db.QueryRow(`SELECT server_id FROM online_devices WHERE node_id = ?`, nodeA).Scan(&deviceServerID); err != nil {
		t.Fatalf("query device: %v", err)
	}
	if deviceServerID != serverA {
		t.Fatalf("device must be attributed to credential server %d, got %d", serverA, deviceServerID)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/devices", map[string]any{
		"batch_seq":   2,
		"recorded_at": now,
		"server_id":   serverB,
		"devices": []map[string]any{
			{"user_id": u1, "node_id": nodeB, "ips": []string{"1.2.3.4"}, "online": 1},
		},
	}, tokenA)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("foreign node device must be 422 regardless of body server_id, got %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/devices", u1), nil, cookie)
	if items := jsonMap(t, body)["items"].([]any); len(items) != 1 {
		t.Fatalf("rejected report must not replace snapshot, got %s", body)
	}
}

func TestAgentVisitIngestion(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()

	serverA := e.seedServer(t, "a")
	serverB := e.seedServer(t, "b")
	nodeA := e.seedNode(t, serverA, "a1", 443)
	nodeB := e.seedNode(t, serverB, "b1", 443)
	tokenA := e.agentKey(t, cookie, serverA)
	tokenB := e.agentKey(t, cookie, serverB)

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
	visit := func(seq int64, user, node int64, host string, port int, network, at string) map[string]any {
		return map[string]any{
			"batch_seq": seq,
			"records": []map[string]any{
				{"user_id": user, "node_id": node, "dest_host": host, "dest_port": port,
					"network": network, "client_ip": "1.2.3.4", "recorded_at": at},
			},
		}
	}

	resp, body := e.doAgent(t, "POST", "/api/agent/visits", visit(1, u1, nodeA, "Example.COM", 443, "tcp", now), tokenA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("visit ingest: %d %s", resp.StatusCode, body)
	}
	m := jsonMap(t, body)
	if m["accepted"] != true || m["records"].(float64) != 1 {
		t.Fatalf("unexpected response: %s", body)
	}
	visits, total, err := e.repo.ListVisits(ctx, repo.VisitFilter{})
	if err != nil || total != 1 {
		t.Fatalf("expected 1 stored visit, total=%d err=%v", total, err)
	}
	if visits[0].DestHost != "example.com" || visits[0].ServerID != serverA {
		t.Fatalf("visit must be lowercased and attributed to credential server, got %+v", visits[0])
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/visits", visit(1, u1, nodeA, "Example.COM", 443, "tcp", now), tokenA)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["accepted"] != true {
		t.Fatalf("duplicate batch must return accepted: %d %s", resp.StatusCode, body)
	}
	if _, total, _ := e.repo.ListVisits(ctx, repo.VisitFilter{}); total != 1 {
		t.Fatalf("duplicate batch must not double store, total=%d", total)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/visits", visit(1, u2, nodeB, "b.example.com", 80, "tcp", now), tokenB)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("same seq from another agent must be accepted: %d %s", resp.StatusCode, body)
	}

	reject := func(name string, payload map[string]any, wantCode int, wantErr string) {
		t.Helper()
		resp, body := e.doAgent(t, "POST", "/api/agent/visits", payload, tokenA)
		if resp.StatusCode != wantCode || (wantErr != "" && errorCode(t, body) != wantErr) {
			t.Fatalf("%s: expected %d %s, got %d %s", name, wantCode, wantErr, resp.StatusCode, body)
		}
	}

	reject("foreign node", visit(2, u2, nodeB, "b.example.com", 80, "tcp", now), http.StatusUnprocessableEntity, "validation")
	reject("unauthorized user", visit(2, u3, nodeA, "a.example.com", 80, "tcp", now), http.StatusUnprocessableEntity, "validation")
	reject("missing user", visit(2, 0, nodeA, "a.example.com", 80, "tcp", now), http.StatusUnprocessableEntity, "validation")
	reject("empty host", visit(2, u1, nodeA, "   ", 80, "tcp", now), http.StatusUnprocessableEntity, "validation")
	reject("negative port", visit(2, u1, nodeA, "a.example.com", -1, "tcp", now), http.StatusUnprocessableEntity, "validation")
	reject("port too large", visit(2, u1, nodeA, "a.example.com", 70000, "tcp", now), http.StatusUnprocessableEntity, "validation")
	reject("illegal network", visit(2, u1, nodeA, "a.example.com", 80, "icmp", now), http.StatusUnprocessableEntity, "validation")
	reject("stale timestamp", visit(2, u1, nodeA, "a.example.com", 80, "tcp",
		time.Now().Add(-25*time.Hour).UTC().Format(time.RFC3339)), http.StatusUnprocessableEntity, "validation")
	reject("bad timestamp", visit(2, u1, nodeA, "a.example.com", 80, "tcp", "not-a-time"), http.StatusUnprocessableEntity, "validation")
	reject("bad batch_seq", visit(0, u1, nodeA, "a.example.com", 80, "tcp", now), http.StatusUnprocessableEntity, "validation")

	longHost := make([]byte, 254)
	for i := range longHost {
		longHost[i] = 'a'
	}
	reject("host too long", visit(2, u1, nodeA, string(longHost), 80, "tcp", now), http.StatusUnprocessableEntity, "validation")

	oversized := map[string]any{"batch_seq": 2, "records": []map[string]any{}}
	for i := 0; i < 1001; i++ {
		oversized["records"] = append(oversized["records"].([]map[string]any), map[string]any{
			"user_id": u1, "node_id": nodeA, "dest_host": "a.example.com", "dest_port": 80,
			"network": "tcp", "recorded_at": now,
		})
	}
	resp, body = e.doAgent(t, "POST", "/api/agent/visits", oversized, tokenA)
	if resp.StatusCode != http.StatusRequestEntityTooLarge || errorCode(t, body) != "payload_too_large" {
		t.Fatalf("oversized batch must be 413, got %d %s", resp.StatusCode, body)
	}

	if err := e.repo.SetSetting(ctx, "collection.visits", "false"); err != nil {
		t.Fatalf("set setting: %v", err)
	}
	resp, body = e.doAgent(t, "POST", "/api/agent/visits", visit(3, u1, nodeA, "disabled.example.com", 443, "tcp", now), tokenA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("disabled collection must still ack: %d %s", resp.StatusCode, body)
	}
	m = jsonMap(t, body)
	if m["accepted"] != false || m["records"].(float64) != 0 {
		t.Fatalf("disabled collection must not store: %s", body)
	}
	if _, total, _ := e.repo.ListVisits(ctx, repo.VisitFilter{}); total != 2 {
		t.Fatalf("disabled collection must not add storage, total=%d", total)
	}

	if err := e.repo.SetSetting(ctx, "collection.visits", "true"); err != nil {
		t.Fatalf("re-enable setting: %v", err)
	}
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/visits", u1), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("user visits: %d %s", resp.StatusCode, body)
	}
	page := jsonMap(t, body)
	if page["total"].(float64) != 1 {
		t.Fatalf("user visits must be scoped, got %s", body)
	}
	if items := page["items"].([]any); items[0].(map[string]any)["username"] != "user-u1" {
		t.Fatalf("user visits must include display names, got %s", body)
	}
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/servers/%d/visits", serverB), nil, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["total"].(float64) != 1 {
		t.Fatalf("server visits must be scoped, got %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "GET", "/api/visits/top?days=7", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("visit top: %d %s", resp.StatusCode, body)
	}
	if items := jsonMap(t, body)["items"].([]any); len(items) != 2 {
		t.Fatalf("expected 2 top hosts, got %s", body)
	}
}

func TestAgentHeartbeatUpdatesServerMetrics(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	serverID := e.seedServer(t, "s1")
	token := e.agentKey(t, cookie, serverID)

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
		"server_id": serverA, "address": "a.example.com", "name": "a-vless", "protocol": "vless", "port": 443,
		"settings": map[string]any{"private_key": realityPrivateKey, "reality_settings": map[string]any{"server_name": "example.com", "short_id": "abcd1234"}},
	}, cookie)
	nodeA1 := int64(jsonMap(t, body)["id"].(float64))

	_, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverA, "address": "a.example.com", "name": "a-ss", "protocol": "shadowsocks", "port": 8388,
		"settings": map[string]any{"cipher": "2022-blake3-aes-128-gcm", "password": "server-secret-pw"},
	}, cookie)
	nodeA2 := int64(jsonMap(t, body)["id"].(float64))

	_, body = e.do(t, "POST", "/api/nodes", map[string]any{
		"server_id": serverB, "address": "b.example.com", "name": "b-ss", "protocol": "shadowsocks", "port": 8443,
		"settings": map[string]any{"cipher": "2022-blake3-aes-128-gcm", "password": "ss-pw"},
	}, cookie)
	nodeB := int64(jsonMap(t, body)["id"].(float64))

	tokenA := e.agentKey(t, cookie, serverA)
	tokenB := e.agentKey(t, cookie, serverB)

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
	tokenA := e.agentKey(t, cookie, serverA)
	tokenB := e.agentKey(t, cookie, serverB)

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
			{"user_id": u1, "node_id": nodeA, "u": 100, "d": 200, "recorded_at": now},
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
	if u.UsedBytes() != 300 {
		t.Fatalf("expected used_bytes 300, got %d", u.UsedBytes())
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/traffic", batch, tokenA)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["accepted"] != true {
		t.Fatalf("duplicate batch must return accepted: %d %s", resp.StatusCode, body)
	}
	u, _ = e.repo.GetUser(ctx, u1)
	if u.UsedBytes() != 300 {
		t.Fatalf("duplicate batch must not double count, got %d", u.UsedBytes())
	}
	upload, download, err := e.repo.SumTraffic(ctx, repo.TrafficFilter{UserID: u1})
	if err != nil || upload != 100 || download != 200 {
		t.Fatalf("traffic records must be counted once: %d %d %v", upload, download, err)
	}

	crossBatch := map[string]any{
		"batch_seq": 1,
		"records": []map[string]any{
			{"user_id": u2, "node_id": nodeB, "u": 30, "d": 20, "recorded_at": now},
		},
	}
	resp, body = e.doAgent(t, "POST", "/api/agent/traffic", crossBatch, tokenB)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("same seq from another agent must be accepted: %d %s", resp.StatusCode, body)
	}
	u2rec, _ := e.repo.GetUser(ctx, u2)
	if u2rec.UsedBytes() != 50 {
		t.Fatalf("expected u2 used 50, got %d", u2rec.UsedBytes())
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
			{"user_id": u2, "node_id": nodeB, "u": 1, "d": 1, "recorded_at": now},
		},
	}, http.StatusUnprocessableEntity, "validation")
	reject("unauthorized user", map[string]any{
		"batch_seq": 2,
		"records": []map[string]any{
			{"user_id": u3, "node_id": nodeA, "u": 1, "d": 1, "recorded_at": now},
		},
	}, http.StatusUnprocessableEntity, "validation")
	reject("negative counter", map[string]any{
		"batch_seq": 2,
		"records": []map[string]any{
			{"user_id": u1, "node_id": nodeA, "u": -1, "d": 1, "recorded_at": now},
		},
	}, http.StatusUnprocessableEntity, "validation")
	reject("old timestamp", map[string]any{
		"batch_seq": 2,
		"records": []map[string]any{
			{"user_id": u1, "node_id": nodeA, "u": 1, "d": 1,
				"recorded_at": time.Now().Add(-25 * time.Hour).UTC().Format(time.RFC3339)},
		},
	}, http.StatusUnprocessableEntity, "validation")
	reject("bad timestamp", map[string]any{
		"batch_seq": 2,
		"records": []map[string]any{
			{"user_id": u1, "node_id": nodeA, "u": 1, "d": 1, "recorded_at": "not-a-time"},
		},
	}, http.StatusUnprocessableEntity, "validation")
	reject("bad batch_seq", map[string]any{
		"batch_seq": 0,
		"records": []map[string]any{
			{"user_id": u1, "node_id": nodeA, "u": 1, "d": 1, "recorded_at": now},
		},
	}, http.StatusUnprocessableEntity, "validation")

	oversized := map[string]any{"batch_seq": 2, "records": []map[string]any{}}
	for i := 0; i < 1001; i++ {
		oversized["records"] = append(oversized["records"].([]map[string]any), map[string]any{
			"user_id": u1, "node_id": nodeA, "u": 1, "d": 1, "recorded_at": now,
		})
	}
	resp, body = e.doAgent(t, "POST", "/api/agent/traffic", oversized, tokenA)
	if resp.StatusCode != http.StatusRequestEntityTooLarge || errorCode(t, body) != "payload_too_large" {
		t.Fatalf("oversized batch must be 413, got %d %s", resp.StatusCode, body)
	}

	u, _ = e.repo.GetUser(ctx, u1)
	if u.UsedBytes() != 300 {
		t.Fatalf("rejected batches must not mutate totals, got %d", u.UsedBytes())
	}
}

func TestAgentDevicesSnapshot(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()

	serverA := e.seedServer(t, "a")
	serverB := e.seedServer(t, "b")
	nodeA := e.seedNode(t, serverA, "a1", 443)
	nodeB := e.seedNode(t, serverB, "b1", 443)
	tokenA := e.agentKey(t, cookie, serverA)

	u1 := e.seedUser(t, "u1")
	if err := e.repo.AuthorizeUserNode(ctx, u1, nodeA); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	reportTime := now.Add(-time.Minute)
	report := func(seq int64, ips []string, online int) map[string]any {
		reportTime = reportTime.Add(time.Second)
		return map[string]any{
			"batch_seq":   seq,
			"recorded_at": reportTime.Format(time.RFC3339),
			"devices": []map[string]any{
				{"user_id": u1, "node_id": nodeA, "ips": ips, "online": online},
			},
		}
	}

	resp, body := e.doAgent(t, "POST", "/api/agent/devices", report(1, []string{"1.2.3.4", "5.6.7.8"}, 2), tokenA)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["devices"].(float64) != 2 {
		t.Fatalf("device report: %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/devices", u1), nil, cookie)
	items := jsonMap(t, body)["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("expected 2 devices, got %s", body)
	}
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d", u1), nil, cookie)
	if jsonMap(t, body)["online_count"].(float64) != 2 {
		t.Fatalf("expected online_count 2, got %s", body)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/devices", report(2, []string{"9.9.9.9"}, 1), tokenA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("second report: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/devices", u1), nil, cookie)
	items = jsonMap(t, body)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["ip"] != "9.9.9.9" {
		t.Fatalf("snapshot must be fully replaced, got %s", body)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/devices", report(3, nil, 0), tokenA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("empty report: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/devices", u1), nil, cookie)
	if items := jsonMap(t, body)["items"].([]any); len(items) != 0 {
		t.Fatalf("empty report must clear devices, got %s", body)
	}
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d", u1), nil, cookie)
	if jsonMap(t, body)["online_count"].(float64) != 0 {
		t.Fatalf("expected online_count 0, got %s", body)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/devices", map[string]any{
		"batch_seq":   4,
		"recorded_at": now.Format(time.RFC3339),
		"devices": []map[string]any{
			{"user_id": u1, "node_id": nodeB, "ips": []string{"1.2.3.4"}, "online": 1},
		},
	}, tokenA)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("foreign node device must be 422, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/devices", map[string]any{
		"batch_seq":   4,
		"recorded_at": now.Format(time.RFC3339),
		"devices": []map[string]any{
			{"user_id": u1, "node_id": nodeA, "ips": []string{""}, "online": 1},
		},
	}, tokenA)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("empty ip must be 422, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/devices", map[string]any{
		"batch_seq": 4,
		"devices": []map[string]any{
			{"user_id": u1, "node_id": nodeA, "ips": []string{"1.2.3.4"}, "online": 1},
		},
	}, tokenA)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("missing recorded_at must be 422, got %d %s", resp.StatusCode, body)
	}

	oversized := map[string]any{"batch_seq": 5, "recorded_at": now.Format(time.RFC3339), "devices": []map[string]any{}}
	for i := 0; i < 1001; i++ {
		oversized["devices"] = append(oversized["devices"].([]map[string]any), map[string]any{
			"user_id": u1, "node_id": nodeA, "ips": []string{"1.2.3.4"}, "online": 1,
		})
	}
	resp, body = e.doAgent(t, "POST", "/api/agent/devices", oversized, tokenA)
	if resp.StatusCode != http.StatusRequestEntityTooLarge || errorCode(t, body) != "payload_too_large" {
		t.Fatalf("oversized device batch must be 413, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.doAgent(t, "POST", "/api/agent/devices", report(3, []string{"1.2.3.4"}, 1), tokenA)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("duplicate batch_seq must be idempotent: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/devices", u1), nil, cookie)
	if items := jsonMap(t, body)["items"].([]any); len(items) != 0 {
		t.Fatalf("duplicate batch_seq must not recreate devices, got %s", body)
	}
}
