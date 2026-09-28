package db

import (
	"context"
	"path/filepath"
	"testing"
)

// TestCustomNodeInsecureTLSMigrationUpgrade replays the pre-0009 schema with a
// custom node, applies 0009 and asserts the new insecure_skip_verify column
// defaults to 0 for existing and new rows.
func TestCustomNodeInsecureTLSMigrationUpgrade(t *testing.T) {
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
	} {
		if err := d.applyMigration(ctx, m.version, m.name); err != nil {
			t.Fatalf("apply %s: %v", m.name, err)
		}
	}

	if _, err := d.ExecContext(ctx, `INSERT INTO custom_nodes (id, name, source_type, content_enc, created_at, updated_at) VALUES (1, 'legacy', 'subscription', X'01', 1, 1)`); err != nil {
		t.Fatalf("seed custom node: %v", err)
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("migrate to 0009: %v", err)
	}

	var legacy int
	if err := d.QueryRowContext(ctx, `SELECT insecure_skip_verify FROM custom_nodes WHERE id = 1`).Scan(&legacy); err != nil {
		t.Fatalf("read insecure_skip_verify: %v", err)
	}
	if legacy != 0 {
		t.Fatalf("existing rows must default insecure_skip_verify to 0, got %d", legacy)
	}

	if _, err := d.ExecContext(ctx, `INSERT INTO custom_nodes (id, name, source_type, content_enc, created_at, updated_at) VALUES (2, 'fresh', 'links', X'02', 1, 1)`); err != nil {
		t.Fatalf("insert fresh node: %v", err)
	}
	var fresh int
	if err := d.QueryRowContext(ctx, `SELECT insecure_skip_verify FROM custom_nodes WHERE id = 2`).Scan(&fresh); err != nil {
		t.Fatalf("read fresh insecure_skip_verify: %v", err)
	}
	if fresh != 0 {
		t.Fatalf("new rows must default insecure_skip_verify to 0, got %d", fresh)
	}
}
