CREATE TABLE custom_nodes (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    source_type TEXT NOT NULL CHECK (source_type IN ('links', 'subscription')),
    content_enc BLOB NOT NULL,
    cached_content TEXT NOT NULL DEFAULT '',
    fetched_at INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE user_custom_nodes (
    user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    custom_node_id INTEGER NOT NULL REFERENCES custom_nodes (id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, custom_node_id)
);

CREATE INDEX idx_user_custom_nodes_node ON user_custom_nodes (custom_node_id);
