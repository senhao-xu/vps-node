# Research: Xboard 模型（Panel 数据模型 / 节点架构 / 订阅 / Panel↔Node API）

- **Query**: Copy Xboard's model 1:1 (servers / nodes / users / subscriptions) and adopt
  Xboard-Node's embedded sing-box architecture. Produce an accurate map of the model.
- **Scope**: external (open-source repos)
- **Date**: 2026-09-22
- **Sources**:
  - Panel (Laravel, branch `master`): https://github.com/cedar2025/Xboard
  - Node (Go, branch `dev`): https://github.com/cedar2025/Xboard-Node
  - Snapshot method: full tarball download via `codeload.github.com` on 2026-09-22
    (`.../tar.gz/refs/heads/master` and `.../refs/heads/dev`). Exact commit SHAs were not
    resolvable from this environment (GitHub API rate limit / raw host timeout); paths below
    are branch-relative and were current at fetch time. All file paths are repo-relative
    (panel = `Xboard`, node = `Xboard-Node`).

> Terminology note: in Xboard a **"server"** row (`v2_server`) IS the proxy **node**. A
> **"machine"** (`v2_server_machine`) is the host process that runs one or more nodes.
> See §2.

---

## 1. Panel data model (`database/migrations/`)

There is **no** per-protocol table set anymore and **no** user↔server join table. The model
was consolidated by migration `2025_01_05_131425_create_v2_server_table.php`, which created a
single `v2_server` table and dropped the legacy `v2_server_{trojan,vmess,vless,shadowsocks,hysteria}`
tables. Protocol-specific configuration moved into a JSON column `protocol_settings`.

### 1.1 `v2_server` — the node/proxy endpoint

Created: `database/migrations/2025_01_05_131425_create_v2_server_table.php`

| Column | Type | Notes |
|---|---|---|
| `id` | bigint PK | |
| `type` | string | protocol: `shadowsocks`/`vmess`/`vless`/`trojan`/`hysteria`/`tuic`/`anytls`/`socks`/`naive`/`http`/`mieru` |
| `code` | string nullable | legacy per-protocol id; `unique(type, code)` |
| `parent_id` | unsigned int nullable | parent node (self FK) |
| `group_ids` | json nullable | array of `v2_server_group.id` (cast to array in model) |
| `route_ids` | json nullable | array of `v2_server_route.id` |
| `name` | string | display name |
| `rate` | decimal(8,2) | traffic multiplier |
| `tags` | json nullable | |
| `host` | string | client-facing address |
| `port` | string | client port; may be a **range** e.g. `20000-30000` |
| `server_port` | int | actual listen port |
| `protocol_settings` | json nullable | see §3 |
| `show` | bool default false | list visibility |
| `sort` | int nullable, indexed | |
| `created_at`/`updated_at` | timestamps | |

Alterations added later:

| Migration | Added columns |
|---|---|
| `2026_04_11_000001_add_machine_support.php` | `machine_id` unsignedBigInteger nullable FK → `v2_server_machine.id` ON DELETE SET NULL; `enabled` bool default true |
| `2026_03_28_161536_add_traffic_fields_to_servers.php` | `transfer_enable` bigint nullable (0/null = unlimited); `u` bigint default 0; `d` bigint default 0 |
| `2026_03_15_060035_add_custom_config_and_cert_to_v2_server_table.php` | `custom_outbounds` json, `custom_routes` json, `cert_config` json |
| `2025_07_13_224539_add_column_rate_time_ranges_to_v2_server_table.php` | `rate_time_enable`, `rate_time_ranges` (time-of-day rate) |

Model: `app/Models/Server.php` (`protected $table = 'v2_server'`).
- Type constants/aliases: `TYPE_ALIASES = ['v2ray'=>'vmess','hysteria2'=>'hysteria']`;
  `VALID_TYPES = [hysteria, vless, trojan, vmess, tuic, shadowsocks, anytls, socks, naive, http, mieru]`.
- Relations: `parent()` (self), `stats()` → `StatServer`, `machine()` → `ServerMachine`,
  `groups()` = `ServerGroup::whereIn('id', group_ids)`, `routes()`.
- Status is derived from cache keys set by node reports: `last_check_at`, `last_push_at`,
  `online`, `is_online`, `available_status` (`0 offline / 1 online-no-push / 2 online`,
  `CHECK_INTERVAL = 300`).
