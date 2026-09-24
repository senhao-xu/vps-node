# Error Handling

> Error envelope, validation rules, and the auth boundary.

---

## Overview

All API errors use the contract envelope (docs/api-contract.md is binding):

```json
{"error": {"code": "...", "message": "..."}}
```

Codes: `invalid_request | unauthorized | forbidden | not_found | conflict | payload_too_large | validation | rate_limited | internal`. Helpers in `internal/web/respond.go`.

---

## Validation & Error Matrix

| Condition | Response |
|---|---|
| Malformed JSON body | 400 invalid_request |
| Body > 1 MB | 413 payload_too_large |
| Field validation failure (ranges, enums, negative counters) | 422 validation |
| Resource missing / not owned by caller | 404 not_found |
| Unique conflict (port, name, token) | 409 conflict |
| No/invalid admin session | 401 unauthorized (cookie: HttpOnly, SameSite=Lax, Secure when TLS) |
| No/invalid/rotated agent bearer | 401 unauthorized |
| Agent acting outside its bound server | 401/404 (server_id always derived from credential, body `server_id` NEVER trusted) |
| Login rate-limit lockout | 429 rate_limited |
| Config pull on disabled server | 403 forbidden |

---

## Auth Boundary (invariant)

- `/api/admin/*` + business routes: DB-backed admin session cookie; guard in `requireAdmin`. Login limiter: in-memory per-IP+username lockout.
- `/api/agent/*`: Bearer **agent key** hashed → `agents.key_hash` lookup; guard in `requireAgent`. There is no unauthenticated agent route (the one-time `POST /api/agent/register` flow was removed; the key is auto-issued when a Server is created and can be re-read/reset on the Server detail page via `GET`/`POST /api/servers/:id/agent-key`). The panel derives `server_id` from the key; the agent asserts the `server_id` returned by heartbeat equals its configured `server_id` and exits on mismatch.
- Neither boundary accepts the other's credentials — regression test `TestAdminAndAgentAuthScopesDoNotCross` must keep passing.

---

## Secret Handling

- bcrypt / SHA-256: admin passwords (bcrypt), user token hashes, agent key hash (`agents.key_hash`). Plaintext user tokens are returned exactly once (create/reset/rotate).
- The agent key is **not** one-time: its SHA-256 is stored for auth lookup (`key_hash`) and its plaintext is stored AES-256-GCM-encrypted (`key_enc`, panel `app_key`) so an admin can re-read it at any time; `POST /api/servers/:id/agent-key` resets it and invalidates the previous key immediately. It is auto-issued on `POST /api/servers` (201 body carries `agent_key`).
- AES-256-GCM (`internal/secrets`, key = config `app_key`): recoverable protocol secrets (`nodes.secret_enc` BLOB).
- Shadowsocks per-user passwords are NOT stored: derived `ss-cred-v1` = base64(HMAC-SHA256(app_key, "ss-cred-v1:"+nodeID+":"+userUUID)) truncated to method key length. Deterministic; tested.
- Never log: rendered sing-box payloads, tokens, passwords, decrypted secrets. Request logger records path only, not query strings.

## Forbidden Pattern

```go
// Wrong: trusting client-declared ownership
serverID := req.Body.ServerID
// Correct: derive from credential bound in middleware context
serverID := AgentServerID(r.Context())
```

---

## Scenario: Server create auto-issues Agent Key

### 1. Scope / Trigger
- Trigger: any change to `POST /api/servers` identity behavior or the Server↔Agent credential bootstrap.

### 2. Signatures
- `POST /api/servers {"name": "..."}`.
- Repo: `CreateServerWithAgentKey(ctx, name, status, keyHash string, keyEnc []byte) (int64, error)`.

### 3. Contracts
- Request: `name` (1-128 chars).
- Response `201`: server DTO plus `agent_key` (plaintext). The same value is returned by `GET /api/servers/:id/agent-key`. No server revision bump.
- The key is usable immediately for `Authorization: Bearer <agent_key>` on `/api/agent/*`.

### 4. Validation & Error Matrix
- Invalid `name` → 422 validation.
- Key generation/encryption failure → 500 internal and the server row is rolled back (no key-less residue).
- Repo-direct `CreateServer` (no key) → `GET /api/servers/:id/agent-key` still `409 conflict`.

### 5. Good/Base/Bad Cases
- Good: server row + agent binding written in one tx; 201 returns the key.
- Base: admin later re-reads or resets the key on the Server detail page.
- Bad: writing the server and the key in separate transactions, leaving a key-less server on partial failure.

### 6. Tests Required
- `TestServerCreateGeneratesAgentKey`: 201 `agent_key` non-empty, `GET` returns the same value, key authenticates `/api/agent/heartbeat`, revision stays 0.
- `TestServerAgentKeyLifecycle` / `TestAgentKeyGetConflictsWithoutStoredKey`: unchanged 409 semantics.

### 7. Wrong vs Correct
#### Wrong
```go
id, _ := repo.CreateServer(ctx, name, active)   // separate tx
repo.UpsertAgentKey(ctx, id, hash, enc)          // failure leaves a key-less server
```

#### Correct
```go
id, _ := repo.CreateServerWithAgentKey(ctx, name, active, hash, enc) // one tx
```

---

## Tests Required

- Auth: login success/failure/lockout, cookie flags, logout invalidation, reset agent key 401, cross-boundary 401.
- Agent key lifecycle: server creation auto-issues a usable key (201 `agent_key` authenticates `/api/agent/heartbeat`; `GET` returns the same value; server revision not bumped) → generate → GET reveals plaintext → reset invalidates the old key (401) → GET with no `key_enc` (migrated agent) returns `409 conflict`. Repo-direct `CreateServer` (test helper `seedServer`) intentionally issues no key, preserving the pre-generation 409 path.
- Envelope: every 4xx path asserts code + shape; agent payload tests assert unknown JSON fields stay ignored (`decodeJSON`, per the binding `api-contract.md`) and can never influence ownership or telemetry totals.
