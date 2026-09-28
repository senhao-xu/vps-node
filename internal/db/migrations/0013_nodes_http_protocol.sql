-- Widen the nodes.protocol CHECK to accept the new HTTP proxy managed
-- protocol. As in 0011 the validated CHECK constraint cannot be changed in
-- place, so the table is rebuilt under a temporary name and renamed into place
-- (the migration runner disables foreign keys around this file), keeping child
-- rows and every live column/index intact. The column set includes the two
-- external chain-exit columns added by 0012 (no rebuild there).

CREATE TABLE nodes_new (
    id INTEGER PRIMARY KEY,
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
