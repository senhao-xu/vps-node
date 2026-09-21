# Research: XBoard subscription implementation

- **Query**: 研究 XBoard 的订阅 URL、token、格式协商、协议字段、状态行为、后台入口，并与本项目需求对照。
- **Scope**: mixed（公开 GitHub 源码 + 本地项目源码）
- **Date**: 2026-09-21
- **Source revision**: `cedar2025/Xboard` clone, `master` at commit `4f48e61a2cbc6db5338872b6bdb45ef954ec1256` (no tag was present in the shallow clone)

## Findings

### Files Found

| File Path | Description |
|---|---|
| `Xboard/routes/web.php:90-92` | Public subscription route: configurable `subscribe_path` (default `s`) plus `/{token}`, with `client` middleware. |
| `Xboard/app/Http/Middleware/Client.php:19-31` | Reads token from query `token` or route token and performs plaintext `User::where('token', ...)` lookup. |
| `Xboard/app/Http/Controllers/V1/Client/ClientController.php:34-87` | Availability check, server selection, format/client dispatch, type and keyword filters. |
| `Xboard/app/Utils/Helper.php:123-142` | Generates subscription URL from token and configurable base URL(s). |
| `Xboard/app/Http/Controllers/V2/Admin/UserController.php:205-213,350-370` | Admin user list/CSV includes generated subscription URL. |
| `Xboard/app/Http/Controllers/V1/User/UserController.php:145-172` | User API returns URL; `resetSecurity` changes UUID and token and returns the new URL. |
| `Xboard/app/Protocols/General.php:155-245,324-367` | VLESS, Hysteria v1/v2 URI rendering. |
| `Xboard/app/Protocols/ClashMeta.php:404-510,589-628` | Clash Meta VLESS and Hysteria YAML field mapping. |
| `Xboard/app/Protocols/Shadowsocks.php:16-58` | SIP008 JSON Shadowsocks output. |
| `Xboard/app/Protocols/Clash.php:23-106` | Clash YAML output and response headers. |
| `Xboard/app/Protocols/SingBox.php:10-126,135-179` | sing-box JSON output, supported flags and user-info headers. |
| `Xboard/app/Services/UserService.php:48-64` | User availability and traffic eligibility. |
| `Xboard/app/Services/ServerService.php:50-105` | Available-node query, including banned server/user and traffic/expiry checks. |
| Local `internal/repo/users.go:151-161` | This project’s analogous eligibility query. |
| Local `internal/web/agent_config.go:210-224` | This project’s protocol credential derivation: SS password, VLESS UUID and Hysteria2 password. |

### URL, authentication, and token lifecycle

- Default URL shape is `https://<panel>/<s>/<user-token>`; both the path segment and `subscribe_path` are configurable (`routes/web.php:90-92`). `Helper::getSubscribeUrl` appends the route to one configured base URL, randomly selecting from a comma-separated `subscribe_url` list when configured (`app/Utils/Helper.php:123-142`).
- The token is the user’s normal `v2_user.token`, not a separate subscription credential. It is looked up directly as plaintext (`Client.php:21-30`), unlike this task’s `token_hash` model. The query-string form `?token=...` is also accepted by the middleware.
- XBoard uses `Helper::guid()` when creating/resetting user security; the admin and user APIs dynamically reconstruct the URL from the current token. `resetSecurity` changes both UUID and token and returns the new URL (`V1/User/UserController.php:164-173`). Thus an old URL is invalidated by reset, and the current token is re-readable by authorized API/admin responses; it is not a one-time-display secret.
- No separate hash-only subscription-token or explicit rotation endpoint was found in the inspected paths. XBoard’s URL is effectively bearer authentication and token reset is coupled to the user UUID reset.

### Output format negotiation and supported formats

