package web_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"vps-node/internal/adminauth"
	"vps-node/internal/repo"
	"vps-node/internal/web"
)

func installConfigClock(t *testing.T, e *testEnv, clock *atomic.Int64) {
	t.Helper()
	r := repo.New(e.db.DB)
	r.Now = func() time.Time { return time.Unix(clock.Load(), 0).UTC() }
	h, err := web.New(web.Options{
		DB: e.db.DB, Repo: r, AppKey: e.appKey,
		Sessions:      adminauth.NewSessions(e.db.DB, time.Hour, false),
		Limiter:       adminauth.NewLimiter(3, time.Minute, time.Minute),
		Logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		AdminUsername: testAdminUser, AdminPassword: testAdminPass,
	})
	if err != nil {
		t.Fatal(err)
	}
	e.ts.Close()
	e.ts = httptest.NewServer(h)
	e.repo = r
	t.Cleanup(e.ts.Close)
}

func TestAgentConfigPollingRevokesDepletedAndExpiredUsers(t *testing.T) {
	for _, protocol := range []string{repo.ProtocolHTTP, repo.ProtocolSocks} {
		for _, trigger := range []string{"quota", "expiry", "expiry-and-quota"} {
			t.Run(protocol+"/"+trigger, func(t *testing.T) {
				e := newTestEnv(t)
				ctx := context.Background()
				start := time.Now().UTC().Truncate(time.Second)
				clock := &atomic.Int64{}
				clock.Store(start.Unix())
				installConfigClock(t, e, clock)
				cookie := e.login(t)
				server := e.seedServer(t, "server")
				node, err := e.repo.CreateNode(ctx, repo.NewNode{ServerID: server, Name: "node", Protocol: protocol, Port: 8080})
				if err != nil {
					t.Fatal(err)
				}
				user := e.seedUser(t, "user")
				expiry := start.Add(time.Minute)
				if trigger != "quota" {
					if err := e.repo.SetUserExpiry(ctx, user, nil, &expiry); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := e.repo.SetUserNodesAndBump(ctx, user, []int64{node}); err != nil {
					t.Fatal(err)
				}
				key := e.agentKey(t, cookie, server)
				poll := func(version int64, want string, users, inbounds int) int64 {
					t.Helper()
					resp, body := e.doAgent(t, "GET", fmt.Sprintf("/api/agent/config?version=%d", version), nil, key)
					if resp.StatusCode != http.StatusOK {
						t.Fatalf("config poll: %d %s", resp.StatusCode, body)
					}
					payload := jsonMap(t, body)
					if payload["status"] != want {
						t.Fatalf("want %s: %s", want, body)
					}
					if want == "updated" {
						if actual := len(payload["users"].([]any)); actual != users {
							t.Fatalf("users=%d want=%d: %s", actual, users, body)
						}
						config := payload["config"].(map[string]any)["singbox"].(map[string]any)
						if actual := len(config["inbounds"].([]any)); actual != inbounds {
							t.Fatalf("inbounds=%d want=%d: %s", actual, inbounds, body)
						}
					}
					return int64(payload["revision"].(float64))
				}
				initial := poll(0, "updated", 1, 1)
				if rev := poll(initial, "current", 0, 0); rev != initial {
					t.Fatalf("unchanged revision=%d", rev)
				}
				var finalBatch map[string]any
				if trigger != "expiry" {
					batch := func(seq, used int64) map[string]any {
						return map[string]any{"batch_seq": seq, "records": []map[string]any{{"user_id": user, "node_id": node, "u": used, "d": 0, "recorded_at": start.Format(time.RFC3339)}}}
					}
					resp, body := e.doAgent(t, "POST", "/api/agent/traffic", batch(1, 999), key)
					if resp.StatusCode != http.StatusOK {
						t.Fatalf("partial usage: %d %s", resp.StatusCode, body)
					}
					if rev := poll(initial, "current", 0, 0); rev != initial {
						t.Fatalf("partial usage revision=%d", rev)
					}
					finalBatch = batch(2, 1)
				}
				if trigger != "quota" {
					clock.Store(expiry.Unix())
				}
				if finalBatch != nil {
					resp, body := e.doAgent(t, "POST", "/api/agent/traffic", finalBatch, key)
					if resp.StatusCode != http.StatusOK {
						t.Fatalf("depletion: %d %s", resp.StatusCode, body)
					}
				}
				revoked := poll(initial, "updated", 0, 0)
				if revoked != initial+1 {
					t.Fatalf("revocation revision=%d want=%d", revoked, initial+1)
				}
				if finalBatch != nil {
					resp, body := e.doAgent(t, "POST", "/api/agent/traffic", finalBatch, key)
					if resp.StatusCode != http.StatusOK {
						t.Fatalf("duplicate: %d %s", resp.StatusCode, body)
					}
				}
				clock.Add(3600)
				if rev := poll(revoked, "current", 0, 0); rev != revoked {
					t.Fatalf("repeated revocation revision=%d", rev)
				}
				installConfigClock(t, e, clock)
				if rev := poll(revoked, "current", 0, 0); rev != revoked {
					t.Fatalf("restart revocation revision=%d", rev)
				}
				u, err := e.repo.GetUser(ctx, user)
				if err != nil || u.Status != repo.UserStatusActive {
					t.Fatalf("stored status must stay active: %+v %v", u, err)
				}
				if trigger != "quota" {
					renewed := time.Unix(clock.Load(), 0).Add(time.Hour)
					if _, err := e.repo.UpdateUserAndBump(ctx, user, repo.UserPatch{SetExpiresAt: true, ExpiresAt: &renewed}); err != nil {
						t.Fatal(err)
					}
				}
				if trigger != "expiry" {
					if _, err := e.repo.ResetUserTrafficAndBump(ctx, user); err != nil {
						t.Fatal(err)
					}
				}
				poll(revoked, "updated", 1, 1)
			})
		}
	}
}
