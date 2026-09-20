package repo_test

import (
	"context"
	"testing"
	"time"

	"vps-node/internal/repo"
)

func TestListEligibleUsersByServer(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	serverID := mustCreateServer(t, r, "s1")
	otherServer := mustCreateServer(t, r, "s2")
	nodeID := mustCreateNode(t, r, serverID, "n1", 443)
	otherNode := mustCreateNode(t, r, otherServer, "n-other", 443)

	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(24 * time.Hour)

	mkUser := func(uuid string, quota, used int64, expires *time.Time) int64 {
		id, err := r.CreateUser(ctx, repo.NewUser{
			UUID: uuid, TokenHash: "hash-" + uuid, Status: repo.UserStatusActive,
			QuotaBytes: quota, ExpiresAt: expires,
		})
		if err != nil {
			t.Fatalf("create user %s: %v", uuid, err)
		}
		if used > 0 {
			if err := r.AddUserUsedBytes(ctx, id, used, 0); err != nil {
				t.Fatalf("add used: %v", err)
			}
		}
		return id
	}

	eligibleActive := mkUser("u-active", 100, 10, nil)
	eligibleUnlimited := mkUser("u-unlimited", 0, 1000, nil)
	eligibleFuture := mkUser("u-future", 100, 0, &future)
	expiredUser := mkUser("u-expired", 100, 0, &past)
	disabledUser := mkUser("u-disabled", 100, 0, nil)
	if err := r.SetUserStatus(ctx, disabledUser, repo.UserStatusDisabled); err != nil {
		t.Fatalf("disable: %v", err)
	}
	quotaReached := mkUser("u-quota-reached", 100, 100, nil)
	quotaExceeded := mkUser("u-quota-exceeded", 100, 101, nil)
	unauthorized := mkUser("u-unauthorized", 100, 0, nil)
	otherServerUser := mkUser("u-other-server", 100, 0, nil)

	authorized := []int64{eligibleActive, eligibleUnlimited, eligibleFuture, expiredUser,
		disabledUser, quotaReached, quotaExceeded}
	for _, uid := range authorized {
		if err := r.AuthorizeUserNode(ctx, uid, nodeID); err != nil {
			t.Fatalf("authorize: %v", err)
		}
	}
	if err := r.AuthorizeUserNode(ctx, otherServerUser, otherNode); err != nil {
		t.Fatalf("authorize other: %v", err)
	}

	eligible, err := r.ListEligibleUsersByServer(ctx, serverID, now)
	if err != nil {
		t.Fatalf("list eligible: %v", err)
	}

	got := map[int64]bool{}
	for _, u := range eligible {
		got[u.ID] = true
	}
	want := map[int64]bool{eligibleActive: true, eligibleUnlimited: true, eligibleFuture: true}
	for _, id := range []int64{expiredUser, disabledUser, quotaReached, quotaExceeded, unauthorized, otherServerUser} {
		want[id] = false
	}
	for id, expected := range want {
		if got[id] != expected {
			t.Fatalf("user %d eligibility = %v, want %v", id, got[id], expected)
		}
	}
	if len(eligible) != 3 {
		t.Fatalf("expected 3 eligible users, got %d", len(eligible))
	}
}
