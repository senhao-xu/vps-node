DROP TRIGGER trg_server_traffic_used;

CREATE TABLE users_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    uuid TEXT NOT NULL UNIQUE,
    username TEXT NOT NULL UNIQUE,
    token_hash TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'expired')),
    transfer_enable INTEGER NOT NULL DEFAULT 0,
    u INTEGER NOT NULL DEFAULT 0,
    d INTEGER NOT NULL DEFAULT 0,
    speed_limit INTEGER NOT NULL DEFAULT 0,
    device_limit INTEGER NOT NULL DEFAULT 0,
    online_count INTEGER NOT NULL DEFAULT 0,
    last_online_at INTEGER,
    started_at INTEGER,
    expires_at INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

INSERT INTO users_new (id, uuid, username, token_hash, status, transfer_enable, u, d, speed_limit, device_limit, online_count, last_online_at, started_at, expires_at, created_at, updated_at)
SELECT id, uuid, username, token_hash, status, transfer_enable, u, d, speed_limit, device_limit, online_count, last_online_at, started_at, expires_at, created_at, updated_at FROM users;
DROP TABLE users;
ALTER TABLE users_new RENAME TO users;
CREATE INDEX idx_users_status ON users (status);
CREATE INDEX idx_users_expires_at ON users (expires_at);

CREATE TABLE servers_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'offline')),
    cpu_percent REAL NOT NULL DEFAULT 0,
    memory_percent REAL NOT NULL DEFAULT 0,
    disk_percent REAL NOT NULL DEFAULT 0,
    uptime_seconds INTEGER NOT NULL DEFAULT 0,
    agent_version TEXT NOT NULL DEFAULT '',
    last_seen_at INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    ip TEXT NOT NULL DEFAULT '',
    region TEXT NOT NULL DEFAULT '',
    price_cents INTEGER NOT NULL DEFAULT 0,
    price_currency TEXT NOT NULL DEFAULT 'USD',
    traffic_limit_bytes INTEGER NOT NULL DEFAULT 0,
    traffic_used_bytes INTEGER NOT NULL DEFAULT 0,
    expires_at INTEGER,
    sort_order INTEGER NOT NULL DEFAULT 0,
    notes TEXT NOT NULL DEFAULT '',
    public_visible INTEGER NOT NULL DEFAULT 1,
    offline_notify INTEGER NOT NULL DEFAULT 0,
    ipv6 TEXT NOT NULL DEFAULT '',
    observed_ip TEXT NOT NULL DEFAULT '',
    traffic_accounting TEXT NOT NULL DEFAULT 'max',
    traffic_reset_day INTEGER NOT NULL DEFAULT 1,
    traffic_correction_bytes INTEGER NOT NULL DEFAULT 0,
    traffic_correction_cycle_start INTEGER NOT NULL DEFAULT 0,
    billing_cycle TEXT NOT NULL DEFAULT 'monthly'
);
INSERT INTO servers_new (id, name, status, cpu_percent, memory_percent, disk_percent, uptime_seconds, agent_version, last_seen_at, created_at, updated_at, ip, region, price_cents, price_currency, traffic_limit_bytes, traffic_used_bytes, expires_at, sort_order, notes, public_visible, offline_notify, ipv6, observed_ip, traffic_accounting, traffic_reset_day, traffic_correction_bytes, traffic_correction_cycle_start, billing_cycle)
SELECT id, name, status, cpu_percent, memory_percent, disk_percent, uptime_seconds, agent_version, last_seen_at, created_at, updated_at, ip, region, price_cents, price_currency, traffic_limit_bytes, traffic_used_bytes, expires_at, sort_order, notes, public_visible, offline_notify, ipv6, observed_ip, traffic_accounting, traffic_reset_day, traffic_correction_bytes, traffic_correction_cycle_start, billing_cycle FROM servers;
DROP TABLE servers;
ALTER TABLE servers_new RENAME TO servers;

CREATE TABLE nodes_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    server_id INTEGER NOT NULL REFERENCES servers (id) ON DELETE CASCADE,
    address TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    protocol TEXT NOT NULL CHECK (protocol IN ('shadowsocks', 'vless', 'hysteria2', 'anytls', 'socks', 'http')),
    port INTEGER NOT NULL CHECK (port > 0 AND port <= 65535),
    protocol_settings TEXT NOT NULL DEFAULT '{}',
    rate REAL NOT NULL DEFAULT 1,
    tags TEXT NOT NULL DEFAULT '[]',
    secret_enc BLOB,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    ipv6_enabled INTEGER NOT NULL DEFAULT 0,
    ipv6_address TEXT NOT NULL DEFAULT '',
    chain_node_id INTEGER REFERENCES nodes (id),
    chain_custom_node_id INTEGER REFERENCES custom_nodes (id),
    chain_custom_entry_key TEXT NOT NULL DEFAULT ''
);

INSERT INTO nodes_new (id, server_id, address, name, protocol, port, protocol_settings, rate, tags, secret_enc, status, created_at, updated_at, ipv6_enabled, ipv6_address, chain_node_id, chain_custom_node_id, chain_custom_entry_key)
SELECT id, server_id, address, name, protocol, port, protocol_settings, rate, tags, secret_enc, status, created_at, updated_at, ipv6_enabled, ipv6_address, chain_node_id, chain_custom_node_id, chain_custom_entry_key
FROM nodes;

DROP TABLE nodes;

ALTER TABLE nodes_new RENAME TO nodes;

CREATE INDEX idx_nodes_server ON nodes (server_id);
CREATE UNIQUE INDEX idx_nodes_port_active ON nodes (server_id, port) WHERE status = 'active';
CREATE INDEX idx_nodes_chain_node ON nodes (chain_node_id);
CREATE INDEX idx_nodes_chain_custom_node ON nodes (chain_custom_node_id);

DELETE FROM sqlite_sequence WHERE name IN ('users', 'servers', 'nodes');
INSERT INTO sqlite_sequence (name, seq)
SELECT 'users', COALESCE(MAX(id), 0) FROM (
    SELECT id FROM users
    UNION ALL SELECT user_id FROM traffic_records
    UNION ALL SELECT user_id FROM visit_records
    UNION ALL SELECT user_id FROM visit_daily_domains
);
INSERT INTO sqlite_sequence (name, seq)
SELECT 'servers', COALESCE(MAX(id), 0) FROM (
    SELECT id FROM servers
    UNION ALL SELECT server_id FROM traffic_records
    UNION ALL SELECT server_id FROM visit_records
    UNION ALL SELECT server_id FROM visit_daily_domains
);
INSERT INTO sqlite_sequence (name, seq)
SELECT 'nodes', COALESCE(MAX(id), 0) FROM (
    SELECT id FROM nodes
    UNION ALL SELECT node_id FROM traffic_records
    UNION ALL SELECT node_id FROM visit_records
    UNION ALL SELECT node_id FROM visit_daily_domains
);

CREATE TRIGGER trg_server_traffic_used AFTER INSERT ON traffic_records
BEGIN
    UPDATE servers SET traffic_used_bytes = traffic_used_bytes + NEW.u + NEW.d WHERE id = NEW.server_id;
END;
