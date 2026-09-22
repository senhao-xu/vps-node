package web_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"vps-node/internal/repo"
)

func TestVisitQueriesAndSettings(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()

	serverID := e.seedServer(t, "s1")
	nodeID := e.seedNode(t, serverID, "n1", 443)
	userID := e.seedUser(t, "v1")
	if err := e.repo.AuthorizeUserNode(ctx, userID, nodeID); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	agentID, err := e.repo.CreateAgent(ctx, serverID, "agent-hash", "1.0.0")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	at := time.Now().UTC().Truncate(time.Second)
	records := []repo.NewVisitRecord{
		{UserID: userID, NodeID: nodeID, ServerID: serverID, DestHost: "a.example.com", DestPort: 443, Network: "tcp", ClientIP: "1.1.1.1", CreatedAt: at},
		{UserID: userID, NodeID: nodeID, ServerID: serverID, DestHost: "b.example.com", DestPort: 80, Network: "tcp", ClientIP: "2.2.2.2", CreatedAt: at},
	}
	if _, _, err := e.repo.IngestVisitBatch(ctx, agentID, 1, serverID, records); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	resp, body := e.do(t, "GET", "/api/visits?host=a.example", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("visit list: %d %s", resp.StatusCode, body)
	}
	m := jsonMap(t, body)
	if m["total"].(float64) != 1 {
		t.Fatalf("host filter must narrow results, got %s", body)
	}

	resp, body = e.do(t, "GET", "/api/visits?page_size=1", nil, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["total"].(float64) != 2 {
		t.Fatalf("paged list: %d %s", resp.StatusCode, body)
	}
	if items := jsonMap(t, body)["items"].([]any); len(items) != 1 {
		t.Fatalf("page_size must limit items, got %s", body)
	}

	resp, body = e.do(t, "GET", "/api/visits?user_id=abc", nil, cookie)
	if resp.StatusCode != http.StatusBadRequest || errorCode(t, body) != "invalid_request" {
		t.Fatalf("invalid user_id must be 400, got %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "GET", "/api/visits/top?days=0", nil, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid days must be 400, got %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "GET", "/api/visits/top?limit=abc", nil, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid limit must be 400, got %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "GET", "/api/users/999999/visits", nil, cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing user must be 404, got %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "GET", "/api/servers/999999/visits", nil, cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing server must be 404, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "GET", "/api/settings", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("settings get: %d %s", resp.StatusCode, body)
	}
	settings := jsonMap(t, body)
	if settings["collection_visits"] != true || settings["retention_visit_days"].(float64) != 7 || settings["retention_visit_aggregate_days"].(float64) != 90 {
		t.Fatalf("unexpected visit settings defaults: %s", body)
	}

	resp, body = e.do(t, "PUT", "/api/settings", map[string]any{
		"collection_visits":              false,
		"retention_visit_days":           3,
		"retention_visit_aggregate_days": 30,
	}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("settings put: %d %s", resp.StatusCode, body)
	}
	settings = jsonMap(t, body)
	if settings["collection_visits"] != false || settings["retention_visit_days"].(float64) != 3 || settings["retention_visit_aggregate_days"].(float64) != 30 {
		t.Fatalf("settings put must persist, got %s", body)
	}

	resp, body = e.do(t, "PUT", "/api/settings", map[string]any{"retention_visit_days": 0}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
		t.Fatalf("out-of-range visit retention must be 422, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/servers/%d/visits?node_id=%d", serverID, nodeID), nil, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["total"].(float64) != 2 {
		t.Fatalf("server visits node filter: %d %s", resp.StatusCode, body)
	}
}