- `generateServerPassword(User)`: returns `user.uuid`, EXCEPT shadowsocks 2022 ciphers where
  it returns `"{serverKey}:{userKey}"` (`Helper::getServerKey(created_at)` + `uuidToBase64`).

### 1.2 `v2_user`

Created: `database/migrations/2023_03_19_000000_create_v2_tables.php`; extended by
`2025_01_10_152139_add_device_limit_column.php` and
`2025_06_21_000003_add_traffic_reset_fields_to_users.php`.

Key columns: `id`, `invite_user_id`, `telegram_id`, `email` (unique), `password`,
`password_algo`, `password_salt`, `balance`, `discount`, `commission_type`,
`commission_rate`, `commission_balance`, `t` (token? seq), `u` (upload), `d` (download),
`transfer_enable`, `banned`, `is_admin`, `is_staff`, `last_login_at`, `last_login_ip`,
`uuid`, **`group_id`** (single group), **`plan_id`**, `speed_limit`, `remind_expire`,
`remind_traffic`, `token`, `expired_at`, `remarks`, `created_at`, `updated_at`;
plus `device_limit`, `online_count`, `last_online_at`;
plus `next_reset_at`, `last_reset_at`, `reset_count`.

Model `app/Models/User.php`:
- `plan()` → `Plan` (belongs to), `group()` → `ServerGroup` (belongs to), `stat()` → `StatUser`.
- `isActive()` = not banned && not expired && `plan_id !== null`.
- `isAvailable()` = active && remaining traffic > 0.
- `getRemainingTraffic()` = `transfer_enable - (u+d)`.

### 1.3 `v2_plan`

Created: `2023_03_19_000000_create_v2_tables.php`; reshaped by
`2025_01_04_optimize_plan_table.php` (added `prices` json + `sell`, changed
`transfer_enable`→bytes, `speed_limit`→Mbps, `reset_traffic_method`, `capacity_limit`;
dropped the eight `*_price` columns), `2025_01_10_152139_add_device_limit_column.php`
(`device_limit`), `2025_07_01_081556_add_tags_to_v2_plan_table.php` (`tags` json).

Current columns: `id`, `group_id`, `transfer_enable`, `name`, `speed_limit`,
`show`, `sort`, `renew`, `content`, `prices` (json), `sell`, `reset_traffic_method`,
`capacity_limit`, `device_limit`, `tags` (json), timestamps.

Model `app/Models/Plan.php`:
- `prices` json keys are periods: `monthly|quarterly|half_yearly|yearly|two_yearly|three_yearly|onetime|reset_traffic`.
- `reset_traffic_method`: null=follow system, 0=1st of month, 1=monthly, 2=never, 3=Jan 1, 4=yearly.
- `group()` hasOne `ServerGroup` via `group_id`.

### 1.4 `v2_order`

Created `2023_03_19_000000_create_v2_tables.php`. Columns: `id`, `invite_user_id`,
`user_id`, `plan_id`, `coupon_id`, `payment_id`, `type` (1 new / 2 renewal / 3 upgrade /
4 reset-traffic), `period`, `trade_no` (unique), `callback_no`, `total_amount`,
`handling_amount`, `discount_amount`, `surplus_amount`, `balance_amount`,
`surplus_order_ids`, `status` (0 pending/1 processing/2 cancelled/3 completed/4 discounted),
`commission_status`, `commission_balance`, `actual_commission_balance`, `paid_at`,
timestamps. `refund_amount` was renamed `surplus_credit` in
`2026_05_30_000000_rename_refund_amount_to_surplus_credit.php`.
Model `app/Models/Order.php` with `plan()`, `user()`, `payment()`, `commission_log()`.

### 1.5 `v2_server_group`

Created `2023_03_19_000000_create_v2_tables.php`. Columns: `id`, `name`, timestamps.
**No membership join table.** Membership is expressed as:
- `v2_server.group_ids` (JSON array) — which groups a node belongs to.
- `v2_user.group_id` and `v2_plan.group_id` (single integer) — which group a user/plan is entitled to.
Model `app/Models/ServerGroup.php` exposes `users()` and `servers()` (via
`whereJsonContains('group_ids', (string)$id)`), plus `server_count`.

