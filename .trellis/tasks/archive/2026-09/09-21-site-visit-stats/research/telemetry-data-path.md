# Research: telemetry data path (mirror target for visited-sites / connection logs)

- **Query**: Exhaustive map of the EXISTING telemetry data path (agent in-process sing-box ConnectionTracker → batching → upload → Panel ingestion → repo idempotency → admin API → Vue UI → migrations/retention/contracts/specs) that a new "visited sites / connection log" feature must mirror.
- **Scope**: internal (read-only) + external (pinned sing-box v1.13.2 library source)
- **Date**: 2026-09-22
- **Task**: `.trellis/tasks/09-21-site-visit-stats`
- **Note**: no active task was set via `task.py current`; the task directory was supplied explicitly by the caller. No code outside this `research/` directory was modified.
- **Pinned dependency**: `github.com/sagernet/sing-box v1.13.2`, `github.com/sagernet/sing v0.8.2` (see `go.mod:7`).

---

## 0. Executive summary (what the existing path is)

```
embedded sing-box router
  └─ ConnTracker.RoutedConnection/RoutedPacketConnection   kernel/singbox/tracker.go:170,186
       └─ tracks per-(user,node) byte counters + live conn entries
Runtime.Snapshot()                                         kernel/singbox/runtime.go:82
  └─ Snapshot{Traffic map[Pair]Traffic, Devices []Device}   kernel/singbox/types.go:32

agentruntime.Loop.telemetryPass (ticker = cfg.TrafficInterval, default 60s)
  ├─ collectDeltas(snapshot.Traffic)  → pending map       loop.go:290
  ├─ flushTraffic → buildTrafficBatches (≤1000/req, batch_seq) → client.Traffic  loop.go:313,376
  └─ flushDevices → buildDeviceBatches → client.Devices    loop.go:409,457
       state.TrafficBatchSeq / DeviceBatchSeq persisted    agentstate/state.go:16-17

agentclient.do/attempt: JSON POST, Bearer token, retry/backoff, error envelope  client.go:199-324

Panel routes: POST /api/agent/traffic, /api/agent/devices  web/web.go:127-128
  └─ requireAgent derives agent_id + server_id from bearer  web/agent.go:29-68
  └─ handleAgentTraffic / handleAgentDevices validate + ingest  web/agent_telemetry.go:67,148
       └─ repo.IngestTrafficBatch / IngestDeviceBatch (ONE tx + batch marker)  repo/telemetry.go:41,121

Admin reads: GET /api/users/:id/devices, /api/users/:id/traffic, dashboard endpoints
```

There is **no destination/domain collection anywhere today**. Inbounds/route carry **no sniff configuration**, so `metadata.Domain` is empty; only `metadata.Destination` (client-declared host for SOCKS-like inbounds) may carry an Fqdn. A prior `connection_logs` table existed and was deliberately removed (see §8).

---

## 1. Agent-side collection & batching

### 1.1 Embedded sing-box runtime

`internal/kernel/singbox/runtime.go`
- `Runtime` wraps `*box.Box` + `*ConnTracker` (L18-22).
- `Start(configJSON, users)` (L28-60): `prepareConfig` strips a top-level `"experimental"` key (L102-112); unmarshals with `singJSON.UnmarshalExtendedContext[option.Options]`; `box.New`; fetches router via `service.FromContext[adapter.Router](ctx)` (L42); creates `NewConnTracker(users)` and calls `router.AppendTracker(tracker)` (L51-52); `inst.Start()`.
- `Reload = Start` (L62-64). `Stop/stopLocked` (L66-80) clears `inst` and `tracker`.
- `Snapshot()` (L82-90): locks, reads `r.tracker`, delegates to `tracker.Snapshot()`; when no tracker returns `Snapshot{Traffic: map[Pair]Traffic{}}`.
- `Validate(configJSON)` (L92-100) — used by integration tests.

`internal/kernel/singbox/tracker.go`
- `pairCounters{upload,download atomic.Int64}` (L27-30).
- `connEntry` (L32-42): `id, userID, nodeID, ip, connectedAt time.Time, upload, download atomic.Int64, pairUp, pairDown *atomic.Int64`. Helpers `addUpload/addDownload/countUp/countDown` (L44-55).
- `Table` (L57-61): `usersByName map[string]int64`, `nodesByTag map[string]int64`, `limits map[int64]int64`. `buildTable` (L63-77) maps `userName(u.ID)` = `"u-"+id` (L93-95) and `inboundTag(protocol,id)` = `protocol+"-"+id` (L97-99). `userID(name)` (L79), `nodeID(metadata)` matches `metadata.Inbound` (L84-87), `deviceLimit(userID)` (L89).
- `ConnTracker` (L101-108): `table`, `mu`, `nextID`, `pairs map[Pair]*pairCounters`, `live map[uint64]*connEntry`.
- `entry(metadata adapter.InboundContext)` (L118-162) — **the only place inbound metadata is read**:
  - `userID, ok := t.table.userID(metadata.User)` (L119)
  - `nodeID, ok := t.table.nodeID(metadata)` → matches `metadata.Inbound` (L123)
  - `ip := sourceAddr(metadata.Source)` (L127; `sourceAddr` L339-344)
  - device-limit gate counts distinct live IPs per user; returns `admissionDenied` (L132-142)
  - creates `connEntry{id, userID, nodeID, ip, connectedAt: time.Now(), pairUp/pairDown}` and stores in `live` (L150-161)
  - Note it does **not** read `metadata.Destination`, `metadata.Domain`, `metadata.Protocol`, or `metadata.Network` today.
- `RoutedConnection` (L170-184) / `RoutedPacketConnection` (L186-196): wrap the connection in `trackedConn` / `trackedPacketConn`; on `admissionDenied` closes the conn.
- `Snapshot()` (L198-245): builds `Traffic` from `pairs` counters; builds `Devices` per pair by aggregating live entries (distinct IP set + online count). Returns aggregates only — **no per-connection rows**.
- `trackedConn` (L247-299): Read/ReadBuffer → `addUpload`; Write/WriteBuffer → `addDownload`; `Close` releases entry (L287-291). `trackedPacketConn` (L301-337) mirrors this for UDP.
- `release(e)` (L164-168) only deletes from `live`; no close timestamp retained.

