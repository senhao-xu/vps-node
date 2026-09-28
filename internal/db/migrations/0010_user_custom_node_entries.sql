CREATE TABLE user_custom_node_entries (
    user_id INTEGER NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    custom_node_id INTEGER NOT NULL REFERENCES custom_nodes (id) ON DELETE CASCADE,
    entry_key TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, custom_node_id, entry_key)
);

CREATE INDEX idx_user_custom_node_entries_user ON user_custom_node_entries (user_id);
