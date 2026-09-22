package janitor_test

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"vps-node/internal/db"
	"vps-node/internal/janitor"
	"vps-node/internal/repo"
)

type fixture struct {
	db   *db.DB
	repo *repo.Repo
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	store := repo.New(d.DB)
	if _, err := store.CreateAdmin(context.Background(), "root", "hash"); err != nil {
		t.Fatalf("create admin: %v", err)
	}
	return &fixture{db: d, repo: store}
}

func mustCount(t *testing.T, f *fixture, query string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := f.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("count %q: %v", query, err)
	}
	return n
}

func seedTraffic(t *testing.T, f *fixture, userID, nodeID, serverID int64, u, d int64, at time.Time) {
	t.Helper()
	if _, err := f.db.ExecContext(context.Background(),
		`INSERT INTO traffic_records (user_id, node_id, server_id, u, d, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		userID, nodeID, serverID, u, d, at.Unix()); err != nil {
		t.Fatalf("seed traffic: %v", err)
	}
}

func TestSweepRemovesOnlyExpiredRows(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	serverID, err := f.repo.CreateServer(ctx, "s1", "1.2.3.4", repo.ServerStatusActive)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	userID, err := f.repo.CreateUser(ctx, repo.NewUser{UUID: "u1", Username: "user-u1", TokenHash: "h1", Status: repo.UserStatusActive})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	nodeID, err := f.repo.CreateNode(ctx, repo.NewNode{ServerID: serverID, Name: "n1", Protocol: repo.ProtocolVLESS, Port: 443})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	recent := now.AddDate(0, 0, -1)
	seedTraffic(t, f, userID, nodeID, serverID, 1, 1, now.AddDate(0, 0, -200))
	seedTraffic(t, f, userID, nodeID, serverID, 2, 2, recent)

	stale := now.AddDate(0, 0, -40)
	if _, err := f.db.ExecContext(ctx,
		`INSERT INTO online_devices (user_id, node_id, server_id, ip, last_seen_at, created_at) VALUES (?, ?, ?, '4.4.4.4', ?, ?)`,
		userID, nodeID, serverID, stale.Unix(), stale.Unix()); err != nil {
		t.Fatalf("seed stale device: %v", err)
	}
	if _, err := f.db.ExecContext(ctx,
		`INSERT INTO online_devices (user_id, node_id, server_id, ip, last_seen_at, created_at) VALUES (?, ?, ?, '3.3.3.3', ?, ?)`,
		userID, nodeID, serverID, now.Unix(), now.Unix()); err != nil {
		t.Fatalf("seed fresh device: %v", err)
	}
	if _, err := f.db.ExecContext(ctx, `UPDATE users SET online_count = 2 WHERE id = ?`, userID); err != nil {
		t.Fatalf("seed online_count: %v", err)
	}

	if _, err := f.db.Exec(
		`INSERT INTO admin_sessions (token_hash, admin_id, created_at, last_seen_at, expires_at)
		 VALUES ('expired-hash', 1, 1, 1, ?)`, stale.Unix()); err != nil {
		t.Fatalf("seed expired admin session: %v", err)
	}
	if _, err := f.db.Exec(
		`INSERT INTO admin_sessions (token_hash, admin_id, created_at, last_seen_at, expires_at)
		 VALUES ('valid-hash', 1, 1, 1, ?)`, now.Add(time.Hour).Unix()); err != nil {
		t.Fatalf("seed valid admin session: %v", err)
	}

	agentID, err := f.repo.CreateAgent(ctx, serverID, "agent-hash", "1.0.0")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if _, err := f.db.Exec(
		`INSERT INTO traffic_batches (agent_id, seq, received_at, records) VALUES (?, 1, ?, 3)`,
		agentID, now.AddDate(0, 0, -200).Unix()); err != nil {
		t.Fatalf("seed old traffic batch: %v", err)
	}
	if _, err := f.db.Exec(
		`INSERT INTO traffic_batches (agent_id, seq, received_at, records) VALUES (?, 2, ?, 1)`,
		agentID, recent.Unix()); err != nil {
		t.Fatalf("seed recent traffic batch: %v", err)
	}
	if _, err := f.db.Exec(
		`INSERT INTO device_batches (agent_id, seq, received_at, devices) VALUES (?, 1, ?, 3)`,
		agentID, now.AddDate(0, 0, -200).Unix()); err != nil {
		t.Fatalf("seed old device batch: %v", err)
	}
	if _, err := f.db.Exec(
		`INSERT INTO device_batches (agent_id, seq, received_at, devices) VALUES (?, 2, ?, 1)`,
		agentID, recent.Unix()); err != nil {
		t.Fatalf("seed recent device batch: %v", err)
	}

	j := janitor.New(f.repo, time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := j.Sweep(ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	if got := mustCount(t, f, `SELECT COUNT(*) FROM traffic_records`); got != 1 {
		t.Fatalf("expected 1 recent traffic record to survive, got %d", got)
	}
	if got := mustCount(t, f, `SELECT COUNT(*) FROM online_devices`); got != 1 {
		t.Fatalf("expected 1 fresh device to survive, got %d", got)
	}
	var onlineCount int64
	if err := f.db.QueryRowContext(ctx, `SELECT online_count FROM users WHERE id = ?`, userID).Scan(&onlineCount); err != nil {
		t.Fatalf("read online_count: %v", err)
	}
	if onlineCount != 1 {
		t.Fatalf("janitor must refresh online_count after pruning stale devices, got %d want 1", onlineCount)
	}
	if got := mustCount(t, f, `SELECT COUNT(*) FROM admin_sessions WHERE token_hash = 'valid-hash'`); got != 1 {
		t.Fatal("valid admin session must survive")
	}
	if got := mustCount(t, f, `SELECT COUNT(*) FROM admin_sessions WHERE token_hash = 'expired-hash'`); got != 0 {
		t.Fatal("expired admin session must be deleted")
	}
	if got := mustCount(t, f, `SELECT COUNT(*) FROM traffic_batches`); got != 1 {
		t.Fatalf("expected 1 recent batch marker to survive, got %d", got)
	}
	if got := mustCount(t, f, `SELECT COUNT(*) FROM device_batches`); got != 1 {
		t.Fatalf("expected 1 recent device marker to survive, got %d", got)
	}
}

func TestSweepRemovesExpiredVisits(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	serverID, err := f.repo.CreateServer(ctx, "s1", "1.2.3.4", repo.ServerStatusActive)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	userID, err := f.repo.CreateUser(ctx, repo.NewUser{UUID: "u1", Username: "user-u1", TokenHash: "h1", Status: repo.UserStatusActive})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	nodeID, err := f.repo.CreateNode(ctx, repo.NewNode{ServerID: serverID, Name: "n1", Protocol: repo.ProtocolVLESS, Port: 443})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}
	agentID, err := f.repo.CreateAgent(ctx, serverID, "agent-hash", "1.0.0")
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	seedVisit := func(at time.Time) {
		t.Helper()
		if _, err := f.db.ExecContext(ctx,
			`INSERT INTO visit_records (user_id, node_id, server_id, dest_host, dest_port, network, client_ip, created_at)
			 VALUES (?, ?, ?, 'example.com', 443, 'tcp', '1.1.1.1', ?)`,
			userID, nodeID, serverID, at.Unix()); err != nil {
			t.Fatalf("seed visit record: %v", err)
		}
	}
	seedVisit(now.AddDate(0, 0, -10))
	seedVisit(now.AddDate(0, 0, -1))

	seedDaily := func(day time.Time) {
		t.Helper()
		if _, err := f.db.ExecContext(ctx,
			`INSERT INTO visit_daily_domains (day, user_id, node_id, server_id, dest_host, hits)
			 VALUES (?, ?, ?, ?, 'example.com', 1)`,
			day.Unix(), userID, nodeID, serverID); err != nil {
			t.Fatalf("seed daily domain: %v", err)
		}
	}
	seedDaily(now.AddDate(0, 0, -200).Truncate(24 * time.Hour))
	seedDaily(now.Truncate(24 * time.Hour))

	seedBatch := func(seq int64, at time.Time) {
		t.Helper()
		if _, err := f.db.ExecContext(ctx,
			`INSERT INTO visit_batches (agent_id, seq, received_at, records) VALUES (?, ?, ?, 1)`,
			agentID, seq, at.Unix()); err != nil {
			t.Fatalf("seed visit batch: %v", err)
		}
	}
	seedBatch(1, now.AddDate(0, 0, -200))
	seedBatch(2, now)

	j := janitor.New(f.repo, time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := j.Sweep(ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	if got := mustCount(t, f, `SELECT COUNT(*) FROM visit_records`); got != 1 {
		t.Fatalf("expected 1 recent visit record to survive, got %d", got)
	}
	if got := mustCount(t, f, `SELECT COUNT(*) FROM visit_daily_domains`); got != 1 {
		t.Fatalf("expected 1 recent daily aggregate to survive, got %d", got)
	}
	if got := mustCount(t, f, `SELECT COUNT(*) FROM visit_batches`); got != 1 {
		t.Fatalf("expected 1 recent visit batch to survive, got %d", got)
	}
}

func TestSweepHonorsSettingsOverride(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	serverID, err := f.repo.CreateServer(ctx, "s1", "1.2.3.4", repo.ServerStatusActive)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	userID, err := f.repo.CreateUser(ctx, repo.NewUser{UUID: "u1", Username: "user-u1", TokenHash: "h1", Status: repo.UserStatusActive})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	nodeID, err := f.repo.CreateNode(ctx, repo.NewNode{ServerID: serverID, Name: "n1", Protocol: repo.ProtocolVLESS, Port: 443})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	if err := f.repo.SetSetting(ctx, "retention.aggregate_days", "1"); err != nil {
		t.Fatalf("set setting: %v", err)
	}

	seedTraffic(t, f, userID, nodeID, serverID, 1, 1, now.AddDate(0, 0, -3))

	j := janitor.New(f.repo, time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := j.Sweep(ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if got := mustCount(t, f, `SELECT COUNT(*) FROM traffic_records`); got != 0 {
		t.Fatalf("traffic older than 1 day must be deleted when retention is overridden, got %d rows", got)
	}
}

func TestSweepEnforcesStorageCaps(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	now := time.Now()

	serverID, err := f.repo.CreateServer(ctx, "s1", "1.2.3.4", repo.ServerStatusActive)
	if err != nil {
		t.Fatalf("create server: %v", err)
	}
	userID, err := f.repo.CreateUser(ctx, repo.NewUser{UUID: "u1", Username: "user-u1", TokenHash: "h1", Status: repo.UserStatusActive})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	nodeID, err := f.repo.CreateNode(ctx, repo.NewNode{ServerID: serverID, Name: "n1", Protocol: repo.ProtocolVLESS, Port: 443})
	if err != nil {
		t.Fatalf("create node: %v", err)
	}

	const total = 25
	records := make([]repo.NewTrafficRecord, 0, total)
	for i := 0; i < total; i++ {
		ts := now.Add(time.Duration(i) * time.Minute)
		records = append(records, repo.NewTrafficRecord{
			UserID: userID, NodeID: nodeID, ServerID: serverID,
			U: 1, D: 1, CreatedAt: ts,
		})
	}
	if err := f.repo.InsertTrafficRecords(ctx, records); err != nil {
		t.Fatalf("seed traffic: %v", err)
	}

	j := janitor.New(f.repo, time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil))).
		WithCaps(12)
	if err := j.Sweep(ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	if got := mustCount(t, f, `SELECT COUNT(*) FROM traffic_records`); got != 12 {
		t.Fatalf("traffic records must be capped to 12, got %d", got)
	}
}