`internal/kernel/singbox/types.go`
```go
type NodeRef struct { ID int64; Protocol string; Port int }
type UserRef struct { ID int64; DeviceLimit int64; Nodes []NodeRef }   // L9-13
type Pair struct { UserID int64; NodeID int64 }                         // L15-18
type Traffic struct { Upload, Download int64 }                          // L20-23
type Device struct { UserID, NodeID int64; IPs []string; Online int }    // L25-30
type Snapshot struct { Traffic map[Pair]Traffic; Devices []Device }     // L32-35
```

### 1.2 Where `metadata.Destination` / `metadata.Domain` live (sing-box v1.13.2)

`adapter/inbound.go:43-99` (`InboundContext`) — accessible fields include:
`Inbound`, `InboundType`, `IPVersion`, `Network string`, `Source M.Socksaddr`, `Destination M.Socksaddr`, `User`, `Outbound`, `Protocol string` (sniffed), `Domain string` (sniffed), `Client`, `SniffContext`, `SnifferNames`, `SniffError`, `OriginDestination`, `RouteOriginalDestination`, `DestinationAddresses []netip.Addr`, `ProcessInfo`, …

`M.Socksaddr` (`sing@v0.8.2/common/metadata/addr.go:12-16`):
```go
type Socksaddr struct { Addr netip.Addr; Port uint16; Fqdn string }
```
Accessors: `IsIP()`, `IsFqdn()` (`metadata/addr.go`), `AddrString()`, `AddrPort()`, `Unmap()`.

How the fields get populated:
- **`metadata.Network`**: set to `"tcp"` before TCP routing (`route/route.go:81`), `"udp"` before UDP routing (`:217`). Constants `N.NetworkTCP`/`N.NetworkUDP`.
- **`metadata.Destination`**: parsed from the inbound protocol request (shadowsocks/VLESS address header) — may be IP or Fqdn. For FakeIP it is rewritten to the domain and `metadata.OriginDestination` keeps the IP (`route/route.go:442-454`).
- **`metadata.Domain` / `metadata.Protocol` (sniffed)**: populated **only when a `sniff` route rule action runs**. Sniffer implementations: TLS SNI `common/sniff/tls.go:24-25`, HTTP Host `common/sniff/http.go:25-26`, QUIC SNI `common/sniff/quic.go:303-309`, etc.
- Tracker call site: `route/route.go:153-155` (TCP) and `:279-280` (UDP). `matchRule(&metadata, …)` at `:95` runs rule actions first, including `*R.RuleActionSniff` (`:551-567` → `actionSniff` `:595-659`); thereafter `metadata.Domain`/`Protocol` are set. So the tracker already receives a fully-populated `InboundContext` **if the route has a sniff rule**.

**Current renderer has no sniff rule** (see §6), therefore `metadata.Domain` will be empty in production today; `metadata.Destination.Fqdn` may still be non-empty for protocols that carry the requested hostname.

### 1.3 Report loop

`internal/agentruntime/loop.go`
- `const maxBatchRecords = 1000` (L18).
- `Kernel` interface (L20-24): `Start/Stop/Snapshot`.
- `Loop` fields (L26-45): `cfg`, `client`, `state`, `statePath`, `kernel`, `metrics`, `logger`, `version`; mutex-guarded `lastSeen map[Pair]Traffic`, `pending map[Pair]Traffic`, `inflight []agentclient.TrafficBatch`, `inflightIdx int`, `deviceInflight []agentclient.DeviceBatch`, `deviceInflightIdx int`, `lastApplyError`, `forceApply`.
- `Run` (L88-136): initial `heartbeatPass`, `syncPass`, `telemetryPass`; tickers `hbTicker` (HeartbeatInterval), `syncTicker` (SyncInterval), `telemetryTicker` (TrafficInterval) (L107-112); `syncNow` channel nudges immediate sync.
- `EnsureIdentity` (L138-184): resets state on token/server change; registers and zeroes `TrafficBatchSeq`/`DeviceBatchSeq` (L178-179).
- `telemetryPass` (L277-288):
  ```go
  snapshot := l.kernel.Snapshot()
  now := time.Now().UTC()
  l.collectDeltas(snapshot.Traffic)
  if l.cfg.Collection.Traffic { l.flushTraffic(ctx, now) }
  l.flushDevices(ctx, snapshot.Devices, now)
  ```
  Note devices are flushed unconditionally; traffic only when `Collection.Traffic`.
- `collectDeltas` (L290-311): delta vs `lastSeen`, clamp negatives to 0, accumulate into `pending`; update `lastSeen`.
- `flushTraffic` (L313-358): if no inflight and pending empty → return; else `buildTrafficBatches(pending, state.TrafficBatchSeq, now)`, clear pending. Sends sequentially. On non-retryable error: `requeueTrafficBatch` under the already-advanced sequence and continue. On retryable error: persist `inflightIdx` and return (frozen batch replayed identically). On success: persist `state.TrafficBatchSeq = batch.BatchSeq` + `saveState()`; log ack.
- `buildTrafficBatches` (L376-407): records sorted by `(UserID, NodeID)`; chunked by 1000; `BatchSeq = base + idx + 1`; each record carries the same `recorded_at` RFC3339.
- `flushDevices` (L409-455): builds batches once (`buildDeviceBatches`); on non-retryable error drops inflight, advances `DeviceBatchSeq`, and relies on the next snapshot; on retryable error freezes and returns; success persists `DeviceBatchSeq`.
- `buildDeviceBatches` (L457-497): reports sorted `(UserID,NodeID)`; **always at least one batch, even empty** (L478-484).
- `requeueTrafficBatch` (L360-374): re-adds record U/D to `pending`, advances `state.TrafficBatchSeq`.
- `toUserRefs` (L499-513) converts `[]agentclient.User` → `[]kernelsingbox.UserRef`.

### 1.4 Agent HTTP client

