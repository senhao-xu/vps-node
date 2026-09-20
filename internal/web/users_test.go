package web_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"vps-node/internal/repo"
)

func TestUserCreateListDetailAndSecretExposure(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	expires := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Second).Format(time.RFC3339)
	resp, body := e.do(t, "POST", "/api/users", map[string]any{
		"quota_bytes": 1000,
		"expires_at":  expires,
		"node_ids":    []int64{},
	}, cookie)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create user: %d %s", resp.StatusCode, body)
	}
	created := jsonMap(t, body)
	token, _ := created["token"].(string)
	if token == "" {
		t.Fatalf("expected token returned once on create, got %s", body)
	}
	id := int64(created["id"].(float64))
	if created["uuid"] == "" || created["status"] != "active" || created["quota_bytes"].(float64) != 1000 {
		t.Fatalf("unexpected create payload: %s", body)
	}

	resp, body = e.do(t, "GET", "/api/users", nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list users: %d %s", resp.StatusCode, body)
	}
	list := jsonMap(t, body)
	if list["total"].(float64) != 1 {
		t.Fatalf("expected total 1, got %s", body)
	}
	assertNoSecrets(t, body, token)

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d", id), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get user: %d %s", resp.StatusCode, body)
	}
	detail := jsonMap(t, body)
	if detail["remaining_bytes"].(float64) != 1000 {
		t.Fatalf("expected remaining 1000, got %s", body)
	}
	if detail["used_percent"].(float64) != 0 {
		t.Fatalf("expected used_percent 0, got %s", body)
	}
	assertNoSecrets(t, body, token)

	if _, ok := detail["token"]; ok {
		t.Fatalf("detail must not contain token: %s", body)
	}
}