### 1.6 `v2_server_machine` (+ load history)

Created `2026_04_11_000001_add_machine_support.php`.

`v2_server_machine`: `id`, `name`, `token` (64, unique), `notes`, `is_active` (default true),
`last_seen_at`, `load_status` (json), timestamps.
Model `app/Models/ServerMachine.php`: `servers()` hasMany `Server.machine_id`,
`loadHistory()`, `generateToken()`, `updateHeartbeat()`; `token` is hidden.

`v2_server_machine_load_history`: `id`, `machine_id` FK (cascade), `cpu` float,
`mem_total`, `mem_used`, `disk_total`, `disk_used`, `net_in_speed`, `net_out_speed`
(added `2026_04_18_000001_add_network_to_machine_load_history.php`), `recorded_at`,
timestamps; index `(machine_id, recorded_at)`.

### 1.7 Traffic stats tables

`v2_stat_user` (created `2023_03_19_...`): `id`, `user_id`, `server_rate` decimal(10),
`u` bigint, `d` bigint, `record_type` char(2) (`d` day / `m` month), `record_at`,
timestamps; unique `(server_rate, user_id, record_at)`.
Model `app/Models/StatUser.php`.

`v2_stat_server` (created `2023_03_19_...`): `id`, `server_id` (indexed), `server_type`
char(11), `u` bigint, `d` bigint, `record_type` char(1) (`d`/`m`), `record_at` (indexed),
timestamps; unique `(server_id, server_type, record_at)`.
Model `app/Models/StatServer.php` with `server()`.

Indexes added in `2025_01_15_000002_add_stat_performance_indexes.php` (user.t/online_count/created_at,
order columns, stat_server u/d, stat_user u/d, etc.).

### 1.8 Other core tables

- `v2_server_route` (`app/Models/ServerRoute.php`): `id`, `remarks`, `match` (json array),
  `action` char(11), `action_value`, timestamps (rules injected into kernel config).
- `v2_subscribe_templates` `2025_07_27_000001_create_v2_subscribe_templates_table.php`:
  `id`, `name` unique (e.g. `singbox`, `clash`, `clashmeta`), `content` mediumtext, timestamps.
  Seeded from `resources/rules/{custom,default}.<name>.{json,yaml}`. Model `SubscribeTemplate`.
- `v2_settings`, `v2_payment`, `v2_coupon`, `v2_notice`, `v2_ticket`, `v2_knowledge`,
  `v2_invite_code`, `v2_commission_log`, gift-card tables, `v2_traffic_reset_logs`.
- Legacy `v2_server_stat` (`ServerStat`) and `v2_server_log` (`ServerLog`) models exist, but
  **no migration creating those tables exists in this repo** — treat as legacy/undeployed.

---

## 2. Terminology & relationship chain

- **node == server**: a single proxy listener, one `v2_server` row. There is no separate
  "node" table. (The Go node process config calls each `v2_server` a "node".)
- **machine**: `v2_server_machine`, a physical host running the `xboard-node` process. It can
  own many `v2_server` rows via `v2_server.machine_id`. In machine mode one process manages
  all nodes bound to it and reports host-level load.
- **server group**: `v2_server_group`, a subscription entitlement bucket, not a host.

Chain (user → plan → group → servers):

```
v2_user ──plan_id──▶ v2_plan
   │  └─group_id──────────────┐
   └─group_id─────────────────┤
                              ▼
                     v2_server_group (id, name)
                              ▲
             v2_server.group_ids (JSON array contains group id)
                              │
                              ▼
                          v2_server (node)
```

Effective node set for a user (`ServerService::getAvailableServers`,
`app/Services/ServerService.php:55`):

```php
Server::whereJsonContains('group_ids', (string) $user->group_id)
    ->where('show', true)
    ->where(fn($q) => $q->whereNull('transfer_enable')
                        ->orWhere('transfer_enable', 0)
                        ->orWhereRaw('u + d < transfer_enable'))
    ->orderBy('sort', 'ASC')
```

Users are **not** individually assigned to nodes; entitlement is purely by group. Each user
has exactly one `group_id` and one `plan_id` (no multi-group join table). A plan also carries
`group_id`; on purchase the plan's group is copied to the user.