`internal/agentclient/client.go`
- Constants (L18-23): `defaultMaxAttempts=4`, `defaultBaseDelay=500ms`, `defaultMaxDelay=8s`, `requestTimeout=15s`.
- `APIError{Status,Code,Message}` + `Retryable()` true for 408/429/5xx (L58-72).
- Wire structs (exact):
```go
type TrafficRecord struct { UserID int64 `json:"user_id"`; NodeID int64 `json:"node_id"`; U int64 `json:"u"`; D int64 `json:"d"`; RecordedAt string `json:"recorded_at"` }   // L133-139
type TrafficBatch struct { BatchSeq int64 `json:"batch_seq"`; Records []TrafficRecord `json:"records"` }                                                                  // L141-144
type TrafficAck struct { Accepted bool `json:"accepted"`; BatchSeq int64 `json:"batch_seq"`; Records int64 `json:"records"` }                                              // L146-150
type DeviceReport struct { UserID int64 `json:"user_id"`; NodeID int64 `json:"node_id"`; IPs []string `json:"ips"`; Online int `json:"online"` }                            // L152-157
type DeviceBatch struct { BatchSeq int64 `json:"batch_seq"`; RecordedAt string `json:"recorded_at"`; Devices []DeviceReport `json:"devices"` }                              // L159-163
type DeviceAck struct { Accepted bool `json:"accepted"`; BatchSeq int64 `json:"batch_seq"`; Devices int64 `json:"devices"` }                                               // L165-169
```
  Also `RegisterRequest/Response` (L74-86), `HeartbeatRequest/Response` (L88-101), `UserNode/User` (L103-121), `ConfigResponse` (L123-131).
- Endpoints/methods: `Register` → `POST /api/agent/register` (L171-177); `Heartbeat` → `/api/agent/heartbeat` (L179-185); `Config` → `GET /api/agent/config?version=` (L187-197); `Traffic` → `POST /api/agent/traffic` (L199-205); `Devices` → `POST /api/agent/devices` (L207-213).
- `do` (L215-252): marshal → retry loop with backoff → `attempt`. `attempt` (L254-301): sets `Accept`, `Content-Type` when body, `Authorization: Bearer <token>`; reads at most 4 MiB (`1<<22`, L276); on non-2xx parses `{"error":{"code","message"}}` (L282-292); success `json.Unmarshal(data, out)`.
- `backoffDelay` (L303-313): exponential `baseDelay << (n-1)` capped at `maxDelay`, plus jitter in `[0, baseDelay]`.
- `Retryable(err)` (L318-324): non-`APIError` errors retryable; API errors per `APIError.Retryable`.
- **No gzip/compression and no explicit max payload size on the agent side**; batching caps at 1000 records.

### 1.5 Agent state

`internal/agentstate/state.go`
```go
type State struct {
  AgentID int64 `json:"agent_id,omitempty"`
  AgentToken string `json:"agent_token,omitempty"`
  ServerID int64 `json:"server_id,omitempty"`
  AppliedRevision int64 `json:"applied_revision,omitempty"`
  TrafficBatchSeq int64 `json:"traffic_batch_seq,omitempty"`
  DeviceBatchSeq int64 `json:"device_batch_seq,omitempty"`
  UpdatedAt time.Time `json:"updated_at"`
}                                            // L11-19
```
`Load` (L21-34) returns empty state on missing file; `Save` (L36-73) writes atomically (temp + `Sync` + `Rename`, chmod 0600, dir 0700).

### 1.6 Agent config

`internal/config/config.go`
- Defaults (L25-36): `defaultHeartbeatInterval=30s`, `defaultSyncInterval=30s`, `defaultTrafficInterval=60s`, `minAgentInterval=5s`, `defaultAgentStatePath="agent_state.json"`, `defaultMaxTrafficRecords=5_000_000`.
- `Collection struct { Traffic bool }` (L116-118); `Agent` struct (L120-131) has `TrafficInterval`, `Collection`, `StatePath`, `ServerID`, `Token`, `RegisterToken`, `PanelURL`, `*Interval`.
- YAML `agentFile` (L150-163): `traffic_interval`, `collection.traffic` (`*bool`).
- `LoadAgent` (L294-400): default `Collection: Collection{Traffic: true}` (L301); env overrides `AGENT_TRAFFIC_INTERVAL` (L369-373), `AGENT_*`; validation requires intervals `>= 5s` (L390-398).
- Panel `Retention` (L49-55): `AggregateDays` (default 90), `SweepInterval` (default 1h), `MaxTrafficRecords`.

### 1.7 DTO duplication (important)

There is **no shared wire package**. Payloads are mirrored manually in two places (by design, per spec `quality-guidelines.md:48`):
- Agent: `internal/agentclient/client.go` (`TrafficRecord/TrafficBatch/TrafficAck`, `DeviceReport/DeviceBatch/DeviceAck`, `User/UserNode`, `ConfigResponse`).
- Panel: handler-local structs in `internal/web/agent_telemetry.go` (`agentTrafficRecord` L59-65, `agentDeviceReport` L141-146) and config-payload structs in `internal/web/agent_config.go` (`agentUserNodeDTO` L72-77, `agentUserDTO` L79-90).
- Frontend mirror: `web/src/api/types.ts`.

Adding destination fields therefore requires coordinated edits in all three mirrors plus `docs/api-contract.md`.

---

## 2. Panel agent-facing ingestion

### 2.1 Routes & middleware

`internal/web/web.go`
- `registerAgentRoutes` (L123-129):
```go
mux.HandleFunc("POST /api/agent/register", h.handleAgentRegister)
mux.HandleFunc("POST /api/agent/heartbeat", h.requireAgent(h.handleAgentHeartbeat))
mux.HandleFunc("GET  /api/agent/config",    h.requireAgent(h.handleAgentConfig))
mux.HandleFunc("POST /api/agent/traffic",   h.requireAgent(h.handleAgentTraffic))
mux.HandleFunc("POST /api/agent/devices",   h.requireAgent(h.handleAgentDevices))
```
- `Options` carries `AgentHeartbeatInterval/AgentSyncInterval/AgentTrafficInterval` (L43-45) → converted to seconds in `Handler` (L58-60, L82-84); defaults in `agent.go:18-20`.

`internal/web/agent.go`
- Context keys `ctxKeyAgentID=2`, `ctxKeyAgentServerID=3` (L15-16).
- `requireAgent` (L29-49): bearer token → `adminauth.HashToken` → `repo.GetAgentByTokenHash`; 401 on missing/invalid; injects agent ID + server ID.
- `bearerToken` (L51-58); `agentIDFrom`/`agentServerIDFrom` (L60-68).
- `handleAgentRegister` (L70-121): validates register token (single-use, expiry); returns `{agent_id, agent_token, server_id, heartbeat_interval_seconds, sync_interval_seconds, traffic_interval_seconds}`.
- `handleAgentHeartbeat` (L131-171): requires version 1-64, all four metrics non-nil, percent 0-100, uptime ≥0; `repo.RecordHeartbeat`; returns `{ok, server_revision, heartbeat_interval_seconds}`.

