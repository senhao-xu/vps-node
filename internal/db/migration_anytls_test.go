package db

import (
	"context"
	"testing"
)

func applyMigrationsThrough0005(t *testing.T, d *DB, ctx context.Context) {
	t.Helper()
	if _, err := d.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	for _, name := range []string{"0001_init.sql", "0002_admin_sessions.sql", "0003_agent_batch_counts.sql", "0004_users_username.sql", "0005_user_subscriptions.sql"} {
		version, err := migrationVersion(name)
		if err != nil {
			t.Fatalf("version %s: %v", name, err)
		}
		if err := d.applyMigration(ctx, version, name); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}
}

func TestAnyTLSProtocolMigrationPreservesDependents(t *testing.T) {
	d := openMigrationTestDB(t)
	ctx := context.Background()
	applyMigrationsThrough0005(t, d, ctx)

	seed := func(script string) {
		t.Helper()
		if _, err := d.ExecContext(ctx, script); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	seed(`INSERT INTO servers (id, name, address, created_at, updated_at) VALUES (1, 's1', 's1.example.com', 1, 1)`)
	seed(`INSERT INTO nodes (id, server_id, name, protocol, port, settings, created_at, updated_at) VALUES (1, 1, 'n1', 'hysteria2', 8443, '{"server_name":"hy2.example.com"}', 1, 1)`)
	seed(`INSERT INTO nodes (id, server_id, name, protocol, port, settings, created_at, updated_at) VALUES (2, 1, 'n2', 'shadowsocks', 8388, '{"method":"2022-blake3-aes-128-gcm"}', 1, 1)`)
	seed(`INSERT INTO nodes (id, server_id, name, protocol, port, settings, created_at, updated_at) VALUES (3, 1, 'n3', 'vless', 443, '{"server_names":["vless.example.com"]}', 1, 1)`)
	seed(`INSERT INTO users (id, uuid, username, token_hash, created_at, updated_at) VALUES (11, 'aaaaaaaa-1111-4aaa-8aaa-aaaaaaaaaaaa', 'user-a', 'h11', 1, 1)`)
	seed(`INSERT INTO user_nodes (user_id, node_id, created_at) VALUES (11, 1, 1)`)
	seed(`INSERT INTO sessions (user_id, node_id, server_id, ip, connected_at, last_seen_at) VALUES (11, 1, 1, '1.1.1.1', 100, 200)`)

	seeded := 0
	if err := d.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM nodes) +
		(SELECT COUNT(*) FROM user_nodes) +
		(SELECT COUNT(*) FROM sessions)`).Scan(&seeded); err != nil {
		t.Fatalf("count seeded: %v", err)
	}
	if seeded != 5 {
		t.Fatalf("expected 5 seeded rows, got %d", seeded)
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("migrate to 0006: %v", err)
	}
	var versionCount int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations`).Scan(&versionCount); err != nil {
		t.Fatalf("count versions: %v", err)
	}
	if versionCount != 6 {
		t.Fatalf("expected 6 recorded versions, got %d", versionCount)
	}

	var protocol, settings string
	if err := d.QueryRowContext(ctx, `SELECT protocol, settings FROM nodes WHERE id = 1`).Scan(&protocol, &settings); err != nil {
		t.Fatalf("node row must survive rebuild: %v", err)
	}
	if protocol != "hysteria2" || settings != `{"server_name":"hy2.example.com"}` {
		t.Fatalf("node fields must survive rebuild, got protocol=%q settings=%q", protocol, settings)
	}
	var nodeCount int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes`).Scan(&nodeCount); err != nil {
		t.Fatalf("count nodes: %v", err)
	}
	if nodeCount != 3 {
		t.Fatalf("all three legacy protocol nodes must survive rebuild, got %d", nodeCount)
	}
	var dependents int
	if err := d.QueryRowContext(ctx, `SELECT
		(SELECT COUNT(*) FROM user_nodes) +
		(SELECT COUNT(*) FROM sessions)`).Scan(&dependents); err != nil {
		t.Fatalf("count dependents: %v", err)
	}
	if dependents != 2 {
		t.Fatalf("dependent rows must survive rebuild, got %d", dependents)
	}

	fkRows, err := d.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	violations := 0
	for fkRows.Next() {
		violations++
	}
	fkRows.Close()
	if violations != 0 {
		t.Fatalf("expected 0 fk violations, got %d", violations)
	}

	if _, err := d.ExecContext(ctx, `INSERT INTO nodes (server_id, name, protocol, port, created_at, updated_at) VALUES (1, 'any', 'anytls', 8444, 1, 1)`); err != nil {
		t.Fatalf("anytls protocol must be accepted after migration: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO nodes (server_id, name, protocol, port, created_at, updated_at) VALUES (1, 'bad', 'snell', 8445, 1, 1)`); err == nil {
		t.Fatal("unknown protocol must still be rejected")
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("re-migrate must be a no-op: %v", err)
	}
}
