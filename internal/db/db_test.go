package db_test

import (
	"context"
	"path/filepath"
	"testing"

	"vps-node/internal/db"
)

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := d.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return d
}

func TestMigrateIdempotent(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("second migrate: %v", err)
	}

	var count int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("count migrations: %v", err)
	}
	if count != 7 {
		t.Fatalf("expected 7 applied migrations, got %d", count)
	}

	for _, table := range []string{
		"admins", "users", "servers", "agents", "nodes", "user_nodes",
		"online_devices", "traffic_records", "server_revisions",
		"traffic_batches", "device_batches", "settings", "admin_sessions",
		"user_subscriptions", "visit_records", "visit_daily_domains",
		"visit_batches", "custom_nodes", "user_custom_nodes",
	} {
		var name string
		err := d.QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("table %s missing: %v", table, err)
		}
	}
}

func tableColumns(t *testing.T, d *db.DB, table string) map[string]bool {
	t.Helper()
	rows, err := d.QueryContext(context.Background(), `PRAGMA table_info(`+table+`)`)
	if err != nil {
		t.Fatalf("table_info %s: %v", table, err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info %s: %v", table, err)
		}
		cols[name] = true
	}
	return cols
}

func TestStatelessAgentSchema(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	agentCols := tableColumns(t, d, "agents")
	if !agentCols["key_hash"] || !agentCols["key_enc"] {
		t.Fatalf("agents must expose key_hash/key_enc, got %v", agentCols)
	}
	if agentCols["token_hash"] {
		t.Fatalf("agents.token_hash must be renamed, got %v", agentCols)
	}
	serverCols := tableColumns(t, d, "servers")
	if serverCols["register_token_hash"] || serverCols["register_token_expires_at"] {
		t.Fatalf("servers register-token columns must be dropped, got %v", serverCols)
	}

	if _, err := d.ExecContext(ctx, `INSERT INTO servers (id, name, created_at, updated_at) VALUES (1, 's1', 1, 1)`); err != nil {
		t.Fatalf("seed server: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO agents (server_id, key_hash, key_enc, created_at, updated_at) VALUES (1, 'h', X'0102', 1, 1)`); err != nil {
		t.Fatalf("insert agent: %v", err)
	}
	var hash string
	var enc []byte
	if err := d.QueryRowContext(ctx, `SELECT key_hash, key_enc FROM agents WHERE server_id = 1`).Scan(&hash, &enc); err != nil {
		t.Fatalf("read agent key: %v", err)
	}
	if hash != "h" || len(enc) != 2 {
		t.Fatalf("unexpected agent key row hash=%q enc=%v", hash, enc)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO traffic_batches (agent_id, seq, received_at) VALUES (1, 1, 1)`); err != nil {
		t.Fatalf("insert batch: %v", err)
	}
	if _, err := d.ExecContext(ctx, `DELETE FROM servers WHERE id = 1`); err != nil {
		t.Fatalf("delete server: %v", err)
	}
	var agents, batches int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM agents`).Scan(&agents); err != nil {
		t.Fatalf("count agents: %v", err)
	}
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM traffic_batches`).Scan(&batches); err != nil {
		t.Fatalf("count batches: %v", err)
	}
	if agents != 0 || batches != 0 {
		t.Fatalf("agent cascade must survive the rebuild, agents=%d batches=%d", agents, batches)
	}

	rows, err := d.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check reported violations after migration")
	}
}

func TestMigrationContentAppliedOnce(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()

	var userCount int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&userCount); err != nil {
		t.Fatalf("query users: %v", err)
	}

	if _, err := d.ExecContext(ctx, `INSERT INTO users (uuid, username, token_hash, created_at, updated_at) VALUES ('u1', 'user-u1', 'h1', 1, 1)`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

func TestUserSubscriptionsConstraintsAndCascade(t *testing.T) {
	d := openTestDB(t)
	ctx := context.Background()
	if _, err := d.ExecContext(ctx, `INSERT INTO users (uuid, username, token_hash, created_at, updated_at) VALUES ('sub-u', 'sub-user', 'user-hash', 1, 1)`); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO users (uuid, username, token_hash, created_at, updated_at) VALUES ('sub-u2', 'sub-user2', 'user-hash2', 1, 1)`); err != nil {
		t.Fatalf("insert second user: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO user_subscriptions (user_id, token_hash, token_enc, created_at, updated_at) VALUES (1, 'sub-hash', X'01', 1, 1)`); err != nil {
		t.Fatalf("insert subscription: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO user_subscriptions (user_id, token_hash, token_enc, created_at, updated_at) VALUES (1, 'other-hash', X'02', 1, 1)`); err == nil {
		t.Fatal("accepted duplicate user subscription")
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO user_subscriptions (user_id, token_hash, token_enc, created_at, updated_at) VALUES (2, 'sub-hash', X'02', 1, 1)`); err == nil {
		t.Fatal("accepted duplicate token hash")
	}
	if _, err := d.ExecContext(ctx, `DELETE FROM users WHERE id = 1`); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	var count int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_subscriptions`).Scan(&count); err != nil {
		t.Fatalf("count subscriptions: %v", err)
	}
	if count != 0 {
		t.Fatalf("subscription did not cascade, count=%d", count)
	}
}

func TestSQLitePragmas(t *testing.T) {
	d := openTestDB(t)

	var foreignKeys int
	if err := d.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("read foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("expected foreign_keys=1, got %d", foreignKeys)
	}

	var journalMode string
	if err := d.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("expected journal_mode=wal, got %q", journalMode)
	}
}

func TestForeignKeysEnforced(t *testing.T) {
	d := openTestDB(t)

	_, err := d.Exec(`INSERT INTO nodes (server_id, name, protocol, port, created_at, updated_at) VALUES (999, 'n', 'vless', 443, 1, 1)`)
	if err == nil {
		t.Fatal("expected FK violation for missing server")
	}
}