`internal/httpx/server.go`
- `NewServer` sets only `ReadHeaderTimeout: 10s` (L23-31); no global body limit.
- `WrapHandler` = recovery + logging middleware (L33-41). `WriteJSON` (L57-61), `WriteError` envelope (L63-67).
- Request size limit is enforced per-handler by `decodeJSON` via `http.MaxBytesReader(..., 1<<20)` (`web/respond.go:15,67-77`) → `413 payload_too_large`.

### 2.2 Handlers

`internal/web/agent_telemetry.go`
- Constants: `maxAgentBatchRecords = 1000` (L15), `agentTimestampWindow = 24h` (L16), `maxIPStrLen = 64` (L17).
- `pairKey{user,node}` (L20-23); `agentScopes(ctx, serverID)` (L25-43) returns `nodeSet` (`ListNodesByServer`) and `pairSet` (`ListUserNodePairsByServer`).
- `parseAgentTimestamp(raw, now)` (L45-57): required, RFC3339, rejects `>24h` skew either direction.
- `handleAgentTraffic` (L67-139): `decodeJSON`; `batch_seq ≥ 1`; `len(records) ≤ 1000` else `413`; per record: user/node ≥1, u/d ≥0, node in `nodeSet`, `(user,node)` in `pairSet`, timestamp valid; builds `[]repo.NewTrafficRecord` with `ServerID: serverID`; `repo.IngestTrafficBatch`; on `repo.ErrConflict` → `repo.TrafficBatchCount` (idempotent replay); response `{accepted:true, batch_seq, records}`.
- `handleAgentDevices` (L148-226): same envelope; `recorded_at` validated once; per report user/node ≥1, online ≥0, scope checks; IPs deduped in-request and length-checked; `onlineByUser` accumulated; `repo.IngestDeviceBatch(agentID, seq, serverID, devices, onlineByUser, seenAt.Unix())`; on conflict `DeviceBatchCount`; response `{accepted:true, batch_seq, devices}`.
- Response shapes are built with `map[string]any` (not typed Ack structs) on the Panel side.

### 2.3 Ownership

- `server_id` is **never** read from the body; it is derived from the agent token in `requireAgent` (`agent.go:45-46`) and passed to ingest. Every `user_id`/`node_id` is validated against the server's node set + `user_nodes` pair set (`agentScopes`). Violations reject the whole batch with `422 validation` and nothing is written (validation precedes the tx).

---

## 3. Repo ingestion & read helpers

`internal/repo/repo.go`
- `ErrNotFound`, `ErrConflict` (L12-15); `Tx(ctx, db, fn)` (L25-35); `mapErr` maps `sql.ErrNoRows`→`ErrNotFound`, unique violation→`ErrConflict` (L37-46); `nowUnix`, `toTime`, `toTimePtr`, `timeArg`, `rowsAffected`; `normalizePage` clamps page≥1, page_size default 20 / max 100 (L79-89).

`internal/repo/telemetry.go`
- `UserNodePair` (L10-13), `UserTrafficDelta` (L15-18), `ListUserNodePairsByServer` (L20-39).
- `IngestTrafficBatch(ctx, agentID, seq, records)` (L41-95): one tx —
  1. `SELECT records FROM traffic_batches WHERE agent_id=? AND seq=?` → if found, `duplicate=true`, return count (no mutation);
  2. per record fetch node `rate` (cached), `scaleBytes`, `INSERT INTO traffic_records(user_id,node_id,server_id,u,d,created_at)`, accumulate `userDeltas`;
  3. `UPDATE users SET u=u+?, d=d+?, updated_at=? WHERE id=?`;
  4. `INSERT INTO traffic_batches(agent_id,seq,received_at,records)`.
- `scaleBytes` (L97-106), `TrafficBatchCount` (L108-119).
- `IngestDeviceBatch(ctx, agentID, seq, serverID, devices, onlineByUser, seenAt)` (L121-195): one tx — dedupe `device_batches`; gather affected users (`SELECT DISTINCT user_id FROM online_devices WHERE server_id=?` ∪ reported); `DELETE FROM online_devices WHERE server_id=? AND last_seen_at < ?`; per device upsert:
  `INSERT ... ON CONFLICT (user_id,node_id,ip) DO UPDATE SET last_seen_at=excluded.last_seen_at, online=excluded.online` (L162-167); `refreshOnlineCountExec` per affected user; `UPDATE users SET last_online_at` per user with online>0; `INSERT INTO device_batches(...)`.
- `DeviceBatchCount` (L197-207).

`internal/repo/batches.go`
- Convenience fixtures `TrafficBatchExists/RecordTrafficBatch/DeviceBatchExists/RecordDeviceBatch` (L7-21) + `batchExists`/`recordBatch`. Spec `database-guidelines.md:52` states `Ingest*` are the only production writers; these exist for tests.

`internal/repo/online_devices.go`
- `OnlineDevice` (L10-19), `NewOnlineDevice` (L21-26), `ListDevicesByUser` (L28-34), `ListDevicesByNode` (L36-42), `DeleteStaleDevicesBatch` (L44-84), `refreshOnlineCountExec` (L86-91), `onlineDeviceSelect` (L93), `collectOnlineDevices` (L95-108).

`internal/repo/traffic.go`
- `TrafficRecord`/`NewTrafficRecord` (L10-27), `TrafficFilter` (L29-35), `InsertTrafficRecords` (L37-49, fixture), `trafficWhere`/`trafficWherePrefixed` (L51-85), `SumTraffic` (L87-93), `SumTrafficBuckets` (L101-125), `DeleteTrafficRecordsBefore` (L127-133).

`internal/repo/counts.go`: `inPlaceholders`/`int64Args` (L8-18), grouped counts (`CountOnlineUsersByServerIDs` L36, `CountOnlineUsersByNodeIDs` L52, `groupedCount` L60), `ListServerIDsByUser` (L78).

---

## 4. Migrations

`internal/db/migrations.go`
- `//go:embed migrations/*.sql` (L13-14).
- `Migrate(ctx)` (L16-57): creates `schema_migrations(version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`; lists `migrations/*.sql` sorted by filename; skips applied versions; applies each missing.
- `applyMigration` (L77-96): reads file, **one tx per file**, executes the whole file, then inserts the version.
- `migrationVersion` (L98-104): integer prefix before `_`.
- `internal/db/db.go`: DSN `file:<path>?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)` (L32-34); `IsUniqueViolation` string match `"UNIQUE constraint failed"` (L36-41).

