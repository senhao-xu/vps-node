package web_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"vps-node/internal/repo"
)

func seedUserTraffic(t *testing.T, e *testEnv) (user1, user2, user3 int64) {
	t.Helper()
	ctx := context.Background()

	server1 := e.seedServer(t, "hk01")
	node1 := e.seedNode(t, server1, "n1", 443)
	node2 := e.seedNode(t, server1, "n2", 444)

	user1 = e.seedUser(t, "du1")
	user2 = e.seedUser(t, "du2")
	user3 = e.seedUser(t, "du3") // no traffic

	todayStart := time.Now().UTC().Truncate(24 * time.Hour)
	if err := e.repo.InsertTrafficRecords(ctx, []repo.NewTrafficRecord{
		{UserID: user1, NodeID: node1, ServerID: server1, UploadBytes: 100, DownloadBytes: 200, CreatedAt: todayStart.Add(time.Hour)},
		{UserID: user1, NodeID: node2, ServerID: server1, UploadBytes: 7, DownloadBytes: 8, CreatedAt: todayStart.Add(2 * time.Hour)},
		{UserID: user1, NodeID: node1, ServerID: server1, UploadBytes: 50, DownloadBytes: 50, CreatedAt: todayStart.Add(-time.Hour)},
		{UserID: user2, NodeID: node2, ServerID: server1, UploadBytes: 10, DownloadBytes: 20, CreatedAt: todayStart.Add(3 * time.Hour)},
	}); err != nil {
		t.Fatalf("insert traffic: %v", err)
	}
	if err := e.repo.AddUserUsedBytes(ctx, user1, 157, 258); err != nil {
		t.Fatalf("add used bytes: %v", err)
	}
	if err := e.repo.AddUserUsedBytes(ctx, user2, 10, 20); err != nil {
		t.Fatalf("add used bytes: %v", err)
	}
	return user1, user2, user3
}

func TestDashboardUserTrafficRequiresAdmin(t *testing.T) {
	e := newTestEnv(t)

	resp, body := e.do(t, "GET", "/api/dashboard/user-traffic", nil, nil)
	if resp.StatusCode != http.StatusUnauthorized || errorCode(t, body) != "unauthorized" {
		t.Fatalf("unauthenticated request must be 401, got %d %s", resp.StatusCode, body)
	}
}

func TestDashboardUserTrafficInvalidRange(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	resp, body := e.do(t, "GET", "/api/dashboard/user-traffic?range=week", nil, cookie)
	if resp.StatusCode != http.StatusBadRequest || errorCode(t, body) != "invalid_request" {
		t.Fatalf("invalid range must be 400 invalid_request, got %d %s", resp.StatusCode, body)
	}
}

func TestDashboardUserTrafficToday(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	user1, user2, user3 := seedUserTraffic(t, e)

	resp, body := e.do(t, "GET", "/api/dashboard/user-traffic?range=today", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("today: %d %s", resp.StatusCode, body)
	}
	d := jsonMap(t, body)
	if d["range"] != "today" {
		t.Fatalf("range: %s", body)
	}
	items := d["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("all users must be listed, got %s", body)
	}

	first := items[0].(map[string]any)
	if first["user_id"].(float64) != float64(user1) || first["total_bytes"].(float64) != 315 ||
		first["upload_bytes"].(float64) != 107 || first["download_bytes"].(float64) != 208 {
		t.Fatalf("today must only count today's records and sort by total desc: %s", body)
	}
	if first["quota_bytes"].(float64) != 1000 {
		t.Fatalf("quota_bytes must always be returned: %s", body)
	}
	nodes := first["nodes"].([]any)
	if len(nodes) != 2 {
		t.Fatalf("user1 must have 2 node entries today: %s", body)
	}
	n0 := nodes[0].(map[string]any)
	if n0["node_name"] != "n1" || n0["server_name"] != "hk01" ||
		n0["upload_bytes"].(float64) != 100 || n0["download_bytes"].(float64) != 200 ||
		n0["total_bytes"].(float64) != 300 {
		t.Fatalf("node detail: %s", body)
	}

	second := items[1].(map[string]any)
	if second["user_id"].(float64) != float64(user2) || second["total_bytes"].(float64) != 30 {
		t.Fatalf("user2 today total: %s", body)
	}

	third := items[2].(map[string]any)
	if third["user_id"].(float64) != float64(user3) || third["total_bytes"].(float64) != 0 {
		t.Fatalf("user without traffic must appear with zero totals: %s", body)
	}
	if nodes, ok := third["nodes"].([]any); !ok || len(nodes) != 0 {
		t.Fatalf("user without traffic must have empty nodes array: %s", body)
	}
	if !strings.Contains(body, `"nodes":[]`) {
		t.Fatalf("empty node detail must serialize as []: %s", body)
	}
}

func TestDashboardUserTrafficTotalUsesUsedBytes(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	user1, user2, _ := seedUserTraffic(t, e)

	resp, body := e.do(t, "GET", "/api/dashboard/user-traffic?range=total", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("total: %d %s", resp.StatusCode, body)
	}
	d := jsonMap(t, body)
	if d["range"] != "total" {
		t.Fatalf("range: %s", body)
	}
	items := d["items"].([]any)
	if len(items) != 3 {
		t.Fatalf("all users must be listed, got %s", body)
	}

	first := items[0].(map[string]any)
	// users.used_bytes = 415，包含昨日记录；与 records 当日合计(315)不同以验证口径。
	if first["user_id"].(float64) != float64(user1) || first["total_bytes"].(float64) != 415 {
		t.Fatalf("total mode must use users.used_bytes: %s", body)
	}
	nodes := first["nodes"].([]any)
	var node1Detail map[string]any
	for _, raw := range nodes {
		n := raw.(map[string]any)
		if n["node_name"] == "n1" {
			node1Detail = n
		}
	}
	if node1Detail == nil || node1Detail["total_bytes"].(float64) != 400 {
		t.Fatalf("total mode node detail must include all retained records: %s", body)
	}

	if items[1].(map[string]any)["user_id"].(float64) != float64(user2) ||
		items[1].(map[string]any)["total_bytes"].(float64) != 30 {
		t.Fatalf("user2 total: %s", body)
	}
}

func TestDashboardUserTrafficTotalReflectsTrafficReset(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	user1, _, _ := seedUserTraffic(t, e)
	ctx := context.Background()

	if err := e.repo.ResetUserTraffic(ctx, user1); err != nil {
		t.Fatalf("reset traffic: %v", err)
	}

	resp, body := e.do(t, "GET", "/api/dashboard/user-traffic?range=total", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("total: %d %s", resp.StatusCode, body)
	}
	items := jsonMap(t, body)["items"].([]any)
	var first map[string]any
	for _, raw := range items {
		item := raw.(map[string]any)
		if item["user_id"].(float64) == float64(user1) {
			first = item
		}
	}
	if first == nil || first["total_bytes"].(float64) != 0 {
		t.Fatalf("reset must zero used_bytes in total mode: %s", body)
	}
	// 节点明细来自留存记录，不受重置影响（已知口径差异）。
	if len(first["nodes"].([]any)) == 0 {
		t.Fatalf("node detail still reflects retained records after reset: %s", body)
	}
}

func TestDashboardUserTrafficDefaultRangeIsToday(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	seedUserTraffic(t, e)

	resp, body := e.do(t, "GET", "/api/dashboard/user-traffic", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("default range: %d %s", resp.StatusCode, body)
	}
	d := jsonMap(t, body)
	if d["range"] != "today" {
		t.Fatalf("default range must be today: %s", body)
	}
}
