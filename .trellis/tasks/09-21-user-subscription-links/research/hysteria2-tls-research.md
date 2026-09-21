# Research: sing-box 1.14.1 Hysteria2 TLS and Panel→Agent certificate boundary

- **Query**: 研究 sing-box 1.14.1 Hysteria2 inbound/outbound 的 TLS 证书要求、证书方式、现有 Panel→Agent apply/deploy 边界及 XBoard Hysteria2 TLS 处理。
- **Scope**: mixed（固定 sing-box v1.14.1 源码、官方文档、本地项目、XBoard 源码）
- **Date**: 2026-09-21
- **Source revisions**: sing-box `v1.14.1`, commit `1ac1a339cb1223e9c70eae14c44411c75033c02d`; XBoard inspected clone at commit `4f48e61a2cbc6db5338872b6bdb45ef954ec1256`.

## Findings

### 1. Hysteria2 inbound certificate requirements and certificate modes

Fixed source confirms Hysteria2 is TLS-required: `protocol/hysteria2/inbound.go:47-54` rejects a missing/disabled `tls` with `C.ErrTLSRequired`, then constructs the TLS server. The TLS object itself accepts either inline PEM arrays (`certificate`, `key`) or paths (`certificate_path`, `key_path`) (`option/tls.go:13-40`). For ordinary server TLS, `common/tls/std_server.go:405-444` reads inline or path material and rejects a missing certificate or missing key; it parses the pair with `tls.X509KeyPair`. Thus a normal Hysteria2 inbound needs a certificate chain and matching private key, supplied inline or by path.

There are three relevant exceptions/modes:

* **ACME**: v1.14.1 supports the newer `certificate_provider` and retains inline `acme` as deprecated. `option/tls.go:33-39` declares both; `std_server.go:328-363` chooses provider/ACME before static certificate loading. The inline ACME options include domains, data directory, CA provider, HTTP/TLS-ALPN toggles, alternate ports and DNS-01 options (`option/acme.go:15-30`). The official TLS page marks inline ACME deprecated in 1.14.0 and says it is removed in 1.16; the migration direction is certificate provider. ACME therefore can automatically obtain/manage the server certificate, but it needs domain/challenge/account/storage and operational reachability/DNS prerequisites.
* **Self-signed / private CA**: technically valid as static PEM certificate + key; sing-box only requires a parsable X.509 key pair. Clients must trust it explicitly (custom CA `tls.certificate`) or use public-key pinning; normal public trust will fail.
* **Reality**: not a Hysteria2 certificate substitute. `common/tls/server.go:42-45` dispatches Reality only when `tls.reality.enabled`; `reality_server.go` uses Reality handshake/private key semantics. The Hysteria2 inbound invokes the normal TLS server (`inbound.go:49-53`), and its Hysteria2 TLS docs require ordinary TLS. Reality is the TLS mode used by the project’s VLESS renderer (`internal/singbox/singbox.go:181-196`), not Hysteria2.

The TLS implementation has one notable dynamic option: `certificate_provider` can be shared or inline (`std_server.go:531-552`), and providers can supply certificates at handshake time. It is not equivalent to a Reality keypair.

### 2. Hysteria2 client outbound fields

The fixed Hysteria2 outbound implementation rejects disabled/missing TLS (`protocol/hysteria2/outbound.go:48-59`). Official structure and `option` types require:

| Field | Role / requirement |
|---|---|
| `server` | Required server address (`protocol/hysteria2` docs; `option.Hysteria2OutboundOptions`). |
| `server_port` | Required unless using newer `server_ports`/Realm mode; ordinary client uses this port. |
| `password` | Authentication password; in this project the current contract is user UUID (`internal/web/agent_config.go:221-222`). |
| `tls.enabled` | Required and must be true for Hysteria2. |
| `tls.server_name` | SNI and certificate hostname identity; sing-box uses it to verify returned certificates. If absent, it falls back to the server address, but a domain matching the certificate should be explicit. `std_client.go:120-127`. |
| `tls.insecure` | Client-only “accept any server certificate”; it disables normal certificate verification (`std_client.go:129-145`). It is not needed for a publicly trusted certificate. |

