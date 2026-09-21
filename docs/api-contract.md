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
| admin  | HttpOnly session cookie `panel_session`, SameSite=Lax  | `/api/admin/*`, `/api/users/*`, `/api/servers/*`, `/api/nodes/*`, `/api/dashboard`, `/api/settings` |
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
- Protocols: `shadowsocks`, `vless`, `hysteria2`.
- Connection log status: `active`, `closed`.

A user is **eligible** for a node iff `status = active` AND `expires_at > now` AND (`quota_bytes = 0` OR `used_bytes < quota_bytes`). Only eligible users authorized on the node's server are included in agent payloads.

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
  "quota_bytes": 1099511627776,
  "used_bytes": 343597383680,
  "started_at": "2026-01-01T00:00:00Z",
  "expires_at": "2026-12-01T00:00:00Z",
  "node_count": 3,
  "session_count": 2,
  "created_at": "2026-01-01T00:00:00Z"
}
```

`username` is required, globally unique, 1–64 chars from `[A-Za-z0-9_.-]`. `token` is never listed. `session_count` counts only fresh snapshots (see Session freshness).

### POST /api/users

```json
{
  "username": "alice",
  "quota_bytes": 1099511627776,
  "started_at": "2026-01-01T00:00:00Z",
  "expires_at": "2026-12-01T00:00:00Z",
  "node_ids": [1, 2]
}
```

`username` is required: missing/empty or invalid format → `422 validation`; already taken → `409 conflict`. Panel generates `uuid` and `token`. Response `201`: full user DTO plus `token` (the only time the plaintext token is returned).

### GET /api/users/:id

Response `200`: user DTO (no token) plus `remaining_bytes` and `used_percent`.

### PUT /api/users/:id

Partial update; included fields are applied:

```json
{ "username": "alice2", "status": "active", "quota_bytes": 214748364800, "started_at": "...", "expires_at": "..." }
```

Optional `username` change follows the same validation as create (format → `422 validation`, taken by another user → `409 conflict`; keeping the user's own value is allowed). Changing `username` does not bump server revisions. `expires_at: null` clears expiry. Setting `expires_at` in the past or `status: "expired"` marks the user expired. Response `200`: updated DTO.

### DELETE /api/users/:id

Response `200`: `{}`. Transactionally removes user, `user_nodes` rows and sessions. Agent removes runtime config on next sync.

### POST /api/users/:id/reset-token

Response `200`: `{ "token": "new-plaintext-token" }`. Old token invalid immediately.

### POST /api/users/:id/reset-traffic

Response `200`: user DTO with `used_bytes = 0`.

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

## User Sessions / Logs / Traffic

### GET /api/users/:id/sessions

Query: `include_stale` (default `false`).

Response `200`:

```json
{ "items": [ { "node_id": 1, "server_id": 1, "ip": "1.2.3.4", "upload_bytes": 1288490188, "download_bytes": 545460846, "connected_at": "...", "last_seen_at": "..." } ] }
```

### GET /api/users/:id/connection-logs

Query: `from`, `to`, `page`, `page_size`. Paginated `connection_logs` entries (see Agent API for the entry shape). Never contains destination host/port or payload data.

### GET /api/users/:id/traffic

Query: `from`, `to`, `bucket` (`hour`|`day`, default `day`).

Response `200`:

```json
{ "total_upload_bytes": 123, "total_download_bytes": 456, "series": [ { "bucket_start": "...", "upload_bytes": 1, "download_bytes": 2 } ] }
```

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

### PUT /api/servers/:id

```json
{ "name": "HK-01", "address": "hk01.example.com", "status": "active" }
```

Response `200`: server DTO. Setting `status: "disabled"` stops config sync for that server.

### DELETE /api/servers/:id

Response `200`: `{}`. Panel marks the server deleted, removes agent, nodes, `user_nodes` referencing its nodes and sessions; traffic and connection logs are retained until retention cleanup.

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

Query (all optional, combinable): `server_id` (exact match), `protocol` (`shadowsocks|vless|hysteria2`), `status` (`active|disabled`), `q` (case-insensitive substring match on node name), plus `page` / `page_size`. Invalid enum values return `400 invalid_request`. Paginated node DTOs:

```json
{ "id": 1, "server_id": 1, "name": "HK-SS", "protocol": "shadowsocks", "port": 8388, "status": "active", "server": { "id": 1, "name": "HK-1" }, "created_at": "..." }
```

Every node DTO carries `server: { id, name }` referencing its owning server.

Protocol secrets are never exposed.

### POST /api/nodes

```json
{ "server_id": 1, "name": "HK-SS", "protocol": "shadowsocks", "port": 8388, "settings": { "method": "2022-blake3-aes-128-gcm" } }
```

`settings` is a protocol-specific structured object validated against an allowlist. Shadowsocks requires a supported `method`; its server password may be omitted and derived by Panel. VLESS requires a valid X25519 `private_key` and at least one `server_names` entry; optional `short_id` is an even-length hexadecimal string of at most 16 characters. Hysteria2 accepts optional non-negative integer `up_mbps` and `down_mbps`. Secrets submitted here are encrypted at rest by Panel. Response `201`: node DTO.

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

Response `200`: node DTO plus `user_count`, `online_users`, `server: { id, name }`.

### PUT /api/nodes/:id

Partial update of `name`, `port`, `settings`, `status`. Protocol settings are merged with stored settings; omitted public fields and secrets remain unchanged. Response `200`: node DTO. Changes bump the owning server revision.

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
  "sessions_current": 456
}
```

