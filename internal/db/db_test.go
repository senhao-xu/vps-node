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
	if count != 4 {
		t.Fatalf("expected 4 applied migrations, got %d", count)
	}

	for _, table := range []string{
		"admins", "users", "servers", "agents", "nodes", "user_nodes",
		"online_devices", "traffic_records", "server_revisions",
		"traffic_batches", "device_batches", "settings", "admin_sessions",
		"user_subscriptions", "visit_records", "visit_daily_domains",
		"visit_batches",
	} {
		var name string
		err := d.QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("table %s missing: %v", table, err)
		}
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