Optional client TLS trust fields include `tls.certificate`/`certificate_path` for a custom CA certificate chain and, since 1.13, `tls.certificate_public_key_sha256` for public-key pinning. Source rejects combining pinning with custom CA (`std_client.go:137-145`). Hysteria2-specific optional fields include bandwidth, obfuscation and network; they are not substitutes for the required TLS/password/server fields.

### 3. Inline encrypted PEM in node secret and leakage surface

This is structurally feasible in the current design. Node updates classify Hysteria2 `password` as a secret and encrypt `secretFields` with `secrets.Encrypt(h.appKey, ...)` (`internal/web/nodes.go:329-369`); `singboxNode` decrypts the blob only while building an Agent payload (`internal/web/agent_config.go:183-207`). The existing renderer already receives `Node.Secret`, and adding certificate/key values there would match that data path. sing-box v1.14.1 explicitly supports line-array PEM fields for server `certificate` and `key` (`option/tls.go:22-29`; `std_server.go:405-443`), so JSON inline rendering is accepted by the pinned version.

The material would nevertheless be exposed in two places by design:

1. The Panel→Agent `/api/agent/config` updated response contains `config.singbox` (`internal/web/agent_config.go:46-57`), so the PEM private key travels in the HTTPS response JSON payload. It is not part of the separate `users` DTO, but it is in the config payload.
2. Agent apply writes the complete JSON to a temporary file with mode `0600`, runs `sing-box check`, renames it to the configured active path, and removes the temporary file (`internal/agentruntime/applier.go:59-104`). The active sing-box config therefore contains the inline private key locally with mode `0600`. The loop logs revision/error/renderer but does not log the payload (`internal/agentruntime/loop.go:204-228`).

The current agent protocol has no certificate-specific side channel: `ConfigResponse` carries only raw `config.singbox` plus user DTOs (`internal/agentclient/client.go:120-128`), and config polling is authenticated by Bearer agent token (`client.go:208-217,283-299`). Consequently, inline PEM is compatible with the existing apply boundary but is not absent from payload or local config. The Panel’s at-rest node secret is encrypted; the Agent’s active config is plaintext JSON protected by filesystem mode. This is a fact about the current path, not a recommendation.

### 4. Why path-only storage does not fit the current Panel→Agent architecture

`certificate_path` and `key_path` are paths interpreted on the machine running sing-box. The Panel stores node data and renders the complete config remotely, while the Agent applies the received JSON on a node machine; there is no existing API field, file-upload protocol, certificate synchronization operation, shared-volume contract, or Agent-side secret-file materialization path. The Agent applier only receives `configJSON []byte` and writes one complete config file (`internal/agentruntime/applier.go:59-93`). Therefore a Panel-only path would point to a path that may not exist on the Agent host, may differ by deployment (systemd versus container), and cannot be inferred from Panel storage.

The path mode can work only if an external process provisions the exact certificate/key files on every Agent host and maintains permissions/rotation. That lifecycle is outside the current Panel→Agent config contract. sing-box itself watches configured certificate/key paths for changes (`std_server.go:223-285`), but this does not create or distribute those files.

### 5. Self-signed client verification: `insecure` versus pinning

`tls.insecure: true` means accept any certificate (`std_client.go:129-134` / official TLS docs); it does not authenticate the server identity and is broader than needed. A self-signed certificate can instead be supplied as the client’s custom trust anchor through `tls.certificate`/`certificate_path`, preserving certificate-chain and hostname verification. Alternatively, `tls.certificate_public_key_sha256` pins the server public key; fixed source sets `InsecureSkipVerify` but verifies the peer public-key hash (`std_client.go:137-145`). Thus, for a self-signed deployment, `insecure` is the compatibility fallback but loses identity verification; custom CA trust or public-key pinning is the authenticated approach. Pinning follows the key, not necessarily the certificate’s expiry/renewal, so rotation requires updating the pinned hash. The official docs call the field public-key SHA-256 pinning, not certificate pinning in the strict whole-certificate sense.

### 6. XBoard Hysteria2 TLS handling

At the inspected XBoard commit, the subscription renderers consume existing server protocol settings; they do not provision server certificates. `app/Protocols/SingBox.php:681-731` emits Hysteria2 outbound `server`, `server_port`, `type=hysteria2`, `password`, and `tls.enabled=true`; it sets `tls.insecure` from `protocol_settings.tls.allow_insecure` and optional `tls.server_name` from `protocol_settings.tls.server_name` (`:688-705`). `ClashMeta.php:589-628` maps equivalent values to `sni`, `skip-cert-verify`, bandwidth and password. Generic URI output adds `sni` and an `insecure=1` query parameter when configured (`General.php:324-348`).