func TestUserTokenQueryAndReset(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	resp, body := e.do(t, "POST", "/api/users", map[string]any{"quota_bytes": 10}, cookie)
	created := jsonMap(t, body)
	id := int64(created["id"].(float64))
	oldToken, _ := created["token"].(string)

	resp, body = e.do(t, "GET", "/api/users?query="+oldToken, nil, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["total"].(float64) != 1 {
		t.Fatalf("token search failed: %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "POST", fmt.Sprintf("/api/users/%d/reset-token", id), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reset-token: %d %s", resp.StatusCode, body)
	}
	newToken := jsonMap(t, body)["token"].(string)
	if newToken == "" || newToken == oldToken {
		t.Fatalf("expected fresh token, got %s", body)
	}

	_, body = e.do(t, "GET", "/api/users?query="+oldToken, nil, cookie)
	if jsonMap(t, body)["total"].(float64) != 0 {
		t.Fatalf("old token must stop matching after reset: %s", body)
	}
	_, body = e.do(t, "GET", "/api/users?query="+newToken, nil, cookie)
	if jsonMap(t, body)["total"].(float64) != 1 {
		t.Fatalf("new token must match: %s", body)
	}

	_, body = e.do(t, "GET", "/api/users?query="+created["uuid"].(string), nil, cookie)
	if jsonMap(t, body)["total"].(float64) != 1 {
		t.Fatalf("uuid search must match: %s", body)
	}
}

func TestUserUpdatePartialSemantics(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	started := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	expires := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	_, body := e.do(t, "POST", "/api/users", map[string]any{
		"quota_bytes": 1000, "started_at": started, "expires_at": expires,
	}, cookie)
	created := jsonMap(t, body)
	id := int64(created["id"].(float64))

	_, body = e.do(t, "PUT", fmt.Sprintf("/api/users/%d", id), map[string]any{"status": "disabled"}, cookie)
	if got := jsonMap(t, body)["status"]; got != "disabled" {
		t.Fatalf("expected disabled, got %s", body)
	}
	if got := jsonMap(t, body)["quota_bytes"].(float64); got != 1000 {
		t.Fatalf("partial update must keep quota, got %s", body)
	}

	_, body = e.do(t, "PUT", fmt.Sprintf("/api/users/%d", id), map[string]any{"expires_at": nil}, cookie)
	if got := jsonMap(t, body)["expires_at"]; got != nil {
		t.Fatalf("expected expires_at cleared, got %s", body)
	}

	_, body = e.do(t, "PUT", fmt.Sprintf("/api/users/%d", id), map[string]any{"expires_at": expires}, cookie)
	if jsonMap(t, body)["status"] != "disabled" {
		t.Fatalf("future expiry must not change status: %s", body)
	}

	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	_, body = e.do(t, "PUT", fmt.Sprintf("/api/users/%d", id), map[string]any{"expires_at": past}, cookie)
	if got := jsonMap(t, body)["status"]; got != repo.UserStatusExpired {
		t.Fatalf("past expiry must mark user expired, got %s", body)
	}

	resp, body := e.do(t, "PUT", fmt.Sprintf("/api/users/%d", id), map[string]any{"status": "bogus"}, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad status, got %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/users/%d", id), map[string]any{"quota_bytes": -5}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
		t.Fatalf("expected 422 for negative quota, got %d %s", resp.StatusCode, body)
	}

	_, body = e.do(t, "PUT", "/api/users/99999", map[string]any{"status": "active"}, cookie)
	if errorCode(t, body) != "not_found" {
		t.Fatalf("expected not_found for missing user, got %s", body)
	}
}

func TestUserResetTrafficAndExpireNow(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	_, body := e.do(t, "POST", "/api/users", map[string]any{"quota_bytes": 100}, cookie)
	id := int64(jsonMap(t, body)["id"].(float64))

	if err := e.repo.AddUserUsedBytes(context.Background(), id, 30, 40); err != nil {
		t.Fatalf("add used: %v", err)
	}

	resp, body := e.do(t, "POST", fmt.Sprintf("/api/users/%d/reset-traffic", id), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reset-traffic: %d %s", resp.StatusCode, body)
	}
	reset := jsonMap(t, body)
	if reset["used_bytes"].(float64) != 0 {
		t.Fatalf("expected used_bytes 0, got %s", body)
	}
	if reset["remaining_bytes"].(float64) != 100 {
		t.Fatalf("expected remaining 100, got %s", body)
	}

	before := time.Now().UTC()
	resp, body = e.do(t, "POST", fmt.Sprintf("/api/users/%d/expire-now", id), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expire-now: %d %s", resp.StatusCode, body)
	}
	expired := jsonMap(t, body)
	if expired["status"] != repo.UserStatusExpired {
		t.Fatalf("expected expired status, got %s", body)
	}
	expiryTime, err := time.Parse(time.RFC3339, expired["expires_at"].(string))
	if err != nil {
		t.Fatalf("bad expires_at: %s", body)
	}
	if expiryTime.Before(before.Add(-5*time.Second)) || expiryTime.After(time.Now().UTC().Add(5*time.Second)) {
		t.Fatalf("expected expires_at ~ now, got %v", expiryTime)
	}
}

func TestUserDelete(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	serverID := e.seedServer(t, "s1")
	nodeID := e.seedNode(t, serverID, "n1", 443)
	_, body := e.do(t, "POST", "/api/users", map[string]any{"node_ids": []int64{nodeID}}, cookie)
	id := int64(jsonMap(t, body)["id"].(float64))

	resp, body := e.do(t, "DELETE", fmt.Sprintf("/api/users/%d", id), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete: %d %s", resp.StatusCode, body)
	}
	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d", id), nil, cookie)
	if resp.StatusCode != http.StatusNotFound || errorCode(t, body) != "not_found" {
		t.Fatalf("expected 404 after delete, got %d %s", resp.StatusCode, body)
	}
	ids, err := e.repo.ListNodeIDsByUser(context.Background(), id)
	if err != nil || len(ids) != 0 {
		t.Fatalf("user_nodes must be cascade deleted: %v %v", ids, err)
	}
}

func TestUserNodesAuthorizationIdempotent(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	serverID := e.seedServer(t, "s1")
	node1 := e.seedNode(t, serverID, "n1", 443)
	node2 := e.seedNode(t, serverID, "n2", 8443)
	disabledNode := e.seedNode(t, serverID, "n3", 9443)
	if err := e.repo.SetNodeStatus(context.Background(), disabledNode, repo.NodeStatusDisabled); err != nil {
		t.Fatalf("disable node: %v", err)
	}

	_, body := e.do(t, "POST", "/api/users", map[string]any{}, cookie)
	id := int64(jsonMap(t, body)["id"].(float64))

	resp, body := e.do(t, "PUT", fmt.Sprintf("/api/users/%d/nodes", id),
		map[string]any{"node_ids": []int64{node1, node1, node2, node1}}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put nodes: %d %s", resp.StatusCode, body)
	}
	payload := jsonMap(t, body)
	nodeIDs := payload["node_ids"].([]any)
	if len(nodeIDs) != 2 {
		t.Fatalf("duplicate authorization must not create duplicates, got %s", body)
	}

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/nodes", id), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get nodes: %d %s", resp.StatusCode, body)
	}
	payload = jsonMap(t, body)
	if len(payload["node_ids"].([]any)) != 2 || len(payload["nodes"].([]any)) != 2 {
		t.Fatalf("unexpected authorization payload: %s", body)
	}
	assertNoSecrets(t, body)

	count, err := e.repo.CountNodesByUser(context.Background(), id)
	if err != nil || count != 2 {
		t.Fatalf("expected 2 stored relations, got %d %v", count, err)
	}

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/users/%d/nodes", id),
		map[string]any{"node_ids": []int64{node1, 99999}}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
		t.Fatalf("unknown node must be 422 validation, got %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/users/%d/nodes", id),
		map[string]any{"node_ids": []int64{disabledNode}}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("disabled node must be rejected, got %d %s", resp.StatusCode, body)
	}

	count, _ = e.repo.CountNodesByUser(context.Background(), id)
	if count != 2 {
		t.Fatalf("failed requests must not change authorization, got %d", count)
	}

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/users/%d/nodes", id),
		map[string]any{"node_ids": []int64{}}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear nodes: %d %s", resp.StatusCode, body)
	}
	if got := len(jsonMap(t, body)["node_ids"].([]any)); got != 0 {
		t.Fatalf("expected empty set, got %s", body)
	}

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/users/%d/nodes", id), map[string]any{}, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("missing node_ids must be 400, got %d %s", resp.StatusCode, body)
	}
}

