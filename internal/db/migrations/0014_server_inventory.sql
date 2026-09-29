ALTER TABLE servers ADD COLUMN ip TEXT NOT NULL DEFAULT '';
ALTER TABLE servers ADD COLUMN region TEXT NOT NULL DEFAULT '';
ALTER TABLE servers ADD COLUMN price_cents INTEGER NOT NULL DEFAULT 0;
ALTER TABLE servers ADD COLUMN price_currency TEXT NOT NULL DEFAULT 'USD';
ALTER TABLE servers ADD COLUMN traffic_limit_bytes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE servers ADD COLUMN traffic_used_bytes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE servers ADD COLUMN expires_at INTEGER;
ALTER TABLE servers ADD COLUMN sort_order INTEGER NOT NULL DEFAULT 0;
UPDATE servers SET sort_order = id;
UPDATE servers SET traffic_used_bytes = COALESCE((SELECT SUM(u + d) FROM traffic_records WHERE server_id = servers.id), 0);
CREATE TRIGGER trg_server_traffic_used AFTER INSERT ON traffic_records
BEGIN
    UPDATE servers SET traffic_used_bytes = traffic_used_bytes + NEW.u + NEW.d WHERE id = NEW.server_id;
END;
