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

---

## Scenario: Custom node entries view & subscription refresh

### 1. Scope / Trigger
- Trigger: any change to `GET /api/custom-nodes/{id}/nodes` or `POST /api/custom-nodes/{id}/refresh`, or to `internal/subscription.Summarize` / `SummarizeEntries` / `looseInt`.

### 2. Signatures
- `GET /api/custom-nodes/{id}/nodes` — read-only digest of a custom node's stored content.
- `POST /api/custom-nodes/{id}/refresh` — force-fetch a `subscription` source.
- `subscription.Summarize(links []string, proxies []map[string]any) ([]NodeSummary, []string)` (`internal/subscription/summary.go`).
- `subscription.SummarizeEntries(appKey []byte, links []string, proxies []map[string]any) ([]EntrySummary, []string)` (`internal/subscription/entrykey.go`) — attaches a stable `Key` per entry.

### 3. Contracts
- Response `{source_type, has_cache, fetched_at, entries:[{key,name,type,server,port}], skipped?}`.
- `key` is the per-entry authorization id: `HMAC-SHA256(app_key)` hex of the entry's canonical connection JSON with the display `name` removed; it survives upstream renames. Unparseable entries have no key and are listed in `skipped`.
- View performs **no network IO**: `subscription` reads `custom_nodes.cached_content` only; a cache miss returns `entries:[]`, `has_cache:false`. `links` parses `content_enc`, so `has_cache:false`, `fetched_at:null` always.
- Refresh ignores the render-path 5-minute `CacheTTL`, calls `subscription.FetchSubscription`, writes the cache + `fetched_at`, then returns the same shape with `has_cache:true`.
- `type` is the Clash proxy type (`ss`/`vless`/`hysteria2`/`anytls`/`http`/`trojan`/`vmess`); `skipped` carries the raw link line or `name`/`proxy #i` for unparseable/incomplete entries.

### 4. Validation & Error Matrix
| Condition | Result |
|---|---|
| Unknown id (view or refresh) | `404 not_found` |
| Refresh a `links` source | `422 validation` |
| Upstream fetch failure (non-2xx/timeout/too large) | `500 internal` + readable message; cache + `fetched_at` untouched |
| No admin session | `401 unauthorized` |

### 5. Good/Base/Bad Cases
- Good: refresh a subscription, `fetched_at` advances, entries reflect the new upstream content.
- Base: view a subscription that was never fetched → empty entries, no upstream request.
- Bad: refresh clears the cache before the fetch succeeds, so a later render loses the stale fallback.

### 6. Tests Required
- View links parses from `content_enc`; view subscription-with-cache makes zero upstream hits; view without cache makes zero upstream hits.
- Refresh success updates `fetched_at` + returns entries; refresh links → `422 validation`; upstream failure keeps the old cache and `fetched_at`; both routes `401` unauthenticated and `404` unknown id.

### 7. Wrong vs Correct
#### Wrong
```go
content, err := subscription.FetchSubscription(ctx, url)
if err != nil { return err }                 // cache already cleared → stale fallback lost
```
#### Correct
```go
content, err := subscription.FetchSubscription(ctx, url)
if err != nil { writeErr(w, errInternal("failed to fetch upstream subscription: "+err.Error())); return }
_ = h.repo.UpdateCustomNodeCache(ctx, cn.ID, content, time.Now().Unix())
```

### Common Mistake: Clash port decoded as `int`
`yaml.v3` decodes an integer `port:` as `int`, not `float64`; `looseInt` must therefore handle `int`/`int64` or valid Clash proxies get classified as `skipped`. Cover port as `int`/`int64`/`float64`/`string` in `summary_test.go`.

---

## Scenario: Custom node upstream fetch options (User-Agent / TLS)

### 1. Scope / Trigger
- Trigger: any change to `custom_nodes.user_agent` / `custom_nodes.insecure_skip_verify`, `subscription.FetchSubscription`, or the custom-node create/update payloads.

### 2. Signatures
- `custom_nodes.user_agent TEXT NOT NULL DEFAULT ''` (migration `0008_custom_nodes_user_agent.sql`).
- `custom_nodes.insecure_skip_verify INTEGER NOT NULL DEFAULT 0` (migration `0009_custom_nodes_insecure_tls.sql`).
- `subscription.DefaultUserAgent = "clash-verge/v2.0.0"`.
- `FetchSubscription(ctx, rawURL, userAgent string, insecureSkipVerify bool) (string, error)` (`internal/subscription/fetch.go`).
- Create/update custom-node payloads accept optional `user_agent` and `insecure_skip_verify`.

### 3. Contracts
- Empty `user_agent` means “use `DefaultUserAgent`”; it is a valid stored value.
- Only `subscription` sources carry a UA; `links` forces empty and never sends one.
- The same UA and TLS flag are used by BOTH upstream fetch paths: render lazy fetch (`fetchCustomNodeContent`) and manual refresh (`POST /api/custom-nodes/{id}/refresh`).
- `insecure_skip_verify=true` selects a client whose `tls.Config.InsecureSkipVerify` is set; `false` (default) uses the system trust store. It is a per-node admin opt-in; `links` forces `false`.
- `customNodeDTO.user_agent` / `insecure_skip_verify` are returned by list / user-custom-nodes / create / update.
- Changing the UA or the TLS flag clears `cached_content`/`fetched_at` (same as changing content), so the next render/refresh re-fetches.

### 4. Validation & Error Matrix
| Condition | Result |
|---|---|
| `user_agent` longer than 255 bytes | `422 validation` |
| `user_agent` contains a control char (`< 0x20` or `0x7f`, incl. CR/LF) | `422 validation` |
| `user_agent` empty | accepted → default UA |
| `user_agent` / `insecure_skip_verify` sent on a `links` source | ignored (stored `""` / `false`) |
| upstream cert not in the trust store and `insecure_skip_verify=false` | fetch fails (rendered as `500 internal` on refresh, stale cache on render) |