func TestUserSessionsLogsTrafficEndpoints(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()

	serverID := e.seedServer(t, "s1")
	nodeID := e.seedNode(t, serverID, "n1", 443)
	userID := e.seedUser(t, "u1")

	now := time.Now().UTC().Truncate(time.Second)
	fresh := []repo.NewSession{
		{UserID: userID, NodeID: nodeID, ServerID: serverID, IP: "1.1.1.1", ConnectedAt: now, LastSeenAt: now},
	}
	if err := e.repo.ReplaceServerSessions(ctx, serverID, fresh); err != nil {
		t.Fatalf("replace sessions: %v", err)
	}
	staleSession := []repo.NewSession{
		{UserID: userID, NodeID: nodeID, ServerID: serverID, IP: "2.2.2.2", ConnectedAt: now.Add(-2 * time.Hour), LastSeenAt: now.Add(-2 * time.Hour)},
	}
	if err := e.repo.ReplaceServerSessions(ctx, serverID, append(append([]repo.NewSession{}, fresh...), staleSession...)); err != nil {
		t.Fatalf("replace sessions 2: %v", err)
	}

	resp, body := e.do(t, "GET", fmt.Sprintf("/api/users/%d/sessions", userID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sessions: %d %s", resp.StatusCode, body)
	}
	items := jsonMap(t, body)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 fresh session, got %s", body)
	}
	session := items[0].(map[string]any)
	if session["ip"] != "1.1.1.1" || session["node_id"].(float64) != float64(nodeID) || session["server_id"].(float64) != float64(serverID) {
		t.Fatalf("unexpected session payload: %s", body)
	}

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/sessions?include_stale=true", userID), nil, cookie)
	if len(jsonMap(t, body)["items"].([]any)) != 2 {
		t.Fatalf("expected 2 sessions with include_stale, got %s", body)
	}

	connected := now.Add(-2 * time.Hour)
	if _, err := e.repo.InsertConnectionLogs(ctx, []repo.NewConnectionLog{
		{UserID: userID, NodeID: nodeID, ServerID: serverID, IP: "1.1.1.1", Protocol: "vless",
			UploadBytes: 1, DownloadBytes: 2, ConnectedAt: connected, Status: repo.UserStatusActive},
		{UserID: userID, NodeID: nodeID, ServerID: serverID, IP: "1.1.1.1", Protocol: "vless",
			UploadBytes: 3, DownloadBytes: 4, ConnectedAt: now.Add(-30 * time.Minute),
			ClosedAt: ptrTime(now.Add(-time.Minute)), Status: "closed"},
	}); err != nil {
		t.Fatalf("insert logs: %v", err)
	}

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/connection-logs", userID), nil, cookie)
	if resp.StatusCode != http.StatusOK || jsonMap(t, body)["total"].(float64) != 2 {
		t.Fatalf("logs list: %d %s", resp.StatusCode, body)
	}
	assertNoSecrets(t, body)

	from := now.Add(-45 * time.Minute).Format(time.RFC3339)
	_, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/connection-logs?from=%s", userID, from), nil, cookie)
	if jsonMap(t, body)["total"].(float64) != 1 {
		t.Fatalf("expected time filter to match 1 log, got %s", body)
	}

	_, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/connection-logs?page=2&page_size=1", userID), nil, cookie)
	page := jsonMap(t, body)
	if page["total"].(float64) != 2 || len(page["items"].([]any)) != 1 || page["page"].(float64) != 2 {
		t.Fatalf("unexpected pagination: %s", body)
	}

	if err := e.repo.InsertTrafficRecords(ctx, []repo.NewTrafficRecord{
		{UserID: userID, NodeID: nodeID, ServerID: serverID, UploadBytes: 10, DownloadBytes: 20, CreatedAt: now.Add(-30 * time.Hour)},
		{UserID: userID, NodeID: nodeID, ServerID: serverID, UploadBytes: 30, DownloadBytes: 40, CreatedAt: now.Add(-time.Hour)},
	}); err != nil {
		t.Fatalf("insert traffic: %v", err)
	}

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/traffic", userID), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("traffic: %d %s", resp.StatusCode, body)
	}
	traffic := jsonMap(t, body)
	if traffic["total_upload_bytes"].(float64) != 40 || traffic["total_download_bytes"].(float64) != 60 {
		t.Fatalf("unexpected traffic totals: %s", body)
	}
	if len(traffic["series"].([]any)) != 2 {
		t.Fatalf("expected 2 daily buckets, got %s", body)
	}

	_, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/traffic?bucket=hour&from=%s", userID, now.Add(-90*time.Minute).Format(time.RFC3339)), nil, cookie)
	traffic = jsonMap(t, body)
	if traffic["total_upload_bytes"].(float64) != 30 || len(traffic["series"].([]any)) != 1 {
		t.Fatalf("unexpected hourly series: %s", body)
	}

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/users/%d/traffic?bucket=week", userID), nil, cookie)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid bucket must be 400, got %d %s", resp.StatusCode, body)
	}
}

