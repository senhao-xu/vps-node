package repo_test

import (
	"context"
	"errors"
	"testing"

	"vps-node/internal/repo"
)

func mustCreateCustomNode(t *testing.T, r *repo.Repo, name, sourceType string) int64 {
	t.Helper()
	id, err := r.CreateCustomNode(context.Background(), repo.NewCustomNode{
		Name: name, SourceType: sourceType, ContentEnc: []byte("ciphertext"),
	})
	if err != nil {
		t.Fatalf("create custom node: %v", err)
	}
	return id
}

func TestCustomNodeCRUD(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	id := mustCreateCustomNode(t, r, "ext-1", repo.CustomNodeSourceLinks)

	n, err := r.GetCustomNode(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if n.Name != "ext-1" || n.SourceType != repo.CustomNodeSourceLinks || n.Status != repo.CustomNodeStatusActive {
		t.Fatalf("unexpected custom node: %+v", n)
	}
	if n.CachedContent != "" || !n.FetchedAt.IsZero() {
		t.Fatalf("fresh node must have no cache: %+v", n)
	}

	if _, err := r.CreateCustomNode(ctx, repo.NewCustomNode{Name: "ext-1", SourceType: repo.CustomNodeSourceLinks, ContentEnc: []byte("x")}); !errors.Is(err, repo.ErrConflict) {
		t.Fatalf("duplicate name: %v", err)
	}

	if err := r.UpdateCustomNodeCache(ctx, id, "cached-body", 123); err != nil {
		t.Fatalf("cache write: %v", err)
	}
	n, _ = r.GetCustomNode(ctx, id)
	if n.CachedContent != "cached-body" || n.FetchedAt.Unix() != 123 {
		t.Fatalf("cache not stored: %+v", n)
	}

	// Content change invalidates the cache.
	if err := r.UpdateCustomNode(ctx, id, "ext-1-renamed", []byte("new-cipher"), repo.CustomNodeStatusDisabled); err != nil {
		t.Fatalf("update: %v", err)
	}
	n, _ = r.GetCustomNode(ctx, id)
	if n.Name != "ext-1-renamed" || n.Status != repo.CustomNodeStatusDisabled || string(n.ContentEnc) != "new-cipher" {
		t.Fatalf("update not applied: %+v", n)
	}
	if n.CachedContent != "" || !n.FetchedAt.IsZero() {
		t.Fatalf("content change must clear cache: %+v", n)
	}

	// nil content keeps the stored content and cache.
	if err := r.UpdateCustomNodeCache(ctx, id, "cached-again", 456); err != nil {
		t.Fatalf("cache write: %v", err)
	}
	if err := r.UpdateCustomNode(ctx, id, "ext-1-final", nil, repo.CustomNodeStatusActive); err != nil {
		t.Fatalf("update without content: %v", err)
	}
	n, _ = r.GetCustomNode(ctx, id)
	if n.Name != "ext-1-final" || string(n.ContentEnc) != "new-cipher" || n.CachedContent != "cached-again" {
		t.Fatalf("update without content clobbered fields: %+v", n)
	}

	if err := r.UpdateCustomNode(ctx, 9999, "x", nil, repo.CustomNodeStatusActive); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}
	if err := r.DeleteCustomNode(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := r.DeleteCustomNode(ctx, id); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
}

func TestUserCustomNodeAuthorization(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	userID := mustCreateUser(t, r, "custom-auth")
	otherUserID := mustCreateUser(t, r, "custom-other")
	cn1 := mustCreateCustomNode(t, r, "cn-a", repo.CustomNodeSourceLinks)
	cn2 := mustCreateCustomNode(t, r, "cn-b", repo.CustomNodeSourceSubscription)

	// Default: nobody sees anything.
	if nodes, err := r.ListActiveCustomNodesByUser(ctx, userID); err != nil || len(nodes) != 0 {
		t.Fatalf("default visibility: %v %v", nodes, err)
	}

	if err := r.SetUserCustomNodes(ctx, userID, []int64{cn1, cn2, cn1}); err != nil {
		t.Fatalf("set: %v", err)
	}
	ids, err := r.ListCustomNodeIDsByUser(ctx, userID)
	if err != nil || len(ids) != 2 || ids[0] != cn1 || ids[1] != cn2 {
		t.Fatalf("ids: %v %v", ids, err)
	}

	// Disabled custom nodes are filtered out of subscription listings.
	if err := r.UpdateCustomNode(ctx, cn2, "cn-b", nil, repo.CustomNodeStatusDisabled); err != nil {
		t.Fatalf("disable: %v", err)
	}
	nodes, err := r.ListActiveCustomNodesByUser(ctx, userID)
	if err != nil || len(nodes) != 1 || nodes[0].ID != cn1 {
		t.Fatalf("active filter: %v %v", nodes, err)
	}
	if err := r.UpdateCustomNode(ctx, cn2, "cn-b", nil, repo.CustomNodeStatusActive); err != nil {
		t.Fatalf("enable: %v", err)
	}

	// Revoke / authorize individually.
	if err := r.RevokeUserCustomNode(ctx, userID, cn1); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	ids, _ = r.ListCustomNodeIDsByUser(ctx, userID)
	if len(ids) != 1 || ids[0] != cn2 {
		t.Fatalf("after revoke: %v", ids)
	}
	if err := r.AuthorizeUserCustomNode(ctx, userID, cn1); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if err := r.AuthorizeUserCustomNode(ctx, userID, cn1); err != nil {
		t.Fatalf("re-authorize must be idempotent: %v", err)
	}
	ids, _ = r.ListCustomNodeIDsByUser(ctx, userID)
	if len(ids) != 2 {
		t.Fatalf("after authorize: %v", ids)
	}

	// Set replaces the whole set.
	if err := r.SetUserCustomNodes(ctx, userID, []int64{}); err != nil {
		t.Fatalf("clear: %v", err)
	}
	ids, _ = r.ListCustomNodeIDsByUser(ctx, userID)
	if len(ids) != 0 {
		t.Fatalf("after clear: %v", ids)
	}

	// Deleting a custom node cascades its authorizations; other users' rows
	// survive.
	if err := r.AuthorizeUserCustomNode(ctx, userID, cn1); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if err := r.AuthorizeUserCustomNode(ctx, otherUserID, cn1); err != nil {
		t.Fatalf("authorize other: %v", err)
	}
	if err := r.DeleteCustomNode(ctx, cn1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	for _, uid := range []int64{userID, otherUserID} {
		ids, _ = r.ListCustomNodeIDsByUser(ctx, uid)
		if len(ids) != 0 {
			t.Fatalf("cascade for user %d: %v", uid, ids)
		}
	}

	// Deleting a user cascades its authorizations.
	if err := r.AuthorizeUserCustomNode(ctx, userID, cn2); err != nil {
		t.Fatalf("authorize: %v", err)
	}
	if err := r.DeleteUserAndBump(ctx, userID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	ids, _ = r.ListCustomNodeIDsByUser(ctx, userID)
	if len(ids) != 0 {
		t.Fatalf("user cascade: %v", ids)
	}
}
