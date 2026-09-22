# Research: current project model (pre-Xboard alignment)

- **Query**: Map the CURRENT project at `/root/vps-node`: DB model, repo/domain layer, admin API + UI, subscription/link generation, node/agent architecture, protocol/node settings model. Then note gaps vs Xboard.
- **Scope**: internal (read-only)
- **Date**: 2026-09-22
- **Task**: `.trellis/tasks/09-22-xboard-parity`
- **Unit of model**: Panel (Go, SQLite) + Agent (Go, per Server) + external `sing-box` process (data plane). One user has one global UUID; `user_nodes` is N:N.

> Note: no active task was set via `task.py current`; the task directory was supplied explicitly by the caller. All findings are code-backed with file:line.

---

## 1. Database model

Source of truth: `internal/db/migrations/`. 6 migrations, applied in order by `internal/db/migrations.go:16-57` (versioned via `schema_migrations`, each file executed in one tx). No `plans`, no `subscriptions` plan table, no `server_configs` blob — auth data lives on `users` + `nodes` + `user_nodes`.

### 1.1 Table inventory

| Table | Defined in | Purpose |
|---|---|---|
| `admins` | `0001_init.sql:1-7` | panel admin login |
| `users` | `0001_init.sql:9-20`; rebuilt `0004_users_username.sql:1-17` | end users (UUID, token, quota, expiry) |
| `servers` | `0001_init.sql:25-40` (+ index `42-44`) | physical VPS / agent host |
| `agents` | `0001_init.sql:46-54` | one agent identity per server |
| `nodes` | `0001_init.sql:56-69`; rebuilt `0006_nodes_anytls_protocol.sql:1-14` | one sing-box inbound per row |
| `user_nodes` | `0001_init.sql:73-80`; rebuilt `0004:75-87`, `0006:55-67` | N:N user↔node authorization |
| `sessions` | `0001_init.sql:82-97`; rebuilt `0004:52-73`, `0006:69-86` | current live connections snapshot |
| `connection_logs` | `0001_init.sql:99-116` | closed/historical connection records |
| `traffic_records` | `0001_init.sql:118-130` | per-user/per-node traffic deltas |
| `server_revisions` | `0001_init.sql:132-136` | monotonic config revision per server |
| `traffic_batches` | `0001_init.sql:138-143`; col added `0003:1` | agent traffic-batch dedupe |
| `connection_log_batches` | `0001_init.sql:145-150`; col added `0003:2` | agent log-batch dedupe |
| `settings` | `0001_init.sql:152-156` | key/value panel settings incl. `app_key` |
| `admin_sessions` | `0002_admin_sessions.sql:1-9` | admin web sessions |
| `user_subscriptions` | `0005_user_subscriptions.sql:1-9` | per-user subscription token |

### 1.2 Columns, keys, enums

**`admins`** (`0001:1-7`)
- `id INTEGER PK`; `username TEXT NOT NULL UNIQUE`; `password_hash TEXT NOT NULL`; `created_at`,`updated_at INTEGER NOT NULL`.

**`users`** (final shape after `0004`)
- `id INTEGER PK`; `uuid TEXT NOT NULL UNIQUE`; `username TEXT NOT NULL UNIQUE` (added `0004:4`; backfilled `user-<uuid8>` at `0004:16`); `token_hash TEXT NOT NULL UNIQUE`; `status TEXT NOT NULL DEFAULT 'active' CHECK IN ('active','disabled','expired')`; `quota_bytes INTEGER NOT NULL DEFAULT 0`; `used_bytes INTEGER NOT NULL DEFAULT 0`; `started_at INTEGER NULL`; `expires_at INTEGER NULL`; `created_at`,`updated_at`.
- Indexes `idx_users_status`, `idx_users_expires_at` (`0004:89-90`).
- Quota semantics: `0` = unlimited (see eligibility §2).
- No per-user `plan_id`, no `group_id`, no `level`.

**`servers`** (`0001:25-44`)
- `id PK`; `name TEXT NOT NULL UNIQUE`; `address TEXT NOT NULL`; `status TEXT NOT NULL DEFAULT 'active' CHECK IN ('active','disabled','offline')`; `cpu_percent REAL DEFAULT 0`; `memory_percent REAL`; `disk_percent REAL`; `uptime_seconds INTEGER`; `agent_version TEXT DEFAULT ''`; `last_seen_at INTEGER NULL`; `register_token_hash TEXT NULL`; `register_token_expires_at INTEGER NULL`; timestamps.
- Partial unique index `idx_servers_register_token` on `register_token_hash WHERE NOT NULL` (`0001:42-44`).
- `offline` is derived at read time (`internal/web/web.go:299-307`), not stored by agents.

**`agents`** (`0001:46-54`)
- `id PK`; `server_id INTEGER NOT NULL UNIQUE REFERENCES servers(id) ON DELETE CASCADE` (1:1); `token_hash TEXT NOT NULL UNIQUE`; `version TEXT DEFAULT ''`; `last_seen_at INTEGER NULL`; timestamps.

