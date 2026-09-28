package db

import (
	"context"
	"path/filepath"
	"testing"
)

// TestCustomNodeUserAgentMigrationUpgrade replays the pre-0008 schema with a
// custom node, applies 0008 and asserts the new user_agent column defaults to
// an empty string for existing and new rows.
func TestCustomNodeUserAgentMigrationUpgrade(t *testing.T) {
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
	} {
		if err := d.applyMigration(ctx, m.version, m.name); err != nil {
			t.Fatalf("apply %s: %v", m.name, err)
		}
	}

	if _, err := d.ExecContext(ctx, `INSERT INTO custom_nodes (id, name, source_type, content_enc, created_at, updated_at) VALUES (1, 'legacy', 'subscription', X'01', 1, 1)`); err != nil {
		t.Fatalf("seed custom node: %v", err)
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("migrate to 0008: %v", err)
	}

	var legacy string
	if err := d.QueryRowContext(ctx, `SELECT user_agent FROM custom_nodes WHERE id = 1`).Scan(&legacy); err != nil {
		t.Fatalf("read user_agent: %v", err)
	}
	if legacy != "" {
		t.Fatalf("existing rows must default to an empty user_agent, got %q", legacy)
	}

	if _, err := d.ExecContext(ctx, `INSERT INTO custom_nodes (id, name, source_type, content_enc, created_at, updated_at) VALUES (2, 'fresh', 'links', X'02', 1, 1)`); err != nil {
		t.Fatalf("insert fresh node: %v", err)
	}
	var fresh string
	if err := d.QueryRowContext(ctx, `SELECT user_agent FROM custom_nodes WHERE id = 2`).Scan(&fresh); err != nil {
		t.Fatalf("read fresh user_agent: %v", err)
	}
	if fresh != "" {
		t.Fatalf("new rows must default to an empty user_agent, got %q", fresh)
	}
}
