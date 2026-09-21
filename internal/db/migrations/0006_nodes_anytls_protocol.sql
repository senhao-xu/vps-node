CREATE TABLE nodes_new (
    id INTEGER PRIMARY KEY,
    server_id INTEGER NOT NULL REFERENCES servers (id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    protocol TEXT NOT NULL CHECK (protocol IN ('shadowsocks', 'vless', 'hysteria2', 'anytls')),
    port INTEGER NOT NULL CHECK (port > 0 AND port <= 65535),
    settings TEXT NOT NULL DEFAULT '{}',
    secret_enc BLOB,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    UNIQUE (server_id, name),
    UNIQUE (server_id, port)
);

INSERT INTO nodes_new (id, server_id, name, protocol, port, settings, secret_enc, status, created_at, updated_at)
SELECT id, server_id, name, protocol, port, settings, secret_enc, status, created_at, updated_at
FROM nodes;

CREATE TABLE sessions_backup (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL,
    node_id INTEGER NOT NULL,
    server_id INTEGER NOT NULL,
    ip TEXT NOT NULL,
    upload_bytes INTEGER NOT NULL DEFAULT 0,
    download_bytes INTEGER NOT NULL DEFAULT 0,
    connected_at INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL,
    UNIQUE (server_id, user_id, node_id, ip, connected_at)
);

INSERT INTO sessions_backup (id, user_id, node_id, server_id, ip, upload_bytes, download_bytes, connected_at, last_seen_at)
SELECT id, user_id, node_id, server_id, ip, upload_bytes, download_bytes, connected_at, last_seen_at
FROM sessions;

CREATE TABLE user_nodes_backup (
    user_id INTEGER NOT NULL,
    node_id INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, node_id)
);

INSERT INTO user_nodes_backup (user_id, node_id, created_at)
SELECT user_id, node_id, created_at
FROM user_nodes;

DROP TABLE sessions;
DROP TABLE user_nodes;
DROP TABLE nodes;
ALTER TABLE nodes_new RENAME TO nodes;

CREATE INDEX idx_nodes_server ON nodes (server_id);

CREATE TABLE user_nodes (
    user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    node_id INTEGER NOT NULL REFERENCES nodes (id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, node_id)
);

INSERT INTO user_nodes (user_id, node_id, created_at)
SELECT user_id, node_id, created_at
FROM user_nodes_backup;

DROP TABLE user_nodes_backup;
CREATE INDEX idx_user_nodes_node ON user_nodes (node_id);

CREATE TABLE sessions (
    id INTEGER PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    node_id INTEGER NOT NULL REFERENCES nodes (id) ON DELETE CASCADE,
    server_id INTEGER NOT NULL REFERENCES servers (id) ON DELETE CASCADE,
    ip TEXT NOT NULL,
    upload_bytes INTEGER NOT NULL DEFAULT 0,
    download_bytes INTEGER NOT NULL DEFAULT 0,
    connected_at INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL,
    UNIQUE (server_id, user_id, node_id, ip, connected_at)
);

INSERT INTO sessions (id, user_id, node_id, server_id, ip, upload_bytes, download_bytes, connected_at, last_seen_at)
SELECT id, user_id, node_id, server_id, ip, upload_bytes, download_bytes, connected_at, last_seen_at
FROM sessions_backup;

DROP TABLE sessions_backup;

CREATE INDEX idx_sessions_user ON sessions (user_id, last_seen_at);
CREATE INDEX idx_sessions_node ON sessions (node_id, last_seen_at);
CREATE INDEX idx_sessions_server ON sessions (server_id, last_seen_at);
