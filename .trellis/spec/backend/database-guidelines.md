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
- History tables (`connection_logs`, `traffic_records`) intentionally have NO FKs to users/nodes/servers — history survives parent deletion until retention cleanup. `sessions` cascades on all three parents.
- Idempotency markers: `traffic_batches` / `connection_log_batches` PK `(agent_id, seq)`.
- `server_revisions`: per-server monotonic counter, the ONLY config-change signal for agents.

### 4. Validation & Error Matrix
- Unique violation → `409 conflict` (port/name within server, token rotation).
- FK/ownership violation → `404 not_found` or `422` per contract; never 500.
- Batch `(agent_id, seq)` already present → return prior result, NO double counting (same tx as totals).

### 5. Good/Base/Bad Cases
- Good: mutation + revision bump + batch marker in ONE tx; crash leaves either all or nothing.
- Base: read paths compute aggregates from `traffic_records` (no denormalized totals except `users.used_bytes`).
- Bad: writing `users.used_bytes` outside the tx that inserts the batch marker → double counting on retry. Forbidden.

### 6. Tests Required
- Migration idempotency (`db_test.go`), unique/FK enforcement, batch duplicate-once (`janitor`/`web` tests), revision monotonic bump per mutation type.

### 7. Wrong vs Correct
#### Wrong
```go
repo.IncrementUserTraffic(ctx, ...) // separate tx from batch marker insert
repo.InsertBatchMarker(ctx, ...)
```
#### Correct
```go
repo.Tx(ctx, func(tx repo.DBTX) error {
    if err := repo.InsertTrafficRecords(tx, ...); err != nil { return err }
    if err := repo.IncrementUserTraffic(tx, ...); err != nil { return err }
    return repo.InsertTrafficBatchMarker(tx, agentID, seq)
})
```

---

## Retention & Caps

- `internal/janitor` deletes: `connection_logs` (raw retention days), `traffic_records` (aggregate retention days), stale `sessions`, expired `admin_sessions`, old batch markers — bounded 500-row batches per loop.
- Storage caps: `retention.max_connection_logs` (default 1,000,000) and `retention.max_traffic_records` (default 5,000,000; `0` = off) delete oldest-beyond-cap. Any new history table needs both a retention-days path AND a cap path in the janitor sweep.

---

## Gotcha: SQLite table rebuild inside tx-based migrations

> **Warning**: the migration runner wraps each file in a transaction and the DSN sets `foreign_keys(1)`. Inside that transaction `PRAGMA foreign_keys=off` and `PRAGMA legacy_alter_table` are **no-ops**, so a plain create-copy-drop-rename on a parent table cascade-wipes or dangles its children.

- To change a parent table (e.g. `users` in 0004): back up child rows → drop children → rebuild parent → recreate children byte-equivalent (columns, PK, CASCADE FKs, UNIQUE constraints, ALL indexes) → reinsert children → all in the same migration tx.
- Always add a migration test asserting: dependent rows survive, `PRAGMA foreign_key_check` is clean, cascade semantics still work post-migration, and recreated indexes exist (`sqlite_master` check).
- Never rely on column order (`SELECT *` / bare `INSERT INTO t VALUES`) — rebuilds may reorder columns.
