package web_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"vps-node/internal/repo"
)

func TestServerInventoryAndOrdering(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	resp, body := e.do(t, "POST", "/api/servers", map[string]any{
		"name": "Hong Kong", "ip": "103.21.244.18", "region": "HK", "price_cents": 3500,
		"price_currency": "USD", "traffic_limit_bytes": int64(1000) * 1024 * 1024 * 1024,
		"expires_at": "2026-12-31T23:59:59Z",
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	first := int64(jsonMap(t, body)["id"].(float64))
	second := e.seedServer(t, "Tokyo")
	user := e.seedUser(t, "server-inventory-user")
	node := e.seedNode(t, first, "hk", 12345)
	if err := e.repo.InsertTrafficRecords(context.Background(), []repo.NewTrafficRecord{{UserID: user, NodeID: node, ServerID: first, U: 10, D: 20, CreatedAt: time.Now()}}); err != nil {
		t.Fatal(err)
	}

	resp, body = e.do(t, "GET", "/api/servers", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list: %d %s", resp.StatusCode, body)
	}
	items := jsonMap(t, body)["items"].([]any)
	inventory := items[0].(map[string]any)
	if inventory["ip"] != "103.21.244.18" || inventory["region"] != "HK" || inventory["traffic_used_bytes"].(float64) != 30 {
		t.Fatalf("inventory or traffic missing: %s", body)
	}
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/servers/%d", first), map[string]any{"expires_at": "", "price_cents": 2800}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update: %d %s", resp.StatusCode, body)
	}
	updated := jsonMap(t, body)
	if updated["expires_at"] != nil || updated["price_cents"].(float64) != 2800 || updated["traffic_used_bytes"].(float64) != 30 {
		t.Fatalf("update payload: %s", body)
	}

	if _, err := e.db.ExecContext(context.Background(), `DELETE FROM traffic_records WHERE server_id = ?`, first); err != nil {
		t.Fatal(err)
	}
	_, body = e.do(t, "GET", fmt.Sprintf("/api/servers/%d", first), nil, cookie)
	if jsonMap(t, body)["traffic_used_bytes"].(float64) != 30 {
		t.Fatalf("retention must not reduce cumulative traffic: %s", body)
	}
	resp, _ = e.do(t, "PUT", fmt.Sprintf("/api/servers/%d", first), map[string]any{"ip": "not-an-ip"}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid IP must fail, got %d", resp.StatusCode)
	}

	resp, body = e.do(t, "PUT", "/api/servers/order", map[string]any{"ids": []int64{second, first}}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reorder: %d %s", resp.StatusCode, body)
	}
	_, body = e.do(t, "GET", "/api/servers", nil, cookie)
	items = jsonMap(t, body)["items"].([]any)
	if int64(items[0].(map[string]any)["id"].(float64)) != second {
		t.Fatalf("order not saved: %s", body)
	}
	resp, _ = e.do(t, "PUT", "/api/servers/order", map[string]any{"ids": []int64{first, first}}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("duplicate order must fail, got %d", resp.StatusCode)
	}
}