**`nodes`** (final shape after `0006`)
- `id PK`; `server_id INTEGER NOT NULL REFERENCES servers(id) ON DELETE CASCADE`; `name TEXT NOT NULL`; `protocol TEXT NOT NULL CHECK IN ('shadowsocks','vless','hysteria2','anytls')`; `port INTEGER NOT NULL CHECK 1..65535`; `settings TEXT NOT NULL DEFAULT '{}'`; `secret_enc BLOB NULL`; `status TEXT NOT NULL DEFAULT 'active' CHECK IN ('active','disabled')`; timestamps.
- `UNIQUE (server_id, name)`, `UNIQUE (server_id, port)`; index `idx_nodes_server` (`0006:53`).
- One row = exactly one sing-box inbound; no `parent_id`, no node group, no per-node `rate_limit`, no `server_name`/TLS columns (those live inside `settings`/`secret_enc` JSON).

**`user_nodes`** (`0004:75-80`, `0006:55-60`)
- `user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE`; `node_id INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE`; `created_at`; `PRIMARY KEY (user_id,node_id)`; index `idx_user_nodes_node` (`0006:67`).

**`sessions`** (`0006:69-86`)
- `id PK`; `user_id REFERENCES users ON DELETE CASCADE`; `node_id REFERENCES nodes ON DELETE CASCADE`; `server_id REFERENCES servers ON DELETE CASCADE`; `ip TEXT`; `upload_bytes`,`download_bytes INTEGER DEFAULT 0`; `connected_at`,`last_seen_at`; `UNIQUE (server_id,user_id,node_id,ip,connected_at)`.
- Indexes on `(user_id,last_seen_at)`, `(node_id,last_seen_at)`, `(server_id,last_seen_at)` (`0006:88-90`).

**`connection_logs`** (`0001:99-116`)
- `id PK AUTOINCREMENT`; `user_id,node_id,server_id INTEGER NOT NULL` (**no FK constraints**); `ip TEXT`; `protocol TEXT DEFAULT ''`; `upload_bytes`,`download_bytes`; `connected_at`; `closed_at INTEGER NULL`; `status TEXT DEFAULT 'active' CHECK IN ('active','closed')`; `created_at`; indexes on user/node/server + `created_at`.

**`traffic_records`** (`0001:118-130`)
- `id PK AUTOINCREMENT`; `user_id,node_id,server_id INTEGER NOT NULL` (**no FK constraints**); `upload_bytes`,`download_bytes`; `created_at`; indexes on user/node/server + `created_at`.

**`server_revisions`** (`0001:132-136`)
- `server_id INTEGER PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE`; `revision INTEGER NOT NULL`; `updated_at`. Upsert-bumped by `bumpRevisionExec` (`internal/repo/mutations.go:22-28`).

**`traffic_batches` / `connection_log_batches`** (`0001:138-150` + `0003`)
- `agent_id REFERENCES agents(id) ON DELETE CASCADE`; `seq INTEGER`; `received_at`; `records`/`logs INTEGER DEFAULT 0`; `PRIMARY KEY (agent_id,seq)` → idempotent batch ingestion.

**`settings`** (`0001:152-156`)
- `key TEXT PRIMARY KEY`; `value TEXT NOT NULL`; `updated_at`. Stores `app_key` (`internal/config/config.go:44`), `retention.*`, `collection.connection_logs`, `session.freshness_seconds`, `server.offline_after_seconds`, `subscribe_urls`, `subscribe_path`, `subscribe_name`, `clash_meta_template` (key constants `internal/web/web.go:19-29`).

**`admin_sessions`** (`0002:1-9`)
- `token_hash TEXT PK`; `admin_id REFERENCES admins(id) ON DELETE CASCADE`; `created_at`,`last_seen_at`,`expires_at`.

**`user_subscriptions`** (`0005:1-9`)
- `user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE` (1:1); `token_hash TEXT NOT NULL UNIQUE`; `token_enc BLOB NOT NULL`; timestamps; unique index on `token_hash`.

### 1.3 Enum/status summary

| Entity | Field | Values | Source |
|---|---|---|---|
| users | status | `active`,`disabled`,`expired` | `0001:13`, `0004:6` |
| servers | status | `active`,`disabled`,`offline` (offline derived) | `0001:29`, `web.go:299` |
| nodes | status | `active`,`disabled` | `0001:64` |
| nodes | protocol | `shadowsocks`,`vless`,`hysteria2`,`anytls` | `0006:5` |
| connection_logs | status | `active`,`closed` | `0001:110` |

### 1.4 Foreign-key / migration caveats

- `connection_logs` and `traffic_records` intentionally have **no FKs** (high-volume append tables).
- Because SQLite can't alter CHECK constraints, protocol/username changes require full table rebuilds. `0004` and `0006` rebuild `nodes`/`users` and back up dependent `sessions`/`user_nodes` (drop in FK order, recreate, restore) — see `0004:19-90`, `0006:20-90` and the contract note in `.trellis/spec/backend/node-protocol-settings.md:55`.
- `internal/db/db.go` presumably runs `PRAGMA foreign_keys`/`foreign_key_check`; `0001:1` uses `CREATE TABLE` without `IF NOT EXISTS` (only first run).