- The route itself has no format suffix. `ClientController::getClientInfo` lowercases `flag` query parameter, otherwise the `User-Agent`, extracts a client name/version, and matches it against registered protocol flags (`ClientController.php:148-190`).
- `types` query parameter restricts protocol types; values are split on `|`, comma, or full-width comma, and `all` means all valid types (`ClientController.php:91-105`). `filter` similarly filters node name/tags (`:110-145`).
- The selected client protocol class is instantiated at `ClientController.php:61-87`. The inspected built-in flags/classes are: `clash` → Clash YAML; `meta`, `verge`, `flclash`, `nekobox`, `clashmetaforandroid` → Clash Meta; `sing-box`, `hiddify`, `sfm` → sing-box JSON; plus `shadowrocket`, `quantumultx`, `surge`, `loon`, `stash`, `surfboard`, and `shadowsocks` protocol handlers. Unknown/no flag falls back to `General`, which emits a line-oriented collection of URI links.
- `General` covers VLESS, Trojan, Hysteria and other URI types. `Clash`/`ClashMeta` emit YAML (`content-type: text/yaml` in `Clash.php:99-106`), `SingBox` emits JSON (`SingBox.php:121-125`), and `Shadowsocks` emits SIP008 JSON (`Shadowsocks.php:44-45`). Templates are configurable in `v2_subscribe_templates` and admin config exposes singbox/clash/clashmeta/stash/surge/surfboard template keys (`ConfigController.php:197-221`).
- Responses commonly include `subscription-userinfo: upload=...; download=...; total=...; expire=...`; sing-box also sets `profile-update-interval: 24`, and Clash adds content disposition and profile page URL (`SingBox.php:121-125`, `Clash.php:101-106`).

### Protocol field mapping

#### VLESS Reality

- URI form in `General::buildVless` is `vless://<uuid>@<host>:<port>?<query>#<name>` (`General.php:240-244`). User credential is UUID; this revision uses the user UUID, not the subscription token.
- Reality maps `security=reality`, `pbk=<public_key>`, `sid=<short_id>`, `sni` and `servername=<server_name>`, `spx=/`; optional uTLS fingerprint is `fp` (`General.php:186-195`). Network-specific fields include WS `path`/`host`, gRPC `serviceName`, H2 `path`/`host`, and HTTP upgrade/xHTTP path/host/mode/extra (`:200-237`).
- Clash Meta maps `server`, `port`, `uuid`, `flow`, `encryption`, `tls=true`, `servername`, and `reality-opts.public-key`/`short-id`; transport maps to `network`, `ws-opts`, `grpc-opts`, `h2-opts`, or `xhttp-opts` (`ClashMeta.php:404-510`). The private Reality key is not rendered.

#### Shadowsocks

- SIP008 output contains `id`, `remarks`, `server`, `server_port`, `password`, and `method` (`Shadowsocks.php:48-58`). The password is server credential data in XBoard’s server model.
- This project differs: `internal/web/agent_config.go:212-218` derives a per-node/per-user password from the app key, node ID, UUID and method (`ss-cred-v1`), rather than exposing a stored server password. A renderer must preserve that contract.

#### Hysteria2

- URI form is `hysteria2://<password>@<host>:<port>?<query>#<name>` (`General.php:338-348`). Query fields include `sni`, `insecure`, optional `obfs=salamander`, `obfs-password`, and optional `mport`; Hysteria v1 instead uses `auth`, bandwidth and xplus obfs fields (`:349-362`).
- Clash Meta JSON/YAML-like mapping uses `type=hysteria2`, `server`, `port`, `sni`, `up`, `down`, `skip-cert-verify`, optional `ports`, and `password`; optional obfs and obfs-password are emitted (`ClashMeta.php:589-628`).
- This project’s agent contract uses the user UUID as Hysteria2 password (`internal/web/agent_config.go:221-222`), so XBoard’s server/password mapping cannot be copied literally.

### Availability, disabled/expired/quota behavior, and HTTP responses

- `ClientController::subscribe` calls `UserService::isAvailable`; if false it returns an empty body with HTTP **403** and `Content-Type: text/plain` (`ClientController.php:43-49`). Invalid/missing token is rejected by `Client` middleware as `ApiException(..., 403)` (`Client.php:21-28`), rather than a distinct 401.
- `isAvailable` requires not banned, nonzero `transfer_enable`, and expiry in the future or null (`UserService.php:48-54`). The node query additionally requires `u+d < transfer_enable`, expiry >= now/null, banned=0, and excludes servers where applicable (`ServerService.php:50-105`). Therefore a quota-exhausted user does not receive a usable subscription; the exact handling of the query path should be treated separately from the early `isAvailable` check because `isAvailable` itself only checks that total allowance is nonzero, not the remaining-byte comparison.
- XBoard’s source shows no special subscription response for a disabled node beyond server selection filtering; unavailable servers are omitted. Invalid user token, missing token, and unavailable user all use 403-family behavior in the inspected code. The repository does not establish a node-specific 404/503 response.
- Local project eligibility is explicit and stricter/clearer: `internal/repo/users.go:157-160` requires active status, expiry > now/null, and quota zero or used < quota. Local agent config also supplies only eligible users/nodes; any subscription endpoint should use those existing rules rather than XBoard’s slightly different `isAvailable` predicate.

