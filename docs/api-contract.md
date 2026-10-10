# API Contract

Single source of truth for Panel Admin API, Panel Agent API, and the Vue frontend. Every later stage must keep this document in sync with the implementation.

## Conventions

- Base URL: Panel origin, e.g. `https://panel.example.com`.
- All request and response bodies are JSON (`Content-Type: application/json`).
- All timestamps are RFC3339 UTC strings, e.g. `2026-09-20T10:00:00Z`.
- All byte counters are non-negative integers (bytes).
- Unknown fields are ignored; unknown enum values are rejected with `invalid_request`.
- IDs are 64-bit integers serialized as JSON numbers. User, server and node IDs are never reused after deletion; upgrades seed their sequences above both live IDs and retained historical IDs.

## Auth Scopes

| Scope  | Mechanism                                              | Applies to      |
| ------ | ------------------------------------------------------ | --------------- |
| admin  | HttpOnly session cookie `panel_session`, SameSite=Lax  | `/api/admin/*`, `/api/users/*`, `/api/servers/*`, `/api/nodes/*`, `/api/visits`, `/api/dashboard`, `/api/settings` |
| agent  | `Authorization: Bearer <agent_key>`                    | `/api/agent/*`  |

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
| 401  | `unauthorized`    | Missing/invalid session or agent key       |
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
- Protocols: `shadowsocks`, `vless`, `hysteria2`, `anytls`, `socks`, `http`.

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
  "notes": "provider note",
  "public_visible": true,
  "offline_notify": false,
  "ip": "103.21.244.18",
  "ipv6": "2001:db8::1",
  "observed_ip": "103.21.244.18",
  "region": "HK",
  "price_cents": 3500,
  "price_currency": "USD",
  "traffic_limit_bytes": 1073741824000,
  "traffic_used_bytes": 50358493184,
  "traffic_accounting": "max",
  "traffic_reset_day": 1,
  "monthly_upload_bytes": 123456,
  "monthly_download_bytes": 654321,
  "monthly_used_bytes": 654321,
  "billing_cycle": "monthly",
  "expires_at": "2026-12-31T23:59:59Z",
  "sort_order": 1,
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
{ "name": "HK-01", "ip": "103.21.244.18", "region": "HK", "price_cents": 3500, "price_currency": "USD", "traffic_limit_bytes": 1073741824000, "expires_at": "2026-12-31T23:59:59Z" }
```

All inventory fields are optional. Empty IP, IPv6 and region mean unspecified; zero traffic limit means unlimited; empty expiry means no expiry. Price is stored in minor currency units. `billing_cycle` is one of `monthly`, `quarterly`, `semiannual`, `yearly`, or `one_time`. `traffic_accounting` is `max` or `sum`, and `traffic_reset_day` is `1..31` with short months clamped to their last day. `traffic_used_bytes` is a read-only lifetime counter. Monthly fields are computed from durable daily totals in the active UTC billing window. Detailed traffic retention and row caps do not reduce these totals. Upgrade backfills available history; traffic already purged before the upgrade cannot be recovered. Lists are ordered by `sort_order`, then id. `public_visible` and `offline_notify` persist preferences for future public-page and notification integrations.

Response `201`: server DTO plus `agent_key`, the plaintext Agent Key issued automatically on creation. `status` starts as `active`. The key uses the same mechanism as `POST /api/servers/:id/agent-key` and can be read back via `GET /api/servers/:id/agent-key`. No server revision is bumped (identity is not runtime config).

### PUT /api/servers/order

Body: `{ "ids": [2, 1, 3] }`, containing every current server id exactly once in the desired order. Response `200`: `{}`. Invalid or duplicate ids return `422 validation`.

### GET /api/servers/:id

Response `200`: server DTO plus `revision`, `agent: { id, version, last_seen_at, connected_at }` and `nodes: [node DTO]`.

### GET /api/servers/:id/visits

Paginated visited-sites records for all nodes of one server. Query: `user_id`, `node_id`, `host`, `from`, `to`, `page`, `page_size`. Response `200`: pagination envelope of visit DTOs. `404 not_found` when the server does not exist.

### PUT /api/servers/:id

```json
{ "name": "HK-01", "status": "active" }
```

Response `200`: server DTO. Setting `status: "disabled"` stops config sync for that server.

`PUT /api/servers/:id` also accepts partial updates to `notes`, `public_visible`, `offline_notify`, `ip`, `ipv6`, `region`, `traffic_accounting`, `traffic_reset_day`, `price_cents`, `price_currency`, `billing_cycle`, `traffic_limit_bytes`, and `expires_at` (RFC3339; empty string clears it). Inventory changes are persisted with the server and returned in list/detail responses.

### DELETE /api/servers/:id

Response `200`: `{}`. Panel marks the server deleted, removes agent, nodes, `user_nodes` referencing its nodes and its online devices; affected users’ online counts are refreshed in the same transaction. Traffic records are retained until retention cleanup. Nodes on other servers chaining into this server's nodes are unlinked (`chain_node_id` cleared) and their servers' revisions are bumped.

### GET /api/servers/:id/agent-key

Returns the long-lived Agent Key of the server. Response `200`: `{ "agent_key": "<plaintext>" }`. The key is stored as a SHA-256 hash for authentication and encrypted with the panel `app_key` for reveal. Returns `409 conflict` when the server has no retrievable key yet (migrated agent without `key_enc`, or no agent binding) — generate/reset it first. `404 not_found` when the server does not exist.

### POST /api/servers/:id/agent-key

Generates or resets the server's Agent Key. Response `200`: `{ "agent_key": "<plaintext>" }`. The previous key is invalid immediately, so the node's agent config must be updated after a reset. No server revision is bumped (identity is not runtime config).

## Nodes

### GET /api/nodes

Query (all optional, combinable): `server_id` (exact match), `protocol` (`shadowsocks|vless|hysteria2|anytls|socks|http`), `status` (`active|disabled`), `q` (case-insensitive substring match on node name), plus `page` / `page_size`. Invalid enum values return `400 invalid_request`. Paginated node DTOs:

```json
{ "id": 1, "server_id": 1, "address": "hk01.example.com", "ipv6_enabled": false, "ipv6_address": "", "name": "HK-SS", "protocol": "shadowsocks", "sni": "", "port": 8388, "rate": 1, "tags": [], "status": "active", "server": { "id": 1, "name": "HK-1", "ip": "103.21.244.18", "ipv6": "2001:db8::1", "observed_ip": "103.21.244.18" }, "chain_node_id": null, "chain_node": null, "chain_custom_node_id": null, "chain_custom_entry_key": "", "chain_custom_node_name": "", "created_at": "..." }
```

Every node DTO carries `server: { id, name, ip, ipv6, observed_ip }` referencing its owning server. The address fields let the admin node list label the server's configured or observed IPv4/IPv6 families even when the node connection address is a domain. `address` is the user-facing connection host used to render subscriptions. `sni` is the safe display value extracted from `reality_settings.server_name` or `tls.server_name` without exposing protocol secrets. `rate` is the traffic multiplier (default `1`) and `tags` is a string array. `ipv6_enabled` (default `false`) advertises an extra IPv6 entry in subscriptions and `ipv6_address` is its connection host (an IPv6 literal or a domain resolving to AAAA); when empty or disabled the subscription renders only `address`. `chain_node_id` links the node to another managed node (any server) used as its chain exit; when set, the DTO also carries `chain_node: { id, name, server_name }` for display. `chain_custom_node_id` + `chain_custom_entry_key` select a single line inside a custom node source as the external chain exit; when set, the DTO also carries the display-only `chain_custom_node_name`. Chained nodes are transparent to subscriptions (only the entry node is rendered).

Protocol secrets are never exposed.

### POST /api/nodes

```json
{ "server_id": 1, "address": "hk01.example.com", "ipv6_enabled": true, "ipv6_address": "2001:db8::1", "name": "HK-SS", "protocol": "shadowsocks", "sni": "", "port": 8388, "rate": 1.5, "tags": ["hk"], "settings": { "cipher": "2022-blake3-aes-128-gcm" } }
```

`address` is required (1-255 characters). Node names are not unique per server (a copied node keeps its source name), but `port` must be unique among the server's `active` nodes; a duplicate active port returns `409 conflict`. `rate` is an optional positive traffic multiplier (default `1`); `tags` is an optional array of at most 20 non-empty strings of at most 32 characters. Invalid values return `422 validation`. `ipv6_enabled` is optional (default `false`); `ipv6_address` accepts a host of at most 255 characters (IPv6 literal or domain, mirroring `address`), and enabling it without an address returns `422 validation`.

`settings` is the protocol settings object, validated against a per-protocol allowlist; unknown keys in a validated section return `422 validation`, and validated sections deep-merge leaf by leaf. The reserved free-form sections `tls_settings`, `network_settings`, `multiplex`, `utls` (vless) and `obfs_settings` (shadowsocks) accept arbitrary JSON objects: a supplied section replaces the stored one as a whole, is preserved verbatim, and is never rendered, so it is an extension placeholder rather than runtime configuration. Public values are stored in `nodes.protocol_settings`; private material is encrypted at rest by Panel and never echoed.

`chain_node_id` is an optional id of another managed node used as this node's chain exit. The target must exist and be `active`, must not be the node itself, and must not close a chain loop (A→B→A at any depth) — violations return `422 validation`. Setting or clearing the link bumps the revisions of both the entry and the exit server. The exit node's inbound carries a derived pseudo user (`relay-<entry server id>`) whose traffic is never attributed to a panel user.

`chain_custom_node_id` + `chain_custom_entry_key` select a **single line inside a custom node source** (external exit) instead of a managed node; the two targeting modes are mutually exclusive (`422 validation` when both are non-null). The source must exist and be `active` (`422` otherwise) and the key must be the 64-character lowercase hex entry `key` from `GET /api/custom-nodes/:id/nodes` (`422` otherwise). The entry-node agent dials the external server with the line's own credentials (no relay user is injected, no panel-side traffic/device/visit accounting for the exit hop); entries the renderer cannot resolve (missing/changed key, disabled source, no subscription cache, unsupported type) fall back to a direct outbound. Only the entry node's owning server revision is bumped (there is no managed exit server).

- `shadowsocks`: required `cipher` (one of `2022-blake3-aes-128-gcm`, `2022-blake3-aes-256-gcm`); optional `obfs`, `obfs_settings`, `plugin`, `plugin_opts`; optional secret `password` (server password, otherwise derived).
- `vless`: required secret `private_key` (X25519, 32 decoded bytes) and `reality_settings.server_name`; `tls` (integer, must be `2`), `reality_settings.server_port` (1-65535, default `443`), `reality_settings.short_id` (even-length hex of at most 16 characters), `reality_settings.allow_insecure`, `flow` (empty or `xtls-rprx-vision`), `network` (empty or `tcp`), `tls_settings`, `network_settings`, `multiplex`, `utls` are optional. `reality_settings.public_key` is derived from `private_key`; a contradicting value returns `422 validation`.
- `hysteria2`: required secret `certificate`/`private_key` (matching PEM pair covering `tls.server_name`) and `tls.server_name`; optional secret `password`, `version` (integer, must be `2`), `bandwidth{up,down}` (non-negative integers), `obfs{open,type,password}` (`type` empty or `salamander`, `password` at most 64 characters), `tls.allow_insecure`, and `hop_interval` (`start-end`, ports within 1-65535; subscription-only, never rendered into the inbound; send an empty string to clear a saved range).
- `anytls`: required secret `certificate`/`private_key` (matching PEM pair covering `tls.server_name`) and `tls.server_name`; optional secret `password`, `tls.allow_insecure`, and `padding_scheme` (string or array of strings).
- `socks`: no protocol settings — `settings` must be `{}` (or omitted) and any supplied key returns `422 validation`. Each authorized user authenticates on the inbound with `username = u-<user id>` and `password = <user uuid>`; the sing-box inbound type is `socks` and the shared link scheme/Clash type are `socks` / `socks5`.
- `http`: optional TLS — a plaintext HTTP proxy accepts `{}` (or omitted); supplying TLS requires `tls.server_name` together with the secret `certificate`/`private_key` (matching PEM pair covering `tls.server_name`, validated exactly like hysteria2/anytls), plus optional `tls.allow_insecure` (client-side only). Supplying only a server name, only part of the pair, or a mismatched pair returns `422 validation`. Each authorized user authenticates with `username = u-<user id>` and `password = <user uuid>`; the sing-box inbound / Clash proxy type are both `http`, and the sing-box outbound type for chain exits is `http`. Once TLS is enabled it cannot be reverted to plaintext through the UI (blank secret fields keep the stored value).

Response `201`: node DTO.

Managed SS2022 ChaCha is rejected because the embedded runtime does not support its multi-user mode. Existing managed ChaCha nodes require an explicit cipher correction; secrets are never silently changed. External/custom-node ChaCha conversion remains supported.

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

Partial update of `address`, `ipv6_enabled`, `ipv6_address`, `name`, `protocol`, `port`, `rate`, `tags`, `settings`, `status`, `chain_node_id`, `chain_custom_node_id`, `chain_custom_entry_key`. Omitted `protocol` keeps the current protocol. Changing it validates `settings` as a fresh configuration for the new protocol and discards all settings and encrypted secrets from the old protocol; required new-protocol fields must be supplied. The node id, user assignments, and chain references are retained. `chain_node_id` and `chain_custom_node_id` are each tri-state: omitting the field keeps the current target, `null` clears it, and an id sets it (same validation as create). They are mutually exclusive — explicitly setting both to an id returns `422 validation`; setting either one to an id clears the other. `chain_custom_entry_key` accompanies a `chain_custom_node_id` (required valid 64-hex when a source is set, empty when no custom target) and is preserved when the field is omitted alongside an unchanged source. When the protocol is unchanged, settings are merged section by section; omitted public fields and secrets remain unchanged, and `certificate`/`private_key` must be replaced as a pair. A supplied reserved free-form section (`tls_settings`/`network_settings`/`multiplex`/`utls`/`obfs_settings`) replaces its stored value as a whole. Response `200`: node DTO. Changes bump the owning server revision; managed chain changes additionally bump the old and new exit servers, and updating a node bumps every server whose nodes chain through it. External exits only affect the entry node's own server. Enabling a node whose port is already used by an active node on the same server returns `409 conflict`.

### POST /api/nodes/:id/copy

Duplicates a node verbatim onto the same server: name, address, `ipv6_enabled`, `ipv6_address`, port, protocol, `protocol_settings` and encrypted secrets are copied as-is. The copy starts `disabled` so it may temporarily reuse the source port; enabling it later requires freeing the port (enforced by the partial unique index on `(server_id, port)` for active nodes). Response `201`: the new node DTO. Bumps the server revision.

### GET /api/nodes/:id/share

Read-only preview of one node's share link(s) for a chosen user. Query: `user_id` (required, an existing user id; missing or unknown → `422 validation`). Response `200`: `{ "node_id": 1, "user_id": 2, "authorized": true, "links": ["ss://...", "vless://..."] }`. `links` are built with the user's credentials exactly as the general subscription renderer does (per-user SS password derivation, user UUID for vless/hysteria2/anytls, `u-<id>` + UUID for socks/http), and include the IPv6 variant when the node advertises one. `authorized` reports whether the user is currently authorized on the node, so the UI can warn about a link that would not connect (the preview is still returned). A node missing required TLS settings (hysteria2/anytls) or using an unsupported protocol returns `422 validation`. This endpoint never echoes stored secret material beyond the derived credentials.

### DELETE /api/nodes/:id

Response `200`: `{}`. Removes `user_nodes` for this node; bumps server revision so agents drop the service. A node that is referenced as another node's chain exit cannot be deleted: `DELETE` returns `409 conflict` with a message naming the referencing nodes, which must be unlinked first.

## Custom Nodes

Admin-managed external nodes merged into subscription output. They never carry
traffic/device statistics. Individual lines may also be selected as a managed entry node's
**external chain exit** (`chain_custom_node_id` on the node): the entry agent dials the external
server directly with the line's own credentials, so no relay user is injected and the exit hop is
outside panel accounting. Adding/updating/removing such a reference keeps the source's own content
untouched; only the entry node's server revision reacts to node changes, while source
`PUT`/`refresh` bumps every server whose entries reference it.

### GET /api/custom-nodes

Response `200`:

```json
{ "items": [ { "id": 1, "name": "airport-A", "source_type": "links", "user_agent": "", "insecure_skip_verify": false, "status": "active", "has_cache": false, "fetched_at": null, "created_at": "...", "updated_at": "..." } ] }
```

`content` is never echoed by the list; use `GET /api/custom-nodes/:id/content` to prefill the edit form. `has_cache` / `fetched_at` describe the cached upstream payload for
`subscription`-type entries (refreshed with a 5-minute TTL at render time; a failed fetch falls
back to the last cache, otherwise the entry is skipped). `user_agent` is the upstream request
header for `subscription` entries; an empty value means the built-in default
(`clash-verge/v2.0.0`). `insecure_skip_verify` disables TLS certificate verification for the
upstream fetch (self-signed upstreams). Both are always empty/`false` for `links` entries.

### POST /api/custom-nodes

```json
{ "name": "airport-A", "source_type": "links", "content": "ss://...\nvless://..." }
```

`source_type` is `links` (one share URI per line; supported schemes `ss`/`vless`/`hysteria2`/`anytls`/`http`/`https`/`trojan`/`vmess`) or `subscription` (an `http(s)` URL, fetched server-side with a 5s timeout, 1 MiB cap and at most 3 redirects). `subscription` entries may set `user_agent` (empty/omitted = default `clash-verge/v2.0.0`) and `insecure_skip_verify` (boolean, default `false`; disables TLS verification for self-signed upstreams); `links` entries ignore both. Response `201`: the custom node DTO plus an optional `warnings` array listing lines that could not be converted to a Clash proxy (unparseable lines are still stored and pass through `flag=general` unchanged). Duplicate `name` → `409 conflict`; empty/invalid content → `422 validation`; `user_agent` longer than 255 characters or containing control characters → `422 validation`.

### PUT /api/custom-nodes/:id

Partial update of `name`, `content` (omit to keep; `source_type` is immutable), `status` and — for `subscription` sources — `user_agent` (empty string restores the default; omitting keeps the stored value) and `insecure_skip_verify` (omit to keep). Changing the content, the `user_agent` or `insecure_skip_verify` invalidates the fetch cache. Response `200`: the DTO plus optional `warnings`. Duplicate `name` → `409 conflict`. The update bumps the revision of every server whose nodes reference this source as their external chain exit, so their agent configs re-render.

### DELETE /api/custom-nodes/:id

Response `204`. Also removes every user authorization referencing it. If any node uses this
source as its external chain exit (`chain_custom_node_id`), deletion returns `409 conflict` with a
message naming the referencing entry nodes, which must be unlinked first.

### GET /api/custom-nodes/:id/nodes

Parses the custom node's own stored content into a display digest. It never performs network
IO: `subscription` sources are read from the fetch cache only, so a cache miss returns an empty
list with `has_cache=false` rather than fetching upstream. Unknown id → `404 not_found`.

```json
{ "source_type": "subscription", "has_cache": true, "fetched_at": "2026-09-28T10:00:00Z", "entries": [ { "key": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", "name": "HK-1", "type": "vless", "server": "1.2.3.4", "port": 443, "chain_supported": true } ], "skipped": ["foo://unsupported"] }
```

`type` is the Clash proxy type (`ss`/`vless`/`hysteria2`/`anytls`/`http`/`trojan`/`vmess`). `chain_supported`
reports whether the type can be converted into a sing-box chain outbound (`ss`/`vless`/`trojan`/
`vmess`/`hysteria2`/`anytls`/`socks5`/`http`); unsupported entries are still renderable in subscriptions
but cannot be chosen as an external chain exit and are greyed out by the node form. Entries
that cannot be parsed or lack `name`/`type`/`server`/`port` are listed in `skipped` (the raw
link line, or the proxy name / `proxy #i`) without affecting the others. For `links` sources
`has_cache` is always `false` and `fetched_at` is `null` (the links are parsed from
`content_enc`, no cache is involved).

`key` is a stable per-entry identifier used for per-user entry authorization: an
`HMAC-SHA256(app_key)` hex digest of the entry's canonical connection parameters with the
display `name` removed, so authorizations survive upstream renames. Unparseable entries have
no `key` and cannot be authorized individually; two entries with identical connection
parameters (only `name` differs) share one `key`.

### GET /api/custom-nodes/:id/share

Read-only export of one custom node's content for administrators, without choosing a user.
Response `200`:

```json
{ "source_type": "links", "has_cache": false, "fetched_at": null, "clash": "proxies:\n    - {name: HK-1, type: ss, server: 1.2.3.4, port: 8388, cipher: aes-128-gcm, password: pw, udp: true}\n", "links": ["ss://...", "vless://..."], "skipped": ["garbage line"] }
```

`clash` is a YAML `proxies:` fragment built exactly like the Clash subscription renderer
(share links parsed into proxies, upstream proxies appended as-is, duplicate names suffixed with
the source id); an empty render is `proxies: []\n`. `links` is the plaintext newline items
(never base64-encoded). `subscription` sources are fetched through the render-path lazy fetch:
within the 5-minute TTL the cache is reused, a stale cache triggers an upstream fetch, and a
failed fetch falls back to the stale cache; `has_cache` / `fetched_at` follow
`GET /api/custom-nodes/:id/nodes`. Items that cannot be converted are listed in `skipped` (the raw
link line, or `proxy #i` for an upstream proxy without a name). No cache and no content renders an
empty but successful response. Unknown id → `404 not_found`; no admin session → `401 unauthorized`.

### GET /api/custom-nodes/:id/content

Echoes the decrypted stored content of one custom node for the edit form. Response `200`:

```json
{ "content": "ss://...\nvless://..." }
```

For `links` sources `content` is the original link text; for `subscription` sources it is the
original upstream URL. The list endpoint never returns `content`. Unknown id → `404 not_found`;
no admin session → `401 unauthorized`; a decrypt failure → `500 internal`.

### POST /api/custom-nodes/:id/refresh

Only valid for `subscription` sources: ignores the render-path 5-minute cache TTL, force-fetches
the upstream URL, writes the cache + `fetched_at` and returns the same shape as
`GET /api/custom-nodes/:id/nodes` with `has_cache=true`. On success it bumps the revision of every
server whose nodes reference this source as their external chain exit. `links` sources →
`422 validation`. Unknown id → `404 not_found`. An upstream failure returns `500 internal` with a
readable message and leaves the previous cache and `fetched_at` untouched (and bumps no revision).

### GET /api/users/:id/custom-nodes

Response `200`:

```json
{ "custom_node_ids": [1], "custom_nodes": [ { "id": 1, "name": "airport-A", "source_type": "links", "user_agent": "", "insecure_skip_verify": false, "status": "active", "has_cache": false, "fetched_at": null, "created_at": "...", "updated_at": "..." } ], "custom_node_entries": [ { "custom_node_id": 1, "entry_keys": ["e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"] } ] }
```

`custom_node_entries` carries the per-source **entry whitelist** (the list of entry `key`s
authorized for the user). An empty whitelist means “all entries of that source”. The array is
always present but only contains sources with a **non-empty** whitelist; `entry_keys` are
sorted.

### PUT /api/users/:id/custom-nodes

Full-set replacement:

```json
{ "custom_node_ids": [1, 3], "custom_node_entries": [ { "custom_node_id": 1, "entry_keys": ["e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"] } ] }
```

`custom_node_ids` is required. `custom_node_entries` is optional: omitting it clears every
whitelist (each listed source stays fully authorized = all entries). Each `custom_node_id` in
`custom_node_entries` must appear in `custom_node_ids`, and each `entry_key` must be a 64-char
lowercase hex digest; violations → `422 validation`. Duplicates and blank entries are dropped,
and an empty `entry_keys` array means “all entries”. Idempotent; duplicates do not create
duplicate relations; unknown `custom_node_id` → `422 validation`. Because custom nodes never
reach agent config, this does **not** bump any server revision. Response `200`: same shape as GET.

Entries whose `entry_key` no longer exists in the source (upstream content changed) are kept
but inert: the render filters each source's entries down to the intersection of the whitelist
and the currently parsed entries. Unparseable `links` lines cannot be whitelisted and are
dropped while a whitelist is active.

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

A node with `ipv6_enabled` and a non-empty `ipv6_address` different from `address` renders a
second entry named `{name}-v6` whose host is `ipv6_address`; `port`, protocol parameters and
credentials are identical to the primary entry. Both entries share the node's single inbound, so
traffic and visit logs stay attributed to that node (the visit `client_ip` distinguishes the
families).

Protocol credentials: a `socks` node renders `socks://<base64url("u-<user id>:<user uuid>")>@host:port#<name>`
under `flag=general` and `{ type: socks5, username: "u-<user id>", password: "<user uuid>", udp: true }`
under `flag=clash-meta`. An `http` node renders `http://u-<user id>:<user uuid>@host:port#<name>` (plain
userinfo, optionally `?tls=1&sni=...&allowInsecure=1` when TLS is configured) under `flag=general` and
`{ type: http, username: "u-<user id>", password: "<user uuid>", udp: true, tls?: true, sni?, skip-cert-verify? }`
under `flag=clash-meta`. Clash Meta templates may group managed proxies with the placeholders
`__ALL_PROXIES__`, `__SHADOWSOCKS_PROXIES__`, `__VLESS_PROXIES__`, `__HYSTERIA2_PROXIES__`,
`__ANYTLS_PROXIES__`, `__SOCKS_PROXIES__` and `__HTTP_PROXIES__`, each expanding to that protocol's proxy names for the
current user.

After all managed nodes, the output appends the user's authorized active custom nodes in
`custom_nodes.id` order. `flag=general` passes `links`-type lines through verbatim;
`flag=clash-meta` converts them to proxies and drops the ones that cannot be parsed. Names that
collide with a managed node or an earlier custom node get a ` <id>` suffix so Clash proxy names
stay unique.

---

## GET /install-agent.sh

Public static shell-script endpoint, embedded in both panel build variants; content type is `text/x-shellscript; charset=utf-8`. It contains no credentials. The generated install command downloads it successfully before execution and passes `PANEL_URL`, `SERVER_ID` and `AGENT_KEY` to the installer shell. Fresh installations require these values. Existing configurations are preserved during upgrades. Default binaries come from the project’s GitHub Release assets; custom mirrors can override `PANEL_DOWNLOAD_BASE`.

---

# Agent API

All Agent endpoints require `Authorization: Bearer <agent_key>`. The agent's `server_id` is **always derived from the key server-side**; request bodies never carry a trusted `server_id`. The agent is stateless: no bootstrap/register endpoint exists, and all resume state is returned by the panel.

Production traffic must use HTTPS with certificate verification.

## POST /api/agent/heartbeat

```json
{ "version": "1.0.0", "cpu_percent": 23, "memory_percent": 52, "disk_percent": 41, "uptime_seconds": 123456, "last_apply_error": "", "public_ip": "", "public_ipv6": "" }
```

`last_apply_error` is an optional extension field: when non-empty it carries the most recent config-apply failure message (see Config below). Panel ignores unknown fields today but must not reject this one.

`public_ip` is an optional extension field: when non-empty it carries the agent's self-detected outbound public IP (HTTP probe against a configurable endpoint, or a static config override). The panel adopts it for the server's `observed_ip` only when it parses as a public IP; invalid or private values are silently ignored and the panel falls back to its connection-derived resolution (forwarding headers from private peers, then the peer address). Display-only: it never participates in authentication or ownership decisions.

`public_ipv6` is the IPv6 counterpart: when non-empty it carries the agent's self-detected outbound public IPv6 (probe against an IPv6-only endpoint, or static override). The panel adopts it for `servers.observed_ipv6` only when it parses as a public IPv6 (v4 or v4-mapped values are rejected); invalid or private values are silently ignored. It never overwrites the admin-managed `servers.ipv6` inventory field.

Response `200`:

```json
{ "ok": true, "server_id": 1, "server_revision": 1024, "heartbeat_interval_seconds": 30, "traffic_seq": 12, "device_seq": 4, "visit_seq": 30 }
```

`server_id` is the credential-derived server id: the agent asserts it equals its configured `server_id` and refuses to bind on mismatch. `traffic_seq` / `device_seq` / `visit_seq` are the panel's `MAX(seq)` per batch stream for this agent; the agent adopts `max(local, resume)` so a restarted (memory-only) agent resumes above every sequence already recorded and idempotency is preserved.

Panel stores `last_seen_at`, metrics and version, and marks the server online. A server is `offline` when `now - last_seen_at > server_offline_after_seconds` (configurable, default 60s).

## GET /api/agent/config?version=<applied-revision>

Returns the server-scoped rendered configuration. Before comparing revisions, the panel reconciles natural expiry using a persistent per-server watermark; expired users remain absent without changing their stored status. Crossing a user’s quota bumps every authorized server revision in the traffic transaction. This upgrade also bumps existing server revisions once so running agents fetch the corrected configuration.

- If `version` equals the current server revision: `200` with `{ "status": "current", "revision": 1024 }` and no config payload.
- Otherwise: `200` with:

```json
{
  "status": "updated",
  "revision": 1024,
  "renderer_version": "singbox-render-v3",
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

`config.singbox` is the complete, rendered sing-box configuration for the server. `renderer_version` identifies the rendering contract (see `internal/singbox.ContractVersion`) so agents can detect renderer changes. `credential` contains protocol-derived secrets (already generated/decrypted by Panel). For `socks` nodes the credential is `{ "contract": "socks-v1", "username": "u-<user id>", "password": "<user uuid>" }` and for `http` nodes `{ "contract": "http-v1", "username": "u-<user id>", "password": "<user uuid>" }`, matching the inbound user the agent must authenticate. `device_limit` and `speed_limit` are copied from the user row so the embedded runtime can enforce them. Only eligible users on the server's nodes are included; expired/disabled/over-transfer users are absent, which instructs the agent to remove them locally.

HTTP/SOCKS nodes with no eligible users and no relay credentials omit their inbound and corresponding chain route/outbound, preventing anonymous access. Relay-only entry points still require relay authentication.

Chained nodes (see `POST /api/nodes` `chain_node_id`) render one extra outbound per chained entry node (`tag: chain-<node id>`, a full protocol client of the exit node, credentials derived from the panel app key) plus a `route.rules` entry mapping the entry inbound tag to that outbound; `route.final` stays `direct`. Exit-side inbounds carry an additional pseudo user named `relay-<entry server id>` with the same derived credential; agents never map that name to a panel user, so relay traffic is neither attributed nor reported.

External chain exits (`chain_custom_node_id` + `chain_custom_entry_key`) render the same `tag: chain-<node id>` outbound and `route.rules` entry, but the outbound is a protocol client built from the selected custom-node line's own credentials (`ss`/`vless`/`trojan`/`vmess`/`hysteria2`/`anytls`/`socks5`/`http`) — no relay user exists on the external server. Config building performs no network IO: `subscription` sources are read from the stored fetch cache, and any unresolvable entry (missing/changed key, disabled source, empty cache, unsupported type) simply renders no chain outbound for that node, leaving it direct. A node whose `chain_node_id` is set ignores `chain_custom_node_id` (they cannot both be set through the API).

Agent applies the config by loading it into the embedded sing-box instance. On a failed replacement start it rebuilds the last successful service with its immutable config and credentials, preserving traffic and visit counters. Existing connections can be interrupted during restoration; restoration failures are reported alongside the apply error. It keeps polling with its applied `version` on failure, reporting via heartbeat extension field `last_apply_error`.

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

1. Agent `server_id` is derived exclusively from the agent key. Body-supplied `server_id` values are ignored for authorization.
2. Every `user_id` or `node_id` referenced in an agent payload is validated to belong to the agent's server; violations reject the whole batch atomically.
3. Admin endpoints require an admin session; agent endpoints require an agent bearer key. Neither accepts the other's credentials.
4. Plaintext secrets (user token, protocol secrets) appear in responses only once, at creation/rotation; thereafter only hashes or ciphertext are stored and never returned or logged. The Agent Key is the exception: it is stored hashed (`key_hash`) and encrypted (`key_enc`) for authentication and admin reveal, and is only returned by the agent-key endpoints.
5. Server revision increments monotonically on any admin change affecting a server's config (user eligibility, user_nodes, node CRUD, server status).
6. Heartbeat is the only liveness source. Traffic/device reports must not affect server online status.
7. Deletion semantics: user deletion cascades `user_nodes` and its online devices; server deletion cascades agent, nodes, authorizations and online devices; traffic records and visit records survive until retention cleanup.