---

## 2. Domain / repo layer (`internal/repo/`)

`Repo` = thin SQL wrapper over `*sql.DB` (`internal/repo/repo.go:17-23`). `ErrNotFound`/`ErrConflict` mapping at `repo.go:12-15,37-46`. No JSON tags on repo structs — DTOs live in `internal/web/dto.go`.

### 2.1 Key types

| Type | File:line | Fields (Go) |
|---|---|---|
| `Server` | `servers.go:9-23` | `ID, Name, Address, Status, CPUPercent, MemoryPercent, DiskPercent, UptimeSeconds, AgentVersion, LastSeenAt *time.Time, RegisterTokenExpiresAt *time.Time, CreatedAt, UpdatedAt` |
| `Node` | `nodes.go:10-22` | `ID, ServerID, Name, Protocol, Port int, Settings string, SecretEnc []byte, Status, ServerName string, CreatedAt, UpdatedAt` |
| `NodeFilter` | `nodes.go:24-31` | `ServerID, Protocol, Status, Query, Page, PageSize` |
| `NewNode` | `nodes.go:33-41` | `ServerID, Name, Protocol, Port, Settings, SecretEnc, Status` |
| `User` | `users.go:10-22` | `ID, UUID, Username, TokenHash, Status, QuotaBytes, UsedBytes, StartedAt *time.Time, ExpiresAt *time.Time, CreatedAt, UpdatedAt` |
| `NewUser` | `users.go:24-32` | `UUID, Username, TokenHash, Status, QuotaBytes, StartedAt, ExpiresAt` |
| `UserFilter` | `users.go:34-41` | `Query, TokenHash, Status, Expiry, Page, PageSize` |
| `UserPatch` | `mutations.go:9-20` | `Set*/…` tri-state patch (`SetUsername, Username, SetStatus, Status, SetQuota, QuotaBytes, SetStartedAt, StartedAt, SetExpiresAt, ExpiresAt`) |
| `Agent` | `agents.go:9-17` | `ID, ServerID, TokenHash, Version, LastSeenAt, CreatedAt, UpdatedAt` |
| `Session` / `NewSession` | `sessions.go:9-30` | `ID, UserID, NodeID, ServerID, IP, UploadBytes, DownloadBytes, ConnectedAt, LastSeenAt` |
| `ConnectionLog` / `NewConnectionLog` | `connection_logs.go:10-36` | `ID, UserID, NodeID, ServerID, IP, Protocol, Upload/DownloadBytes, ConnectedAt, ClosedAt *time.Time, Status, CreatedAt` |
| `TrafficRecord` / `NewTrafficRecord` | `traffic.go:10-27` | `ID, UserID, NodeID, ServerID, Upload/DownloadBytes, CreatedAt` |
| `UserSubscription` | `subscriptions.go:9-15` | `UserID, TokenHash, TokenEnc []byte, CreatedAt, UpdatedAt` |
| `SubscriptionNode` | `subscriptions.go:17-20` | embeds `Node` + `ServerAddress string` |

Constants: `ServerStatus*` (`servers.go:25-29`), `NodeStatus*`/`Protocol*` (`nodes.go:43-51`), `UserStatus*` (`users.go:43-47`).

### 2.2 Key query / mutation functions

**Servers** (`servers.go`): `CreateServer:40`, `GetServer:54`, `ListServers:59`, `ListServersPage:67`, `GetServerByRegisterTokenHash:97`, `UpdateServer:102`, `SetServerStatus:109`, `SetServerRegisterTokenHash:114`, `UpdateServerMetrics:121`, `DeleteServer:129`.

**Nodes** (`nodes.go`): `CreateNode:76`, `GetNode:80`, `ListNodes:85`, `ListNodesPage:93`, `ListNodesByServer:139`, `ListNodesByIDs:147`, `UpdateNodeSpec:163`, `SetNodeStatus:170`, `DeleteNode:175`. All list queries share `nodeSelectWithServer` (`nodes.go:55-56`).

**Users** (`users.go`): `CreateUser:77`, `GetUser:81`, `GetUserByUUID:85`, `GetUserByTokenHash:90`, `ListUsers:95` (filters query/status/expiry; query matches uuid OR token_hash OR username at `:98-105`), `ListUsersAll:145`, `CountUsers:163`, `SetUserStatus:196`, `SetUserQuota:201`, `SetUserExpiry:206`, `ResetUserTokenHash:213`, `ResetUserTraffic:218`, `AddUserUsedBytes:223`.

**Eligibility** — `ListEligibleUsersByServer` (`users.go:169-194`):
```sql
FROM users u JOIN user_nodes un ON un.user_id=u.id JOIN nodes n ON n.id=un.node_id
WHERE n.server_id=? AND u.status='active'
  AND (u.expires_at IS NULL OR u.expires_at > ?)
  AND (u.quota_bytes = 0 OR u.used_bytes < u.quota_bytes)
GROUP BY u.id
```
So "eligible" = authorized on a node of that server AND active AND not expired AND under quota. Verified by `eligibility_test.go:11-85` (excludes expired/disabled/quota-reached/quota-exceeded/unauthorized/other-server).

