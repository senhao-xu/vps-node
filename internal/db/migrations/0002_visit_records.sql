CREATE TABLE visit_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    node_id INTEGER NOT NULL,
    server_id INTEGER NOT NULL,
    dest_host TEXT NOT NULL,
    dest_port INTEGER NOT NULL DEFAULT 0,
    network TEXT NOT NULL DEFAULT '',
    client_ip TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);

CREATE INDEX idx_visit_records_user ON visit_records (user_id, created_at);
CREATE INDEX idx_visit_records_node ON visit_records (node_id, created_at);
CREATE INDEX idx_visit_records_server ON visit_records (server_id, created_at);
CREATE INDEX idx_visit_records_host ON visit_records (dest_host, created_at);

CREATE TABLE visit_daily_domains (
    day INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    node_id INTEGER NOT NULL,
    server_id INTEGER NOT NULL,
    dest_host TEXT NOT NULL,
    hits INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (day, user_id, node_id, dest_host)
);

CREATE INDEX idx_visit_daily_user ON visit_daily_domains (user_id, day);
CREATE INDEX idx_visit_daily_node ON visit_daily_domains (node_id, day);
CREATE INDEX idx_visit_daily_host ON visit_daily_domains (dest_host, day);

CREATE TABLE visit_batches (
    agent_id INTEGER NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    seq INTEGER NOT NULL,
    received_at INTEGER NOT NULL,
    records INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (agent_id, seq)
);
