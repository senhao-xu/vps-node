# API Contract

Single source of truth for Panel Admin API, Panel Agent API, and the Vue frontend. Every later stage must keep this document in sync with the implementation.

## Conventions

- Base URL: Panel origin, e.g. `https://panel.example.com`.
- All request and response bodies are JSON (`Content-Type: application/json`).
- All timestamps are RFC3339 UTC strings, e.g. `2026-09-20T10:00:00Z`.
- All byte counters are non-negative integers (bytes).
- Unknown fields are ignored; unknown enum values are rejected with `invalid_request`.
- IDs are 64-bit integers serialized as JSON numbers.

## Auth Scopes

| Scope  | Mechanism                                              | Applies to      |
| ------ | ------------------------------------------------------ | --------------- |
| admin  | HttpOnly session cookie `panel_session`, SameSite=Lax  | `/api/admin/*`, `/api/users/*`, `/api/servers/*`, `/api/nodes/*`, `/api/visits`, `/api/dashboard`, `/api/settings` |
| agent  | `Authorization: Bearer <agent_token>`                  | `/api/agent/*`  |

- Admin and Agent APIs share no middleware and no token space.
- Unauthenticated requests return `401` with error code `unauthorized`.
- Admin sessions expire and are refreshed by activity; logout invalidates the session.

## Error Envelope

Every non-2xx response:

```json
{ "error": { "code": "not_found", "message": "user 42 not found" } }
```

| HTTP | code              | Meaning                                   |
| ---- | ----------------- | ----------------------------------------- |
| 400  | `invalid_request` | Malformed body, bad enum, bad pagination  |
| 401  | `unauthorized`    | Missing/invalid session or agent token    |
| 403  | `forbidden`       | Authenticated but not allowed             |
| 404  | `not_found`       | Unknown resource                          |
| 409  | `conflict`        | Unique constraint, duplicate, stale state |
| 413  | `payload_too_large` | Batch over size limit                   |
| 422  | `validation`      | Field-level validation failure            |
| 500  | `internal`        | Unexpected server error                   |

## Pagination

List endpoints accept `?page=` (default 1) and `?page_size=` (default 20, max 100) and return:

```json
{ "items": [], "total": 128, "page": 1, "page_size": 20 }
```

## Enums

- User status: `active`, `disabled`, `expired`.
- Server status: `active`, `disabled`, `offline` (`offline` is computed from heartbeat, not stored by admins).
- Node status: `active`, `disabled`.
- Protocols: `shadowsocks`, `vless`, `hysteria2`, `anytls`.

A user is **eligible** for a node iff `status = active` AND `expires_at > now` AND (`transfer_enable = 0` OR (`u` + `d`) < `transfer_enable`). Only eligible users authorized on the node's server are included in agent payloads.

---

# Admin API

## Auth

### POST /api/admin/login

```json
{ "username": "root", "password": "..." }
```

Response `200`:

```json
{ "admin": { "id": 1, "username": "root" } }
```

Sets `panel_session` cookie. Passwords are stored as hashes; never returned.

### POST /api/admin/logout

Response `200`: `{}`. Invalidates the current session.

### GET /api/admin/me

Response `200`:

```json
{ "admin": { "id": 1, "username": "root" } }
```

## Users

### GET /api/users

Query: `query` (exact match on username, uuid, or token), `status`, `expiry` (`valid`|`expired`), `page`, `page_size`.

Response `200`: paginated list of:

```json
{
  "id": 1001,
  "uuid": "xxxxxxxx-xxxx-...",
  "username": "alice",
  "status": "active",
  "transfer_enable": 1099511627776,
  "u": 100000000000,
  "d": 243597383680,
  "used_bytes": 343597383680,
  "speed_limit": 0,
  "device_limit": 3,
  "online_count": 2,
  "last_online_at": "2026-01-02T00:00:00Z",
  "started_at": "2026-01-01T00:00:00Z",
  "expires_at": "2026-12-01T00:00:00Z",
  "node_count": 3,
  "created_at": "2026-01-01T00:00:00Z"
}
```