**User-node assignment** (`user_nodes.go`): `SetUserNodes:9` (delete+insert in tx), `AuthorizeUserNode:27` (upsert-do-nothing), `RevokeUserNode:35`, `ListNodeIDsByUser:40`, `ListUserIDsByNode:58`, `ListNodeIDsByServerWithUsers:76`, `CountNodesByUser:97`, `CountUsersByNode:103`.

**Revision-bumping mutations** (`mutations.go`): `bumpRevisionExec:22`; `CreateUserWithNodes:84`; `UpdateUserAndBump:117` (bumps only when status/quota/start/expiry change, `:159-173`); `DeleteUserAndBump:183`; `ResetUserTrafficAndBump:205`; `SetUserNodesAndBump:234` (bumps union of old+new servers, `:258`); `CreateNodeAndBump:271`; `UpdateNodeAndBump:287`; `DeleteNodeAndBump:314`; `UpdateServerAndBump:327`; `DeleteServerCascade:351`.

**Subscriptions** (`subscriptions.go`): `CreateUserWithNodesAndSubscription:28` (user + subscription + user_nodes + revision in one tx); `GetUserSubscription:60`; `CreateUserSubscription:64`; `RotateUserSubscription:73`; `ResolveSubscription:84` (join users↔user_subscriptions on `token_hash`); `ListSubscriptionNodes:90` (only `node.status='active' AND server.status='active'`).

**Telemetry ingestion** (`telemetry.go`): `ListUserNodePairsByServer:14`; `IngestTrafficBatch:35` (dedupe on `(agent_id,seq)`; inserts records + increments `users.used_bytes` via `userDeltas`); `IngestLogBatch:89`; batch-count lookups `:76,125`.

**Agents** (`agents.go`, `agent_ops.go`): `CreateAgent:19`; `GetAgentByTokenHash:36`; `GetAgentByServerID:42`; `RotateAgentTokenHash:48`; `UpdateAgentHeartbeat:53`; `RegisterAgent:10` (upsert per server + clears register token); `RecordHeartbeat:50` (updates agent + server metrics/revision in tx).

**Counts/stats**: `counts.go` grouped counts for list pages (`CountNodesByUserIDs:21`, `CountFreshSessionsByUserIDs:29`, `CountNodesByServerIDs:38`, `CountFreshUsersByServerIDs:46`, `CountUsersByNodeIDs:55`, `CountFreshSessionsByNodeIDs:63`); `stats.go` dashboard aggregates + `SumTrafficByUser:46`, `SumTrafficByUserNode:77`; `cleanup.go` retention/cap deletes.

**Revisions**: `revisions.go` `GetServerRevision:9` (missing row ⇒ 0), `BumpServerRevision:21`.

---

## 3. Admin API + UI (`internal/web/`)

### 3.1 Route table (`web.go`)

- Public: `GET /healthz` (`:94`), `GET /` public subscription dispatcher (`:95,101-117`).
- Agent (`registerAgentRoutes`, `:126-133`): `POST /api/agent/register|heartbeat|traffic|sessions|connection-logs`, `GET /api/agent/config`.
- Admin (`registerAdminRoutes`, `:135-176`), all `requireAdmin` except login:
  - Auth: `POST /api/admin/login|logout`, `GET /api/admin/me` (`:136-138`).
  - Users: list/create/get/update/delete + `reset-token`, `reset-traffic`, `expire-now`, `GET|PUT /{id}/nodes`, `GET /{id}/sessions|connection-logs|traffic`, `GET|POST /{id}/subscription`, `POST /{id}/subscription/rotate` (`:140-155`).
  - Servers: list/create/get/update/delete + `POST /{id}/register-token`, `POST /{id}/agent-token` (`:157-163`).
  - Nodes: list/create/get/update/delete + `POST /api/nodes/reality-keypair` (`:165-170`).
  - Dashboard/settings: `GET /api/dashboard`, `GET /api/dashboard/user-traffic`, `GET|PUT /api/settings` (`:172-175`).
- Admin auth: cookie session via `adminauth.Sessions.Resolve` (`web.go:208-223`); agent auth Bearer token hashed and matched to `agents.token_hash` (`agent.go:29-49`).
- Bootstrap admin created from config only when `admins` empty (`web.go:178-206`).

### 3.2 Handler highlights & entity form fields

**Servers** (`servers.go`):
- Create accepts `{name, address}` (`:55-68`); update accepts `{name?, address?, status?}` with status restricted to `active|disabled` (`:12-15,159-182`). Validation name 1-128 / address 1-255 (`:81-89`).
- `handleServerGet` returns `serverDetailDTO` incl. `revision`, `agent`, `nodes` (`:91-146`).
- `register-token` returns a one-time token + 24h expiry (`:221-247`); `agent-token` rotates an existing agent token (`:249-277`).
- Effective status computed with `offlineAfter` setting (`web.go:299-307`).

