CREATE TABLE server_traffic_daily (
    server_id INTEGER NOT NULL,
    day INTEGER NOT NULL,
    u INTEGER NOT NULL DEFAULT 0,
    d INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (server_id, day)
);

INSERT INTO server_traffic_daily (server_id, day, u, d)
SELECT server_id, created_at - ((created_at % 86400 + 86400) % 86400), SUM(u), SUM(d)
FROM traffic_records
GROUP BY server_id, created_at - ((created_at % 86400 + 86400) % 86400);

CREATE TRIGGER trg_server_traffic_daily AFTER INSERT ON traffic_records
BEGIN
    INSERT INTO server_traffic_daily (server_id, day, u, d)
    VALUES (NEW.server_id, NEW.created_at - ((NEW.created_at % 86400 + 86400) % 86400), NEW.u, NEW.d)
    ON CONFLICT (server_id, day) DO UPDATE SET u = u + excluded.u, d = d + excluded.d;
END;

ALTER TABLE server_revisions ADD COLUMN expiry_checked_at INTEGER NOT NULL DEFAULT 0;

INSERT INTO server_revisions (server_id, revision, updated_at)
SELECT id, 1, strftime('%s', 'now') FROM servers WHERE 1
ON CONFLICT (server_id) DO UPDATE SET revision = server_revisions.revision + 1, updated_at = excluded.updated_at;