Inverse (users a node serves): `ServerService::getAvailableUsers(Server)`
(`ServerService.php:90`) selects users whose `group_id` ∈ node's `group_ids` with
`u + d < transfer_enable`, not expired, not banned, returning
`[id, uuid, speed_limit, device_limit]`.

---

## 3. Node `type` and `protocol_settings`

`protocol_settings` is the JSON blob on `v2_server`; its shape depends on `type`. The
authoritative schema/defaults live in `app/Models/Server.php`
(`PROTOCOL_CONFIGURATIONS`, lines ~216–324). The model casts/merges defaults on read/write
via `getProtocolSettingsAttribute` / `setProtocolSettingsAttribute`.

Per-type keys (defaults shown):

- **trojan**: `tls` (int, 1), `network`, `network_settings`, `server_name`, `allow_insecure`,
  `tls_settings{server_name, allow_insecure, ech{...}}`, `reality_settings{server_name,
  server_port, public_key, private_key, short_id, allow_insecure}`, `multiplex{...}`, `utls{...}`.
- **vmess**: `tls` (0), `network`, `rules`, `network_settings`, `tls_settings{...}`,
  `multiplex`, `utls`.
- **vless**: `tls` (0), `tls_settings{...}`, `flow`, `encryption{enabled, encryption,
  decryption}`, `network`, `network_settings`, `reality_settings{...}`, `multiplex`, `utls`.
- **shadowsocks**: `cipher`, `obfs`, `obfs_settings`, `plugin`, `plugin_opts`.
- **hysteria**: `version` (2), `bandwidth{up,down}`, `obfs{open,type,password}`,
  `tls{server_name, allow_insecure, ech{...}}`, `hop_interval`.
- **tuic**: `version` (5), `congestion_control` (cubic), `alpn` (['h3']),
  `udp_relay_mode` (native), `tls{...}`.
- **anytls**: `padding_scheme` (string list), `tls{...}`.
- **socks / naive / http**: `tls` (0), `tls_settings{...}`.
- **mieru**: `transport` (TCP), `traffic_pattern`, `multiplex`.

Shared nested config shapes:
- `multiplex`: `{enabled, protocol(yamux), max_connections, padding, brutal{enabled,up_mbps,down_mbps}}`.
- `reality_settings`: `{server_name, server_port, public_key, private_key, short_id, allow_insecure}`.
- `utls`: `{enabled, fingerprint(chrome)}`.
- `ech`: `{enabled, config, query_server_name, key, key_path, config_path}`.
- `tls_settings`/`tls`: `{server_name, allow_insecure, ...ech}`.

Storage of the top-level fields (`app/Models/Server.php`): `host` (client address),
`port` (client port string, possibly `"start-end"` range), `server_port` (listen int),
`type` (protocol string), `group_ids` (JSON array of group ids), `route_ids`, `tags`,
`machine_id`, `enabled`.

Protocol class registry: `app/Providers/ProtocolServiceProvider.php` +
`app/Support/ProtocolManager.php` map client flags (User-Agent) to classes in
`app/Protocols/`: `General, SingBox, Clash, ClashMeta, Stash, Surfboard, Surge, Loon,
QuantumultX, Shadowrocket, Shadowsocks`.

Downstream conversion into the node's kernel config is done panel-side in
`ServerService::buildNodeConfig(Server)` (`ServerService.php:251`): it flattens
`protocol_settings` into a flat `protocol`-discriminated payload (e.g. `cipher`, `plugin`,
`server_key`, `tls`, `tls_settings`, `flow`, `decryption`, `server_name`, `up_mbps`,
`down_mbps`, `obfs`, `obfs-password`, `congestion_control`, `padding_scheme`,
`transport`, `traffic_pattern`, `multiplex`), plus `routes`, `custom_outbounds`,
`custom_routes`, `cert_config`. For `tls == 2` it substitutes `reality_settings` in place of
`tls_settings`.

---

## 4. Subscription generation (`app/Protocols/`, `ClientController`)

Entry: `routes/web.php` → `GET /{subscribe_path}/{token}` →
`App\Http\Controllers\V1\Client\ClientController::subscribe`
(`getAvailableServers` → hook filters → client-info by `flag`/User-Agent → protocol class
`handle()`).