func TestUserEndpointsNotFound(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)

	for _, path := range []string{
		"/api/users/99999",
		"/api/users/99999/nodes",
		"/api/users/99999/sessions",
		"/api/users/99999/connection-logs",
		"/api/users/99999/traffic",
	} {
		resp, body := e.do(t, "GET", path, nil, cookie)
		if resp.StatusCode != http.StatusNotFound || errorCode(t, body) != "not_found" {
			t.Fatalf("%s: expected 404, got %d %s", path, resp.StatusCode, body)
		}
	}

	resp, body := e.do(t, "GET", "/api/users/not-a-number", nil, cookie)
	if resp.StatusCode != http.StatusBadRequest || errorCode(t, body) != "invalid_request" {
		t.Fatalf("invalid id must be 400, got %d %s", resp.StatusCode, body)
	}
}

func TestUserListFiltersAndPagination(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	ctx := context.Background()

	e.seedUser(t, "u-1")
	u2 := e.seedUser(t, "u-2")
	if err := e.repo.SetUserStatus(ctx, u2, repo.UserStatusDisabled); err != nil {
		t.Fatalf("disable: %v", err)
	}
	past := time.Now().Add(-time.Hour)
	if err := e.repo.SetUserExpiry(ctx, u2, nil, &past); err != nil {
		t.Fatalf("expire: %v", err)
	}
	e.seedUser(t, "u-3")

	_, body := e.do(t, "GET", "/api/users?status=active", nil, cookie)
	list := jsonMap(t, body)
	if list["total"].(float64) != 2 {
		t.Fatalf("expected 2 active users, got %s", body)
	}

	_, body = e.do(t, "GET", "/api/users?expiry=expired", nil, cookie)
	if jsonMap(t, body)["total"].(float64) != 1 {
		t.Fatalf("expected 1 expired user, got %s", body)
	}

	_, body = e.do(t, "GET", "/api/users?status=bogus", nil, cookie)
	if errorCode(t, body) != "invalid_request" {
		t.Fatalf("bad status filter must be invalid_request, got %s", body)
	}

	_, body = e.do(t, "GET", "/api/users?page=1&page_size=2", nil, cookie)
	list = jsonMap(t, body)
	if list["total"].(float64) != 3 || len(list["items"].([]any)) != 2 || list["page_size"].(float64) != 2 {
		t.Fatalf("unexpected pagination: %s", body)
	}

	_, body = e.do(t, "GET", "/api/users?page=0", nil, cookie)
	if errorCode(t, body) != "invalid_request" {
		t.Fatalf("page 0 must be invalid_request, got %s", body)
	}
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
