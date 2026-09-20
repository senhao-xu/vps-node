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
	if count != 3 {
		t.Fatalf("expected 3 applied migrations, got %d", count)
	}

	for _, table := range []string{
		"admins", "users", "servers", "agents", "nodes", "user_nodes",
		"sessions", "connection_logs", "traffic_records", "server_revisions",
		"traffic_batches", "connection_log_batches", "settings", "admin_sessions",
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

	if _, err := d.ExecContext(ctx, `INSERT INTO users (uuid, token_hash, created_at, updated_at) VALUES ('u1', 'h1', 1, 1)`); err != nil {
		t.Fatalf("insert user: %v", err)
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
