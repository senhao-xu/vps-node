package db

import (
	"context"
	"path/filepath"
	"testing"
)

// TestUserCustomNodeEntriesMigrationUpgrade replays the pre-0010 schema with an
// existing source authorization, applies 0010 and asserts the new whitelist
// table is created, is idempotent, and cascades from both parents.
func TestUserCustomNodeEntriesMigrationUpgrade(t *testing.T) {
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
	} {
		if err := d.applyMigration(ctx, m.version, m.name); err != nil {
			t.Fatalf("apply %s: %v", m.name, err)
		}
	}

	if _, err := d.ExecContext(ctx, `INSERT INTO users (id, uuid, username, token_hash, created_at, updated_at) VALUES (1, 'u1', 'user-u1', 'h1', 1, 1)`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO custom_nodes (id, name, source_type, content_enc, created_at, updated_at) VALUES (1, 'legacy', 'links', X'01', 1, 1)`); err != nil {
		t.Fatalf("seed custom node: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO user_custom_nodes (user_id, custom_node_id, created_at) VALUES (1, 1, 1)`); err != nil {
		t.Fatalf("seed authorization: %v", err)
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("migrate to 0010: %v", err)
	}

	var name string
	if err := d.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'user_custom_node_entries'`).Scan(&name); err != nil {
		t.Fatalf("user_custom_node_entries table missing: %v", err)
	}
	var index string
	if err := d.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'index' AND name = 'idx_user_custom_node_entries_user'`).Scan(&index); err != nil {
		t.Fatalf("user_custom_node_entries index missing: %v", err)
	}

	// An existing whole-source authorization survives with no whitelist rows.
	var authorizations int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_custom_nodes WHERE user_id = 1`).Scan(&authorizations); err != nil {
		t.Fatalf("count authorizations: %v", err)
	}
	if authorizations != 1 {
		t.Fatalf("authorization did not survive the migration, got %d", authorizations)
	}

	if _, err := d.ExecContext(ctx, `INSERT INTO user_custom_node_entries (user_id, custom_node_id, entry_key, created_at) VALUES (1, 1, 'abc', 1)`); err != nil {
		t.Fatalf("insert entry: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO user_custom_node_entries (user_id, custom_node_id, entry_key, created_at) VALUES (1, 1, 'abc', 2)`); err == nil {
		t.Fatal("duplicate (user_id, custom_node_id, entry_key) must be rejected")
	}

	// Applying the migration again is a no-op and keeps the row.
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	var entries int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_custom_node_entries`).Scan(&entries); err != nil {
		t.Fatalf("count entries: %v", err)
	}
	if entries != 1 {
		t.Fatalf("entries after idempotent migrate = %d", entries)
	}

	// Deleting the custom node cascades the whitelist.
	if _, err := d.ExecContext(ctx, `DELETE FROM custom_nodes WHERE id = 1`); err != nil {
		t.Fatalf("delete custom node: %v", err)
	}
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_custom_node_entries`).Scan(&entries); err != nil {
		t.Fatalf("count entries: %v", err)
	}
	if entries != 0 {
		t.Fatalf("custom node delete must cascade the whitelist, got %d", entries)
	}

	// Deleting the user cascades the whitelist too.
	if _, err := d.ExecContext(ctx, `INSERT INTO custom_nodes (id, name, source_type, content_enc, created_at, updated_at) VALUES (2, 'other', 'links', X'02', 1, 1)`); err != nil {
		t.Fatalf("seed custom node: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO user_custom_node_entries (user_id, custom_node_id, entry_key, created_at) VALUES (1, 2, 'def', 1)`); err != nil {
		t.Fatalf("insert entry: %v", err)
	}
	if _, err := d.ExecContext(ctx, `DELETE FROM users WHERE id = 1`); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_custom_node_entries`).Scan(&entries); err != nil {
		t.Fatalf("count entries: %v", err)
	}
	if entries != 0 {
		t.Fatalf("user delete must cascade the whitelist, got %d", entries)
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
