# Technical Design

## Boundaries

This feature adds a public subscription read path, admin subscription management, two client renderers, restricted global Clash Meta overrides, and complete Hysteria2 TLS material. It does not add a user portal, change Agent authentication, or introduce a general-purpose template engine.

The Panel remains the only owner of users, authorization, protocol secrets, and subscription credentials. The Agent continues to receive a complete rendered sing-box configuration through the existing revisioned config endpoint.

## Persistence

Add a `user_subscriptions` table instead of rebuilding `users`:

```sql
CREATE TABLE user_subscriptions (
    user_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    token_enc BLOB NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);
```

`token_hash` is `adminauth.HashToken(token)` and is used for lookup. `token_enc` is AES-256-GCM ciphertext produced by `internal/secrets` with the Panel `app_key`; it exists only so an authenticated administrator can retrieve the current URL. Generation and rotation write both values atomically. Rotation replaces the row so the old hash stops resolving immediately.

New-user creation generates the business Token and subscription Token in the same transaction as the user and initial node authorizations. Existing users have no migration row and create one explicitly from the detail page.

The existing `settings` table stores:

- `subscribe_urls`: comma-separated validated HTTP(S) origins, empty by default.
- `subscribe_path`: one safe path segment, default `s`.
- `clash_meta_template`: one validated restricted YAML document with a built-in default.

## API Contracts

Admin routes remain behind `requireAdmin`:

```text
GET  /api/users/{id}/subscription
POST /api/users/{id}/subscription
POST /api/users/{id}/subscription/rotate
```

`GET` returns `{ "configured": false, "url": null }` for a historical user with no credential, or `{ "configured": true, "url": "https://.../s/<token>" }`. `POST` creates only when absent and returns the configured shape; an existing row returns `409 conflict`. `rotate` requires an existing row, atomically replaces it, and returns the new URL.

User creation response adds `subscription_url` beside the existing one-time business `token`. Generic user list/detail DTOs remain unchanged.

Public route:

```text
GET /{subscribe_path}/{token}?flag=general|clash-meta
```

The dispatcher reads the validated `subscribe_path` setting on every request, so changing it takes effect immediately without a Panel restart. The default route is `/s/{token}`.

Format selection:

- Explicit `flag=general` or `flag=clash-meta` wins.
- Mihomo, Clash Meta, Clash Verge, FlClash, NekoBox and compatible User-Agents select `clash-meta`.
- Unknown clients select `general`.
- Unknown flags return `400 invalid_request`.

Public response behavior:

- Unknown or rotated token: `404 not_found` without revealing whether a user exists.
- Disabled, expired, not-yet-started, or quota-exhausted user: `403 forbidden`.
- Eligible user with no eligible nodes: successful format-specific empty subscription.
- Renderer/storage failure: `500 internal` without secret material.

The response includes `subscription-userinfo` with used bytes, total quota, and expiry when present. It never includes management credentials or server-side private keys.

## URL Construction

`subscribe_urls` follows the XBoard link behavior but uses validated origins. Each value must be an absolute `http` or `https` URL with no userinfo, query, fragment, or non-root path. When configured, one origin is selected using `crypto/rand`; otherwise the admin request Origin is built from its scheme and Host. The public subscription renderer does not reconstruct its own URL.

`subscribe_path` must match `[A-Za-z0-9_-]{1,32}`. The public dispatcher reads the validated setting on every request, so a successful settings update changes the active path immediately without a restart.

## Eligibility And Node Selection

Create one repository query that resolves a subscription token hash to the user and lists only authorized nodes whose node and owning Server statuses are `active`. User eligibility is evaluated once with the existing semantics:

- status is `active`;
- `started_at` is null or not in the future;
- `expires_at` is null or in the future;
- quota is unlimited (`0`) or `used_bytes < quota_bytes`.

Server offline heartbeat state does not remove a node; only administratively disabled Server status does. This avoids subscription churn during transient outages.

