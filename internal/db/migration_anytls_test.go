package db

import (
	"context"
	"path/filepath"
	"testing"
)

func TestNodesProtocolAndDeviceSchema(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "schema.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := context.Background()
	if err := d.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	if _, err := d.ExecContext(ctx, `INSERT INTO servers (id, name, address, created_at, updated_at) VALUES (1, 's1', 's1.example.com', 1, 1)`); err != nil {
		t.Fatalf("seed server: %v", err)
	}
	for i, protocol := range []string{"shadowsocks", "vless", "hysteria2", "anytls"} {
		if _, err := d.ExecContext(ctx,
			`INSERT INTO nodes (server_id, name, protocol, port, created_at, updated_at) VALUES (1, ?, ?, ?, 1, 1)`,
			protocol, protocol, 8000+i); err != nil {
			t.Fatalf("protocol %s must be accepted: %v", protocol, err)
		}
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO nodes (server_id, name, protocol, port, created_at, updated_at) VALUES (1, 'bad', 'snell', 8445, 1, 1)`); err == nil {
		t.Fatal("unknown protocol must be rejected")
	}

	var rate float64
	var tags, settings string
	if err := d.QueryRowContext(ctx, `SELECT rate, tags, protocol_settings FROM nodes WHERE name = 'vless'`).Scan(&rate, &tags, &settings); err != nil {
		t.Fatalf("read node defaults: %v", err)
	}
	if rate != 1 || tags != "[]" || settings != "{}" {
		t.Fatalf("unexpected node defaults rate=%v tags=%q settings=%q", rate, tags, settings)
	}

	if _, err := d.ExecContext(ctx, `INSERT INTO users (id, uuid, username, token_hash, created_at, updated_at) VALUES (11, 'u-11', 'user-11', 'h11', 1, 1)`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := d.ExecContext(ctx,
		`INSERT INTO online_devices (user_id, node_id, server_id, ip, last_seen_at, created_at) VALUES (11, 1, 1, '1.1.1.1', 1, 1)`); err != nil {
		t.Fatalf("insert device: %v", err)
	}
	if _, err := d.ExecContext(ctx,
		`INSERT INTO online_devices (user_id, node_id, server_id, ip, last_seen_at, created_at) VALUES (11, 1, 1, '1.1.1.1', 2, 2)`); err == nil {
		t.Fatal("duplicate (user_id,node_id,ip) must be rejected")
	}

	if _, err := d.ExecContext(ctx, `INSERT INTO traffic_records (user_id, node_id, server_id, u, d, created_at) VALUES (11, 1, 1, 5, 6, 100)`); err != nil {
		t.Fatalf("insert traffic: %v", err)
	}

	if _, err := d.ExecContext(ctx, `DELETE FROM users WHERE id = 11`); err != nil {
		t.Fatalf("delete user: %v", err)
	}
	var devices int
	if err := d.QueryRowContext(ctx, `SELECT COUNT(*) FROM online_devices`).Scan(&devices); err != nil {
		t.Fatalf("count devices: %v", err)
	}
	if devices != 0 {
		t.Fatalf("online devices must cascade with the user, got %d", devices)
	}
}