- `flag` query or `User-Agent` selects a protocol class via
  `app('protocols.manager')->matchProtocolClassName()`, default `General`.
- `types` and `filter` query params filter by server type / name/tags.
- Optional info lines prepended as pseudo-nodes when `show_info_to_server_enable`: remaining
  traffic / reset days / expiry; protocol prefixes when `show_protocol_to_server_enable`
  (`[ss]`, `[vmess]`, `[vless]`, `[trojan]`, `[Hy2]`, `[tuic]`, `[socks]`, `[anytls]`).

URIs are built in `app/Protocols/General.php`:
- `buildShadowsocks`: `ss://base64url(cipher:password)@host:port[?plugin=...]#name`;
  password may be `serverKey:userKey` for SS2022.
- `buildVmess`: `vmess://base64(json{v,ps,add,port,id,aid,net,type,host,path,tls,sni,fp,...})`.
- `buildVless`: `vless://uuid@host:port?security=tls|reality&type=...&pbk/sid/sni/fp...#name`.
- `buildTrojan`: `trojan://password@host:port?security=tls|reality&...`.
- `buildHysteria`: v2 → `hysteria2://password@host:port?...`; v1 → `hysteria://host:port?...`.
- `buildTuic`, `buildAnyTLS`, `buildSocks`, `buildHttp` similarly.

Client-specific generators:
- `app/Protocols/SingBox.php` (flags `sing-box`, `hiddify`, `sfm`): emits a full sing-box
  client JSON (templates from `resources/rules/{custom,default}.sing-box.json`).
  Headers: `profile-title: base64:<app name>`, `subscription-userinfo`, `profile-update-interval: 24`.
- `app/Protocols/ClashMeta.php` / `Clash.php`: YAML; headers
  `content-type: text/yaml`, `subscription-userinfo`, `profile-update-interval: 24`,
  `content-disposition: attachment;filename*=UTF-8''<appName>`, and (Clash) `profile-web-page-url`.
- `General.php` return:
  ```php
  response(base64_encode($uri))
      ->header('content-type', 'text/plain')
      ->header('subscription-userinfo',
          "upload={$user['u']}; download={$user['d']}; total={$user['transfer_enable']}; expire={$user['expired_at']}");
  ```

`subscription-userinfo` is the standard `upload/download/total/expire` header; units are bytes
and `expired_at` is a Unix timestamp. `profile-title` (sing-box) is `base64:`-prefixed;
clash-family uses `profile-title`/`content-disposition` + `profile-update-interval`.

Version gating: `app/Support/AbstractProtocol.php` filters servers not supported by the
detected client+version (whitelists per protocol/transport, e.g. sing-box `xhttp` → 9999.0.0).

---

## 5. Node backend architecture (`Xboard-Node`, branch `dev`)

Module `github.com/cedar2025/xboard-node`. Config file (YAML, `internal/config/config.go`):
`panel`, `node`, `kernel`, `cert`, `log`, `runtime`, `ws`, plus `nodes[]` (multi-node),
`standalone`, `machine`. Kernel selection:
`kernel.type ∈ {"singbox","xray"}` (`internal/service/service.go:142`).

`internal/kernel/kernel.go` defines the `Kernel` interface (Start/Reload/Stop/IsRunning,
AddUsers/RemoveUsers/UpdateUsers, GetUserTraffic, SetSpeedLimitFunc, SetDeviceLimitFunc,
UpdateGlobalDevices/ClearGlobalDevices, CloseConnection, Capabilities/Protocols).

### 5.1 sing-box embedded as a library

`internal/kernel/singbox/singbox.go`:
- Builds a native sing-box config map (`buildConfig` in `config.go`) then
  `singJSON.UnmarshalExtendedContext[option.Options](include.Context(ctx), data)` and
  `box.New(box.Options{Context, Options})` → `instance.Start()`. So sing-box runs **in-process**
  as a Go library (imports `github.com/sagernet/sing-box/...`).
- User hot-swap uses sing-box's native `adapter.UpdatableInbound[T]` interfaces
  (VMess/VLESS/Trojan/Hysteria2/TUIC/AnyTLS/Mieru/auth.User for Naive/Socks/HTTP) and
  `adapter.UpdatableShadowsocksInbound.UpdateUsersByOptions`; inbounds are reconstructed only
  when the config hash/TLS changes.