**Nodes** (`nodes.go`):
- Create `{server_id, name, protocol, port, settings}` (`createNodeRequest:76-82`); update `{name?, port?, settings?, status?}` (`updateNodeRequest:204-209`). Protocol is immutable on update (form disables it).
- `validProtocols`/`shadowsocksMethods` allowlists (`:25-36`); `validateNodeSpec:156`.
- `POST /api/nodes/reality-keypair` generates X25519 private/public + 8-byte short_id, does **not** persist (`:138-154`).
- `buildNodeSettings:292-459` is the single merge/validate/encrypt entry point; per-protocol field handling table (`:324-438`):
  - shadowsocks: plain `method` (3 supported ciphers), secret `password` (optional).
  - vless: secret `private_key` (X25519), plain `short_id`, plain `server_names[]`.
  - hysteria2: secret `password`, `server_name` (stored plain though conceptually TLS), secret `certificate`/`private_key` PEM, plain `up_mbps`/`down_mbps`/`obfs_password`/`hop_ports`.
  - anytls: secret `password`, plain `server_name`, secret `certificate`/`private_key`.
- `validateProtocolSettings:461` + `validateTLSMaterial:517` (keypair match, validity window, `VerifyHostname`).

**Users** (`users.go`):
- Create `{username, quota_bytes?, started_at?, expires_at?, node_ids?}` (`createUserRequest:140-146`); requires username matching `^[A-Za-z0-9_.-]{1,64}$` (`:24`); generates v4 UUID + user token + subscription credential (`:190-214`). Response adds one-time `token` + `subscription_url` (`userCreatedDTO:148-152`).
- Update `{status?, username?, quota_bytes?, started_at?, expires_at?}` (`updateUserRequest:264-270`); `quota_bytes` may be `null`→0 (unlimited); expiry in the past auto-sets status `expired` (`repo/mutations.go:129-131`).
- User list filters: `query` (uuid/token_hash/username), `status`, `expiry=valid|expired` (`:80-114`).

**User↔node** (`user_nodes.go`): `GET` returns `{node_ids, nodes}`; `PUT` accepts `{node_ids:[...]}` and rejects unknown/inactive nodes (`validateNodeIDs` `users.go:422-444`).

**Settings** (`settings.go`): keys listed in `web.go:19-29`; DTO `settingsDTO` in `dto.go:255-265` (retention days, collection toggle, session freshness, server offline threshold, subscribe URLs/path/name, clash template). `settingsFromMap` supplies defaults (`web.go:238-250`).

### 3.3 DTOs (`dto.go`)

`userDTO:21-33` (`id, uuid, username, status, quota_bytes, used_bytes, started_at, expires_at, node_count, session_count, created_at`), `userDetailDTO:35-39` (+`remaining_bytes, used_percent`), `nodeRefDTO:72`, `nodeDTO:77-86` (`id, server_id, name, protocol, port, status, server{id,name}, created_at` — **settings never returned**), `nodeDetailDTO:88-93`, `agentInfoDTO:108-113`, `serverDTO:115-129`, `serverDetailDTO:131-136`, `sessionDTO:156-176`, `connectionLogDTO:178-206`, traffic/dashboard/settings DTOs `:208-265`.

### 3.4 Vue UI (`web/src/`)

Router (`web/src/router/index.ts:13-71`): `/` dashboard, `/users`, `/users/:id`, `/servers`, `/nodes`, `/servers/:id`, `/settings`, `/login`.

| Page | File | Entity surface |
|---|---|---|
| `ServersPage` | `web/src/pages/ServersPage.vue` | table: name, address, status, agent_version, last_seen_at, node_count, online_users (`:37-46`); actions detail/edit/enable-disable/delete (`:60-70`) |
| `ServerDetailPage` | `web/src/pages/ServerDetailPage.vue` | register/agent token one-time reveal + binary/docker install command (`:131-161,354-412`) |
| `NodesPage` | `web/src/pages/NodesPage.vue` | table: name, protocol, port, server, status, enabled, created_at (`:53-62`); filters server/protocol/status/query (`:32-36`); actions edit/enable-disable/delete (`:116-125`) |
| `UsersPage` | `web/src/pages/UsersPage.vue` | table: id, username, traffic progress, expires_at, node_count, session_count, created_at, status (`:39-49`); filters query/status/expiry (`:30-32`) |

**Forms / components:**
- `ServerFormDialog.vue`: name, address, status toggle (edit only) (`:24-26,83-115`).
- `NodeFormDialog.vue`: name, port, protocol select (disabled on edit), status toggle (edit), and per-protocol fields (`:29-49,111-140`): SS method; VLESS private_key/public_key/short_id/server_names + generate; Hysteria2 up/down/obfs/hop + TLS server_name/certificate/private_key; AnyTLS same TLS fields. Client validations mirror server (`:142-186`).
- `UserCreateDialog.vue`: username, unlimited toggle + quota value/unit (MB/GB/TB), started_at, expires_at, node checklist; after create shows one-time Token + subscription URL (`:29-35,99-105,124-146`).
- User detail widgets: `UserDetailBasic.vue` (username, status, reset token, expire-now, subscription provision/rotate), `UserDetailExpiry.vue` (expires_at / never), `UserDetailTraffic.vue` (quota edit + reset traffic), `UserNodeAuth.vue` (node checklist + save), `UserSessions.vue`, `UserConnectionLogs.vue`, `UserTrafficStats.vue`.
- API client: `web/src/api/types.ts` mirrors all DTOs; `NodeBrief:45-54`, `NodeDetail:61-65`, `User:21-38`, `Server:108-135`, `Settings:174-184`, `Subscription:186`, `CreateUserInput:206-212`, `CreateNodeInput:233-239`.

