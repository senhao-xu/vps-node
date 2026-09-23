# Node Protocol Settings

> Executable contracts for node settings, Reality key generation, and partial updates.

## 1. Scope / Trigger

- Applies whenever an admin node API, protocol setting, sing-box node renderer, or node form changes.
- Node settings contain both public JSON (`nodes.settings`) and encrypted recoverable secrets (`nodes.secret_enc`); changes must preserve both halves consistently.

## 2. Signatures

- `POST /api/nodes/reality-keypair` generates, but does not persist, a Reality keypair and Short ID.
- `POST /api/nodes` creates a complete protocol configuration plus `rate`/`tags` and transactionally bumps the owning Server revision.
- `PUT /api/nodes/{id}` applies a `rate`/`tags`/settings patch and transactionally bumps the owning Server revision.

## 3. Contracts

Reality generation response:

```json
{
  "private_key": "43-character-base64url",
  "public_key": "43-character-base64url",
  "short_id": "16-lowercase-hex-characters"
}
```

- Keys are X25519 32-byte values encoded with unpadded URL-safe Base64.
- Short ID is eight random bytes encoded as lowercase hexadecimal.
- Generation is admin-only, non-persistent, and does not bump a revision.
- `rate` is a positive number (default `1`); `tags` is at most 20 non-empty strings of at most 32 characters.
- `protocol_settings` is the Xboard-style nested object; private material (`vless` `private_key`, TLS `certificate`/`private_key`, server `password`) is AES-GCM encrypted in `secret_enc` and never appears in generic Node DTOs.
- Validated sections (`reality_settings`, `tls`, `bandwidth`, `obfs`) reject unknown keys and deep-merge leaf by leaf. The reserved free-form sections `tls_settings`, `network_settings`, `multiplex`, `utls` (vless) and `obfs_settings` (shadowsocks) are accepted as arbitrary JSON objects, stored verbatim (a supplied section replaces the stored one as a whole), and are inert: the renderer and subscription code never read them, so they never reach agents or subscription output.
- Update `settings` is a **section-wise patch**: omitted nested public fields and encrypted secret fields retain their stored values.

## 4. Validation & Error Matrix

| Condition | Result |
|---|---|
| `rate` not a positive number, or `tags` malformed | `422 validation` |
| Shadowsocks `cipher` missing or unsupported | `422 validation` |
| VLESS `private_key` missing, malformed, or not 32 decoded bytes | `422 validation` |
| VLESS `reality_settings.server_name` empty | `422 validation` |
| VLESS `reality_settings.short_id` odd-length, non-hex, or over 8 bytes | `422 validation` |
| VLESS `reality_settings.public_key` contradicts the derived public key | `422 validation` |
| Hysteria2 `bandwidth.up`/`bandwidth.down` negative, fractional, or not numeric | `422 validation` |
| Hysteria2 `obfs.password` over 64 chars | `422 validation` |
| Hysteria2 `hop_interval` not `start-end`, out of 1-65535, or start > end | `422 validation` |
| Unknown protocol setting key (top-level, or nested inside a validated section) | `422 validation` |
| Reserved free-form section (`tls_settings`/`network_settings`/`multiplex`/`utls`/`obfs_settings`) containing arbitrary keys | accepted, stored verbatim, inert |
| Reality generation without admin session | `401 unauthorized` |

## 4.2 AnyTLS

- Fourth protocol (`anytls`), reuses the Hysteria2 TLS-certificate pattern: allowlist is `tls.server_name` (plain) + `certificate`/`private_key` (secret) + reserved `password` (secret) + optional `padding_scheme`; hy2-only fields (`bandwidth`/`obfs`/`hop_interval`) and vless fields are rejected.
- TLS validation is shared with Hysteria2 via `validateTLSMaterial` in `internal/web/node_settings.go` (key pair match, validity window, `VerifyHostname`).
- User UUID is the anytls password; sing-box inbound is `type: anytls` with inline PEM `certificate`/`key` split on `\n` (same shape as hy2).
- Subscriptions: `anytls://<uuid>@host:port?sni=<server_name>#<name>` URI, Clash `{type: anytls, password, sni, skip-cert-verify: false, udp: true}`, template placeholder `__ANYTLS_PROXIES__`.
- Requires sing-box >= 1.12 on agents (image pins 1.14.1).
- Adding a new protocol requires a DB migration that rebuilds `nodes` to widen the `protocol` CHECK.

