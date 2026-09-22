# Database Guidelines

> SQLite access, migrations, and the two cross-cutting invariants (revision bump, batch idempotency).

---

## Scenario: Any table or query change

### 1. Scope / Trigger
- Trigger: any schema/migration change, new repo query, or new write path.

### 2. Signatures
- Open: `internal/db` — WAL mode, `busy_timeout=5000`, `foreign_keys=ON`, `IsUniqueViolation(err)` helper.
- Migrations: embedded `internal/db/migrations/*.sql`, sequential runner, `schema_migrations` table, per-migration tx, apply-twice = no-op (tested).
- Tx helper: `internal/repo` `Tx(ctx, ...)`; repos accept `DBTX` (db or tx) so handlers compose atomically.

### 3. Contracts (schema conventions)
- INTEGER unix-second timestamps everywhere; byte counters as INTEGER.
- Ownership FKs: `nodes.server_id`, `agents.server_id` UNIQUE (Server 1:1 Agent), `user_nodes(user_id,node_id)` PK.
- `agents.token_hash` UNIQUE; `servers.register_token_hash` single-use (cleared after use).
- `traffic_records` intentionally has NO FKs to users/nodes/servers — history survives parent deletion until retention cleanup. `online_devices` (current state, not history) cascades on user/node/server deletion.
- Idempotency markers: `traffic_batches` / `device_batches` PK `(agent_id, seq)`.
- `server_revisions`: per-server monotonic counter, the ONLY config-change signal for agents.

### 4. Validation & Error Matrix
- Unique violation → `409 conflict` (port/name within server, token rotation).
- FK/ownership violation → `404 not_found` or `422` per contract; never 500.
- Batch `(agent_id, seq)` already present → return prior result, NO double counting (same tx as totals).

### 5. Good/Base/Bad Cases
- Good: mutation + revision bump + batch marker in ONE tx; crash leaves either all or nothing.
- Base: read paths compute aggregates from `traffic_records` (no denormalized totals except `users.u`/`users.d`).
- Bad: writing `users.u`/`users.d` outside the tx that inserts the batch marker → double counting on retry. Forbidden.

### 6. Tests Required
- Migration idempotency (`db_test.go`), unique/FK enforcement, batch duplicate-once (`janitor`/`web` tests), revision monotonic bump per mutation type.

### 7. Wrong vs Correct
#### Wrong
```go
repo.InsertTrafficRecords(ctx, records)   // separate tx from users.u/d + batch marker
repo.AddUserUsedBytes(ctx, userID, u, d)
repo.RecordTrafficBatch(ctx, agentID, seq)
```

#### Correct
```go
// ONE tx: traffic_records (scaled by nodes.rate) + users.u/d + traffic_batches marker
repo.IngestTrafficBatch(ctx, agentID, seq, records)
```

`IngestTrafficBatch` / `IngestDeviceBatch` are the only production writers for these paths; they scale each record by the owning node's `rate`, accumulate `users.u`/`users.d`, upsert/prune `online_devices`, refresh `online_count`, and insert the batch marker in the same transaction. `InsertTrafficRecords` / `AddUserUsedBytes` / `RecordTrafficBatch` exist for test fixtures only — never call them from handlers.

---

## Retention & Caps

- `internal/janitor` deletes: `traffic_records` (aggregate retention days), `online_devices` stale for >24h, expired `admin_sessions`, and old `traffic_batches`/`device_batches` markers — bounded 500-row batches per loop. Deleting devices recomputes each affected user's `online_count` in the same tx.
- Storage cap: `retention.max_traffic_records` (default 5,000,000; `0` = off) deletes oldest-beyond-cap. `online_devices` is bounded by the stale-device sweep, not a row cap.

---

## Convention: Aggregate queries over traffic_records

- Per-user / per-(user,node) rollups live in `internal/repo/stats.go`: `SumTrafficByUser`, `SumTrafficByUserNode` (JOINs `nodes` + `servers` for display names). Both reuse the shared `TrafficFilter` via `trafficWherePrefixed(f, prefix)` — the prefixed variant exists because JOINed queries make bare `server_id` / `created_at` ambiguous; pass `""` to get the original behavior.
- Dashboard "total" semantics: user-level total = `users.used_bytes` (matches the user list; resets with traffic reset), while per-node splits always come from retained `traffic_records` (bounded by `retention.aggregate_days`, unaffected by resets). The two sums may legitimately differ; state this in the API contract when adding such endpoints.
- Dashboard "today" boundary is `time.Now().UTC().Truncate(24 * time.Hour)` — reuse this exact expression so all dashboard figures share one timezone basis.

---

## Gotcha: SQLite table rebuild inside tx-based migrations

> **Warning**: the migration runner wraps each file in a transaction and the DSN sets `foreign_keys(1)`. Inside that transaction `PRAGMA foreign_keys=off` and `PRAGMA legacy_alter_table` are **no-ops**, so a plain create-copy-drop-rename on a parent table cascade-wipes or dangles its children.

- To change a parent table (e.g. `users` in 0004): back up child rows → drop children → rebuild parent → recreate children byte-equivalent (columns, PK, CASCADE FKs, UNIQUE constraints, ALL indexes) → reinsert children → all in the same migration tx.
- Always add a migration test asserting: dependent rows survive, `PRAGMA foreign_key_check` is clean, cascade semantics still work post-migration, and recreated indexes exist (`sqlite_master` check).
- Never rely on column order (`SELECT *` / bare `INSERT INTO t VALUES`) — rebuilds may reorder columns.