- `Capabilities()`: `PerUserSpeedLimit: true, DeviceLimit: true, BuiltInTrafficStats: false,
  AliveIPTracking: true, ForceCloseConnection: false, ForceCloseUser: true`.
- `router.AppendTracker(s.connTracker)` registers the custom connection tracker once.

`internal/kernel/singbox/config.go` `buildInbound()` maps node protocol → sing-box inbound:
`shadowsocks` (method=cipher; SS2022 user password = base64(uuid) padded to key size),
`vmess` (name=uuid, alterId 0), `vless` (name/uuid, optional flow, tls 1 or reality 2),
`trojan` (name/password=uuid), `hysteria` v1/`hysteria2` v2, `tuic`, `anytls`, `naive`
(username=user id, password=uuid), `socks`, `http`, `mieru`. Transport (`ws/grpc/httpupgrade/h2/xhttp`),
proxy-protocol, multiplex, TLS/reality/ECH are applied via helpers. All users are keyed by
`uuid` (`buildUserMap` UUID→userID).

### 5.2 Per-user traffic / alive IP / device limit / speed limit

`internal/kernel/singbox/conntracker.go` — `ConnTracker` implements
`adapter.ConnectionTracker` (`RoutedConnection` TCP, `RoutedPacketConnection` UDP):
- Per-user `userStats` = lock-free `atomic.Int64 upload/download` + a small mutex-guarded
  `ips map[string]int` (source IP → refcount) + `connCount`.
- Wrapping: `trackedConn`/`trackedPacketConn` implement `UnwrapReader/Writer` and
  `UnwrapPacketReader/Writer` returning `N.CountFunc` callbacks → zero-copy byte counting.
  Semantics (per source comments): **read from inbound = user upload**; **write to inbound =
  user download**.
- `GetUserTraffic()` returns `(map[userID][2]int64{u,d}, map[userID]map[ip]bool, connCount)`
  in O(users).
- **Device limit** gate at connect time: `checkDeviceGate` merges local IP set with
  `globalDevices` (synced from panel, stale after 60s); over-limit connections are closed.
  `SetDeviceLimitFunc(func(uuid)(int,bool))`.
- **Speed limit**: `SetSpeedLimitFunc(func(uuid)*rate.Limiter)`; `RateLimitedWriter/Reader`
  use `golang.org/x/time/rate` with `AllowN` + context-aware `ReserveN`.
- `CloseByID` force-closes a conn; `CloseByUUID` is a documented no-op for sing-box.

### 5.3 Traffic delta computation

`internal/tracker/tracker.go` — `Tracker`:
- `Process(cumTraffic, kernelAliveIPs, connCount)` computes per-user deltas vs
  `lastSeen` (guards negative = kernel restart), accumulates into `pendingTraffic`, and
  atomically publishes an immutable `snapshot` (lock-free readers).
- `FlushTraffic()` drains pending deltas; `RestoreTraffic()` puts them back on push failure.
- `FlushAliveIPs()` returns per-user IP lists, using a SHA-256 change hash to skip unchanged
  reports; `CurrentOnline()` = distinct IP count per user.
- `InboundSpeed()/OutboundSpeed()` = last cycle bytes / 10 (track interval is 10s).

### 5.4 Service loop / push to panel

`internal/service/service.go` (`Service.Run`) tickers:
- `trackTicker` (`node.track_interval`, default 10s) → `trackAndEnforce`:
  `kernel.GetUserTraffic` → `tracker.Process`.
- `reportTicker` (`push_interval`, from handshake, default 60s, min 5s) →
  `pushReportAsync` → `sink.Report(ReportPayload{Traffic, Alive, Online, CPU, Mem, Swap,
  Disk, Metrics})`; on failure restores traffic/alive and backs off.
- `pullTicker` (`pull_interval`, default 60s) → `pullViaAPIAsync` (skipped while WS connected).
- `deviceReportTicker` (`device_report_interval`, default 30s) → WS-only `report.devices`.
- WS discovery + status handling; config changes → `kernel.Reload`; user changes →
  `applyUserUpdate`/`applyUserDelta` → hot-swap.