## 4.1 Extended Hysteria2 Settings (obfs / hop)

Both are **plain settings** (never `secretFields`) and optional everywhere (create + patch):

- `obfs` section `{open, type, password}`: `password` 1-64 chars, `type` empty or `salamander`; when enabled with a password it renders `"obfs": {"type":"salamander","password":...}` in the sing-box inbound and adds `obfs=salamander&obfs-password=` (URI) / `obfs`+`obfs-password` (Clash) to subscriptions.
- `hop_interval`: `start-end` string, 1-65535, start<=end. **Subscription-only** (`mport` URI param, `ports` Clash field) — must never enter the sing-box inbound; server-side NAT redirect is the admin's job.
- VLESS flow is hardcoded to `xtls-rprx-vision`, the Reality handshake target is `reality_settings.server_name:server_port` (default `443`), and `public_key` is derived from `private_key`; configurable client `flow`/`dest` were considered and rejected as unnecessary surface area.
- List/user-side `nodeDTO`s never echo settings; only the admin detail endpoint (`GET /api/nodes/{id}`) echoes public `protocol_settings` as `settings` (secrets never). The edit form therefore shows stored public values as-is and submits them back; secret fields stay "blank = keep current", and there is still no way to clear a once-set field via the UI.


## 5. Good/Base/Bad Cases

- Good: generate a keypair, submit its private key with `reality_settings.server_name`, then copy the public key before closing the form.
- Base: create Hysteria2 with no bandwidth settings; both limits are optional.
- Good update: submit only `reality_settings.short_id`; existing `reality_settings.server_name` and encrypted `private_key` remain unchanged.
- Bad: replace public settings with only the submitted map or clear `secret_enc` when an update omits secret fields.

## 6. Tests Required

- Decode the generated private key, derive its X25519 public key, and assert it equals the response public key.
- Assert generated Short ID decodes to exactly eight bytes.
- Assert generation requires admin authentication and changes neither storage nor Server revision.
- Assert malformed VLESS keys, missing `reality_settings.server_name`, malformed Short IDs, invalid `rate`/`tags`, and invalid Hysteria2 bandwidth return `422 validation`.
- Assert a partial VLESS update retains the encrypted private key and omitted public settings.
- Use valid generated X25519 keys in Web, Agent, and E2E fixtures; placeholder strings are intentionally rejected.

## 7. Wrong vs Correct

### Wrong

```go
plain := parsed
secretFields := map[string]any{}
```

This turns an update into replacement and silently deletes omitted settings and secrets.

### Correct

```go
plain := decodeStoredSettings(currentSettings)
secretFields := decryptStoredSecrets(currentSecret)
mergeValidatedFields(plain, secretFields, parsed)
```

Start from stored values, merge only allowlisted supplied fields, validate the complete result, then persist it.

## 8. Shared nodeDTO Population

- `nodeDTO` is serialized by every node-returning path: `GET /api/nodes`, `GET /api/servers/{id}`, `GET /api/users/{id}/nodes`, and the create/update node responses.
- When a field is added to `nodeDTO` (e.g. `server: {id, name}`), EVERY query path that feeds it must populate it: `ListNodesPage`, `ListNodesByServer`, `ListNodesByIDs`, plus the create/update handlers. A path that skips population emits a zero-value object and silently violates the contract.
- Prefer one shared SELECT helper (join `servers`, shared row scanner) across node list queries instead of per-method inline SQL.
- Regression pattern: handler test asserts the field is present and non-zero for each endpoint family.
