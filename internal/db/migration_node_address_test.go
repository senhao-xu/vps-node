package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

// TestNodeAddressMigrationUpgrade replays the pre-0003 schema with data, then
// applies 0003, and asserts that the node rebuild preserves child rows and
// backfills the new node address from the legacy server address.
func TestNodeAddressMigrationUpgrade(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "upgrade.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	if _, err := d.ExecContext(ctx, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	if err := d.applyMigration(ctx, 1, "0001_init.sql"); err != nil {
		t.Fatalf("apply 0001: %v", err)
	}
	if err := d.applyMigration(ctx, 2, "0002_visit_records.sql"); err != nil {
		t.Fatalf("apply 0002: %v", err)
	}

	if _, err := d.ExecContext(ctx, `INSERT INTO servers (id, name, address, created_at, updated_at) VALUES (1, 's1', 'legacy.example.com', 1, 1)`); err != nil {
		t.Fatalf("seed server: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO nodes (id, server_id, name, protocol, port, created_at, updated_at) VALUES (1, 1, 'n1', 'vless', 443, 1, 1)`); err != nil {
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
		t.Fatalf("migrate to 0003: %v", err)
	}

	var address string
	if err := d.QueryRowContext(ctx, `SELECT address FROM nodes WHERE id = 1`).Scan(&address); err != nil {
		t.Fatalf("read node address: %v", err)
	}
	if address != "legacy.example.com" {
		t.Fatalf("node address not backfilled: %q", address)
	}

	for table, want := range map[string]int{"user_nodes": 1, "online_devices": 1} {
		var count int
		if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		if count != want {
			t.Fatalf("%s rows lost during rebuild: got %d want %d", table, count, want)
		}
	}

	hasAddress, err := columnExists(ctx, d, "servers", "address")
	if err != nil {
		t.Fatalf("inspect servers: %v", err)
	}
	if hasAddress {
		t.Fatal("servers.address must be dropped")
	}

	// Duplicate names are allowed; ports only unique among enabled nodes.
	if _, err := d.ExecContext(ctx, `INSERT INTO nodes (server_id, address, name, protocol, port, status, created_at, updated_at) VALUES (1, 'x', 'n1', 'vless', 8443, 'disabled', 1, 1)`); err != nil {
		t.Fatalf("duplicate name must be allowed: %v", err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO nodes (server_id, address, name, protocol, port, status, created_at, updated_at) VALUES (1, 'x', 'dup-active', 'vless', 443, 'active', 1, 1)`); err == nil {
		t.Fatal("active duplicate port must be rejected")
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO nodes (server_id, address, name, protocol, port, status, created_at, updated_at) VALUES (1, 'x', 'dup-disabled', 'vless', 443, 'disabled', 1, 1)`); err != nil {
		t.Fatalf("disabled duplicate port must be allowed: %v", err)
	}
}

func columnExists(ctx context.Context, d *DB, table, column string) (bool, error) {
	rows, err := d.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}
