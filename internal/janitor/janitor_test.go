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

	old := now.AddDate(0, 0, -40)
	recent := now.AddDate(0, 0, -1)

	if _, err := f.db.ExecContext(ctx,
		`INSERT INTO connection_logs (user_id, node_id, server_id, ip, protocol, connected_at, status, created_at)
		 VALUES (?, ?, ?, '1.1.1.1', 'vless', ?, 'closed', ?)`,
		userID, nodeID, serverID, old.Unix(), old.Unix()); err != nil {
		t.Fatalf("seed old log: %v", err)
	}
	if _, err := f.repo.InsertConnectionLogs(ctx, []repo.NewConnectionLog{
		{UserID: userID, NodeID: nodeID, ServerID: serverID, IP: "2.2.2.2", Protocol: "vless", ConnectedAt: recent, Status: "closed"},
	}); err != nil {
		t.Fatalf("seed recent log: %v", err)
	}

	if _, err := f.db.Exec(
		`INSERT INTO traffic_records (user_id, node_id, server_id, upload_bytes, download_bytes, created_at)
		 VALUES (?, ?, ?, 1, 1, ?)`,
		userID, nodeID, serverID, now.AddDate(0, 0, -200).Unix()); err != nil {
		t.Fatalf("seed old traffic: %v", err)
	}
	if err := f.repo.InsertTrafficRecords(ctx, []repo.NewTrafficRecord{
		{UserID: userID, NodeID: nodeID, ServerID: serverID, UploadBytes: 2, DownloadBytes: 2, CreatedAt: recent},
	}); err != nil {
		t.Fatalf("seed recent traffic: %v", err)
	}

	if err := f.repo.ReplaceServerSessions(ctx, serverID, []repo.NewSession{
		{UserID: userID, NodeID: nodeID, ServerID: serverID, IP: "3.3.3.3", ConnectedAt: now, LastSeenAt: now},
	}); err != nil {
		t.Fatalf("seed fresh session: %v", err)
	}
	if _, err := f.db.Exec(
		`INSERT INTO sessions (user_id, node_id, server_id, ip, connected_at, last_seen_at)
		 VALUES (?, ?, ?, '4.4.4.4', ?, ?)`,
		userID, nodeID, serverID, old.Unix(), old.Unix()); err != nil {
		t.Fatalf("seed stale session: %v", err)
	}

	if _, err := f.db.Exec(
		`INSERT INTO admin_sessions (token_hash, admin_id, created_at, last_seen_at, expires_at)
		 VALUES ('expired-hash', 1, 1, 1, ?)`, old.Unix()); err != nil {
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

	j := janitor.New(f.repo, time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := j.Sweep(ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	if got := mustCount(t, f, `SELECT COUNT(*) FROM connection_logs`); got != 1 {
		t.Fatalf("expected 1 recent connection log to survive, got %d", got)
	}
	if got := mustCount(t, f, `SELECT COUNT(*) FROM traffic_records`); got != 1 {
		t.Fatalf("expected 1 recent traffic record to survive, got %d", got)
	}
	if got := mustCount(t, f, `SELECT COUNT(*) FROM sessions`); got != 1 {
		t.Fatalf("expected 1 fresh session to survive, got %d", got)
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

	if err := f.repo.SetSetting(ctx, "retention.raw_log_days", "1"); err != nil {
		t.Fatalf("set setting: %v", err)
	}

	threeDaysAgo := now.AddDate(0, 0, -3)
	if _, err := f.db.ExecContext(ctx,
		`INSERT INTO connection_logs (user_id, node_id, server_id, ip, protocol, connected_at, status, created_at)
		 VALUES (?, ?, ?, '1.1.1.1', 'vless', ?, 'closed', ?)`,
		userID, nodeID, serverID, threeDaysAgo.Unix(), threeDaysAgo.Unix()); err != nil {
		t.Fatalf("seed log: %v", err)
	}

	j := janitor.New(f.repo, time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err := j.Sweep(ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if got := mustCount(t, f, `SELECT COUNT(*) FROM connection_logs`); got != 0 {
		t.Fatalf("log older than 1 day must be deleted when retention is overridden, got %d rows", got)
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
	logs := make([]repo.NewConnectionLog, 0, total)
	records := make([]repo.NewTrafficRecord, 0, total)
	for i := 0; i < total; i++ {
		ts := now.Add(time.Duration(i) * time.Minute)
		logs = append(logs, repo.NewConnectionLog{
			UserID: userID, NodeID: nodeID, ServerID: serverID,
			IP: "1.1.1.1", Protocol: "vless", ConnectedAt: ts, Status: "closed",
		})
		records = append(records, repo.NewTrafficRecord{
			UserID: userID, NodeID: nodeID, ServerID: serverID,
			UploadBytes: 1, DownloadBytes: 1, CreatedAt: ts,
		})
	}
	if _, err := f.repo.InsertConnectionLogs(ctx, logs); err != nil {
		t.Fatalf("seed logs: %v", err)
	}
	if err := f.repo.InsertTrafficRecords(ctx, records); err != nil {
		t.Fatalf("seed traffic: %v", err)
	}

	j := janitor.New(f.repo, time.Hour, slog.New(slog.NewTextHandler(io.Discard, nil))).
		WithCaps(10, 12)
	if err := j.Sweep(ctx); err != nil {
		t.Fatalf("sweep: %v", err)
	}

	if got := mustCount(t, f, `SELECT COUNT(*) FROM connection_logs`); got != 10 {
		t.Fatalf("connection logs must be capped to 10, got %d", got)
	}
	if got := mustCount(t, f, `SELECT COUNT(*) FROM traffic_records`); got != 12 {
		t.Fatalf("traffic records must be capped to 12, got %d", got)
	}

	var oldestKeptID int64
	if err := f.db.QueryRow(
		`SELECT MIN(id) FROM connection_logs`).Scan(&oldestKeptID); err != nil {
		t.Fatalf("query oldest kept log: %v", err)
	}
	if want := int64(total - 10 + 1); oldestKeptID != want {
		t.Fatalf("cap must delete the oldest rows first, oldest kept id %d want %d", oldestKeptID, want)
	}
}