---

## 4. Subscription / link generation

### 4.1 Credential + URL

- Per-user subscription token generated at user create (`web/users.go:201-214`) or on demand/rotate (`web/subscriptions.go:58-99`). Token = `adminauth.NewURLToken(16)`; stored as `token_hash` + AES-GCM-encrypted `token_enc` (`:101-111`).
- Display URL: `<subscribe_url origin>/<subscribe_path>/<token>`; origin randomly chosen from setting `subscribe_urls` or falls back to request host/scheme (`:113-139`).

### 4.2 Public endpoint

- Dispatcher `handlePublicSubscriptionDispatcher` matches `/{subscribe_path}/{token}` (`web.go:101-117`).
- `handlePublicSubscription` (`web/subscriptions.go:141-224`):
  1. Resolve token hash → user (`repo.ResolveSubscription`).
  2. Eligibility gate (`:148`): status active, not started-future, not expired, not over quota — else 403.
  3. Load nodes via `ListSubscriptionNodes` (active node + active server only).
  4. Decode each node's `settings` JSON and decrypt `secret_enc`; skip hysteria2/anytls nodes lacking `server_name` or cert/key (`:159-179`).
  5. Format selection via `?flag=` or User-Agent sniffing (`clash/mihomo/flclash/nekobox` → `clash-meta`, else `general`) (`:180-192`).
  6. Set `Subscription-Userinfo` header = `upload=0; download=<used>; total=<quota>[; expire=<ts>]` (`:196,226-232`); optional profile title/base64 name (`:197-205`).
  7. General format → base64 of newline-joined share links (`subscription.RenderGeneralLinks`); clash-meta → YAML from template (`subscription.RenderClashFiltered`).

### 4.3 Link rendering (`internal/subscription/render.go`)

- `renderURI:100-154` per protocol:
  - shadowsocks: `ss://base64url(method:password)@host:port#name` where password = HMAC-derived (`singbox.DeriveSSPassword`).
  - vless: `vless://<uuid>@host:port?security=reality&encryption=none&flow=xtls-rprx-vision&sni=<names[0]>&pbk=<derived pubkey>&type=tcp[&sid=]`.
  - hysteria2: `hysteria2://<uuid>@host:port?sni=<server_name>[&obfs=salamander&obfs-password=][&mport=hop_ports]`.
  - anytls: `anytls://<uuid>@host:port?sni=<server_name>`.
- `renderProxy:238-286` emits the equivalent Clash Meta proxy map (reality-opts, obfs, ports, etc.).
- `ValidateTemplate:168` / `assembleClash:221` / `expandGroups:288` enforce a safe template allowlist and expand placeholders `__ALL_PROXIES__`, `__SHADOWSOCKS_PROXIES__`, `__VLESS_PROXIES__`, `__HYSTERIA2_PROXIES__`, `__ANYTLS_PROXIES__` (`:314`). Default template `DefaultClashMetaTemplate:16-38`.
- Node skipped on render error via `onSkip` callback (`:65-98`) so one broken node doesn't break the whole sub.

### 4.4 Credential derivation (`internal/singbox/singbox.go`)

- `ContractVersion = "singbox-render-v1"` (`:16`); `ssCredDomain = "ss-cred-v1"` (`:18`).
- SS user password = `HMAC-SHA256(appKey, "ss-cred-v1:<nodeID>:<userUUID>")[:keyLen]` base64-std (`:44-61`); SS server password = `...:server:<nodeID>` (`:63-65`). Keys derive from `PANEL_APP_KEY`/settings `app_key` (`internal/config/config.go:42-120`).
- VLESS/hysteria2/anytls use the user's global UUID directly (UUID = password for hy2/anytls).

---

## 5. Node / agent architecture (current)

**Topology** (README `:7-22`): Panel (SQLite, single source of truth) → one Agent per Server → one external `sing-box` process hosting all Nodes on that server.

### 5.1 Agent lifecycle (`internal/agentruntime/loop.go`)

- Entry point `cmd/agent/main.go:27-101`: loads config (`agent.yaml`, `AGENT_*` env), loads state, builds `Applier`, `MetricsCollector`, runs `Loop`.
- `EnsureIdentity:118-164`: uses existing token or `register_token` → `POST /api/agent/register`; persists `{AgentID, AgentToken, ServerID, AppliedRevision, TrafficBatchSeq, LogBatchSeq}` to state file (`agentstate.State`).
- Tickers (`:90-115`): heartbeat (default 30s), sync (30s), telemetry (traffic interval, default 60s).
- `heartbeatPass:172-204`: collects CPU/mem/disk/uptime (`agentruntime/metrics_linux.go`) and POSTs heartbeat incl. `last_apply_error`; response carries `server_revision`; a newer revision nudges an immediate sync.
- `syncPass:206-251`: `GET /api/agent/config?version=<applied>`; on `updated`, calls `Applier.Apply`.
- `buildCollector:253-263`: extracts Clash API endpoint/secret from the rendered config (`EndpointFromConfig`) and builds a `ClashClient` + attribution table from the user payload.

