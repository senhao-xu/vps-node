CREATE TABLE admins (
    id INTEGER PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE users (
    id INTEGER PRIMARY KEY,
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

CREATE INDEX idx_users_status ON users (status);
CREATE INDEX idx_users_expires_at ON users (expires_at);

CREATE TABLE servers (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    address TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'offline')),
    cpu_percent REAL NOT NULL DEFAULT 0,
    memory_percent REAL NOT NULL DEFAULT 0,
    disk_percent REAL NOT NULL DEFAULT 0,
    uptime_seconds INTEGER NOT NULL DEFAULT 0,
    agent_version TEXT NOT NULL DEFAULT '',
    last_seen_at INTEGER,
    register_token_hash TEXT,
    register_token_expires_at INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE UNIQUE INDEX idx_servers_register_token
    ON servers (register_token_hash)
    WHERE register_token_hash IS NOT NULL;

CREATE TABLE agents (
    id INTEGER PRIMARY KEY,
    server_id INTEGER NOT NULL UNIQUE REFERENCES servers (id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    version TEXT NOT NULL DEFAULT '',
    last_seen_at INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE nodes (
    id INTEGER PRIMARY KEY,
    server_id INTEGER NOT NULL REFERENCES servers (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    protocol TEXT NOT NULL CHECK (protocol IN ('shadowsocks', 'vless', 'hysteria2', 'anytls')),
    port INTEGER NOT NULL CHECK (port > 0 AND port <= 65535),
    protocol_settings TEXT NOT NULL DEFAULT '{}',
    rate REAL NOT NULL DEFAULT 1,
    tags TEXT NOT NULL DEFAULT '[]',
    secret_enc BLOB,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (server_id, name),
    UNIQUE (server_id, port)
);

CREATE INDEX idx_nodes_server ON nodes (server_id);

CREATE TABLE user_nodes (
    user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    node_id INTEGER NOT NULL REFERENCES nodes (id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, node_id)
);

CREATE INDEX idx_user_nodes_node ON user_nodes (node_id);

CREATE TABLE online_devices (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    node_id INTEGER NOT NULL REFERENCES nodes (id) ON DELETE CASCADE,
    server_id INTEGER NOT NULL REFERENCES servers (id) ON DELETE CASCADE,
    ip TEXT NOT NULL,
    online INTEGER NOT NULL DEFAULT 0,
    last_seen_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    UNIQUE (user_id, node_id, ip)
);

CREATE INDEX idx_online_devices_user ON online_devices (user_id, last_seen_at);
CREATE INDEX idx_online_devices_node ON online_devices (node_id, last_seen_at);
CREATE INDEX idx_online_devices_server ON online_devices (server_id, last_seen_at);

CREATE TABLE traffic_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    node_id INTEGER NOT NULL,
    server_id INTEGER NOT NULL,
    u INTEGER NOT NULL DEFAULT 0,
    d INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL
);

CREATE INDEX idx_traffic_records_user ON traffic_records (user_id, created_at);
CREATE INDEX idx_traffic_records_node ON traffic_records (node_id, created_at);
CREATE INDEX idx_traffic_records_server ON traffic_records (server_id, created_at);

CREATE TABLE server_revisions (
    server_id INTEGER PRIMARY KEY REFERENCES servers (id) ON DELETE CASCADE,
    revision INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE traffic_batches (
    agent_id INTEGER NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    seq INTEGER NOT NULL,
    received_at INTEGER NOT NULL,
    records INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (agent_id, seq)
);

CREATE TABLE device_batches (
    agent_id INTEGER NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    seq INTEGER NOT NULL,
    received_at INTEGER NOT NULL,
    devices INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (agent_id, seq)
);

CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE admin_sessions (
    token_hash TEXT PRIMARY KEY,
    admin_id INTEGER NOT NULL REFERENCES admins (id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);

CREATE INDEX idx_admin_sessions_admin ON admin_sessions (admin_id);

CREATE TABLE user_subscriptions (
    user_id INTEGER PRIMARY KEY REFERENCES users (id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    token_enc BLOB NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE UNIQUE INDEX idx_user_subscriptions_token_hash ON user_subscriptions (token_hash);