XBoard’s admin validation accepts `tls.server_name`, `tls.allow_insecure`, and ECH, but no Hysteria2 certificate/key upload fields (`app/Http/Requests/Admin/ServerSave.php:207-217`). The inspected XBoard subscription code therefore assumes the server-side Hysteria2 certificate is managed outside the subscription renderer; it exposes SNI and an opt-in insecure flag to clients. This differs from the local project’s current Hysteria2 inbound renderer, which emits only `tls: {enabled:true}` and no certificate material (`internal/singbox/singbox.go:200-219`).

### 7. Certificate lifecycle options: factual comparison

| Lifecycle | What exists / operational facts | Main data crossing current boundary |
|---|---|---|
| Administrator uploads PEM | Panel can validate/store certificate and private key as encrypted node secret; renderer can emit inline arrays accepted by v1.14.1. Renewal is an administrator-driven replacement and config revision/apply. | PEM private key is in updated config JSON and active Agent config (0600); no extra sync protocol needed. |
| Panel generates self-signed | Panel can create a matching certificate/key pair and store it encrypted; clients need the generated certificate as trust material or its public-key hash, otherwise `insecure` is required. Renewal changes trust material/hash unless key is retained. | Server key crosses in config payload/local file; client trust material must also be distributed in subscription output if strict verification is desired. |
| External ACME | External certbot/Caddy/another issuer writes certificate/key files on the Agent host, or an Agent-side manager owns issuance. sing-box v1.14.1 also has built-in inline/provider ACME, but inline ACME is deprecated in favor of certificate providers and needs challenge/storage configuration. | With external file ownership, Panel must coordinate a host path and does not transport PEM; with sing-box ACME config, ACME credentials/domain/provider settings become Agent config inputs. |
| Agent-managed ACME/provider | sing-box certificate providers can obtain certificates dynamically and reload/provider-select them; the current Agent can pass ordinary JSON but has no dedicated certificate-provider management API or host-level issuance UX. | Provider configuration/credentials would be in config payload unless separately provisioned; resulting private key is managed on Agent rather than necessarily rendered inline. |

The table records implementation/data-flow consequences only. It does not select a lifecycle.

## External References

- [sing-box v1.14.1 Hysteria2 inbound](https://sing-box.sagernet.org/configuration/inbound/hysteria2/) — official fields; TLS required.
- [sing-box v1.14.1 Hysteria2 outbound](https://sing-box.sagernet.org/configuration/outbound/hysteria2/) — required server/server_port/password/TLS structure.
- [sing-box TLS](https://sing-box.sagernet.org/configuration/shared/tls/) — server PEM, client SNI/insecure/custom CA/public-key hash, ACME and Reality distinctions.
- [sing-box source at v1.14.1](https://github.com/SagerNet/sing-box/tree/v1.14.1) — fixed source revision used above.
- [XBoard source](https://github.com/cedar2025/Xboard/tree/4f48e61a2cbc6db5338872b6bdb45ef954ec1256) — inspected subscription renderers and TLS settings at recorded commit.

## Related Specs

- `.trellis/tasks/09-21-user-subscription-links/prd.md:29,61` — R13 requires complete Hysteria2 TLS consistency; open question asks for certificate lifecycle.
- `internal/web/agent_config.go:46-57,183-207` — current Panel config payload and encrypted secret decryption boundary.
- `internal/agentruntime/applier.go:59-104` — current Agent temp-file/check/rename/apply boundary.
- `internal/singbox/singbox.go:200-219` — current Hysteria2 inbound emits enabled TLS but no certificate fields.

## Caveats / Not Found

- The official documentation URL is current-site content, while all behavioral claims and version-sensitive field support above were checked against the pinned v1.14.1 source commit. Current docs may describe later releases too; source takes precedence for v1.14.1 claims.
- No XBoard Hysteria2 server-side certificate issuance/upload implementation was found in the inspected source. Its subscription layer only renders client TLS flags/settings; server certificate management may be deployment-specific or outside the repository.
- The active-task CLI reported no current task; the user supplied the task path explicitly, so this report was persisted there.
