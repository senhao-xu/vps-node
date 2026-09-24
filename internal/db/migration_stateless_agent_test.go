package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestStatelessAgentMigrationPreservesLegacyData(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "legacy.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()

	conn, err := d.Conn(ctx)
	if err != nil {
		t.Fatalf("conn: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("disable fk: %v", err)
	}
	if _, err := conn.ExecContext(ctx, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		t.Fatalf("create schema_migrations: %v", err)
	}
	for _, name := range []string{"0001_init.sql", "0002_visit_records.sql", "0003_node_address.sql", "0004_node_ipv6.sql"} {
		content, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if _, err := conn.ExecContext(ctx, string(content)); err != nil {
			t.Fatalf("apply %s: %v", name, err)
		}
		version, err := migrationVersion(name)
		if err != nil {
			t.Fatalf("version %s: %v", name, err)
		}
		if _, err := conn.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, 0)`, version); err != nil {
			t.Fatalf("record %s: %v", name, err)
		}
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		t.Fatalf("enable fk: %v", err)
	}
	_ = conn.Close()

	seed := []string{
		`INSERT INTO servers (id, name, status, register_token_hash, register_token_expires_at, created_at, updated_at)
		 VALUES (1, 'legacy', 'active', 'reg-hash', 1800000000, 1, 1)`,
		`INSERT INTO nodes (id, server_id, address, name, protocol, port, status, created_at, updated_at)
		 VALUES (1, 1, 'legacy.example.com', 'n1', 'vless', 443, 'active', 1, 1)`,
		`INSERT INTO users (id, uuid, username, token_hash, created_at, updated_at)
		 VALUES (1, 'u1', 'user-u1', 'user-hash', 1, 1)`,
		`INSERT INTO user_nodes (user_id, node_id, created_at) VALUES (1, 1, 1)`,
		`INSERT INTO agents (id, server_id, token_hash, version, created_at, updated_at)
		 VALUES (1, 1, 'legacy-hash', '0.9.0', 1, 1)`,
		`INSERT INTO online_devices (user_id, node_id, server_id, ip, online, last_seen_at, created_at)
		 VALUES (1, 1, 1, '1.1.1.1', 1, 1, 1)`,
		`INSERT INTO traffic_records (user_id, node_id, server_id, u, d, created_at) VALUES (1, 1, 1, 5, 6, 1)`,
		`INSERT INTO traffic_batches (agent_id, seq, received_at, records) VALUES (1, 9, 1, 1)`,
		`INSERT INTO device_batches (agent_id, seq, received_at, devices) VALUES (1, 3, 1, 1)`,
		`INSERT INTO visit_batches (agent_id, seq, received_at, records) VALUES (1, 2, 1, 1)`,
	}
	for _, stmt := range seed {
		if _, err := d.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("seed: %v\n%s", err, stmt)
		}
	}

	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("migrate forward: %v", err)
	}

	var keyHash string
	var keyEnc []byte
	if err := d.QueryRowContext(ctx, `SELECT key_hash, key_enc FROM agents WHERE id = 1`).Scan(&keyHash, &keyEnc); err != nil {
		t.Fatalf("read migrated agent: %v", err)
	}
	if keyHash != "legacy-hash" {
		t.Fatalf("token_hash must survive as key_hash, got %q", keyHash)
	}
	if keyEnc != nil {
		t.Fatalf("legacy key_enc must stay NULL, got %v", keyEnc)
	}

	var registerCols int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('servers') WHERE name IN ('register_token_hash', 'register_token_expires_at')`).Scan(&registerCols); err != nil {
		t.Fatalf("count register columns: %v", err)
	}
	if registerCols != 0 {
		t.Fatalf("register-token columns must be dropped, found %d", registerCols)
	}

	counts := map[string]int{
		"user_nodes":      0,
		"online_devices":  0,
		"traffic_records": 0,
		"traffic_batches": 0,
		"device_batches":  0,
		"visit_batches":   0,
	}
	for table := range counts {
		var n int
		if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", table, err)
		}
		counts[table] = n
		if n == 0 {
			t.Fatalf("dependent rows in %s must survive the migration", table)
		}
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