`users_online` = distinct users with a fresh session. `servers_online` = servers with heartbeat within the offline threshold. Heartbeat is the only liveness source; traffic/session reports never mark a server online.

## Settings

### GET /api/settings

Response `200`:

```json
{
  "retention_raw_log_days": 7,
  "retention_aggregate_days": 90,
  "collection_connection_logs": true,
  "session_freshness_seconds": 300,
  "server_offline_after_seconds": 60
}
```

### PUT /api/settings

Partial update of the same fields. Response `200`: settings DTO.

Settings additionally include `subscribe_urls`, `subscribe_path`, and `clash_meta_template`.
`subscribe_urls` is a comma-separated list of HTTP(S) origins; `subscribe_path` is a safe
single path segment. The Clash Meta template is restricted and cannot contain generated
proxies, proxy providers, listeners, controllers, authentication, or credentials.

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
      "quota_bytes": 107374182400,
      "used_bytes": 20401094656,
      "expires_at": "2026-12-01T00:00:00Z",
      "nodes": [ { "id": 1, "protocol": "vless", "port": 443, "credential": { } } ]
    }
  ]
}
```

`config.singbox` is the complete, rendered sing-box configuration for the server. `renderer_version` identifies the rendering contract (see `internal/singbox.ContractVersion`) so agents can detect renderer changes. `credential` contains protocol-derived secrets (already generated/decrypted by Panel). Only eligible users on the server's nodes are included; expired/disabled/over-quota users are absent, which instructs the agent to remove them locally.

Agent applies the config (validate via `sing-box check`, atomic replace, reload). On failure it retains the previous config and keeps polling with its applied `version`, reporting the failure via heartbeat extension field `last_apply_error`.

## POST /api/agent/traffic

Idempotent batch ingestion. Body:

```json
{
  "batch_seq": 42,
  "records": [
    { "user_id": 1001, "node_id": 1, "upload_bytes": 12345678, "download_bytes": 98765432, "recorded_at": "2026-09-20T10:00:00Z" }
  ]
}
```

Rules:

- `batch_seq` is a monotonically increasing integer per agent; `(agent_id, batch_seq)` is the idempotency key.
- Bounded size: max 1000 records per request → otherwise `413 payload_too_large`.
- Counters must be non-negative → otherwise `422 validation`.
- `recorded_at` must be within an acceptance window (e.g. ±24h) → otherwise `422 validation`.

Response `200`:

```json
{ "accepted": true, "batch_seq": 42, "records": 1 }
```

A duplicate `batch_seq` returns the previously accepted result (`accepted: true`) without double counting; user/server/node totals are incremented exactly once per batch inside one transaction.

## POST /api/agent/sessions

Snapshot report (full replacement for the agent's server):

```json
{
  "reported_at": "2026-09-20T10:20:00Z",
  "sessions": [
    {
      "user_id": 1001,
      "node_id": 1,
      "ip": "1.2.3.4",
      "upload_bytes": 123456,
      "download_bytes": 456789,
      "connected_at": "2026-09-20T10:00:00Z",
      "last_seen_at": "2026-09-20T10:20:00Z"
    }
  ]
}
```

Response `200`:

```json
{ "accepted": true, "sessions": 1 }
```

Panel replaces all sessions of that server with the reported set inside one transaction. `user_id` and `node_id` must belong to the agent's server → otherwise `422 validation` and nothing is replaced. Sessions are considered "online" only while `now - last_seen_at <= session_freshness_seconds` (default 300s).

## POST /api/agent/connection-logs

Idempotent like traffic:

```json
{
  "batch_seq": 17,
  "logs": [
    {
      "user_id": 1001,
      "node_id": 1,
      "ip": "1.2.3.4",
      "protocol": "vless",
      "upload_bytes": 123456,
      "download_bytes": 456789,
      "connected_at": "2026-09-20T10:00:00Z",
      "closed_at": "2026-09-20T10:05:00Z",
      "status": "closed"
    }
  ]
}
```

Response `200`:

```json
{ "accepted": true, "batch_seq": 17, "logs": 1 }
```

Constraints identical to traffic batches (size limit, sequence idempotency, non-negative counters, timestamp window). Logs must never contain destination host, destination port, payload data, tokens or passwords; Panel rejects or strips such fields if they ever appear (`422 validation`).

---

# Ownership Rules (binding for all stages)

1. Agent `server_id` is derived exclusively from the agent token or registration token. Body-supplied `server_id` values are ignored for authorization.
2. Every `user_id` or `node_id` referenced in an agent payload is validated to belong to the agent's server; violations reject the whole batch atomically.
3. Admin endpoints require an admin session; agent endpoints require an agent bearer token. Neither accepts the other's credentials.
4. Plaintext secrets (user token, agent token, registration token, protocol secrets) appear in responses only once, at creation/rotation; thereafter only hashes or ciphertext are stored and never returned or logged.
5. Server revision increments monotonically on any admin change affecting a server's config (user eligibility, user_nodes, node CRUD, server status).
6. Heartbeat is the only liveness source. Traffic/session/log reports must not affect online status.
7. Deletion semantics: user deletion cascades `user_nodes` + sessions; server deletion cascades agent + nodes + sessions and authorizations; traffic records and connection logs survive until retention cleanup.
