package db

import (
	"context"
	"path/filepath"
	"testing"
)

// TestNodeChainMigrationUpgrade replays the pre-0007 schema with data, then
// applies 0007 and asserts the chain_node_id column appears nullable with a
// working self-FK and index, while existing rows stay unlinked.
func TestNodeChainMigrationUpgrade(t *testing.T) {
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
	} {
		if err := d.applyMigration(ctx, m.version, m.name); err != nil {
			t.Fatalf("apply %s: %v", m.name, err)
		}
	}

	if _, err := d.ExecContext(ctx, `INSERT INTO servers (id, name, created_at, updated_at) VALUES (1, 's1', 1, 1)`); err != nil {
		t.Fatalf("seed server: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO nodes (id, server_id, address, name, protocol, port, created_at, updated_at) VALUES (1, 1, 'a.example.com', 'entry', 'vless', 443, 1, 1)`); err != nil {
		t.Fatalf("seed entry node: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO nodes (id, server_id, address, name, protocol, port, created_at, updated_at) VALUES (2, 1, 'b.example.com', 'exit', 'hysteria2', 8443, 1, 1)`); err != nil {
		t.Fatalf("seed exit node: %v", err)
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("migrate to 0007: %v", err)
	}

	var chain any
	if err := d.QueryRowContext(ctx, `SELECT chain_node_id FROM nodes WHERE id = 1`).Scan(&chain); err != nil {
		t.Fatalf("read chain_node_id: %v", err)
	}
	if chain != nil {
		t.Fatalf("existing rows must stay unlinked, got %v", chain)
	}

	if _, err := d.ExecContext(ctx, `UPDATE nodes SET chain_node_id = 2 WHERE id = 1`); err != nil {
		t.Fatalf("set chain_node_id: %v", err)
	}
	if _, err := d.ExecContext(ctx, `UPDATE nodes SET chain_node_id = 999 WHERE id = 2`); err == nil {
		t.Fatal("expected FK violation for a dangling chain_node_id")
	}
	if _, err := d.ExecContext(ctx, `DELETE FROM nodes WHERE id = 2`); err == nil {
		t.Fatal("expected FK violation deleting a referenced chain exit")
	}

	var indexName string
	if err := d.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'index' AND name = 'idx_nodes_chain_node'`).Scan(&indexName); err != nil {
		t.Fatalf("chain index missing: %v", err)
	}

	rows, err := d.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check reported a violation")
	}
}
