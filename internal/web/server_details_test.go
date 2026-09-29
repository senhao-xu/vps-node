package web_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"vps-node/internal/repo"
)

func TestServerDetailsBillingAndTraffic(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	resp, body := e.do(t, "POST", "/api/servers", map[string]any{
		"name":               "Frankfurt",
		"notes":              "provider note",
		"public_visible":     false,
		"offline_notify":     true,
		"ipv6":               "2001:db8::1",
		"traffic_accounting": "sum",
		"traffic_reset_day":  1,
		"billing_cycle":      "yearly",
		"price_cents":        1500,
		"price_currency":     "EUR",
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create: %d %s", resp.StatusCode, body)
	}
	created := jsonMap(t, body)
	serverID := int64(created["id"].(float64))
	if created["notes"] != "provider note" || created["public_visible"] != false || created["offline_notify"] != true || created["billing_cycle"] != "yearly" {
		t.Fatalf("server details missing from create response: %s", body)
	}

	userID := e.seedUser(t, "server-details-user")
	nodeID := e.seedNode(t, serverID, "de", 24444)
	if err := e.repo.InsertTrafficRecords(context.Background(), []repo.NewTrafficRecord{{
		UserID: userID, NodeID: nodeID, ServerID: serverID, U: 40, D: 60, CreatedAt: time.Now(),
	}}); err != nil {
		t.Fatal(err)
	}
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/servers/%d", serverID), nil, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["monthly_used_bytes"].(float64) != 100 {
		t.Fatalf("monthly sum usage: %d %s", resp.StatusCode, body)
	}

	invalidUpdates := []map[string]any{
		{"ipv6": "127.0.0.1"},
		{"traffic_accounting": "average"},
		{"traffic_reset_day": 32},
		{"billing_cycle": "weekly"},
	}
	for _, input := range invalidUpdates {
		resp, _ = e.do(t, "PUT", fmt.Sprintf("/api/servers/%d", serverID), input, cookie)
		if resp.StatusCode != http.StatusUnprocessableEntity {
			t.Fatalf("invalid update %#v must return 422, got %d", input, resp.StatusCode)
		}
	}
}