- Metrics built in `buildMetrics` (uptime, goroutines, active/total connections, active/total
  users, in/out speed, per-core CPU, load, speed_limiter, GC, api success/failure, ws status,
  limits/device-limit events, kernel_status).

### 5.5 Xray kernel path (brief)

`internal/kernel/xray/dispatcher.go` replaces xray's default dispatcher factory via
`//go:linkname` into `common.typeCreatorRegistry`, installing `LimitDispatcher` which wraps
xray's `DefaultDispatcher`. It enforces per-user device limits (`checkDeviceLimit`, lowest-IPs
lexicographic policy, lock-free `sync.Map` for unlimited users) and tracks alive IPs /
connection counts. **Traffic bytes are left to xray's built-in stats pipeline** —
`GetConnectionState()` returns only `aliveIPs` + `connCount`.

---

## 6. Panel ↔ Node API

Routes:
- `app/Http/Routes/V1/ServerRoute.php` (legacy): prefix `server/UniProxy`, middleware `server`
  → `GET config`, `GET user`, `POST push`, `POST alive`, `GET alivelist`, `POST status`.
- `app/Http/Routes/V2/ServerRoute.php` (current): prefix `server`, middleware `server.v2`
  → `GET|POST handshake`, `POST report`, `GET config`, `GET user`, `POST push`,
  `POST alive`, `GET alivelist`, `POST status`; plus prefix `server/machine` (no auth
  middleware; controller validates token) → `POST nodes`, `POST status`.
- Exact URLs are under `/api/v1/...` and `/api/v2/...`.

Auth (`app/Http/Middleware/ServerV2.php`, `Server.php`):
- Node mode: `token` must equal `admin_setting('server_token')`, plus `node_id`
  (and legacy `node_type`).
- Machine mode: `machine_id` + `token` matched against `v2_server_machine`; node must belong
  to that machine and be `enabled`. `last_seen_at` updated.
- GET auth via query params; POST via JSON body (`token`, `node_id`/`machine_id`).

Endpoints / payloads:

| Endpoint | Direction | Payload | Handler |
|---|---|---|---|
| `GET /api/v{1,2}/server/{UniProxy/,}config` | panel→node | `buildNodeConfig(node)` + `base_config{push_interval,pull_interval}`; ETag/304 | `UniProxyController::config` |
| `GET .../user` | panel→node | `{users:[{id,uuid,speed_limit,device_limit}]}`; ETag/304 | `UniProxyController::user` |
| `GET .../alivelist` | panel→node | `{alive:{userID:[ips]}}` for users with `device_limit>0` | `UniProxyController::alivelist` |
| `POST /api/v2/server/report` | node→panel | `{traffic:{uid:[u,d]}, alive:{uid:[ips]}, online:{uid:conn}, status:{cpu,mem,swap,disk}, metrics:{...}}` | `V2\Server\ServerController::report` |
| `POST .../push` (v1) | node→panel | `{uid:[u,d]}` raw JSON | `UniProxyController::push` |
| `POST .../alive` (v1) | node→panel | `{uid:[ips]}` | `UniProxyController::alive` |
| `POST .../status` (v1) | node→panel | `{cpu, mem.total/used, swap..., disk...}` | `UniProxyController::status` |
| `POST /api/v2/server/handshake` | panel→node | `{websocket:{enabled,ws_url}}` (+ node client also reads `settings`) | `V2\Server\ServerController::handshake` |
| `POST /api/v2/server/machine/nodes` | node→panel | → `{nodes:[{id,type,name}], base_config:{push_interval,pull_interval}}` | `V2\Server\MachineController::nodes` |
| `POST /api/v2/server/machine/status` | node→panel | `{cpu, mem, swap, disk, net{in_speed,out_speed}}` | `V2\Server\MachineController::status` |

Panel-side report processing (`app/Services/ServerService.php`):
- `processTraffic` → caches `SERVER_<TYPE>_ONLINE_USER` / `LAST_PUSH_AT`, then
  `UserService::trafficFetch` → `StatUserJob` upserts `v2_stat_user`
  (delta × `server.rate`, keyed by user+rate+record_at) and accumulates `v2_user.u/d`.
- `processAlive` → `DeviceStateService::setDevices` (Redis).
- `processOnline`, `processStatus`, `updateMetrics`, `touchNode`.