### 5. Good/Base/Bad Cases
- Good: a node with `user_agent="Mihomo/1.18.0"` sends that header on refresh and on render; changing it clears the cache.
- Good: a self-signed upstream node with `insecure_skip_verify=true` fetches successfully.
- Base: `user_agent=""` sends `clash-verge/v2.0.0`; `insecure_skip_verify=false` verifies certificates.
- Bad: enabling `insecure_skip_verify` by default or globally — it disables MITM protection for every subscription.

### 6. Tests Required
- `FetchSubscription`: explicit UA is received by the upstream; empty UA sends `DefaultUserAgent`; a self-signed (`httptest.NewTLSServer`) upstream fails without the flag and succeeds with it.
- Web: create/list/update return `user_agent` + `insecure_skip_verify`; refresh + render lazy fetch send the configured UA/TLS flag; UA/TLS change clears cache; `links` ignores both; control-char/overlong UA → `422 validation`.
- Repo: all read paths return both columns; `UpdateCustomNode` clears cache on UA/TLS change and preserves it otherwise.
- DB: migration upgrade on a pre-0008/pre-0009 row defaults `user_agent` to `''` and `insecure_skip_verify` to `0`; migrations are idempotent.

### 7. Wrong vs Correct
#### Wrong
```go
req.Header.Set("User-Agent", "vps-node-panel")   // hardcoded; ignores the node's stored UA
http.DefaultClient.Do(req)                        // never honors the per-node TLS flag
```
#### Correct
```go
ua := userAgent
if ua == "" { ua = DefaultUserAgent }
req.Header.Set("User-Agent", ua)
client := fetchClient
if insecureSkipVerify { client = insecureFetchClient }
resp, err := client.Do(req)
```

---

## Scenario: Custom node share & content echo

### 1. Scope / Trigger
- Trigger: any change to `GET /api/custom-nodes/{id}/share`, `GET /api/custom-nodes/{id}/content`, or `subscription.RenderCustomProxies` / `subscription.RenderProxiesFragment`.

### 2. Signatures
- `GET /api/custom-nodes/{id}/share` — admin export of one source, no user.
- `GET /api/custom-nodes/{id}/content` — admin echo of one source's decrypted content.
- `subscription.RenderCustomProxies(source CustomSource, onSkip func(sourceID int64, item string, err error)) []map[string]any` — converts one source's links + upstream proxies into Clash proxies; `RenderClashFilteredMerged` calls the shared unexported `renderCustomProxies(source, usedNames, onSkip)` so merged output stays byte-identical while the export uses a fresh name set.
- `subscription.RenderProxiesFragment(proxies []map[string]any) ([]byte, error)` — `yaml.Marshal({"proxies": proxies})`; empty input renders `proxies: []\n`.

### 3. Contracts
- Share response `{source_type, has_cache, fetched_at, clash, links, skipped?}`.
- `clash` is a YAML `proxies:` fragment built exactly like the Clash subscription renderer (links parsed via `ParseShareURI`, upstream proxies appended, duplicate names suffixed with the source id).
- `links` is the plaintext newline items (**never** base64). A Clash-only upstream yields an empty `links`.
- `subscription` share uses the render-path lazy fetch (`fetchCustomNodeContent`: TTL cache, stale fallback); a failed fetch returns the stale/empty content with `200`, not `500`.
- `skipped` carries the raw link line, or `proxy #i` for a nameless upstream proxy.
- Content response `{content}`: `links` → original link text, `subscription` → original upstream URL.
- Neither list endpoint (`GET /api/custom-nodes`, `GET /api/users/{id}/custom-nodes`) returns content; echo goes through the single-node content route only.

### 4. Validation & Error Matrix
| Condition | Result |
|---|---|
| Unknown id (share or content) | `404 not_found` |
| No admin session | `401 unauthorized` |
| Decrypt failure | `500 internal` |
| No cache / no content | `200` empty-but-successful |

### 5. Good/Base/Bad Cases
- Good: `links` source share returns a proxies fragment plus the plaintext lines; an unparseable line lands in `skipped` without breaking the rest.
- Base: a `links` source that cannot convert any line returns `clash: "proxies: []\n"`, `links: [...]`, `skipped: [...]`.
- Bad: base64-encoding `links` (siblings render base64 for `flag=general`, but the admin export is deliberately plaintext) or routing a custom source through the user-credentialed render path.

### 6. Tests Required
- `internal/web/custom_nodes_share_test.go`: links share (fragment + plaintext + `skipped`); subscription share populates `has_cache` via lazy fetch; base64-links vs Clash-only upstream (`links` empty, `clash` non-empty); content echo for both source types; both routes `401` unauthenticated and `404` unknown id; list has no `content` field.
- `internal/subscription/custom_test.go` gate tests (`TestRenderClashFilteredWithoutCustomMatchesLegacy`, `TestRenderClashFilteredMergedCustomSources`) must keep passing after the extraction.

### 7. Wrong vs Correct
#### Wrong
```go
body := subscription.RenderGeneralLinksMerged(h.appKey, subscription.User{ID: u.ID, UUID: u.UUID}, nodes, custom, skip) // user-scoped; needs credentials
```
#### Correct
```go
proxies := subscription.RenderCustomProxies(subscription.CustomSource{ID: cn.ID, Name: cn.Name, Links: links, Proxies: proxies}, onSkip)
fragment, _ := subscription.RenderProxiesFragment(proxies)
```