### 5.2 Config apply — external binary

- `Applier.Apply` (`agentruntime/applier.go:59-106`): writes temp file to config dir (mode 0600), runs `checker.Check` = `sing-box check -c <file>` (`checker.go:21-46`), atomic `rename` over `singbox.config_path` (default `/etc/sing-box/config.json`, `config.go:32`), then runs the optional `singbox.reload_command` (single binary + fixed args, `applier.go:27-39,108-120`).
- `cmd/singbox-reload/main.go` is the bundled helper: SIGTERM old PID from `/run/singbox/singbox.pid`, start `sing-box run -c <config>`, write new PID (`:49-78`). Docker agent image downloads a pinned sing-box tarball at build time (`deploy/Dockerfile.agent:39-51`), and the all-in-one container bundles agent + sing-box + reload helper (README `:98-103`).
- The agent never embeds sing-box as a library; sing-box is a separate long-running process controlled by file replace + reload.

### 5.3 Config rendering (Panel side)

- `internal/singbox/singbox.go` `Render:87-108` builds a sing-box config with `experimental.clash_api` (random port 20000-39999 avoiding node ports + random hex secret, `NewClashAPI:318-334`), one inbound per active node, a single `direct` outbound, route final direct.
- Inbound renderers: `renderShadowsocks:129-162` (multi-user `users[]` with derived passwords, server password), `renderVLESS:164-203` (Reality, `flow=xtls-rprx-vision`, handshake `server_names[0]:443`, `users[]` UUIDs), `renderHysteria2:205-240` (users password=UUID, inline TLS cert/key, optional up/down mbps + obfs), `renderAnyTLS:242-267` (users password=UUID, inline TLS).
- Panel payload (`web/agent_config.go:94-181`): active nodes + `ListEligibleUsersByServer`; per-user `Nodes[]` with `credential` contract (`agentCredential:210-226`: `ss-cred-v1` / `uuid-v1`); response shape `{status, revision, renderer_version, config.singbox, users}` (`:51-57`).
- Agent ignores users not present in the config → removes them locally (api-contract `docs/api-contract.md:490`).

### 5.4 Traffic/session collection — Clash API polling (known weak point)

- Agent polls `GET <clash_api>/connections` with Bearer secret (`agentruntime/clash.go:62-83`).
- Attribution (`clash.go:169-222`): resolve node by connection metadata `inbound` tag (`<protocol>-<nodeID>`) or `inboundPort`; resolve user by `inboundUser` metadata name (`u-<userID>`) via `usersByName`; require the `(user,node)` pair to exist.
- `Collector.Poll` (`collector.go:82-156`) computes per-connection deltas vs. tracked totals, aggregates to `TrafficDelta`, emits live `SessionSnapshot`s and `ClosedLog`s for connections that disappear.
- `loop.go:279-282` logs `"connections without user attribution are excluded from reports"` — **this is the per-user weakness**: attribution relies entirely on sing-box's Clash API `/connections` metadata carrying `inboundUser` (+ matching inbound tag/port). When sing-box does not expose `inboundUser` for a protocol/version, or tags differ, connections land in `Unattributed` and are dropped from traffic/session/log reports (no fallback to sing-box's own per-user stats API / V2Ray stats). No in-process statistics are collected.
- Report ingestion Panel-side (`web/agent_telemetry.go`): traffic batches validated against node set + `user_nodes` pair set (`agentScopes:28-46`), idempotent via `(agent_id,batch_seq)`; sessions replace the whole server's session rows (`handleAgentSessions:156-237`); logs appended (`handleAgentConnectionLogs:268-365`). Traffic increments `users.used_bytes` (`repo/telemetry.go:58-64`).

### 5.5 Key files

- `cmd/agent/main.go`, `internal/agentruntime/{loop,applier,checker,clash,collector,metrics_linux}.go`, `internal/agentclient/client.go`, `internal/agentstate/state.go`.
- Panel: `internal/web/agent.go`, `agent_config.go`, `agent_telemetry.go`; `internal/singbox/singbox.go`.
- `docs/api-contract.md` (agent config payload contract at `:490`).

---

## 6. Protocol / node settings model

### 6.1 Storage split

