package db

import (
	"context"
	"path/filepath"
	"testing"
)

// TestNodesChainCustomExitMigrationUpgrade replays the pre-0012 schema with
// data, then applies 0012 and asserts the external chain columns appear with
// the expected defaults, a working FK/index, and clean referential integrity.
func TestNodesChainCustomExitMigrationUpgrade(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "upgrade.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	if _, err := d.ExecContext(ctx, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	for version, name := range []string{
		"0001_init.sql",
		"0002_visit_records.sql",
		"0003_node_address.sql",
		"0004_node_ipv6.sql",
		"0005_stateless_agent.sql",
		"0006_custom_nodes.sql",
		"0007_node_chain.sql",
		"0008_custom_nodes_user_agent.sql",
		"0009_custom_nodes_insecure_tls.sql",
		"0010_user_custom_node_entries.sql",
		"0011_nodes_socks_protocol.sql",
	} {
		if err := d.applyMigration(ctx, int64(version+1), name); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
	}

	if _, err := d.ExecContext(ctx, `INSERT INTO servers (id, name, created_at, updated_at) VALUES (1, 's1', 1, 1)`); err != nil {
		t.Fatalf("seed server: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO custom_nodes (id, name, source_type, content_enc, created_at, updated_at) VALUES (1, 'ext', 'links', X'00', 1, 1)`); err != nil {
		t.Fatalf("seed custom node: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO nodes (id, server_id, address, name, protocol, port, created_at, updated_at) VALUES (1, 1, 'a.example.com', 'entry', 'shadowsocks', 1001, 1, 1)`); err != nil {
		t.Fatalf("seed entry node: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO nodes (id, server_id, address, name, protocol, port, created_at, updated_at) VALUES (2, 1, 'b.example.com', 'other', 'shadowsocks', 1002, 1, 1)`); err != nil {
		t.Fatalf("seed other node: %v", err)
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("migrate to 0012: %v", err)
	}

	var customNodeID any
	var entryKey string
	if err := d.QueryRowContext(ctx, `SELECT chain_custom_node_id, chain_custom_entry_key FROM nodes WHERE id = 1`).Scan(&customNodeID, &entryKey); err != nil {
		t.Fatalf("read external chain columns: %v", err)
	}
	if customNodeID != nil || entryKey != "" {
		t.Fatalf("existing rows must default to NULL/'', got %v/%q", customNodeID, entryKey)
	}

	if _, err := d.ExecContext(ctx, `UPDATE nodes SET chain_custom_node_id = 1, chain_custom_entry_key = 'abc' WHERE id = 1`); err != nil {
		t.Fatalf("set external chain target: %v", err)
	}
	if _, err := d.ExecContext(ctx, `UPDATE nodes SET chain_custom_node_id = 999 WHERE id = 2`); err == nil {
		t.Fatal("expected FK violation for a dangling chain_custom_node_id")
	}
	if _, err := d.ExecContext(ctx, `DELETE FROM custom_nodes WHERE id = 1`); err == nil {
		t.Fatal("expected FK violation deleting a referenced custom node")
	}

	var indexName string
	if err := d.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'index' AND name = 'idx_nodes_chain_custom_node'`).Scan(&indexName); err != nil {
		t.Fatalf("external chain index missing: %v", err)
	}

	rows, err := d.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check reported a violation")
	}

	// Re-running the migration is a no-op.
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if _, err := d.ExecContext(ctx, `UPDATE nodes SET chain_custom_node_id = NULL, chain_custom_entry_key = '' WHERE id = 1`); err != nil {
		t.Fatalf("clear external chain target after second migrate: %v", err)
	}
}
