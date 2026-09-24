-- Node gains an optional IPv6 entry: `ipv6_enabled` toggles advertising the
-- extra IPv6 entry in subscriptions, `ipv6_address` is the client-facing IPv6
-- host. Both default to off/empty so existing nodes render exactly as before.
--
-- Plain ADD COLUMN keeps the `nodes` table in place, so child foreign keys
-- (user_nodes, online_devices) are untouched.

ALTER TABLE nodes ADD COLUMN ipv6_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE nodes ADD COLUMN ipv6_address TEXT NOT NULL DEFAULT '';