`username` is required, globally unique, 1–64 chars from `[A-Za-z0-9_.-]`. `token` is never listed. `transfer_enable = 0` means unlimited. `used_bytes` is a derived convenience equal to `u + d`. `speed_limit` is in Mbps (`0` = unlimited). `device_limit` bounds distinct concurrent source IPs per user (`0` = unlimited). `online_count` reflects the current distinct source IPs reported by agents; `last_online_at` is the most recent report time.

### POST /api/users

```json
{
  "username": "alice",
  "transfer_enable": 1099511627776,
  "speed_limit": 0,
  "device_limit": 3,
  "started_at": "2026-01-01T00:00:00Z",
  "expires_at": "2026-12-01T00:00:00Z",
  "node_ids": [1, 2]
}
```

`username` is required: missing/empty or invalid format → `422 validation`; already taken → `409 conflict`. `transfer_enable`, `speed_limit` and `device_limit` default to `0` and must be `>= 0`. Panel generates `uuid` and `token`. Response `201`: full user DTO plus `token` (the only time the plaintext token is returned).

### GET /api/users/:id

Response `200`: user DTO (no token) plus `remaining_bytes` and `used_percent`.

### PUT /api/users/:id

Partial update; included fields are applied:

```json
{ "username": "alice2", "status": "active", "transfer_enable": 214748364800, "speed_limit": 100, "device_limit": 5, "started_at": "...", "expires_at": "..." }
```