Current state of the tree:
- **Only `internal/db/migrations/0001_init.sql` exists** (166 lines). It defines: `admins`, `users`, `servers`, `agents`, `nodes` (protocol CHECK incl. `anytls`), `user_nodes`, `online_devices` (UNIQUE `(user_id,node_id,ip)`, indexes on user/node/server), `traffic_records` (AUTOINCREMENT, no FKs, indexes user/node/server + created_at), `server_revisions`, `traffic_batches` (PK `(agent_id,seq)`), `device_batches` (PK `(agent_id,seq)`), `settings`, `admin_sessions`, `user_subscriptions`.
- There is **no `connection_logs` table today**. Migrations `0002`–`0006` existed historically but were **squashed into `0001_init.sql`** during the xboard-parity refactor (see `.trellis/tasks/09-22-xboard-parity/implement.md` §session 2).

Pattern to add a new table/column:
- New file `internal/db/migrations/NNNN_name.sql` (next would be `0002_*.sql`), sequential + idempotent via `schema_migrations`. Convention stated in `spec/backend/directory-structure.md:49`.
- **Gotcha**: `internal/db/db_test.go:36` asserts `COUNT(schema_migrations) == 1`. Adding a migration file makes that assertion fail unless it is updated. `TestMigrateIdempotent` (L24-53) also enumerates expected tables.
- `internal/db/migration_anytls_test.go` is a **schema-behavior** test (seeds server/nodes/users, asserts protocol CHECK + `online_devices` UNIQUE + cascade) — it is not an example of `ALTER TABLE`; there is **no ALTER-based column-add example in the current tree**.
- SQLite constraint caveat (binding, `spec/backend/database-guidelines.md:71-77`): inside a tx-based migration, `PRAGMA foreign_keys=off` is a no-op, so parent-table rebuilds must back up/drop/recreate children within the same tx.

---

## 5. Admin API + repo read path

### 5.1 Response/validation primitives — `internal/web/respond.go`
- `maxBodyBytes = 1 << 20` (L15).
- `apiError` (L17-25) + constructors: `errInvalid` 400 `invalid_request` (L27), `errUnauthorized` 401 (L31), `errValidation` 422 (L35), `errNotFound` 404 (L39), `errConflict` 409 (L43), `errTooLarge` 413 (L47).
- `writeErr` (L51-65) maps `repo.ErrNotFound`/`ErrConflict` and otherwise 500 `internal`.
- `decodeJSON` (L67-77): `http.MaxBytesReader`; **uses plain `json.Decoder` — unknown fields are silently ignored** (no `DisallowUnknownFields`; `grep` confirms `decodeJSONStrict` does not exist yet).
- `pathID` (L79-85); `pageQuery`/`parsePageQuery` (L87-109); `writePage` → `{items,total,page,page_size}` (L111-118); `parseTimeParam` (L120-130); `parseTimeBody` (L132-144).

### 5.2 DTO conventions — `internal/web/dto.go`
- `rfc3339`/`rfc3339Ptr` (L10-20).
- `userDTO` (L22-39), `userDetailDTO` (L41-45), `toUserDTO`/`toUserDetailDTO` (L47-82).
- `nodeRefDTO`/`nodeDTO`/`nodeDetailDTO` (L84-107); `agentInfoDTO` (L135-140); `serverDTO`/`serverDetailDTO` (L142-163); `deviceDTO` (L183-199); traffic series DTOs (L201-211); dashboard DTOs (L213-246); `settingsDTO` (L248-255).
- Repo structs carry **no JSON tags**; DTOs in this file own snake_case JSON.

### 5.3 Endpoint patterns
- `internal/web/users.go`: `handleUserList` (L80-131) pagination + filters (`status`, `expiry`, `query`; query hashed for token match L106-108) + `CountNodesByUserIDs` + `writePage`; `userDetail` helper (L250-260); `handleUserGet` (L262-274); create/update/delete/reset/expire handlers (L149-450); tri-state `optInt64`/`optString` (L37-68).
- `internal/web/user_data.go`: `handleUserDevices` (L18-38) — `pathID` → `GetUser` (404 guard) → `ListDevicesByUser` → `{items: []deviceDTO}`; `handleUserTraffic` (L40-97) — `from`/`to` params, `bucket` `day|hour` (default day) → `SumTraffic` + `SumTrafficBuckets` → `trafficSeriesDTO`.
- `internal/web/servers.go`: `handleServerList` (L17-53) pagination + counts + effective status; `handleServerGet` (L91-146) assembles `serverDetailDTO{revision, agent, nodes}`.
- `internal/web/nodes.go`: `handleNodeList` (L25-61) filters `server_id`/`protocol`/`status`/`q` → `writePage`.
- `internal/web/dashboard.go`: `handleDashboard` (L12-56) aggregate counters; `handleDashboardUserTraffic` (L58-147) `range=today|total`, per-user + per-node sums, sorted desc.
- `internal/web/settings.go`: `handleSettingsGet` (L47-54), `handleSettingsPut` (L56-124); `settingsRanges` (L39-45).
- Route registration for all admin endpoints: `web.go:131-171`.

### 5.4 Settings & retention
- Setting key constants `internal/web/web.go:19-26`: `retention.aggregate_days`, `server.offline_after_seconds`, `subscribe_urls`, `subscribe_path`, `subscribe_name`, `clash_meta_template`; `app_key` in `config.AppKeySettingKey` (`config.go:40`).
- `internal/repo/settings.go`: `GetSetting`/`SetSetting` (upsert `ON CONFLICT (key) DO UPDATE`)/`SetSettings` (tx)/`ListSettings`/`GetSettingOr`.
- `internal/janitor/janitor.go`: `Sweep` (L57-109) drains, in order: `traffic_records` older than retention days, `online_devices` stale >24h (`staleDeviceHorizon` L14), expired admin sessions, `traffic_batches`, `device_batches`, then enforces `max_traffic_records` cap. `drain` deletes in 500-row batches (L134-146). `retentionDays` reads `retention.aggregate_days` (L148-154).
- `internal/repo/cleanup.go`: `DeleteTrafficRecordsBatch` (L8), `DeleteExpiredAdminSessionsBatch` (L14), `DeleteTrafficBatchesBatch` (L20), `DeleteDeviceBatchesBatch` (L26), `CountTrafficRecords` (L32), `DeleteTrafficRecordsBeyondCap` (L36).
- Dashboard aggregates `internal/repo/stats.go`: `CountUsersOnline`/`CountOnlineDevices`/`SumTrafficSince`; `SumTrafficByUser` (L53), `SumTrafficByUserNode` (L84).

