package db

import (
	"context"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestIdentityAndAccountingMigrationPreservesPopulatedDatabase(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "upgrade.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()
	if _, err := d.ExecContext(ctx, `CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		version, err := migrationVersion(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		if version < 16 {
			if err := d.applyMigration(ctx, version, entry.Name()); err != nil {
				t.Fatal(err)
			}
		}
	}
	seeds := []string{
		`INSERT INTO users (id, uuid, username, token_hash, status, transfer_enable, u, d, speed_limit, device_limit, online_count, last_online_at, started_at, expires_at, created_at, updated_at) VALUES (1, 'uuid', 'user', 'hash', 'active', 999, 12, 13, 14, 15, 1, 16, 17, 9999999999, 18, 19)`,
		`INSERT INTO servers (id, name, status, cpu_percent, memory_percent, disk_percent, uptime_seconds, agent_version, last_seen_at, created_at, updated_at, ip, region, price_cents, price_currency, traffic_limit_bytes, traffic_used_bytes, expires_at, sort_order, notes, public_visible, offline_notify, ipv6, observed_ip, traffic_accounting, traffic_reset_day, traffic_correction_bytes, traffic_correction_cycle_start, billing_cycle) VALUES (1, 'server', 'offline', 1.5, 2.5, 3.5, 4, 'v', 5, 6, 7, 'ip', 'region', 123, 'EUR', 1000, 55, 9999999999, 9, 'notes', 0, 1, 'ipv6', 'observed', 'sum', 15, 29, 30, 'yearly')`,
		`INSERT INTO servers (id, name, created_at, updated_at) VALUES (2, 'exit-server', 1, 1)`,
		`INSERT INTO servers (id, name, status, created_at, updated_at) VALUES (3, 'disabled-server', 'disabled', 1, 1)`,
		`INSERT INTO agents (id, server_id, key_hash, key_enc, version, last_seen_at, created_at, updated_at) VALUES (1, 1, 'agent', X'0102', 'v', 1, 2, 3)`,
		`INSERT INTO custom_nodes (id, name, source_type, content_enc, created_at, updated_at) VALUES (1, 'custom', 'links', X'03', 1, 1)`,
		`INSERT INTO nodes (id, server_id, address, name, protocol, port, protocol_settings, rate, tags, secret_enc, status, created_at, updated_at, ipv6_enabled, ipv6_address) VALUES (1, 1, 'address', 'entry', 'http', 8080, '{"tls":{}}', 1.5, '["tag"]', X'04', 'active', 1, 2, 1, '::1')`,
		`INSERT INTO nodes (id, server_id, name, protocol, port, created_at, updated_at) VALUES (2, 2, 'exit', 'socks', 1080, 1, 1)`,
		`UPDATE nodes SET chain_node_id = 2 WHERE id = 1`,
		`INSERT INTO nodes (id, server_id, name, protocol, port, chain_custom_node_id, chain_custom_entry_key, created_at, updated_at) VALUES (3, 1, 'custom-entry', 'http', 8081, 1, 'key', 1, 1)`,
		`INSERT INTO user_nodes (user_id, node_id, created_at) VALUES (1, 1, 1)`,
		`INSERT INTO user_subscriptions (user_id, token_hash, token_enc, created_at, updated_at) VALUES (1, 'sub', X'05', 1, 1)`,
		`INSERT INTO user_custom_nodes (user_id, custom_node_id, created_at) VALUES (1, 1, 1)`,
		`INSERT INTO user_custom_node_entries (user_id, custom_node_id, entry_key, created_at) VALUES (1, 1, 'key', 1)`,
		`INSERT INTO online_devices (user_id, node_id, server_id, ip, online, last_seen_at, created_at) VALUES (1, 1, 1, 'client', 2, 1, 1)`,
		`INSERT INTO server_revisions (server_id, revision, updated_at) VALUES (1, 5, 1), (3, 0, 1)`,
		`INSERT INTO traffic_batches (agent_id, seq, received_at, records) VALUES (1, 7, 1, 1)`,
		`INSERT INTO device_batches (agent_id, seq, received_at, devices) VALUES (1, 8, 1, 1)`,
		`INSERT INTO visit_batches (agent_id, seq, received_at, records) VALUES (1, 9, 1, 1)`,
		`INSERT INTO traffic_records (user_id, node_id, server_id, u, d, created_at) VALUES (1, 1, 1, 10, 20, 86401), (1, 1, 1, 3, 4, 172799), (71, 81, 91, 30, 40, 86401)`,
		`INSERT INTO visit_records (user_id, node_id, server_id, dest_host, created_at) VALUES (72, 82, 92, 'old.example', 1)`,
		`INSERT INTO visit_daily_domains (day, user_id, node_id, server_id, dest_host, hits) VALUES (0, 73, 83, 93, 'older.example', 1)`,
	}
	for _, seed := range seeds {
		if _, err := d.ExecContext(ctx, seed); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	tables := map[string]string{
		"users":                    "id, uuid, username, token_hash, status, transfer_enable, u, d, speed_limit, device_limit, online_count, last_online_at, started_at, expires_at, created_at, updated_at",
		"servers":                  "id, name, status, cpu_percent, memory_percent, disk_percent, uptime_seconds, agent_version, last_seen_at, created_at, updated_at, ip, region, price_cents, price_currency, traffic_limit_bytes, traffic_used_bytes, expires_at, sort_order, notes, public_visible, offline_notify, ipv6, observed_ip, traffic_accounting, traffic_reset_day, traffic_correction_bytes, traffic_correction_cycle_start, billing_cycle",
		"nodes":                    "id, server_id, address, name, protocol, port, protocol_settings, rate, tags, secret_enc, status, created_at, updated_at, ipv6_enabled, ipv6_address, chain_node_id, chain_custom_node_id, chain_custom_entry_key",
		"agents":                   "id, server_id, key_hash, key_enc, version, last_seen_at, created_at, updated_at",
		"user_nodes":               "user_id, node_id, created_at",
		"user_subscriptions":       "user_id, token_hash, token_enc, created_at, updated_at",
		"user_custom_nodes":        "user_id, custom_node_id, created_at",
		"user_custom_node_entries": "user_id, custom_node_id, entry_key, created_at",
		"online_devices":           "id, user_id, node_id, server_id, ip, online, last_seen_at, created_at",
		"traffic_batches":          "agent_id, seq, received_at, records",
		"device_batches":           "agent_id, seq, received_at, devices",
		"visit_batches":            "agent_id, seq, received_at, records",
		"traffic_records":          "id, user_id, node_id, server_id, u, d, created_at",
		"visit_records":            "id, user_id, node_id, server_id, dest_host, dest_port, network, client_ip, created_at",
		"visit_daily_domains":      "day, user_id, node_id, server_id, dest_host, hits",
	}
	snapshot := func(table, columns string) []string {
		t.Helper()
		rows, err := d.QueryContext(ctx, `SELECT `+columns+` FROM `+table+` ORDER BY rowid`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		result := []string{}
		for rows.Next() {
			values := make([]any, len(strings.Split(columns, ",")))
			pointers := make([]any, len(values))
			for i := range values {
				pointers[i] = &values[i]
			}
			if err := rows.Scan(pointers...); err != nil {
				t.Fatal(err)
			}
			result = append(result, fmt.Sprintf("%#v", values))
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return result
	}
	before := map[string][]string{}
	for table, columns := range tables {
		before[table] = snapshot(table, columns)
	}
	var migratedRevisions []string
	for i := 0; i < 2; i++ {
		if err := d.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
		for table, columns := range tables {
			if after := snapshot(table, columns); !reflect.DeepEqual(before[table], after) {
				t.Fatalf("%s changed: before %v after %v", table, before[table], after)
			}
		}
		for serverID, want := range map[int64]int64{1: 6, 2: 1, 3: 1} {
			var revision, checkedAt int64
			if err := d.QueryRowContext(ctx, `SELECT revision, expiry_checked_at FROM server_revisions WHERE server_id = ?`, serverID).Scan(&revision, &checkedAt); err != nil || revision != want || checkedAt != 0 {
				t.Fatalf("upgrade pass %d server %d revision=%d want=%d expiry_checked_at=%d: %v", i, serverID, revision, want, checkedAt, err)
			}
		}
		after := snapshot("server_revisions", "server_id, revision, updated_at, expiry_checked_at")
		if i == 0 {
			migratedRevisions = after
		} else if !reflect.DeepEqual(migratedRevisions, after) {
			t.Fatalf("repeat migration changed revisions: before %v after %v", migratedRevisions, after)
		}
	}
	assertForeignKeys := func() {
		t.Helper()
		rows, err := d.QueryContext(ctx, `PRAGMA foreign_key_check`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		if rows.Next() {
			t.Fatal("foreign_key_check reported a violation")
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
	}
	assertForeignKeys()
	for _, index := range []string{"idx_users_status", "idx_users_expires_at", "idx_nodes_server", "idx_nodes_port_active", "idx_nodes_chain_node", "idx_nodes_chain_custom_node", "idx_online_devices_user", "idx_user_nodes_node", "idx_user_subscriptions_token_hash"} {
		var name string
		if err := d.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, index).Scan(&name); err != nil {
			t.Fatalf("index %s: %v", index, err)
		}
	}
	var up, down int64
	if err := d.QueryRowContext(ctx, `SELECT u, d FROM server_traffic_daily WHERE server_id = 1 AND day = 86400`).Scan(&up, &down); err != nil || up != 13 || down != 24 {
		t.Fatalf("backfill = %d/%d: %v", up, down, err)
	}
	inserts := []struct {
		table, query  string
		historicalMax int64
	}{
		{"users", `INSERT INTO users (uuid, username, token_hash, created_at, updated_at) VALUES ('new', 'new', 'new', 1, 1)`, 73},
		{"servers", `INSERT INTO servers (name, created_at, updated_at) VALUES ('new', 1, 1)`, 93},
		{"nodes", `INSERT INTO nodes (server_id, name, protocol, port, created_at, updated_at) VALUES (2, 'new', 'http', 8082, 1, 1)`, 83},
	}
	for _, insert := range inserts {
		res, err := d.ExecContext(ctx, insert.query)
		if err != nil {
			t.Fatal(err)
		}
		id, err := res.LastInsertId()
		if err != nil || id <= insert.historicalMax {
			t.Fatalf("%s id=%d must exceed historical id=%d: %v", insert.table, id, insert.historicalMax, err)
		}
		if _, err := d.ExecContext(ctx, `DELETE FROM `+insert.table+` WHERE id = ?`, id); err != nil {
			t.Fatal(err)
		}
		res, err = d.ExecContext(ctx, insert.query)
		if err != nil {
			t.Fatal(err)
		}
		nextID, err := res.LastInsertId()
		if err != nil || nextID <= id {
			t.Fatalf("%s reused deleted id %d: next=%d err=%v", insert.table, id, nextID, err)
		}
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO traffic_records (user_id, node_id, server_id, u, d, created_at) VALUES (1, 1, 1, 5, 6, 86402)`); err != nil {
		t.Fatal(err)
	}
	var total int64
	if err := d.QueryRowContext(ctx, `SELECT traffic_used_bytes FROM servers WHERE id = 1`).Scan(&total); err != nil || total != 103 {
		t.Fatalf("cumulative trigger total=%d: %v", total, err)
	}
	if err := d.QueryRowContext(ctx, `SELECT u, d FROM server_traffic_daily WHERE server_id = 1 AND day = 86400`).Scan(&up, &down); err != nil || up != 18 || down != 30 {
		t.Fatalf("daily trigger = %d/%d: %v", up, down, err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO nodes (server_id, name, protocol, port, created_at, updated_at) VALUES (1, 'duplicate', 'http', 8080, 1, 1)`); err == nil {
		t.Fatal("lost active-port uniqueness")
	}
	if _, err := d.ExecContext(ctx, `UPDATE nodes SET chain_node_id = 999 WHERE id = 1`); err == nil {
		t.Fatal("lost chain FK")
	}
	if _, err := d.ExecContext(ctx, `DELETE FROM servers WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"agents", "traffic_batches", "device_batches", "visit_batches", "online_devices", "user_nodes"} {
		var count int
		if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("server cascade %s count=%d: %v", table, count, err)
		}
	}
	var revisionRows int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM server_revisions WHERE server_id = 1`).Scan(&revisionRows); err != nil || revisionRows != 0 {
		t.Fatalf("deleted server revision count=%d: %v", revisionRows, err)
	}
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM server_revisions`).Scan(&revisionRows); err != nil || revisionRows != 2 {
		t.Fatalf("surviving server revisions count=%d: %v", revisionRows, err)
	}
	if _, err := d.ExecContext(ctx, `DELETE FROM users WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"user_subscriptions", "user_custom_nodes", "user_custom_node_entries"} {
		var count int
		if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("user cascade %s count=%d: %v", table, count, err)
		}
	}
	assertForeignKeys()
}