## Protocol Rendering

The subscription renderer builds a minimal client model from `Server.Address`, node port/name/settings, and only the required decrypted secret fields.

### General Base64

Build one percent-encoded URI per eligible node, join with `\n`, then standard-Base64 encode the complete UTF-8 text.

- Shadowsocks: `ss://` with supported 2022 method and the existing per-user `ss-cred-v1` derived password.
- VLESS Reality: `vless://<uuid>@<address>:<port>` with `security=reality`, `encryption=none`, `flow=xtls-rprx-vision`, first Server Name as `sni`, optional Short ID as `sid`, and X25519 public key derived from the stored private key as `pbk`. The private key is never emitted.
- Hysteria2: `hysteria2://<uuid>@<address>:<port>` with `sni=<server_name>`. It does not emit `insecure=1`.

### Clash Meta

Parse the validated global template, expand proxy-group placeholders, and finally assign `proxies` to the generated client proxy objects. The template cannot supply `proxies` or `proxy-providers`, so generated credentials cannot be replaced or supplemented.

Allowed top-level template keys are a documented allowlist covering safe client runtime settings, `dns`, `proxy-groups`, `rules`, and inline `rule-providers`. Remote provider URLs, local paths, Authorization headers, listeners, TUN/inbounds, external controller, authentication, secret, scripting and process execution fields are rejected recursively.

Placeholder expansion replaces each placeholder item with ordered generated node names for its protocol. Unknown placeholders and dangling group/rule targets fail validation. A default template contains select and URL-test groups, safe DNS defaults, and final rules.

## Hysteria2 TLS

Hysteria2 settings add public `server_name`. Encrypted secret fields add `certificate` and `private_key` PEM strings. Create requires all three. Update treats certificate/private key as a pair: both omitted retains the current pair; supplying only one returns `422 validation`.

Validation uses `tls.X509KeyPair`, parses the leaf certificate, verifies current validity and `VerifyHostname(server_name)`, and rejects malformed/mismatched material. Successful persistence bumps only the owning Server revision.

The sing-box inbound renderer writes the PEM values to inline `tls.certificate` and `tls.key` arrays accepted by pinned sing-box 1.14.1. The material therefore travels only in the authenticated Agent config response and the Agent's mode-0600 active config. Generic node DTOs and subscription output never include either PEM private key or VLESS Reality private key.

## Logging And Secret Handling

The request logger normalizes any request matching the configured subscription prefix before logging, for example `/s/actual-token` becomes `/s/:token`. Query strings are already excluded. Handler errors never interpolate the token.

Automated secret sweeps cover admin DTOs, public subscription bodies, error bodies, and captured logs. The subscription URL is returned only by dedicated authenticated subscription endpoints and user creation, never generic DTOs.

## Frontend

`UserDetailBasic.vue` gains a subscription section that loads the dedicated admin endpoint, displays a copyable URL, creates one for historical users, and rotates with confirmation. `UserCreateDialog.vue` displays the generated subscription URL alongside the one-time business Token.

`SettingsPage.vue` adds a Subscription section for base URLs and path, and a Clash Meta Override section with a multiline YAML editor, validation errors, documented placeholders, and restore-default action. API types remain centralized in `web/src/api/types.ts`.

`NodeFormDialog.vue` adds Hysteria2 Server Name, certificate PEM, and private-key PEM fields. Edit mode never fetches stored PEM; leaving all secret fields blank retains the pair.

## Compatibility And Rollback

- Existing users, nodes, and Agent identities remain valid.
- Historical users begin without subscription credentials.
- Existing Hysteria2 nodes remain readable but are not subscription-renderable and fail a config-changing update until complete TLS material is supplied. The node form reports this state without exposing secrets.
- Rollback can stop serving the public/admin routes and ignore the additive table/settings. Existing encrypted subscription and Hysteria2 materials remain inert; no destructive data rollback is required.