---

## 6. Web UI

### 6.1 Router & navigation
- `web/src/router/index.ts`: routes `/`, `/users`, `/users/:id`, `/servers`, `/nodes`, `/servers/:id`, `/settings`, `/login`, `/:pathMatch(.*)*` (L15-70); route meta `title/parentTitle/parentPath`; guard `router.beforeEach` hydrates auth (L73-80); `document.title` from meta (L82-84).
- `web/src/components/AppLayout.vue`: `navItems` (L44-49) drives desktop + mobile nav: `仪表盘`, `用户`, `服务器`, `节点`, `设置` with lucide icons. A new standalone page would add an entry here + a route.

### 6.2 API client & types
- `web/src/api/http.ts`: `request<T>(path, {method,query,body,authRedirect})` (L46-82); `buildUrl`; decodes error envelope `{error:{code,message}}` (L67-79); `401` triggers `unauthorizedHandler`; `errorMessage` (L84-88). Query values `undefined/null/''` are skipped (L39).
- `web/src/api/types.ts` is the single source of frontend contract types (L1-300): `Paged<T>` (L13-18), `User`/`UserDetail` (L20-47), `NodeBrief`/`NodeDetail` (L49-76), `OnlineDevice`/`OnlineDevices` (L78-88), `TrafficPoint`/`TrafficSeries` (L90-100), `Server`/`ServerDetail` (L102-129), `Dashboard*` (L131-166), `Settings` (L168-175).
- Endpoint modules: `users.ts` (`getUserDevices` L95, `getUserTraffic` L99-110), `dashboard.ts` (L4-12), `servers.ts`, `nodes.ts`, `settings.ts`.

### 6.3 Table & pagination components
- `web/src/components/DataTable.vue` (generic `T extends Record<string,unknown>`): required `columns`, `rows`, `rowKey`; options `loading`, `selectable`+`v-model:selected`, `sortKey`/`sortDir`+`@sort`, `skeletonRows`, `totalCount`, `bordered`; slots `cell-<key>`, `row-extra`, `empty` (L1-238). Loading renders skeletons; empty renders `EmptyState` (L153-185).
- `web/src/components/TablePaginator.vue`: props `page/pageSize/total/pageSizeOptions`, emits `change(page,pageSize)` (L1-97).

### 6.4 Detail-page composition patterns
- `web/src/pages/UserDetailPage.vue`: loads user + node/server lists + node auth in parallel (L47-56); composes `components/user/*` cards: `UserDetailBasic`, `UserDetailTraffic`, `UserDetailExpiry`, `UserDetailLimits`, `UserDevices`, `UserNodeAuth`, `UserTrafficStats` (L112-144). Two-column grid via `.two-col` (L116-136).
- `web/src/components/user/UserDevices.vue`: self-contained card; `getUserDevices(userId)` on mount + watch on `userId` (L32-54); `columns` (L24-30); renders `DataTable :bordered="false"` with `#cell-*` slots and `#empty` (L76-102). This is the closest structural mirror for a per-user "visited sites" panel.
- `web/src/components/user/UserTrafficStats.vue`: card with `<select>` range + bucket controls and `TrafficChart`; same load-on-watch pattern (L33-59). No shared tab component — range toggles are hand-rolled selects here.
- `web/src/pages/ServerDetailPage.vue`: stacked `.card` sections — 基础信息 (L265-295), Agent + install tabs (L297-418), 系统指标 (L420-442), and a 节点 `DataTable` section with `card-head` + actions (L444-488). Its `install-tabs` (L369-390) is a local tab pattern; `SegmentedControl` is the shared segmented control elsewhere.
- `web/src/pages/DashboardPage.vue` uses `StatCard` (L145-177), `SegmentedControl` (L203), and `DataTable` with `row-extra` drill-down (L211-315).
- `web/src/pages/SettingsPage.vue`: side-nav sections (`sections` L13-18, `activeSection` L20), `form-grid` fields, single save row (L257-275); adding a retention knob for visits follows this pattern.
- Labels/formatting: no i18n framework — Chinese strings are inline in templates; shared label maps in `web/src/utils/labels.ts` (`protocolLabel`, `userStatusInfo`, …), formatting in `web/src/utils/format.ts` (`formatBytes`, `formatDateTime`, `formatRelative`, `formatDuration`), name maps in `web/src/utils/names.ts`.

---

## 7. Contract & docs

`docs/api-contract.md`
- Conventions (L5-12): JSON bodies, **RFC3339 UTC timestamps**, non-negative byte counters, unknown fields currently "ignored", "unknown enum values are rejected with `invalid_request`".
- Auth scopes table (L14-23); error envelope + HTTP/code matrix (L25-42); pagination `{items,total,page,page_size}` (L44-50); enums (L52-59); eligibility rule (L59).
- Agent API (L433-554): register (L439-453), heartbeat incl. `last_apply_error` (L455-469), config (L471-503), **traffic** (L505-532 — batch_seq monotonic, `(agent_id,batch_seq)` idempotency key, ≤1000 records → `413`, non-negative counters → `422`, `recorded_at` ±24h window, per-node `rate` scaling, single tx), **devices** (L534-554 — whole-server snapshot, upsert + prune older `last_seen_at`, split across requests with same `recorded_at`, scope validation → `422`).
- Ownership rules (L558-566): server_id only from credential; every user/node validated; neither auth boundary accepts the other; secrets one-time; revision monotonic; heartbeat is the only liveness source; deletion semantics (traffic survives retention).

`internal/singbox/singbox.go` (Panel-side server config renderer)
- `Render(appKey, nodes)` (L83-98) returns:
  ```go
  map[string]any{
    "log":       map[string]any{"level":"info","timestamp":true},
    "inbounds":  inbounds,
    "outbounds": []map[string]any{{"type":"direct","tag":"direct"}},
    "route":     map[string]any{"final": "direct"},
  }
  ```
  **No `route.rules`, no sniff action, no `sniff_override_destination`, no inbound `sniff` field.** `ContractVersion = "singbox-render-v1"` (L13).
