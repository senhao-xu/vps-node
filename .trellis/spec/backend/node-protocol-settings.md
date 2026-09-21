# Node Protocol Settings

> Executable contracts for node settings, Reality key generation, and partial updates.

## 1. Scope / Trigger

- Applies whenever an admin node API, protocol setting, sing-box node renderer, or node form changes.
- Node settings contain both public JSON (`nodes.settings`) and encrypted recoverable secrets (`nodes.secret_enc`); changes must preserve both halves consistently.

## 2. Signatures

- `POST /api/nodes/reality-keypair` generates, but does not persist, a Reality keypair and Short ID.
- `POST /api/nodes` creates a complete protocol configuration and transactionally bumps the owning Server revision.
- `PUT /api/nodes/{id}` applies a settings patch and transactionally bumps the owning Server revision.

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
- VLESS secrets remain AES-GCM encrypted at rest and never appear in generic Node DTOs.
- Update `settings` is a patch: omitted public fields and encrypted secret fields retain their stored values.

## 4. Validation & Error Matrix

| Condition | Result |
|---|---|
| Shadowsocks method missing or unsupported | `422 validation` |
| VLESS private key missing, malformed, or not 32 decoded bytes | `422 validation` |
| VLESS Server Names empty or contain invalid entries | `422 validation` |
| VLESS Short ID is odd-length, non-hex, or over 8 bytes | `422 validation` |
| Hysteria2 bandwidth is negative, fractional, or not numeric | `422 validation` |
| Hysteria2 `obfs_password` over 64 chars | `422 validation` |
| Hysteria2 `hop_ports` not `start-end`, out of 1-65535, or start > end | `422 validation` |
| Unknown protocol setting field | `422 validation` |
| Reality generation without admin session | `401 unauthorized` |

## 4.1 Extended Hysteria2 Settings (obfs / hop)

Both are **plain settings** (never `secretFields`) and optional everywhere (create + patch):

- `obfs_password`: 1-64 chars; non-empty renders `"obfs": {"type":"salamander","password":...}` in the sing-box inbound and adds `obfs=salamander&obfs-password=` (URI) / `obfs`+`obfs-password` (Clash) to subscriptions.
- `hop_ports`: `start-end` string, 1-65535, start<=end. **Subscription-only** (`mport` URI param, `ports` Clash field) — must never enter the sing-box inbound; server-side NAT redirect is the admin's job.
- VLESS flow is hardcoded to `xtls-rprx-vision` and the Reality handshake target is always `server_names[0]:443`; configurable `flow`/`dest` settings were considered and rejected as unnecessary surface area.
- Node DTOs never echo settings, so the edit form treats every new field as "blank = keep current"; there is no way to clear a once-set field via the UI.


## 5. Good/Base/Bad Cases

- Good: generate a keypair, submit its private key with a Server Name, then copy the public key before closing the form.
- Base: create Hysteria2 with no bandwidth settings; both limits are optional.
- Good update: submit only `short_id`; existing `server_names` and encrypted `private_key` remain unchanged.
- Bad: replace public settings with only the submitted map or clear `secret_enc` when an update omits secret fields.

## 6. Tests Required

- Decode the generated private key, derive its X25519 public key, and assert it equals the response public key.
- Assert generated Short ID decodes to exactly eight bytes.
- Assert generation requires admin authentication and changes neither storage nor Server revision.
- Assert malformed VLESS keys, missing Server Names, malformed Short IDs, and invalid Hysteria2 bandwidth return `422 validation`.
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
