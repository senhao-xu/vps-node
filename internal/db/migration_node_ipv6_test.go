package db

import (
	"context"
	"path/filepath"
	"testing"
)

// TestNodeIPv6MigrationUpgrade replays the pre-0004 schema with data, then
// applies 0004 and asserts the two IPv6 columns appear with off/empty defaults
// while child rows and foreign-key integrity survive.
func TestNodeIPv6MigrationUpgrade(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "upgrade.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	if _, err := d.ExecContext(ctx, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	for _, m := range []struct {
		version int64
		name    string
	}{
		{1, "0001_init.sql"},
		{2, "0002_visit_records.sql"},
		{3, "0003_node_address.sql"},
	} {
		if err := d.applyMigration(ctx, m.version, m.name); err != nil {
			t.Fatalf("apply %s: %v", m.name, err)
		}
	}

	if _, err := d.ExecContext(ctx, `INSERT INTO servers (id, name, created_at, updated_at) VALUES (1, 's1', 1, 1)`); err != nil {
		t.Fatalf("seed server: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO nodes (id, server_id, address, name, protocol, port, created_at, updated_at) VALUES (1, 1, 'hk01.example.com', 'n1', 'vless', 443, 1, 1)`); err != nil {
		t.Fatalf("seed node: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO users (id, uuid, username, token_hash, created_at, updated_at) VALUES (1, 'u1', 'user-u1', 'h1', 1, 1)`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO user_nodes (user_id, node_id, created_at) VALUES (1, 1, 1)`); err != nil {
		t.Fatalf("seed user_nodes: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO online_devices (user_id, node_id, server_id, ip, last_seen_at, created_at) VALUES (1, 1, 1, '1.1.1.1', 1, 1)`); err != nil {
		t.Fatalf("seed device: %v", err)
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("migrate to 0004: %v", err)
	}

	var enabled int
	var address string
	if err := d.QueryRowContext(ctx, `SELECT ipv6_enabled, ipv6_address FROM nodes WHERE id = 1`).Scan(&enabled, &address); err != nil {
		t.Fatalf("read ipv6 columns: %v", err)
	}
	if enabled != 0 || address != "" {
		t.Fatalf("expected ipv6 defaults 0/\"\", got %d/%q", enabled, address)
	}

	for table, want := range map[string]int{"user_nodes": 1, "online_devices": 1} {
		var count int
		if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != want {
			t.Fatalf("%s rows lost during migration: got %d want %d", table, count, want)
		}
	}

	rows, err := d.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check reported a violation")
	}

	if _, err := d.ExecContext(ctx, `INSERT INTO nodes (server_id, address, ipv6_enabled, ipv6_address, name, protocol, port, created_at, updated_at) VALUES (1, 'hk01.example.com', 1, '2001:db8::1', 'n2', 'vless', 8443, 1, 1)`); err != nil {
		t.Fatalf("insert node with ipv6 columns: %v", err)
	}
}