- Per-protocol inbounds: SS (L119-152), VLESS Reality (L154-197), Hysteria2 (L199-241), AnyTLS (L243-268). Each user name is `"u-"+id`, which is what the tracker keys on.
- Consequence: `metadata.Domain`/`metadata.Protocol` (sniffed) are **never** populated today. `metadata.Destination` does carry the client-requested host for SS/VLESS (address header) but the renderer/route does not enable domain sniffing. Enabling sniffing requires a renderer change (`route.rules: [{action:"sniff"}]`) plus a renderer version/contract bump.

`internal/subscription/render.go` is the **client** subscription generator (share links + Clash Meta YAML). It contains no sniff/sing-box server config; `mix-port` etc. are client-side. `ValidateTemplate` allow-list (L162-192) is unrelated to agent telemetry.

---

## 8. Prior art: the removed `connection_logs` (do not miss)

The repo previously implemented a historical connection log and removed it during the xboard-parity refactor. The archived model doc is at `.trellis/tasks/09-22-xboard-parity/research/current-model.md`:
- Old tables `connection_logs` (`0001:99-116`) and `connection_log_batches` (`0001:145-150` + `0003`) were dropped; migration files `0002`–`0006` were squashed into a single `0001_init.sql` (`.trellis/tasks/09-22-xboard-parity/implement.md`, session 2).
- Old columns (archived doc §1.2): `connection_logs(id PK AUTOINCREMENT, user_id, node_id, server_id [no FKs], ip, protocol, upload_bytes, download_bytes, connected_at, closed_at NULL, status CHECK('active','closed'), created_at)`, indexes on user/node/server + `created_at`.
- Archived PRD noted the old logs deliberately **did not store destination address** (`.trellis/tasks/archive/2026-09/09-20-multi-node-panel/prd.md:39`), which is exactly the gap the new task fills.
- Old ingestion was `POST /api/agent/connection-logs` append-style (`agent_telemetry.go` old `handleAgentConnectionLogs`), replaced by the current `devices` snapshot model.
- Spec `error-handling.md:65` and `quality-guidelines.md:11` still reference `connection-logs` / `destination_host` and a "strict JSON decode" test — artifacts of the removed feature; `decodeJSONStrict` is **not present** in the current code.

---

## 9. Specs — concrete conventions imposed on this feature

### `spec/backend/database-guidelines.md`
- Embedded sequential migration runner, `schema_migrations`, per-file tx, apply-twice no-op (tested).
- Schema conventions: INTEGER unix-second timestamps; byte counters INTEGER; `traffic_records` intentionally has **no FKs** (history survives parent deletion); `online_devices` cascades.
- Idempotency markers: `traffic_batches`/`device_batches` PK `(agent_id, seq)`; **duplicate batch must return prior result without double counting, written in the SAME tx as the totals** (L28, L31-33, L46-52).
- `server_revisions` is the only config-change signal.
- Retention & caps in `internal/janitor`; bounded 500-row batches; deleting devices recomputes `online_count` in the same tx (L58).
- Table-rebuild caveat inside tx migrations (`PRAGMA foreign_keys=off` no-op) (L71-77).
- Wrong vs correct: never write telemetry totals outside the batch-marker tx.

### `spec/backend/error-handling.md`
- Envelope `{"error":{"code","message"}}`; codes incl. `payload_too_large`, `validation`. Body >1 MB → 413; field failure → 422; ownership violation → 404/422 never 500.
- Agent `server_id` always derived from credential; body `server_id` never trusted.
- Tests: strict JSON decode so undeclared `destination_host` cannot be stored → 422.

### `spec/backend/quality-guidelines.md`
- Go 1.25, stdlib-first, no web framework/ORM; no code comments unless necessary.
- **Strict JSON decode for inbound agent payloads (`decodeJSONStrict`)** so undeclared fields (destination host/port) cannot enter (L11). (Not implemented today — see Gaps.)
- Forbidden: storing/logging plaintext secrets or rendered configs; trusting body ownership; telemetry totals outside the batch-marker tx; marking a server online outside heartbeat; `any`/raw fetch/hand-cast JSON in frontend.
- Validation commands (L29-40): `go build ./... && go vet ./... && gofmt -l .`; `go test -count=1 ./...`; `go test -race -count=1 ./...`; `go test -count=1 -tags integration,with_quic,with_utls ./...`; `go build -tags with_quic,with_utls ./...`; `go build -tags embed_ui ./...`; `cd web && npm run typecheck && npm run build && npm run lint`; `make build`.

### `spec/backend/directory-structure.md`
- Handlers own decode/validate; repos own SQL; never SQL in handlers.
- New agent payloads: extend `docs/api-contract.md` FIRST, then panel handler + `agentclient` + `web/src/api/types.ts` together (contract frozen per task; deviations reported).
- New tables: append `NNNN_name.sql` under `internal/db/migrations/`.
- Naming: files lowercase_with_underscores; DB timestamps unix seconds; API JSON snake_case.
- Adding an agent endpoint: `internal/web/agent.go` + `internal/agentclient/client.go`.

### `spec/frontend/directory-structure.md`
- `api/` is the SINGLE owner of backend types + decoding; pages never hand-cast JSON.
- `DataTable` `rowKey` required; there is **no `#toolbar` slot** (use `.table-toolbar` above the table); state feedback trio mandatory: loading skeleton, `EmptyState`, `ErrorBanner`.
- Semantic design tokens only; dark-mode override for every new token; new side-by-side card grids must reset `.card + .card` margin.
- Scripts `dev | build | typecheck | lint` must pass.

### `spec/frontend/component-guidelines.md`
- `<script setup lang="ts">`, scoped styles, zh-CN text; lucide icons only.
- `PageHeader` for every page; `StatCard`/`SegmentedControl` for stats/range toggles; no hand-rolled tables; `ConfirmDialog` instead of `window.confirm`; overlays must support outside-click/Esc/keyboard.

### `spec/frontend/type-safety.md`
- All backend payload types in `web/src/api/types.ts`; update together with Go DTOs.
- Runtime validation only at `http.ts` boundary; `any` and unguarded `as` casts forbidden; no `fetch` outside `http.ts`.

---

## 10. Reusable patterns to mirror

