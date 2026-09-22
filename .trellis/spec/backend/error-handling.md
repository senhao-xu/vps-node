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
- `/api/agent/*`: Bearer token hashed → `agents.token_hash` lookup; guard in `requireAgent` (register is the only unauthenticated agent route, via single-use server register token).
- Neither boundary accepts the other's credentials — regression test `TestAdminAndAgentAuthScopesDoNotCross` must keep passing.

---

## Secret Handling

- bcrypt: admin passwords, agent token hashes, user token hashes. Plaintext agent/user tokens returned exactly once (create/reset/rotate responses only).
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

- Auth: login success/failure/lockout, cookie flags, logout invalidation, rotated agent token 401, cross-boundary 401.
- Envelope: every 4xx path asserts code + shape; agent payload tests assert unknown JSON fields stay ignored (`decodeJSON`, per the binding `api-contract.md`) and can never influence ownership or telemetry totals.
