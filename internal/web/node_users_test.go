package web_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"vps-node/internal/repo"
)

func TestNodeUsersGetEmpty(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	server := e.seedServer(t, "server")
	node := e.seedNode(t, server, "node", 8080)

	resp, body := e.do(t, "GET", "/api/nodes/999/users", nil, cookie)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown node: %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "GET", "/api/nodes/0/users", nil, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated: %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "GET", fmt.Sprintf("/api/nodes/%d/users", node), nil, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get: %d %s", resp.StatusCode, body)
	}
	m := jsonMap(t, body)
	if ids, ok := m["user_ids"].([]any); !ok || len(ids) != 0 {
		t.Fatalf("want empty user_ids: %s", body)
	}
	users, ok := m["users"].([]any)
	if !ok || len(users) != 0 {
		t.Fatalf("want empty users: %s", body)
	}
}

func TestNodeUsersPutRoundTrip(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	cookie := e.login(t)
	server := e.seedServer(t, "server")
	node := e.seedNode(t, server, "node", 8080)
	u1 := e.seedUser(t, "u1")
	u2 := e.seedUser(t, "u2")

	if rev := e.revision(t, server); rev != 0 {
		t.Fatalf("initial revision=%d", rev)
	}

	resp, body := e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d/users", node), map[string]any{"user_ids": []int64{u1, u2}}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("put: %d %s", resp.StatusCode, body)
	}
	m := jsonMap(t, body)
	if ids := m["user_ids"].([]any); len(ids) != 2 {
		t.Fatalf("want 2 authorized: %s", body)
	}
	if rev := e.revision(t, server); rev != 1 {
		t.Fatalf("revision after grant=%d want=1", rev)
	}
	got, err := e.repo.ListNodeIDsByUser(ctx, u1)
	if err != nil || len(got) != 1 || got[0] != node {
		t.Fatalf("u1 nodes=%v err=%v", got, err)
	}

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d/users", node), map[string]any{"user_ids": []int64{u2}}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("overwrite: %d %s", resp.StatusCode, body)
	}
	if rev := e.revision(t, server); rev != 2 {
		t.Fatalf("revision after revoke=%d want=2", rev)
	}
	if got, _ := e.repo.ListNodeIDsByUser(ctx, u1); len(got) != 0 {
		t.Fatalf("u1 must be revoked, got %v", got)
	}

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d/users", node), map[string]any{"user_ids": []int64{u2}}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("idempotent put: %d %s", resp.StatusCode, body)
	}
	if rev := e.revision(t, server); rev != 2 {
		t.Fatalf("no-change put must not bump revision, got %d", rev)
	}

	ids, err := e.repo.ListUserIDsByNode(ctx, node)
	if err != nil || len(ids) != 1 || ids[0] != u2 {
		t.Fatalf("authorized users=%v err=%v", ids, err)
	}
}

func TestNodeUsersPutValidation(t *testing.T) {
	e := newTestEnv(t)
	cookie := e.login(t)
	server := e.seedServer(t, "server")
	node := e.seedNode(t, server, "node", 8080)

	resp, body := e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d/users", node), map[string]any{}, cookie)
	if resp.StatusCode != http.StatusBadRequest || errorCode(t, body) != "invalid_request" {
		t.Fatalf("missing user_ids: %d %s", resp.StatusCode, body)
	}

	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d/users", node), map[string]any{"user_ids": []int64{424242}}, cookie)
	if resp.StatusCode != http.StatusUnprocessableEntity || errorCode(t, body) != "validation" {
		t.Fatalf("unknown user: %d %s", resp.StatusCode, body)
	}

	if _, err := e.repo.CreateUser(context.Background(), repo.NewUser{
		UUID: "u3", Username: "user-u3", TokenHash: "hash-u3", Status: repo.UserStatusDisabled, TransferEnable: 1000,
	}); err != nil {
		t.Fatal(err)
	}
	resp, body = e.do(t, "PUT", fmt.Sprintf("/api/nodes/%d/users", node), map[string]any{"user_ids": []int64{}}, cookie)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("empty set: %d %s", resp.StatusCode, body)
	}
}