Optional `username` change follows the same validation as create (format → `422 validation`, taken by another user → `409 conflict`; keeping the user's own value is allowed). Changing `username` does not bump server revisions. `expires_at: null` clears expiry. Setting `expires_at` in the past or `status: "expired"` marks the user expired. Response `200`: updated DTO.

### DELETE /api/users/:id

Response `200`: `{}`. Transactionally removes user and `user_nodes` rows. Agent removes runtime config on next sync.

### POST /api/users/:id/reset-token

Response `200`: `{ "token": "new-plaintext-token" }`. Old token invalid immediately.

### POST /api/users/:id/reset-traffic

Response `200`: user DTO with `u = 0`, `d = 0`.

### POST /api/users/:id/expire-now

Response `200`: user DTO with `expires_at = now` (status becomes `expired`).

## User ↔ Node Authorization

### GET /api/users/:id/nodes

Response `200`:

```json
{ "node_ids": [1, 2, 5], "nodes": [ { "id": 1, "name": "HK-01", "server_id": 1, "protocol": "vless", "port": 443, "status": "active" } ] }
```

### PUT /api/users/:id/nodes

Full-set replacement:

```json
{ "node_ids": [1, 2, 5] }
```

Response `200`: same shape as GET. Idempotent; duplicates in `node_ids` do not create duplicate relations. Unknown `node_id` → `422 validation`. Any change bumps the affected servers' revisions.

## User Devices / Traffic / Visits

### GET /api/users/:id/devices

Response `200`:

```json
{ "items": [ { "node_id": 1, "server_id": 1, "ip": "1.2.3.4", "online": 3, "last_seen_at": "..." } ] }
```

Rows are the current source IPs a user is connected from per `(user, node)`, as last reported by agents. `online` on each row is the live connection count the agent reported for that `(user, node)` (it is identical across the IP rows of the same pair). `online_count` on the user DTO equals the number of **distinct** source IPs across these rows, so one client IP connected through several nodes counts once.

### GET /api/users/:id/traffic

Query: `from`, `to`, `bucket` (`hour`|`day`, default `day`).

Response `200`:

```json
{ "total_upload_bytes": 123, "total_download_bytes": 456, "series": [ { "bucket_start": "...", "upload_bytes": 1, "download_bytes": 2 } ] }
```

### GET /api/users/:id/visits

Paginated visited-sites records for one user. Query: `node_id`, `host`, `from`, `to`, `page`, `page_size`. Response `200`: pagination envelope `{ items, total, page, page_size }` of visit DTOs (see [Visits](#visits)). `404 not_found` when the user does not exist.

## Visits

Visited-sites records are reported by agents (see [POST /api/agent/visits](#post-apiagentvisits)) and retained per the visit settings. All admin visit endpoints return RFC3339 `created_at` and the common pagination envelope.

Visit DTO:

```json
{
  "id": 42,
  "user_id": 1001, "username": "user-1",
  "node_id": 1, "node_name": "HK-SS",
  "server_id": 1, "server_name": "HK",
  "dest_host": "example.com", "dest_port": 443,
  "network": "tcp", "client_ip": "1.2.3.4",
  "created_at": "2026-09-22T10:00:00Z"
}
```

### GET /api/visits

Global visited-sites list. Query: `user_id`, `server_id`, `node_id`, `host` (case-insensitive substring), `client_ip` (substring), `from`, `to`, `page`, `page_size`. Response `200`: pagination envelope of visit DTOs, newest first.

### GET /api/visits/top

Most-visited hosts aggregated per UTC day over the last `days` (default `7`, clamped to `1..365`), for `user_id`/`server_id`/`node_id`/`host` filters. `limit` (default `20`, clamped to `1..200`) caps the rows. Response `200`: pagination envelope whose `items` are `{ "dest_host": "example.com", "hits": 12 }` ordered by `hits` descending then host ascending.

## Servers

### GET /api/servers

Response `200`: paginated list of:

```json
{
  "id": 1,
  "name": "HK-01",
  "address": "hk01.example.com",
  "status": "active",
  "cpu_percent": 23.5,
  "memory_percent": 52.1,
  "disk_percent": 41.0,
  "uptime_seconds": 123456,
  "agent_version": "1.0.0",
  "last_seen_at": "...",
  "node_count": 4,
  "online_users": 32,
  "created_at": "..."
}
```

### POST /api/servers

```json
{ "name": "HK-01", "address": "hk01.example.com" }
```

Response `201`: server DTO. `status` starts as `active`.

### GET /api/servers/:id

Response `200`: server DTO plus `revision`, `agent: { id, version, last_seen_at, connected_at }` and `nodes: [node DTO]`.

### GET /api/servers/:id/visits

Paginated visited-sites records for all nodes of one server. Query: `user_id`, `node_id`, `host`, `from`, `to`, `page`, `page_size`. Response `200`: pagination envelope of visit DTOs. `404 not_found` when the server does not exist.

### PUT /api/servers/:id

```json
{ "name": "HK-01", "address": "hk01.example.com", "status": "active" }
```

Response `200`: server DTO. Setting `status: "disabled"` stops config sync for that server.

### DELETE /api/servers/:id

Response `200`: `{}`. Panel marks the server deleted, removes agent, nodes, `user_nodes` referencing its nodes and its online devices; traffic records are retained until retention cleanup.

### POST /api/servers/:id/register-token

Generates a one-time registration token for agent bootstrap. Response `200`:

```json
{ "register_token": "plaintext-once", "expires_at": "2026-09-21T00:00:00Z" }
```

Only the hash is stored. Generating a new token invalidates the previous one.

### POST /api/servers/:id/agent-token

Rotates the agent token of the server's agent. Response `200`: `{ "agent_token": "plaintext-once", "expires_hint": null }`. Old agent token invalid immediately.

## Nodes

### GET /api/nodes

Query (all optional, combinable): `server_id` (exact match), `protocol` (`shadowsocks|vless|hysteria2|anytls`), `status` (`active|disabled`), `q` (case-insensitive substring match on node name), plus `page` / `page_size`. Invalid enum values return `400 invalid_request`. Paginated node DTOs:

```json
{ "id": 1, "server_id": 1, "name": "HK-SS", "protocol": "shadowsocks", "port": 8388, "rate": 1, "tags": [], "status": "active", "server": { "id": 1, "name": "HK-1" }, "created_at": "..." }
```

Every node DTO carries `server: { id, name }` referencing its owning server. `rate` is the traffic multiplier (default `1`) and `tags` is a string array.

Protocol secrets are never exposed.

### POST /api/nodes

```json
{ "server_id": 1, "name": "HK-SS", "protocol": "shadowsocks", "port": 8388, "rate": 1.5, "tags": ["hk"], "settings": { "cipher": "2022-blake3-aes-128-gcm" } }
```

`rate` is an optional positive traffic multiplier (default `1`); `tags` is an optional array of at most 20 non-empty strings of at most 32 characters. Invalid values return `422 validation`.

`settings` is the protocol settings object, validated against a per-protocol allowlist; unknown keys in a validated section return `422 validation`, and validated sections deep-merge leaf by leaf. The reserved free-form sections `tls_settings`, `network_settings`, `multiplex`, `utls` (vless) and `obfs_settings` (shadowsocks) accept arbitrary JSON objects: a supplied section replaces the stored one as a whole, is preserved verbatim, and is never rendered, so it is an extension placeholder rather than runtime configuration. Public values are stored in `nodes.protocol_settings`; private material is encrypted at rest by Panel and never echoed.

- `shadowsocks`: required `cipher` (one of `2022-blake3-aes-128-gcm`, `2022-blake3-aes-256-gcm`, `2022-blake3-chacha20-poly1305`); optional `obfs`, `obfs_settings`, `plugin`, `plugin_opts`; optional secret `password` (server password, otherwise derived).
- `vless`: required secret `private_key` (X25519, 32 decoded bytes) and `reality_settings.server_name`; `tls` (integer, must be `2`), `reality_settings.server_port` (1-65535, default `443`), `reality_settings.short_id` (even-length hex of at most 16 characters), `reality_settings.allow_insecure`, `flow` (empty or `xtls-rprx-vision`), `network` (empty or `tcp`), `tls_settings`, `network_settings`, `multiplex`, `utls` are optional. `reality_settings.public_key` is derived from `private_key`; a contradicting value returns `422 validation`.
- `hysteria2`: required secret `certificate`/`private_key` (matching PEM pair covering `tls.server_name`) and `tls.server_name`; optional secret `password`, `version` (integer, must be `2`), `bandwidth{up,down}` (non-negative integers), `obfs{open,type,password}` (`type` empty or `salamander`, `password` at most 64 characters), `tls.allow_insecure`, and `hop_interval` (`start-end`, ports within 1-65535; subscription-only, never rendered into the inbound).
- `anytls`: required secret `certificate`/`private_key` (matching PEM pair covering `tls.server_name`) and `tls.server_name`; optional secret `password`, `tls.allow_insecure`, and `padding_scheme` (string or array of strings).

Response `201`: node DTO.

### POST /api/nodes/reality-keypair

Admin-only generation of a Reality X25519 keypair and eight-byte Short ID. The operation is non-persistent and does not bump any server revision. Response `200`:

```json
{
  "private_key": "base64url-without-padding",
  "public_key": "base64url-without-padding",
  "short_id": "0123456789abcdef"
}
```

The private key is submitted through `POST /api/nodes` when creating a VLESS node. The public key is not stored or exposed by generic node DTOs.

### GET /api/nodes/:id

Response `200`: node DTO plus `user_count`, `online_users`, `server: { id, name }`, and `settings` — the node's public `protocol_settings` object (`{}` when unset). `settings` echoes only public configuration; secret material (vless `private_key`, TLS `certificate`/`private_key`, server `password`) is encrypted at rest and never echoed. Note that hysteria2 `obfs.password` is a public setting and is included. List endpoints (`GET /api/nodes`, user-scoped node lists) never include `settings`.

### PUT /api/nodes/:id

Partial update of `name`, `port`, `rate`, `tags`, `settings`, `status`. Protocol settings are merged with stored settings section by section; omitted public fields and secrets remain unchanged, and `certificate`/`private_key` must be replaced as a pair. A supplied reserved free-form section (`tls_settings`/`network_settings`/`multiplex`/`utls`/`obfs_settings`) replaces its stored value as a whole. Response `200`: node DTO. Changes bump the owning server revision.

### DELETE /api/nodes/:id

Response `200`: `{}`. Removes `user_nodes` for this node; bumps server revision so agents drop the service.

## Dashboard

### GET /api/dashboard

Response `200`:

```json
{
  "users_total": 1280,
  "users_online": 123,
  "servers_total": 18,
  "servers_online": 17,
  "traffic_today_bytes": 98765432100,
  "devices_current": 456
}
```

`users_online` = distinct users with a current device report. `devices_current` = online devices, counted as distinct `(user_id, source IP)` pairs so one client IP connected through several of a user's nodes counts once (the same basis as each user's `online_count`). `servers_online` = servers with heartbeat within the offline threshold. Heartbeat is the only liveness source; traffic/device reports never mark a server online.

### GET /api/dashboard/user-traffic?range=today|total

Per-user traffic overview with per-node drill-down. `range` defaults to `today`; any other value → `400 invalid_request`.

Response `200`:

```json
{
  "range": "today",
  "items": [
    {
      "user_id": 1001,
      "username": "alice",
      "status": "active",
      "transfer_enable": 107374182400,
      "upload_bytes": 100,
      "download_bytes": 200,
      "total_bytes": 300,
      "nodes": [
        {
          "node_id": 3,
          "node_name": "hy2-443",
          "server_id": 1,
          "server_name": "HK01",
          "upload_bytes": 60,
          "download_bytes": 120,
          "total_bytes": 180
        }
      ]
    }
  ]
}
```

`items` contains every user (zero-traffic users appear with zeroed counters and `nodes: []`), sorted by `total_bytes` descending then `user_id` ascending.

- `range=today`: user totals and node details both come from `traffic_records` with `created_at >=` today 00:00 UTC (same basis as `traffic_today_bytes`).
- `range=total`: user `total_bytes` is `users.u + users.d` (same basis as the user list, reset together with traffic reset), while `upload_bytes`/`download_bytes` and node details come from all retained `traffic_records`. Node details are bounded by retention and unaffected by traffic reset, so their sum may differ from `total_bytes` — this is an accepted discrepancy.

## Settings

### GET /api/settings

Response `200`:

```json
{
  "retention_aggregate_days": 90,
  "retention_visit_days": 7,
  "retention_visit_aggregate_days": 90,
  "collection_visits": true,
  "server_offline_after_seconds": 60
}
```

### PUT /api/settings

Partial update of the same fields. Response `200`: settings DTO.

`collection_visits` is the global visited-sites collection switch (default `true`); when
disabled the panel stops storing new visits and answers agent visit reports with
`accepted: false`. `retention_visit_days` (default `7`) bounds raw `visit_records`;
`retention_visit_aggregate_days` (default `90`) bounds the `visit_daily_domains` aggregate and
`visit_batches` markers. Both must be `1..3650`.

Settings additionally include `subscribe_urls`, `subscribe_path`, `subscribe_name`, and
`clash_meta_template`. `subscribe_urls` is a comma-separated list of HTTP(S) origins;
`subscribe_path` is a safe single path segment. `subscribe_name` (≤64 chars, no line breaks)
sets the subscription display name via `profile-title` and `content-disposition` response
headers; empty leaves naming to the client. The Clash Meta template is restricted and cannot
contain generated proxies, proxy providers, listeners, controllers, authentication, or
credentials.

## Subscriptions

`GET /api/users/:id/subscription` returns `{ "configured": false, "url": null }` until an
administrator explicitly creates a subscription. `POST` creates it and
`POST /api/users/:id/subscription/rotate` replaces it; both return the configured URL.
Subscription URLs use a separate encrypted, hash-authenticated bearer credential and never
change the user's business token. The URL token is a 22-character URL-safe Base64 string
(16 bytes of entropy); rotation generates a fresh one, while previously issued longer tokens
remain resolvable until rotated.

The public endpoint is `GET /{subscribe_path}/:token`; the dispatcher reads the validated
current `subscribe_path` setting on every request, so a saved path takes effect immediately.
The default endpoint is `/s/:token`. It supports
`flag=general` for standard Base64-encoded protocol links and `flag=clash-meta` for YAML.
Matching Clash/Mihomo user agents select Clash Meta automatically. Invalid credentials return
`404`; unavailable users return `403`. Responses contain only eligible authorized nodes.

---

# Agent API

All Agent endpoints require `Authorization: Bearer <agent_token>` (except `register`, which uses a registration token). The agent's `server_id` is **always derived from the token server-side**; request bodies never carry a trusted `server_id`.

Production traffic must use HTTPS with certificate verification.

## POST /api/agent/register

One-time bootstrap. Body:

```json
{ "register_token": "plaintext-once", "version": "1.0.0" }
```

Panel resolves the server from the registration token hash, creates (or replaces) the agent binding, and returns:

```json
{ "agent_id": 7, "agent_token": "plaintext-once", "server_id": 1, "heartbeat_interval_seconds": 30, "sync_interval_seconds": 30, "traffic_interval_seconds": 60 }
```

Re-registering an already-bound server rotates the agent token. Invalid/expired registration token → `401 unauthorized`.

## POST /api/agent/heartbeat

```json
{ "version": "1.0.0", "cpu_percent": 23, "memory_percent": 52, "disk_percent": 41, "uptime_seconds": 123456, "last_apply_error": "" }
```

`last_apply_error` is an optional extension field: when non-empty it carries the most recent config-apply failure message (see Config below). Panel ignores unknown fields today but must not reject this one.

Response `200`:

```json
{ "ok": true, "server_revision": 1024, "heartbeat_interval_seconds": 30 }
```

Panel stores `last_seen_at`, metrics and version, and marks the server online. A server is `offline` when `now - last_seen_at > server_offline_after_seconds` (configurable, default 60s).

## GET /api/agent/config?version=<applied-revision>

Returns the server-scoped rendered configuration.

- If `version` equals the current server revision: `200` with `{ "status": "current", "revision": 1024 }` and no config payload.
- Otherwise: `200` with:

```json
{
  "status": "updated",
  "revision": 1024,
  "renderer_version": "singbox-render-v1",
  "config": { "singbox": { } },
  "users": [
    {
      "id": 1001,
      "uuid": "xxxxxxxx",
      "status": "active",
      "transfer_enable": 107374182400,
      "u": 20401094656,
      "d": 0,
      "speed_limit": 100,
      "device_limit": 3,
      "expires_at": "2026-12-01T00:00:00Z",
      "nodes": [ { "id": 1, "protocol": "vless", "port": 443, "credential": { } } ]
    }
  ]
}
```

`config.singbox` is the complete, rendered sing-box configuration for the server. `renderer_version` identifies the rendering contract (see `internal/singbox.ContractVersion`) so agents can detect renderer changes. `credential` contains protocol-derived secrets (already generated/decrypted by Panel). `device_limit` and `speed_limit` are copied from the user row so the embedded runtime can enforce them. Only eligible users on the server's nodes are included; expired/disabled/over-transfer users are absent, which instructs the agent to remove them locally.

Agent applies the config by loading it into the embedded sing-box instance; it keeps the previous config and keeps polling with its applied `version` on failure, reporting the failure via heartbeat extension field `last_apply_error`.

## POST /api/agent/traffic

Idempotent batch ingestion. Body:

```json
{
  "batch_seq": 42,
  "records": [
    { "user_id": 1001, "node_id": 1, "u": 12345678, "d": 98765432, "recorded_at": "2026-09-20T10:00:00Z" }
  ]
}
```

Rules:

- `batch_seq` is a monotonically increasing integer per agent; `(agent_id, batch_seq)` is the idempotency key.
- Bounded size: max 1000 records per request → otherwise `413 payload_too_large`.
- Counters (`u`, `d`) must be non-negative → otherwise `422 validation`.
- `recorded_at` must be within an acceptance window (e.g. ±24h) → otherwise `422 validation`.
- Each record is scaled by the owning node's `rate` (default `1`) before it is stored in `traffic_records` and added to `users.u`/`users.d`, so reported totals and per-node drill-downs share the same multiplier.

Response `200`:

```json
{ "accepted": true, "batch_seq": 42, "records": 1 }
```

A duplicate `batch_seq` returns the previously accepted result (`accepted: true`) without double counting; `users.u`/`users.d`, `traffic_records` and the batch marker are written in one transaction.

## POST /api/agent/devices

Idempotent device snapshot report for the agent's server. Body:

```json
{
  "batch_seq": 17,
  "recorded_at": "2026-09-20T10:20:00Z",
  "devices": [
    { "user_id": 1001, "node_id": 1, "ips": ["1.2.3.4", "5.6.7.8"], "online": 3 }
  ]
}
```

Response `200`:

```json
{ "accepted": true, "batch_seq": 17, "devices": 2 }
```

`devices` is the complete current set for the agent's server. Inside one transaction the panel upserts the reported rows and deletes that server's `online_devices` rows whose `last_seen_at` is older than `recorded_at`, then refreshes each affected user's `online_count`/`last_online_at`. A snapshot larger than the per-request cap may be split into several requests provided every request carries the same `recorded_at`; a batch with a later `recorded_at` prunes devices absent from the new snapshot. `user_id`/`node_id` must belong to the agent's server → otherwise `422 validation` and nothing is applied. `ips` is the list of distinct source IPs currently connected for that `(user, node)`; `online` is the live connection count for that `(user, node)`, stored on each of its IP rows and returned by `GET /api/users/:id/devices`. Duplicate `batch_seq` is a no-op returning the original count. `devices` in the response is the number of `(user, node, ip)` rows inserted or refreshed by that batch.

## POST /api/agent/visits

Idempotent visited-sites batch for the agent's server. Body:

```json
{
  "batch_seq": 9,
  "records": [
    { "user_id": 1001, "node_id": 1, "dest_host": "example.com", "dest_port": 443, "network": "tcp", "client_ip": "1.2.3.4", "recorded_at": "2026-09-22T10:00:00Z" }
  ]
}
```

Each record is one accepted connection's destination, collected in-process from the embedded sing-box tracker's `metadata.Destination` (the client-requested host; no sniffing is enabled). `dest_host` is the requested domain or IP (1-253 characters, lowercased); `dest_port` is `0-65535`; `network` is `tcp`, `udp`, or empty; `client_ip` is at most 64 characters. Records whose destination is empty are dropped on the agent and never reported.

Rules:

- `batch_seq` is a monotonically increasing integer per agent; `(agent_id, batch_seq)` is the idempotency key.
- Bounded size: max 1000 records per request → otherwise `413 payload_too_large`.
- `user_id`/`node_id` must be `>= 1` and belong to the agent's server (`node` in the server's nodes, user authorized on that node) → otherwise `422 validation` and the whole batch is rejected.
- `recorded_at` must be RFC3339 within an acceptance window (e.g. ±24h) → otherwise `422 validation`.
- `dest_host`, `dest_port`, and `network` are validated as above → otherwise `422 validation`.
- When `collection_visits` is disabled, the panel stores nothing and returns `200 { "accepted": false, "batch_seq": ..., "records": 0 }` so the agent does not retry forever.

Response `200`:

```json
{ "accepted": true, "batch_seq": 9, "records": 1 }
```

A duplicate `batch_seq` returns the previously accepted count without re-storing; `visit_records`, the `visit_daily_domains` daily aggregate, and the `visit_batches` marker are written in one transaction.

---

# Ownership Rules (binding for all stages)

1. Agent `server_id` is derived exclusively from the agent token or registration token. Body-supplied `server_id` values are ignored for authorization.
2. Every `user_id` or `node_id` referenced in an agent payload is validated to belong to the agent's server; violations reject the whole batch atomically.
3. Admin endpoints require an admin session; agent endpoints require an agent bearer token. Neither accepts the other's credentials.
4. Plaintext secrets (user token, agent token, registration token, protocol secrets) appear in responses only once, at creation/rotation; thereafter only hashes or ciphertext are stored and never returned or logged.
5. Server revision increments monotonically on any admin change affecting a server's config (user eligibility, user_nodes, node CRUD, server status).
6. Heartbeat is the only liveness source. Traffic/device reports must not affect server online status.
7. Deletion semantics: user deletion cascades `user_nodes` and its online devices; server deletion cascades agent, nodes, authorizations and online devices; traffic records and visit records survive until retention cleanup.
