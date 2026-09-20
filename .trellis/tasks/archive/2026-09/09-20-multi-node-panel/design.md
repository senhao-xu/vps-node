# Technical Design

## Architecture

```text
Browser --HTTPS/Admin session--> Panel API + Web UI --SQLite
                                      ^
                                      | HTTPS REST + Agent Bearer Token
                                      v
                               Agent (one per Server)
                                      |
                                      v
                         sing-box (shared process, many Nodes)
```

Panel owns business state. Agent owns local identity, Panel URL, runtime state and the last successful configuration. Agent initiates control-plane connections.

## Domain Boundaries

- `Server` is a physical VPS with system metrics, online status and exactly one Agent.
- `Node` is one sing-box service entry with protocol, port and settings. A Server owns many Nodes.
- `UserNode` is the authorization boundary. Agent payloads are Server-scoped and contain only eligible users.
- `Session` is a replaceable current snapshot; `ConnectionLog` is historical append data with cleanup.
- `TrafficRecord` stores idempotent increments and updates totals transactionally.

## Credentials

Users have one global UUID. VLESS and Hysteria2 use it as the protocol credential. Shadowsocks credentials are derived from Node settings and UUID under a versioned contract. Panel returns final generated configuration to Agent, so Agent does not own business credential derivation.

Recoverable protocol secrets are encrypted at rest with an application key outside the database. Passwords and Agent tokens are hashed where recovery is unnecessary. Responses and logs never expose secrets.

## Configuration Flow

1. Admin changes User, Node, or UserNode.
2. Panel validates and commits the source-of-truth transaction.
3. The affected Server revision increments monotonically.
4. Agent polls its revision; unchanged revisions return no full payload.
5. Panel renders complete Server-scoped sing-box configuration.
6. Agent writes a restricted temporary file, runs `sing-box check`, atomically replaces the active file and reloads/restarts.
7. Failure retains the previous file/configuration and reports the failed revision.

## Agent API Contracts

```text
POST /api/agent/register
POST /api/agent/heartbeat
GET  /api/agent/config?version=<applied-version>
POST /api/agent/traffic
POST /api/agent/sessions
POST /api/agent/connection-logs
```

Authenticated requests derive `server_id` from the credential, not a trusted request body. Traffic and log batches include an idempotency key or sequence number, bounded size, timestamp validation and non-negative counters.

## Runtime Data Flow

```text
sing-box stats/log source -> Agent normalizer -> HTTPS Agent API
-> ownership validation -> idempotent transaction
-> sessions/traffic/logs -> admin DTOs and dashboard aggregates
```

Implementation must verify what the pinned sing-box version exposes for user-level flow and connection metadata. Missing fields are not inferred.

## Failure And Rollback

- Panel unavailable: bounded retry; continue last valid configuration.
- Config validation/restart failure: retain old config and report failure.
- Duplicate batch: return prior accepted result without double counting.
- Heartbeat timeout: mark Server offline; retain data and permissions.
- User deletion: remove authorization and sessions transactionally; next revision removes runtime user.
- Server deletion: disable sync first, process dependent authorization/Agent, preserve history by retention rules.

## Operational Constraints

- HTTPS and certificate verification are mandatory for production Agent communication.
- sing-box values are structured and allowlisted; no arbitrary shell, systemd, path or executable from Admin API.
- Raw connection logs default to 7 days and aggregates to 90 days, configurable with storage limits.
- SQLite backup must preserve WAL/SHM consistency and the application encryption key.
- Start with one Panel and one canary Agent; pin sing-box for validation and integration tests.