Node client (`internal/panel/client.go`): `NewClient(cfg)` with `configPath()`/`userPath()`
choosing v2 machine endpoints (`/api/v2/server/config|user`) when `machineID>0`, else v1
`/api/v1/server/UniProxy/*`. `Handshake`, `GetConfig`/`GetUsers` (ETag), `Report`, legacy
`PushTraffic`/`PushAlive`/`PushStatus`, `GetMachineNodes`, `ReportMachineStatus`.
`internal/panel/types.go` defines `NodeConfig` (protocol, listen_ip, server_port, network,
networkSettings, base_config, routes, kernel_type/kernel_log_level, cert_config, protocol
fields, multiplex, …), `User{id,uuid,speed_limit,device_limit}`, `HandshakeResponse`.

### 6.1 Dynamic user/config delivery (WebSocket + Redis)

Panel WS server: `App\Console\Commands\NodeWebSocketServer` (`php artisan ws-server`,
default `--port=8076`) using Workerman; implementation `app/WebSocket/NodeWorker.php`
(+ `NodeEventHandlers.php`).

- Node connects to `ws_url` from handshake with query `token`+`node_id` (or `machine_id`).
- Server replies `auth.success` then pushes a full sync (`pushFullSync`): `sync.config`
  + `sync.users`.
- Panel→node events: `ping`, `sync.config`, `sync.users`, `sync.user.delta`
  (`{action:'add'|'remove', users:[...]}`), `sync.devices` (global IP state),
  `sync.nodes` (machine node list changed). Node→panel: `pong`, `node.status`,
  `report.devices`, `request.devices`.
- Publisher: `app/Services/NodeSyncService.php` publishes to Redis channel `node:push`;
  NodeWorker subscribes and forwards to the connected node/machine. Triggers:
  `UserObserver` (sync fields: group_id, uuid, speed_limit, device_limit, banned,
  expired_at, transfer_enable, u, d, plan_id → `NodeUserSyncJob` → `sync.user.delta`),
  `ServerObserver` (group_ids → full sync; protocol/port/type/routes/custom_*/cert → config
  update; machine_id/enabled → machine `sync.nodes`).
- `internal/panel/ws.go` (`WSClient`) implements the client: auth via query, `ping`/`pong`,
  `node.status` every `ws.status_interval` (10s), `report.devices` via `SendDeviceReport`,
  exponential backoff with jitter, 10MB read limit. `internal/controlplane/panel.go`
  translates WS events into `Event`s; `internal/controlplane/machine.go` + `internal/machine/machine.go`
  implement machine mode (shared WS mux, per-node mailbox `NodeMailbox`, per-node
  `MachinePanelControlPlane`, REST snapshot per node).

### 6.2 Device / alive IP state

- Node reports alive IPs via `/report` (`alive`) or `/alive`; panel stores in
  `DeviceStateService` Redis hash `user_devices:{userId}` with fields `{nodeId}:{ip}`,
  TTL 300s; `getAliveList(users)` feeds `/alivelist`; `notifyUpdate` clears cached state.
- Node→panel WS `report.devices` handled in `NodeEventHandlers::handleDeviceReport`
  (diff add/remove) and pushed back to nodes via `sync.devices`.

---

## Caveats / Not Found

- Exact commit SHAs not captured (GitHub API rate-limit + raw host timeouts in this
  environment); content is from the `master`/`dev` branch tarballs downloaded 2026-09-22.
  Re-verify against a pinned commit before coding against it.
- The panel's legacy path also includes `app/Http/Routes/V1/ServerRoute.php` controllers
  `DeepbworkController`, `ShadowsocksTidalabController`, `TrojanTidalabController` which were
  not examined in depth (deprecated per-protocol sync APIs).
- `v2_server_stat` (`ServerStat`) and `v2_server_log` (`ServerLog`) models reference tables
  with no creating migration in this repo — likely legacy; not part of the current schema.
- `internal/kernel/singbox/config.go` is 1019 lines; protocol→config mappings for transports
  (`ws/grpc/httpupgrade/h2/xhttp`), TLS/reality/ECH and mieru were summarized, not exhaustively
  quoted. Read the file directly for exact keys.
- Subscription template seed file lists (`resources/rules/*`) and plugin system
  (`app/Services/Plugin`, `HookManager`) were not analyzed.
