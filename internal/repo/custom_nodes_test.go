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
	if err := r.UpdateCustomNode(ctx, id, "ext-1-renamed", []byte("new-cipher"), repo.CustomNodeStatusDisabled, "", false, false); err != nil {
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
	if err := r.UpdateCustomNode(ctx, id, "ext-1-final", nil, repo.CustomNodeStatusActive, "", false, false); err != nil {
		t.Fatalf("update without content: %v", err)
	}
	n, _ = r.GetCustomNode(ctx, id)
	if n.Name != "ext-1-final" || string(n.ContentEnc) != "new-cipher" || n.CachedContent != "cached-again" {
		t.Fatalf("update without content clobbered fields: %+v", n)
	}

	if err := r.UpdateCustomNode(ctx, 9999, "x", nil, repo.CustomNodeStatusActive, "", false, false); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("update missing: %v", err)
	}
	if err := r.DeleteCustomNode(ctx, id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if err := r.DeleteCustomNode(ctx, id); !errors.Is(err, repo.ErrNotFound) {
		t.Fatalf("delete missing: %v", err)
	}
}

func TestCustomNodeUserAgent(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	userID := mustCreateUser(t, r, "ua-user")
	id, err := r.CreateCustomNode(ctx, repo.NewCustomNode{
		Name: "ua-node", SourceType: repo.CustomNodeSourceSubscription,
		UserAgent: "Mihomo/1.18.0", ContentEnc: []byte("cipher"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := r.SetUserCustomNodes(ctx, userID, []int64{id}); err != nil {
		t.Fatalf("authorize: %v", err)
	}

	// Every read path carries the new column.
	n, err := r.GetCustomNode(ctx, id)
	if err != nil || n.UserAgent != "Mihomo/1.18.0" {
		t.Fatalf("get user agent: %+v %v", n, err)
	}
	listed, err := r.ListCustomNodes(ctx)
	if err != nil || len(listed) != 1 || listed[0].UserAgent != "Mihomo/1.18.0" {
		t.Fatalf("list user agent: %+v %v", listed, err)
	}
	active, err := r.ListActiveCustomNodesByUser(ctx, userID)
	if err != nil || len(active) != 1 || active[0].UserAgent != "Mihomo/1.18.0" {
		t.Fatalf("active user agent: %+v %v", active, err)
	}

	// A User-Agent change with invalidateCache clears the fetch cache even
	// though the stored content is untouched.
	if err := r.UpdateCustomNodeCache(ctx, id, "cached", 123); err != nil {
		t.Fatalf("cache write: %v", err)
	}
	if err := r.UpdateCustomNode(ctx, id, "ua-node", nil, repo.CustomNodeStatusActive, "sing-box/1.10.0", false, true); err != nil {
		t.Fatalf("update ua: %v", err)
	}
	n, _ = r.GetCustomNode(ctx, id)
	if n.UserAgent != "sing-box/1.10.0" || string(n.ContentEnc) != "cipher" {
		t.Fatalf("update ua clobbered fields: %+v", n)
	}
	if n.CachedContent != "" || !n.FetchedAt.IsZero() {
		t.Fatalf("ua change must clear cache: %+v", n)
	}

	// Without invalidation the cache survives.
	if err := r.UpdateCustomNodeCache(ctx, id, "cached-again", 456); err != nil {
		t.Fatalf("cache write: %v", err)
	}
	if err := r.UpdateCustomNode(ctx, id, "ua-node", nil, repo.CustomNodeStatusActive, "sing-box/1.10.0", false, false); err != nil {
		t.Fatalf("update without invalidation: %v", err)
	}
	n, _ = r.GetCustomNode(ctx, id)
	if n.CachedContent != "cached-again" || n.FetchedAt.Unix() != 456 {
		t.Fatalf("cache must survive: %+v", n)
	}

	// Clearing to the empty string restores the default-UA behavior.
	if err := r.UpdateCustomNode(ctx, id, "ua-node", nil, repo.CustomNodeStatusActive, "", false, false); err != nil {
		t.Fatalf("clear ua: %v", err)
	}
	n, _ = r.GetCustomNode(ctx, id)
	if n.UserAgent != "" {
		t.Fatalf("expected empty user agent, got %q", n.UserAgent)
	}
}

func TestCustomNodeInsecureSkipVerify(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	id, err := r.CreateCustomNode(ctx, repo.NewCustomNode{
		Name: "insecure-node", SourceType: repo.CustomNodeSourceSubscription,
		InsecureSkipVerify: true, ContentEnc: []byte("cipher"),
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	n, err := r.GetCustomNode(ctx, id)
	if err != nil || !n.InsecureSkipVerify {
		t.Fatalf("get insecure flag: %+v %v", n, err)
	}
	if err := r.UpdateCustomNode(ctx, id, "insecure-node", nil, repo.CustomNodeStatusActive, "", false, false); err != nil {
		t.Fatalf("update insecure flag: %v", err)
	}
	n, _ = r.GetCustomNode(ctx, id)
	if n.InsecureSkipVerify {
		t.Fatalf("expected insecure flag cleared: %+v", n)
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
	if err := r.UpdateCustomNode(ctx, cn2, "cn-b", nil, repo.CustomNodeStatusDisabled, "", false, false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	nodes, err := r.ListActiveCustomNodesByUser(ctx, userID)
	if err != nil || len(nodes) != 1 || nodes[0].ID != cn1 {
		t.Fatalf("active filter: %v %v", nodes, err)
	}
	if err := r.UpdateCustomNode(ctx, cn2, "cn-b", nil, repo.CustomNodeStatusActive, "", false, false); err != nil {
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

func TestUserCustomNodeEntryAuthorization(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	userID := mustCreateUser(t, r, "entry-auth")
	otherUserID := mustCreateUser(t, r, "entry-other")
	cn1 := mustCreateCustomNode(t, r, "cn-entry-a", repo.CustomNodeSourceLinks)
	cn2 := mustCreateCustomNode(t, r, "cn-entry-b", repo.CustomNodeSourceLinks)
	cn3 := mustCreateCustomNode(t, r, "cn-entry-c", repo.CustomNodeSourceLinks)

	// Default: no authorizations, no whitelists.
	if entries, err := r.ListCustomNodeEntryKeysByUser(ctx, userID); err != nil || len(entries) != 0 {
		t.Fatalf("default whitelists: %v %v", entries, err)
	}

	// Set writes sources plus non-empty whitelists; duplicates collapse and
	// blank whitelists (and unauthorized ids) are omitted.
	if err := r.SetUserCustomNodesAndEntries(ctx, userID, []int64{cn1, cn2}, map[int64][]string{
		cn1: {"key-a", "key-b", "key-a"},
		cn2: {},
		cn3: {"orphan-key"},
	}); err != nil {
		t.Fatalf("set: %v", err)
	}
	ids, err := r.ListCustomNodeIDsByUser(ctx, userID)
	if err != nil || len(ids) != 2 || ids[0] != cn1 || ids[1] != cn2 {
		t.Fatalf("ids: %v %v", ids, err)
	}
	entries, err := r.ListCustomNodeEntryKeysByUser(ctx, userID)
	if err != nil {
		t.Fatalf("whitelists: %v", err)
	}
	if len(entries) != 1 || len(entries[cn1]) != 2 {
		t.Fatalf("whitelists must only carry non-empty sources: %v", entries)
	}
	if _, ok := entries[cn1]["key-a"]; !ok {
		t.Fatalf("key-a missing: %v", entries)
	}
	if _, ok := entries[cn1]["key-b"]; !ok {
		t.Fatalf("key-b missing: %v", entries)
	}
	if _, ok := entries[cn2]; ok {
		t.Fatalf("empty whitelist must be omitted: %v", entries)
	}
	if _, ok := entries[cn3]; ok {
		t.Fatalf("unauthorized source whitelist must be ignored: %v", entries)
	}

	// The set replaces both the sources and the whitelists.
	if err := r.SetUserCustomNodesAndEntries(ctx, userID, []int64{cn2}, map[int64][]string{cn2: {"key-c"}}); err != nil {
		t.Fatalf("replace: %v", err)
	}
	ids, _ = r.ListCustomNodeIDsByUser(ctx, userID)
	if len(ids) != 1 || ids[0] != cn2 {
		t.Fatalf("after replace ids: %v", ids)
	}
	entries, _ = r.ListCustomNodeEntryKeysByUser(ctx, userID)
	if len(entries) != 1 || len(entries[cn2]) != 1 {
		t.Fatalf("after replace whitelists: %v", entries)
	}

	// Clearing the authorized set clears every whitelist row.
	if err := r.SetUserCustomNodesAndEntries(ctx, userID, []int64{}, nil); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if entries, _ := r.ListCustomNodeEntryKeysByUser(ctx, userID); len(entries) != 0 {
		t.Fatalf("after clear whitelists: %v", entries)
	}

	// Other users' whitelists are independent.
	if err := r.SetUserCustomNodesAndEntries(ctx, otherUserID, []int64{cn1}, map[int64][]string{cn1: {"other-key"}}); err != nil {
		t.Fatalf("other set: %v", err)
	}
	if entries, _ := r.ListCustomNodeEntryKeysByUser(ctx, otherUserID); len(entries[cn1]) != 1 {
		t.Fatalf("other whitelist: %v", entries)
	}
	if entries, _ := r.ListCustomNodeEntryKeysByUser(ctx, userID); len(entries) != 0 {
		t.Fatalf("user whitelist must stay empty: %v", entries)
	}

	// Deleting a custom node cascades the whitelist.
	if err := r.DeleteCustomNode(ctx, cn1); err != nil {
		t.Fatalf("delete custom node: %v", err)
	}
	if entries, _ := r.ListCustomNodeEntryKeysByUser(ctx, otherUserID); len(entries) != 0 {
		t.Fatalf("custom node cascade: %v", entries)
	}

	// Deleting a user cascades the whitelist.
	if err := r.SetUserCustomNodesAndEntries(ctx, userID, []int64{cn2}, map[int64][]string{cn2: {"key-d"}}); err != nil {
		t.Fatalf("re-set: %v", err)
	}
	if err := r.DeleteUserAndBump(ctx, userID); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if entries, _ := r.ListCustomNodeEntryKeysByUser(ctx, userID); len(entries) != 0 {
		t.Fatalf("user cascade: %v", entries)
	}
}
