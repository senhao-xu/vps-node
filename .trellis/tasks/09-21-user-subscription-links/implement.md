# Implementation Plan

## 1. Contract And Persistence

- Update `docs/api-contract.md` with subscription management, public subscription behavior, settings fields, user-create response, and Hysteria2 TLS settings.
- Add an additive migration for `user_subscriptions` with cascade deletion, unique token hash, encrypted token value, and timestamps; add migration idempotency/FK/index tests.
- Add repository operations for create/get/rotate subscription credentials and eligible authorized node lookup with owning Server data.
- Keep generic user DTOs unchanged and extend only the user-created DTO and dedicated subscription DTO.

## 2. Subscription Management And Safe URLs

- Add settings keys for subscription origins, path, and Clash Meta template with strict validation.
- Implement XBoard-style URL construction: configured origins first, current admin request Origin fallback, safe path joining.
- Generate the subscription credential during new-user creation transaction; implement admin get/create/rotate endpoints for historical users.
- Normalize subscription paths in request logging before any request line is emitted.
- Test no-token DTO sweeps, rotation invalidation, rollback on encryption/storage failure, and log redaction.

## 3. Hysteria2 TLS Completion

- Extend node settings validation with public `server_name` and encrypted PEM certificate/private key fields.
- Validate PEM pair, certificate validity, and hostname; require paired replacement and preserve omitted secrets on update.
- Render inline Hysteria2 TLS certificate/key arrays into sing-box config.
- Update node form fields and Chinese validation/help text while preserving edit-mode non-disclosure.
- Add node CRUD, partial-update, secret sweep, Agent render, and pinned sing-box integration coverage.

## 4. Subscription Renderers

- Introduce a small subscription package with a typed client-node model shared by both output formats.
- Implement standard-Base64 general URI rendering for Shadowsocks 2022, VLESS Reality, and Hysteria2.
- Derive the Reality public key from its encrypted private key and reuse the existing Shadowsocks user credential derivation.
- Implement restricted Clash Meta YAML parsing, forbidden-key validation, placeholder expansion, dynamic proxy injection, reference checks, and deterministic output.
- Add golden tests for every protocol, mixed subscriptions, escaping/IPv6, empty node sets, invalid templates, and secret absence.

## 5. Public Endpoint

- Register the validated subscription path and implement token-hash lookup, eligibility checks, active node/Server filtering, User-Agent/flag negotiation, metadata headers, and response content types.
- Return stable non-secret errors for unknown, rotated, disabled, expired, not-started, and quota-exhausted users.
- Add API and E2E tests proving authorization isolation, old-link invalidation, filtering, negotiation, and no request-log leakage.

## 6. Admin UI

- Extend centralized API types/functions for subscription management and settings.
- Add copy/create/rotate subscription controls to user details and show the initial URL after user creation.
- Add subscription URL/path settings plus restricted Clash Meta YAML editor, placeholder help, validation display, and default restoration.
- Add Hysteria2 TLS fields to the node dialog with responsive layout and edit-mode retention semantics.

## 7. Integration Review

- Verify a newly created user's link works immediately and a historical user stays unprovisioned until explicit creation.
- Verify rotation invalidates the old link without changing user UUID or business Token.
- Verify the same URL produces general Base64 by default and Clash Meta for matching User-Agent or explicit flag.
- Verify disabled/expired/not-started/over-quota users and disabled nodes/Servers follow the documented behavior.
- Verify Hysteria2 certificate material reaches only the encrypted database field and authenticated Agent config, never generic/admin/public DTOs or logs.
- Verify Clash templates cannot introduce or mutate proxies, credentials, listeners, controllers, authentication, remote providers, paths, or headers.

## Validation

```bash
gofmt -w <changed-go-files>
go build ./...
go vet ./...
test -z "$(gofmt -l .)"
go test -count=1 ./...
go test -race -count=1 ./...
go test -count=1 -tags integration ./...
go build -tags embed_ui ./...
cd web && npm run typecheck
cd web && npm run build
cd web && npm run lint
make build
```

## Review And Rollback Gates

- Stop if the chosen Shadowsocks 2022 URI representation is rejected by supported clients; record a fixture against at least one target parser before claiming general-format support.
- Stop if pinned sing-box rejects inline Hysteria2 PEM material; do not fall back to `insecure` or untracked Agent filesystem paths.
- Reject rather than partially save a Clash template that cannot be structurally validated.
- Keep schema changes additive. A rollback removes routes/UI usage without deleting subscription rows or encrypted node secrets.
