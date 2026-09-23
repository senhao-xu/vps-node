-- Node gains a connection address; servers no longer carry one.
-- At the same time relax the node constraints so a node can be copied:
--   * duplicate names per server are allowed;
--   * a port is only unique among enabled nodes, so a disabled copy may
--     temporarily reuse its source port until the operator edits it.
--
-- The runner disables foreign keys around migrations; the new table is built
-- under a temporary name and renamed into place so child foreign keys keep
-- pointing at `nodes`.

CREATE TABLE nodes_new (
    id INTEGER PRIMARY KEY,
    server_id INTEGER NOT NULL REFERENCES servers (id) ON DELETE CASCADE,
    address TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    protocol TEXT NOT NULL CHECK (protocol IN ('shadowsocks', 'vless', 'hysteria2', 'anytls')),
    port INTEGER NOT NULL CHECK (port > 0 AND port <= 65535),
    protocol_settings TEXT NOT NULL DEFAULT '{}',
    rate REAL NOT NULL DEFAULT 1,
    tags TEXT NOT NULL DEFAULT '[]',
    secret_enc BLOB,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'disabled')),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

INSERT INTO nodes_new (id, server_id, address, name, protocol, port, protocol_settings, rate, tags, secret_enc, status, created_at, updated_at)
SELECT n.id,
       n.server_id,
       COALESCE((SELECT s.address FROM servers s WHERE s.id = n.server_id), ''),
       n.name,
       n.protocol,
       n.port,
       n.protocol_settings,
       n.rate,
       n.tags,
       n.secret_enc,
       n.status,
       n.created_at,
       n.updated_at
FROM nodes n;

DROP TABLE nodes;

ALTER TABLE nodes_new RENAME TO nodes;

CREATE INDEX idx_nodes_server ON nodes (server_id);
CREATE UNIQUE INDEX idx_nodes_port_active ON nodes (server_id, port) WHERE status = 'active';

ALTER TABLE servers DROP COLUMN address;
