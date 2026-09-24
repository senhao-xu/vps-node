ALTER TABLE agents RENAME COLUMN token_hash TO key_hash;
ALTER TABLE agents ADD COLUMN key_enc BLOB;

DROP INDEX IF EXISTS idx_servers_register_token;
ALTER TABLE servers DROP COLUMN register_token_hash;
ALTER TABLE servers DROP COLUMN register_token_expires_at;