- **Public settings** → `nodes.settings TEXT` (JSON object, default `{}`). Rendered to clients/agents as-is; never echoed to admin node DTOs.
- **Secrets** → `nodes.secret_enc BLOB` (AES-256-GCM ciphertext of a JSON object). Encrypt/decrypt via `internal/secrets/secrets.go:13-39` using the panel `app_key` (32-byte; env > yaml > auto-generated & persisted in `settings.app_key`, `config.go:42-120`).
- Update is a **patch**, not a replace: `buildNodeSettings` starts from stored `settings` + decrypted secrets, merges only allowlisted supplied fields, validates the complete result, then re-encrypts (`web/nodes.go:292-459`; contract `.trellis/spec/backend/node-protocol-settings.md:83-102`).
- `nodeDTO` never contains settings/secrets (`dto.go:77-86`), so the edit form treats blank = keep current (there is no UI to clear a once-set field; spec `:64`).

### 6.2 Per-protocol settings (allowlists enforce unknown-field rejection)

| Protocol | Plain (`settings`) | Secret (`secret_enc`) | sing-box inbound |
|---|---|---|---|
| `shadowsocks` | `method` (2022-blake3-aes-128-gcm / aes-256-gcm / chacha20-poly1305) | `password` (optional server password; otherwise derived) | multi-user, per-user derived password |
| `vless` | `server_names[]`, `short_id` | `private_key` (X25519, 32 bytes) | Reality, flow `xtls-rprx-vision`, handshake `server_names[0]:443`, user UUIDs |
| `hysteria2` | `server_name`, `up_mbps`, `down_mbps`, `obfs_password`, `hop_ports` | `password`, `certificate` PEM, `private_key` PEM | TLS inline cert/key; users password=UUID; `hop_ports` subscription-only |
| `anytls` | `server_name` | `password`, `certificate` PEM, `private_key` PEM | TLS inline cert/key; users password=UUID (sing-box ≥1.12) |

- Supported protocols constant: `repo/nodes.go:47-51`, `singbox.go:20-24`; web allowlist `web/nodes.go:25-30`.
- TLS material contract (hy2/anytls shared): `validateTLSMaterial` (`web/nodes.go:517-533`) verifies key pair match, certificate validity window, and `VerifyHostname(server_name)`.
- Reality keypair generation is non-persistent (`web/nodes.go:138-154`; spec `:30`).
- `hop_ports` is subscription-only and must never enter the inbound (`spec:62`; enforced at `singbox.go:230-238` only for up/down/obfs, hop not read).
- Spec `.trellis/spec/backend/node-protocol-settings.md` documents signatures, validation/error matrix, TLS/AnyTLS rules, partial-update contract, and the protocol-add migration recipe (`:55`).

---

## Gaps vs Xboard (obvious, from this read)

> Factual deltas only; no recommendations. Xboard itself was not fetched in this pass (external verification pending).

1. **No plans / subscription plans model.** Only per-user `quota_bytes`/`expires_at` on `users` and one link token in `user_subscriptions`. No `plans`, `plan_*`, or server/plan grouping tables.
2. **No `servers` hierarchy/groups.** Flat `servers` + flat `nodes`; no `parent_id`, node groups, or machine/node separation as Xboard has (Xboard "server" concept differs; here Server = physical host and Node = inbound).
3. **Node is a single inbound row** (one protocol/port); no multi-protocol or multi-inbound node config JSONB, no per-node `server_name`/`flow`/`network`/`tls_settings` columns, no per-node rate limit columns.
4. **Agent does NOT embed sing-box** — it writes a config file and shells out to an external `sing-box` binary plus a reload command/helper (`applier.go`, `cmd/singbox-reload`). Xboard-Node's embedded-sing-box architecture is absent.
5. **Traffic accounting is external Clash-API polling** with metadata-based user attribution (`clash.go`, `collector.go`), with no reliable per-user fallback; Xboard-Node tracks per-user stats in-process. Unattributed connections are dropped (`loop.go:279`).
6. **Users: one global UUID + token + subscription token**, no multi-credential (`v2ray_uuid`, `trojan_password`, etc.), no `last_login_at`, no invite/commission/plan fields, no user groups/levels.
7. **No admin roles/permissions** — `admins` has only username/password; no RBAC beyond a single admin type.
8. **No per-node user override** (e.g., node-level rate/limit or device limits); authorization is a flat `user_nodes` membership.
9. **Subscription output limited to `general` (base64 URI list) and `clash-meta`**, with a restricted template allowlist; no Clash/Stash/Surge/Sing-box/Quantumult variants.
10. **Settings are flat key/value** in one `settings` table; no config versioning or structured settings groups.
11. **Sessions replace-whole-server on each report** (`ReplaceServerSessions`) rather than incremental session entity; connection_logs/traffic_records have no FKs and retention is window/cap based.
12. **No migrations beyond protocol CHECK rebuilds**; schema changes to `nodes`/`users` require table rebuilds (SQLite constraint limitation).

## Caveats / not found

- No active Trellis task was set (`task.py current` → none); output written to the explicitly provided task dir.
- Xboard / Xboard-Node source was **not** fetched or compared in this pass; the "gaps" above are stated against the task's description of Xboard's model, not a code-level diff.
- `internal/db/db.go` was not read in full; pragma/`foreign_key_check` behavior assumed standard.
- `internal/web/dashboard.go`, `settings.go`, `admin_auth.go`, `respond.go`, `user_data.go` were not read line-by-line (routes/DTOs captured via `web.go`/`dto.go`).
