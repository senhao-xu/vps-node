package web_test

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"vps-node/internal/repo"
)

func TestSettingsDefaultsAndPartialUpdate(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	resp, body := e.do(t, "GET", "/api/settings", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get settings: %d %s", resp.StatusCode, body)
	}
	s := jsonMap(t, body)
	if s["retention_raw_log_days"].(float64) != 7 ||
		s["retention_aggregate_days"].(float64) != 90 ||
		s["collection_connection_logs"] != true ||
		s["session_freshness_seconds"].(float64) != 300 ||
		s["server_offline_after_seconds"].(float64) != 60 {
		t.Fatalf("unexpected defaults: %s", body)
	}

	resp, body = e.do(t, "PUT", "/api/settings", map[string]any{"retention_raw_log_days": 14}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put settings: %d %s", resp.StatusCode, body)
	}
	s = jsonMap(t, body)
	if s["retention_raw_log_days"].(float64) != 14 || s["retention_aggregate_days"].(float64) != 90 {
		t.Fatalf("partial update must keep other fields: %s", body)
	}

	resp, body = e.do(t, "PUT", "/api/settings", map[string]any{
		"collection_connection_logs":   false,
		"session_freshness_seconds":    600,
		"server_offline_after_seconds": 120,
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put settings 2: %d %s", resp.StatusCode, body)
	}
	s = jsonMap(t, body)
	if s["collection_connection_logs"] != false ||
		s["session_freshness_seconds"].(float64) != 600 ||
		s["server_offline_after_seconds"].(float64) != 120 {
		t.Fatalf("unexpected update result: %s", body)
	}

	for name, value := range map[string]int{
		"retention_raw_log_days":       0,
		"retention_aggregate_days":     -1,
		"session_freshness_seconds":    5,
		"server_offline_after_seconds": 999999,
	} {
		resp, body = e.do(t, "PUT", "/api/settings", map[string]any{name: value}, cookie)
		if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
			t.Fatalf("%s=%d must be 422 validation, got %d %s", name, value, resp.StatusCode, body)
		}
	}
}

func TestSettingsAffectLivenessAndFreshness(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()

	resp, body := e.do(t, "PUT", "/api/settings", map[string]any{
		"server_offline_after_seconds": 10,
		"session_freshness_seconds":    10,
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put settings: %d %s", resp.StatusCode, body)
	}

	serverID := e.seedServer(t, "s1")
	if err := e.repo.UpdateServerMetrics(ctx, serverID, 10, 10, 10, 100, time.Now().Add(-20*time.Second)); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	_, body = e.do(t, "GET", "/api/servers", nil, cookie)
	items := jsonMap(t, body)["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["status"] != repo.ServerStatusOffline {
		t.Fatalf("heartbeat older than configured threshold must be offline, got %s", body)
	}
}

func TestDashboardCounts(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()

	server1 := e.seedServer(t, "fresh")
	e.seedServer(t, "no-heartbeat")
	server3 := e.seedServer(t, "disabled")
	node1 := e.seedNode(t, server1, "n1", 443)

	if err := e.repo.UpdateServerMetrics(ctx, server1, 10, 20, 30, 999, time.Now()); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if err := e.repo.UpdateServerMetrics(ctx, server3, 10, 20, 30, 999, time.Now()); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if err := e.repo.SetServerStatus(ctx, server3, repo.ServerStatusDisabled); err != nil {
		t.Fatalf("disable: %v", err)
	}

	user1 := e.seedUser(t, "u1")
	e.seedUser(t, "u2")

	now := time.Now().UTC().Truncate(time.Second)
	if err := e.repo.ReplaceServerSessions(ctx, server1, []repo.NewSession{
		{UserID: user1, NodeID: node1, ServerID: server1, IP: "1.1.1.1", ConnectedAt: now, LastSeenAt: now},
	}); err != nil {
		t.Fatalf("sessions: %v", err)
	}
	if err := e.repo.InsertTrafficRecords(ctx, []repo.NewTrafficRecord{
		{UserID: user1, NodeID: node1, ServerID: server1, UploadBytes: 100, DownloadBytes: 200, CreatedAt: now},
	}); err != nil {
		t.Fatalf("traffic: %v", err)
	}

	resp, body := e.do(t, "GET", "/api/dashboard", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("dashboard: %d %s", resp.StatusCode, body)
	}
	d := jsonMap(t, body)
	if d["users_total"].(float64) != 2 {
		t.Fatalf("users_total: %s", body)
	}
	if d["users_online"].(float64) != 1 {
		t.Fatalf("users_online: %s", body)
	}
	if d["servers_total"].(float64) != 3 {
		t.Fatalf("servers_total: %s", body)
	}
	if d["servers_online"].(float64) != 1 {
		t.Fatalf("servers_online must count only enabled servers with fresh heartbeat: %s", body)
	}
	if d["traffic_today_bytes"].(float64) != 300 {
		t.Fatalf("traffic_today_bytes: %s", body)
	}
	if d["sessions_current"].(float64) != 1 {
		t.Fatalf("sessions_current: %s", body)
	}
}

func TestRequestBodySizeLimit(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	body := append([]byte(`{"x":"`), append(bytes.Repeat([]byte("a"), 2<<20), '"')...)
	body = append(body, '}')
	req, err := http.NewRequest("POST", e.ts.URL+"/api/users", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	resp, err := e.ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized body must be 413, got %d", resp.StatusCode)
	}
}

func TestMalformedJSONRejected(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	req, err := http.NewRequest("POST", e.ts.URL+"/api/users", bytes.NewReader([]byte("{not-json")))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	resp, err := e.ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("malformed JSON must be 400, got %d", resp.StatusCode)
	}
}