1. **In-process collection surface**: add a new field to `connEntry` (e.g. destination host/port/network/connectedAt) and populate it in `ConnTracker.entry(metadata)` from `metadata.Destination.Fqdn`/`.AddrString()`, `metadata.Destination.Port`, `metadata.Network`, and (if sniff enabled) `metadata.Domain`/`metadata.Protocol`. `RoutedConnection`/`RoutedPacketConnection` are the observation points.
2. **Snapshot → delta → batch**: extend `kernelsingbox.Snapshot` with a new slice type; in `Loop.telemetryPass`, mirror `flushDevices`/`flushTraffic` with a new `siteInflight` + `SiteBatchSeq`. Batching helper pattern: sort, chunk by `maxBatchRecords` (1000), `BatchSeq = base + idx + 1`, shared RFC3339 timestamp.
3. **Retry/requeue semantics**: retryable → freeze inflight & return (same payload replayed); non-retryable → advance seq + drop/requeue. Persist seq via `agentstate.State` atomic save.
4. **Agent client method**: add `Sites`/`Visits(ctx, batch)` to `agentclient` alongside `Traffic`/`Devices`, new `POST /api/agent/<name>` route under `requireAgent`.
5. **Panel idempotent ingestion**: new `internal/repo` `Ingest*Batch` doing dedupe `SELECT` on a new `*_batches(agent_id, seq)` marker, inserts, marker in ONE `Tx`; `handleAgent*` validates scope via `agentScopes` (nodeSet + pairSet), record count ≤1000, timestamp ±24h, then on `ErrConflict` returns the prior count.
6. **Migration**: append `0002_<name>.sql` (no FKs for high-volume append history table; indexes on `user_id`/`node_id`/`server_id`/`created_at`; batch-marker table with PK `(agent_id, seq)`); update `db_test.go` expected migration count.
7. **Admin read endpoints**: `GET /api/users/:id/<res>`, `GET /api/servers/:id/<res>`, standalone `GET /api/<res>` — use `pathID` + `GetUser`/`GetServer` 404 guard, `parsePageQuery` + `writePage`, `parseTimeParam`, DTO in `dto.go` with snake_case, `httpx.WriteJSON`.
8. **Retention**: add janitor drain calls (`Delete*Batch` in `repo/cleanup.go`) + a new settings key following `retention.aggregate_days` format (INTEGER string), surfaced in `settingsDTO`, `settingsUpdateRequest`, `settingsRanges`, `SettingsPage.vue`.
9. **Frontend**: add types to `api/types.ts`; add endpoint functions to a new/existing `api/*.ts`; render with `DataTable` (`rowKey` required, `bordered=false` inside a card) + `TablePaginator`; new per-user card under `components/user/`; new per-server card/section in `ServerDetailPage.vue`; standalone page via `router/index.ts` + `AppLayout.vue` `navItems`.
10. **Contract-first**: update `docs/api-contract.md` (agent endpoint + admin endpoints) before/with code; mirror agent DTOs in `agentclient`, Panel handler-local structs, and TS types.
11. **Tests to mirror**: `internal/web/agent_test.go` `TestAgentTrafficIngestion` (L496-630) / `TestAgentDevicesSnapshot` (L632+): duplicate seq no double count, cross-agent same seq accepted, foreign node / unauthorized user / negative counter / stale & malformed timestamp / bad seq → 422, >1000 → 413, rejected batches don't mutate. `internal/e2e/agent_flow_test.go` uses a `stubKernel` implementing `Kernel` (L18-46) and calls `loop.TelemetryOnce` (L134) — the natural place to exercise a new snapshot field end-to-end. `internal/e2e/integration_embed_test.go` starts the real embedded sing-box.

---

## 11. Gaps / questions

1. **`metadata.Domain` is never populated today.** `internal/singbox/singbox.go` `Render` emits no `route.rules` sniff action and no inbound `sniff` field (L92-97). Without a renderer change, a visited-sites feature can only use `metadata.Destination` (Fqdn or IP from the inbound address header), which will be empty/IP for some clients. Enabling sniff raises `renderer_version`/contract questions.
2. **No per-connection output from the tracker.** `Snapshot()` returns only aggregates (`Traffic` map, `Devices` slice). A connection log needs either (a) a new in-tracker append buffer written in `entry`/`release`, or (b) a new per-connection snapshot structure. `connEntry` currently has `connectedAt` but no `closedAt`; `release` only deletes.
3. **No destination DTO/type anywhere** — must be introduced in `types.go`, `agentclient`, `web/agent_telemetry.go`, and `web/src/api/types.ts`.
4. **`decodeJSON` is not strict** (`respond.go:67-77`); `decodeJSONStrict` referenced by specs does not exist. `docs/api-contract.md:11` says unknown fields are ignored, while `quality-guidelines.md:11` / `error-handling.md:65` demand strict decode for agent payloads so `destination_host` can't be smuggled in. Contradiction to resolve before implementing.
5. **Migration count test**: `internal/db/db_test.go:36` hard-asserts exactly one applied migration. Any new `NNNN_*.sql` requires updating that test (and potentially the table list at L40-45). There is no ALTER-based migration example in the tree.
6. **`repo.ErrConflict` is produced via `IsUniqueViolation` string match** (`db.go:36-41`) and is reused for batch replay; a new batch table must use the same PK `(agent_id, seq)` and rely on `Ingest*` returning/duplicating correctly.
7. **Retention/settings are opt-in**: `collection.Traffic` exists for traffic only; `Collection` has no toggle for devices/logs. A visited-sites feature likely needs its own `collection.*` toggle + server-provided interval (register/heartbeat responses carry `traffic_interval_seconds`; no site interval exists).
8. **Request size cap is 1 MB** (`respond.go:15`) and the record cap is 1000 (`agent_telemetry.go:15`); destination strings will add per-record size — validate against the 1 MB body cap for a full 1000-record batch.
9. **Privacy/redaction fields are absent** from the current model; the task PRD explicitly lists privacy & retention as open (`.trellis/tasks/09-21-site-visit-stats/prd.md:11`). No existing redaction/hashing utility was found in `internal/`.
10. **`agentScopes` rejects any `(user,node)` not present in `user_nodes`**; a visited-sites report must be validated the same way or it will 422 on transiently-unauthorized users.
11. **`README.md` / `demand.md`** may describe telemetry but were not exhaustively read in this pass; the binding source is `docs/api-contract.md` + specs.
12. **No active Trellis task** was set (`task.py current` → none); output written to the explicitly provided task dir.