### Admin location and client entry points

- In the inspected XBoard admin implementation, the user list transformation adds `subscribe_url` to each row (`V2/Admin/UserController.php:205-213`); CSV exports also include a “订阅地址” column (`:350-370`). This is the clearest admin copy/export entry point found. The user-facing dashboard API also returns `subscribe_url` (`V1/User/UserController.php:145-161`).
- `resources/views/client/subscribe.blade.php:260` displays the URL in a read-only input, indicating a user dashboard copy affordance. XBoard does not appear to have a separate per-client URL: one bearer URL selects output by User-Agent/`flag`, with optional `types`/`filter` query parameters.
- Multiple client “entries” are protocol flags/classes, not separate credentials or URL paths. The broad set includes Clash, Clash Meta derivatives, sing-box/Hiddify, Shadowrocket, Quantumult X, Surge, Loon, Stash, Surfboard, Shadowsocks, and generic URI output.

### Relevance to this project

The following are observations about applicability, not product-code changes:

- **Compatible design patterns**: one public subscription endpoint; client detection from User-Agent with an explicit override parameter; `types` filtering; JSON/YAML/URI renderers; standard `subscription-userinfo` and update headers; filtering authorized/eligible nodes before rendering; never rendering Reality private keys.
- **Do not copy directly**: XBoard stores and re-echoes a plaintext user token, while this project only stores `token_hash` and explicitly requires separate hash-only subscription credentials. XBoard couples token reset with UUID reset; this project requires explicit subscription rotation without changing the business token. XBoard’s SS and Hysteria credentials are server/user fields, while this project derives SS credentials and uses UUID-based VLESS/Hysteria2 credentials (`agent_config.go:210-224`). XBoard’s Laravel/Eloquent data model and broad protocol/template surface do not match the local Go/sing-box contract.
- **Status/security boundary**: XBoard’s bearer URL is a long-lived user secret and its request-log middleware only redacts selected POST request data; it is not evidence of safe query-string logging for this project. Local PRD requirements R3/R4/R7/R8 are stricter and should remain authoritative.

### First-version and later-extension evidence

- XBoard demonstrates a wide compatibility model, but its core dispatch already supports a useful staged shape: generic URI text (`General`), sing-box JSON (`SingBox`), and Clash Meta YAML (`ClashMeta`). The source does not define this project’s first-version choice; it only documents viable output families.
- XBoard’s later/template-driven expansion points are protocol-specific handlers, database-backed templates, client-version compatibility requirements, `types`/`filter` selectors, metadata headers, and many dedicated client formats. These are extension seams rather than requirements already present in the local codebase.

## External References

- [Xboard GitHub repository](https://github.com/cedar2025/Xboard) — public source inspected at commit `4f48e61a2cbc6db5338872b6bdb45ef954ec1256`.
- [Xboard subscription route](https://github.com/cedar2025/Xboard/blob/4f48e61a2cbc6db5338872b6bdb45ef954ec1256/routes/web.php#L90-L92) — configurable path and token route.
- [Xboard ClientController](https://github.com/cedar2025/Xboard/blob/4f48e61a2cbc6db5338872b6bdb45ef954ec1256/app/Http/Controllers/V1/Client/ClientController.php) — availability, UA/flag dispatch and filters.
- [Xboard protocol renderers](https://github.com/cedar2025/Xboard/tree/4f48e61a2cbc6db5338872b6bdb45ef954ec1256/app/Protocols) — built-in format-specific output implementations.

## Related Specs

- `.trellis/tasks/09-21-user-subscription-links/prd.md` — local requirements R1–R9 and open question on first output format.
- Local `internal/repo/users.go` and `internal/web/agent_config.go` — existing eligibility and credential contracts that bound any analogous implementation.

## Caveats / Not Found

- The active-task CLI reported no current task, but the user supplied `/root/vps-node/.trellis/tasks/09-21-user-subscription-links`; output was persisted there as explicitly requested.
- The XBoard shallow clone had no tag list available; commit SHA is recorded above. Findings describe source at that revision, not an immutable release tag.
- No ghcr image inspection was needed because the public repository source was available. No definitive node-disabled HTTP status was found; source only establishes omission during server selection and 403 for unavailable users/invalid tokens.
- XBoard’s quota behavior has a subtle split: `isAvailable` checks nonzero total allowance, while `ServerService` checks remaining traffic. Do not infer an exact empty-body/HTTP response for quota exhaustion without exercising the full deployed route and version.
