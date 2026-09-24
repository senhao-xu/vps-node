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
- `/api/agent/*`: Bearer **agent key** hashed → `agents.key_hash` lookup; guard in `requireAgent`. There is no unauthenticated agent route (the one-time `POST /api/agent/register` flow was removed; the key is issued by an admin on the Server detail page). The panel derives `server_id` from the key; the agent asserts the `server_id` returned by heartbeat equals its configured `server_id` and exits on mismatch.
- Neither boundary accepts the other's credentials — regression test `TestAdminAndAgentAuthScopesDoNotCross` must keep passing.

---

## Secret Handling

- bcrypt / SHA-256: admin passwords (bcrypt), user token hashes, agent key hash (`agents.key_hash`). Plaintext user tokens are returned exactly once (create/reset/rotate).
- The agent key is **not** one-time: its SHA-256 is stored for auth lookup (`key_hash`) and its plaintext is stored AES-256-GCM-encrypted (`key_enc`, panel `app_key`) so an admin can re-read it at any time; `POST /api/servers/:id/agent-key` resets it and invalidates the previous key immediately.
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

## Tests Required

- Auth: login success/failure/lockout, cookie flags, logout invalidation, reset agent key 401, cross-boundary 401.
- Agent key lifecycle: generate → GET reveals plaintext → reset invalidates the old key (401) → GET with no `key_enc` (migrated agent) returns `409 conflict`.
- Envelope: every 4xx path asserts code + shape; agent payload tests assert unknown JSON fields stay ignored (`decodeJSON`, per the binding `api-contract.md`) and can never influence ownership or telemetry totals.
