package db

import (
	"context"
	"path/filepath"
	"testing"
)

func openMigrationTestDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(filepath.Join(t.TempDir(), "migration.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func applyMigrationsThrough(t *testing.T, d *DB, ctx context.Context, target int64) {
	t.Helper()
	if _, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	for _, name := range []string{"0001_init.sql", "0002_admin_sessions.sql", "0003_agent_batch_counts.sql", "0004_users_username.sql"} {
		version, err := migrationVersion(name)
		if err != nil {
			t.Fatalf("version %s: %v", name, err)
		}
		if version > target {
			return
		}
		if err := d.applyMigration(ctx, version, name); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
}

func TestUsernameMigrationBackfillPreservesDependents(t *testing.T) {
	d := openMigrationTestDB(t)
	ctx := context.Background()
	applyMigrationsThrough(t, d, ctx, 3)

	seed := func(script string) {
		t.Helper()
		if _, err := d.ExecContext(ctx, script); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	seed(`INSERT INTO servers (id, name, address, created_at, updated_at) VALUES (1, 's1', 's1.example.com', 1, 1)`)
	seed(`INSERT INTO nodes (id, server_id, name, protocol, port, created_at, updated_at) VALUES (1, 1, 'n1', 'vless', 443, 1, 1)`)
	seed(`INSERT INTO users (id, uuid, token_hash, created_at, updated_at) VALUES
		(11, 'aaaaaaaa-1111-4aaa-8aaa-aaaaaaaaaaaa', 'h11', 1, 1),
		(12, 'bbbbbbbb-2222-4bbb-8bbb-bbbbbbbbbbbb', 'h12', 1, 1)`)
	seed(`INSERT INTO user_nodes (user_id, node_id, created_at) VALUES (11, 1, 1), (12, 1, 1)`)
	seed(`INSERT INTO sessions (user_id, node_id, server_id, ip, connected_at, last_seen_at) VALUES
		(11, 1, 1, '1.1.1.1', 100, 200),
		(12, 1, 1, '2.2.2.2', 100, 200)`)
	seed(`INSERT INTO traffic_records (user_id, node_id, server_id, upload_bytes, download_bytes, created_at) VALUES (11, 1, 1, 5, 6, 100)`)
	seed(`INSERT INTO connection_logs (user_id, node_id, server_id, ip, connected_at, status, created_at) VALUES (11, 1, 1, '1.1.1.1', 100, 'closed', 100)`)

	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("migrate to 0004: %v", err)
	}
	var versionCount int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&versionCount); err != nil {
		t.Fatalf("count versions: %v", err)
	}
	if versionCount != 6 {
		t.Fatalf("expected 6 recorded versions, got %d", versionCount)
	}

	type row struct {
		id       int64
		username string
	}
	rows := []row{}
	rrows, err := d.QueryContext(ctx, `SELECT id, username FROM users ORDER BY id`)
	if err != nil {
		t.Fatalf("query users: %v", err)
	}
	for rrows.Next() {
		var r row
		if err := rrows.Scan(&r.id, &r.username); err != nil {
			t.Fatalf("scan: %v", err)
		}
		rows = append(rows, r)
	}
	rrows.Close()
	want := []row{{11, "user-aaaaaaaa"}, {12, "user-bbbbbbbb"}}
	if len(rows) != len(want) {
		t.Fatalf("expected %d users, got %v", len(want), rows)
	}
	for i, r := range rows {
		if r != want[i] {
			t.Fatalf("expected backfill %v, got %v", want, rows)
		}
	}

	var sessions, userNodes, traffic, logs int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&sessions); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_nodes`).Scan(&userNodes); err != nil {
		t.Fatalf("count user_nodes: %v", err)
	}
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM traffic_records`).Scan(&traffic); err != nil {
		t.Fatalf("count traffic: %v", err)
	}
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM connection_logs`).Scan(&logs); err != nil {
		t.Fatalf("count logs: %v", err)
	}
	if sessions != 2 || userNodes != 2 || traffic != 1 || logs != 1 {
		t.Fatalf("dependent rows must survive: sessions=%d user_nodes=%d traffic=%d logs=%d", sessions, userNodes, traffic, logs)
	}

	fkRows, err := d.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer fkRows.Close()
	violations := 0
	for fkRows.Next() {
		violations++
	}
	fkRows.Close()
	if violations != 0 {
		t.Fatalf("expected 0 fk violations, got %d", violations)
	}

	wantIndexes := map[string]string{
		"idx_users_status":     "users",
		"idx_users_expires_at": "users",
		"idx_sessions_user":    "sessions",
		"idx_sessions_node":    "sessions",
		"idx_sessions_server":  "sessions",
		"idx_user_nodes_node":  "user_nodes",
	}
	irows, err := d.QueryContext(ctx, `SELECT name, tbl_name FROM sqlite_master WHERE type = 'index' AND name IN ('idx_users_status', 'idx_users_expires_at', 'idx_sessions_user', 'idx_sessions_node', 'idx_sessions_server', 'idx_user_nodes_node')`)
	if err != nil {
		t.Fatalf("query indexes: %v", err)
	}
	gotIndexes := map[string]string{}
	for irows.Next() {
		var name, tbl string
		if err := irows.Scan(&name, &tbl); err != nil {
			t.Fatalf("scan index: %v", err)
		}
		gotIndexes[name] = tbl
	}
	irows.Close()
	if err := irows.Err(); err != nil {
		t.Fatalf("iterate indexes: %v", err)
	}
	for name, tbl := range wantIndexes {
		if gotIndexes[name] != tbl {
			t.Fatalf("index %s must exist on %s after rebuild, got %q", name, tbl, gotIndexes[name])
		}
	}

	if _, err := d.ExecContext(ctx,
		`INSERT INTO users (uuid, username, token_hash, created_at, updated_at) VALUES ('cccccccc-3333-4ccc-8ccc-cccccccccccc', 'user-aaaaaaaa', 'h13', 1, 1)`); err == nil {
		t.Fatal("duplicate username must be rejected after migration")
	}

	if _, err := d.ExecContext(ctx, `DELETE FROM users WHERE id = 11`); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	var cascaded int
	if err := d.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM sessions) +
		(SELECT COUNT(*) FROM user_nodes)`).Scan(&cascaded); err != nil {
		t.Fatalf("count cascaded: %v", err)
	}
	if cascaded != 2 {
		t.Fatalf("expected cascade to leave 2 combined rows, got %d", cascaded)
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("re-migrate must be a no-op: %v", err)
	}
}
