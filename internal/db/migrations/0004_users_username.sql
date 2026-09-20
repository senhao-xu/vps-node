CREATE TABLE users_new (
    id INTEGER PRIMARY KEY,
    uuid TEXT NOT NULL UNIQUE,
    username TEXT NOT NULL UNIQUE,
    token_hash TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled', 'expired')),
    quota_bytes INTEGER NOT NULL DEFAULT 0,
    used_bytes INTEGER NOT NULL DEFAULT 0,
    started_at INTEGER,
    expires_at INTEGER,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

INSERT INTO users_new (id, uuid, username, token_hash, status, quota_bytes, used_bytes, started_at, expires_at, created_at, updated_at)
SELECT id, uuid, 'user-' || substr(uuid, 1, 8), token_hash, status, quota_bytes, used_bytes, started_at, expires_at, created_at, updated_at
FROM users;

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
DROP TABLE users;
ALTER TABLE users_new RENAME TO users;

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

CREATE INDEX idx_users_status ON users (status);
CREATE INDEX idx_users_expires_at ON users (expires_at);
