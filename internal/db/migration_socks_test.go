package db

import (
	"context"
	"path/filepath"
	"testing"
)

// TestNodesSocksProtocolMigrationUpgrade replays the pre-0011 schema with data,
// applies 0011 and asserts the widened protocol CHECK accepts socks while
// existing nodes, child rows, indexes and the unique active-port rule survive;
// re-running the migration is a no-op.
func TestNodesSocksProtocolMigrationUpgrade(t *testing.T) {
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
		{4, "0004_node_ipv6.sql"},
		{5, "0005_stateless_agent.sql"},
		{6, "0006_custom_nodes.sql"},
		{7, "0007_node_chain.sql"},
		{8, "0008_custom_nodes_user_agent.sql"},
		{9, "0009_custom_nodes_insecure_tls.sql"},
		{10, "0010_user_custom_node_entries.sql"},
	} {
		if err := d.applyMigration(ctx, m.version, m.name); err != nil {
			t.Fatalf("apply %s: %v", m.name, err)
		}
	}

	if _, err := d.ExecContext(ctx, `INSERT INTO servers (id, name, created_at, updated_at) VALUES (1, 's1', 1, 1)`); err != nil {
		t.Fatalf("seed server: %v", err)
	}
	if _, err := d.ExecContext(ctx,
		`INSERT INTO nodes (id, server_id, address, name, protocol, port, ipv6_enabled, ipv6_address, created_at, updated_at)
		 VALUES (1, 1, 'entry.example.com', 'entry', 'vless', 443, 1, '2001:db8::1', 1, 1)`); err != nil {
		t.Fatalf("seed entry node: %v", err)
	}
	if _, err := d.ExecContext(ctx,
		`INSERT INTO nodes (id, server_id, address, name, protocol, port, created_at, updated_at)
		 VALUES (2, 1, 'exit.example.com', 'exit', 'hysteria2', 8443, 1, 1)`); err != nil {
		t.Fatalf("seed exit node: %v", err)
	}
	if _, err := d.ExecContext(ctx, `UPDATE nodes SET chain_node_id = 2 WHERE id = 1`); err != nil {
		t.Fatalf("seed chain link: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO users (id, uuid, username, token_hash, created_at, updated_at) VALUES (1, 'u1', 'user-u1', 'h1', 1, 1)`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO user_nodes (user_id, node_id, created_at) VALUES (1, 1, 1)`); err != nil {
		t.Fatalf("seed user_nodes: %v", err)
	}
	if _, err := d.ExecContext(ctx,
		`INSERT INTO online_devices (user_id, node_id, server_id, ip, last_seen_at, created_at) VALUES (1, 1, 1, '1.1.1.1', 1, 1)`); err != nil {
		t.Fatalf("seed device: %v", err)
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("migrate to 0011: %v", err)
	}

	for i, protocol := range []string{"shadowsocks", "vless", "hysteria2", "anytls", "socks"} {
		if _, err := d.ExecContext(ctx,
			`INSERT INTO nodes (server_id, address, name, protocol, port, created_at, updated_at) VALUES (1, 'x', ?, ?, ?, 1, 1)`,
			"proto-"+protocol, protocol, 9000+i); err != nil {
			t.Fatalf("protocol %s must be accepted: %v", protocol, err)
		}
	}
	if _, err := d.ExecContext(ctx,
		`INSERT INTO nodes (server_id, address, name, protocol, port, created_at, updated_at) VALUES (1, 'x', 'bad', 'snell', 9445, 1, 1)`); err == nil {
		t.Fatal("unknown protocol must be rejected")
	}

	// The rebuilt table keeps the live columns and row values.
	var address, ipv6Address string
	var ipv6Enabled int
	var chainNodeID int64
	if err := d.QueryRowContext(ctx,
		`SELECT address, ipv6_enabled, ipv6_address, chain_node_id FROM nodes WHERE id = 1`).
		Scan(&address, &ipv6Enabled, &ipv6Address, &chainNodeID); err != nil {
		t.Fatalf("read surviving node: %v", err)
	}
	if address != "entry.example.com" || ipv6Enabled != 1 || ipv6Address != "2001:db8::1" || chainNodeID != 2 {
		t.Fatalf("node row not preserved: address=%q ipv6=%d %q chain=%d", address, ipv6Enabled, ipv6Address, chainNodeID)
	}

	for table, want := range map[string]int{"user_nodes": 1, "online_devices": 1, "nodes": 7} {
		var count int
		if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != want {
			t.Fatalf("%s rows after rebuild: got %d want %d", table, count, want)
		}
	}

	for _, index := range []string{"idx_nodes_server", "idx_nodes_port_active", "idx_nodes_chain_node"} {
		var name string
		if err := d.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, index).Scan(&name); err != nil {
			t.Fatalf("index %s missing: %v", index, err)
		}
	}

	// Active ports stay unique; disabled duplicates remain allowed.
	if _, err := d.ExecContext(ctx,
		`INSERT INTO nodes (server_id, address, name, protocol, port, status, created_at, updated_at) VALUES (1, 'x', 'dup-active', 'socks', 443, 'active', 1, 1)`); err == nil {
		t.Fatal("active duplicate port must be rejected")
	}
	if _, err := d.ExecContext(ctx,
		`INSERT INTO nodes (server_id, address, name, protocol, port, status, created_at, updated_at) VALUES (1, 'x', 'dup-disabled', 'socks', 443, 'disabled', 1, 1)`); err != nil {
		t.Fatalf("disabled duplicate port must be allowed: %v", err)
	}

	// The self-FK on the rebuilt chain_node_id still guards the exit.
	if _, err := d.ExecContext(ctx, `UPDATE nodes SET chain_node_id = 999 WHERE id = 1`); err == nil {
		t.Fatal("expected FK violation for a dangling chain_node_id")
	}
	if _, err := d.ExecContext(ctx, `DELETE FROM nodes WHERE id = 2`); err == nil {
		t.Fatal("expected FK violation deleting a referenced chain exit")
	}

	rows, err := d.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check reported a violation")
	}

	// Applying the migration again is a no-op.
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	var nodes int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes`).Scan(&nodes); err != nil {
		t.Fatalf("count nodes: %v", err)
	}
	if nodes != 8 {
		t.Fatalf("nodes after idempotent migrate = %d", nodes)
	}
}
