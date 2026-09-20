# Directory Structure

> How backend code is organized in this project.

---

## Overview

Single Go module at repo root (`module vps-node`), Go 1.25, stdlib `net/http` + `database/sql` + `slog`. SQLite via `modernc.org/sqlite` (pure Go, no cgo). Two binaries: `cmd/panel` (control plane + embedded UI) and `cmd/agent` (node runtime).

---

## Directory Layout

```
vps-node/
├── cmd/
│   ├── panel/main.go        # wiring: config → db → repos → web.Handler → janitor → webui
│   └── agent/main.go        # wiring: config → agentstate → agentclient → agentruntime loop
├── internal/
│   ├── config/              # YAML + env overrides (PANEL_*/AGENT_*); required app_key (32-byte hex)
│   ├── db/                  # sqlite open (WAL, FK ON), embedded migration runner, migrations/*.sql
│   ├── repo/                # context-aware repositories; Tx helper; mutation+bump helpers
│   ├── web/                 # panel HTTP handlers: router, admin auth, users, servers, nodes,
│   │                        #   agent*.go (bearer), agent_telemetry.go, dto.go, respond.go
│   ├── adminauth/           # bcrypt, DB-backed sessions, login rate limiter
│   ├── secrets/             # AES-256-GCM (app_key) for recoverable protocol secrets
│   ├── singbox/             # pure renderer: Node+eligible users → sing-box inbound JSON (singbox-render-v1)
│   ├── agentclient/         # typed agent→panel HTTP client (retry/backoff/jitter)
│   ├── agentruntime/        # applier (check→atomic replace→optional reload), metrics, clash collector, loop
│   ├── agentstate/          # 0600 JSON state file: identity, applied revision, batch seqs
│   ├── janitor/             # retention/storage-cap cleanup goroutine
│   ├── httpx/               # logging middleware, panic recovery, /healthz, JSON helpers
│   ├── logx/                # slog factory
│   ├── webui/               # embed of web/dist behind `embed_ui` build tag + SPA fallback
│   └── e2e/                 # hermetic end-to-end test (real handlers + fake sing-box)
├── web/                     # Vue 3 frontend (own npm project; nested go.mod barrier — do not remove)
├── docs/api-contract.md     # BINDING cross-layer contract (single source of truth)
└── deploy/                  # systemd units, example configs, install script
```

---

## Module Organization

- Handlers own request decode/validate; repos own SQL; **never** put SQL in handlers or DTO shaping in repos.
- Any mutation that affects runtime config MUST go through `internal/repo/mutations.go` helpers so the affected `server_revisions` bump transactionally.
- New agent payloads: extend `docs/api-contract.md` FIRST, then panel handler + `agentclient` + frontend `web/src/api/types.ts` together (contract is frozen per task; deviations must be reported, not silently fixed).
- New tables: append `NNNN_name.sql` under `internal/db/migrations/`; the runner is sequential and idempotent (`schema_migrations`).

---

## Naming Conventions

- Packages: short lowercase (`repo`, `web`, `httpx`, `agentstate`). Files: lowercase with underscores.
- DB timestamps: INTEGER unix seconds. Byte counters: integer bytes. JSON: snake_case in all API payloads.
- User status: `active|disabled|expired` (expired is derived at read/filter time from `expires_at`; never a stored state machine).
- Node protocol: `shadowsocks|vless|hysteria2` (contract spelling — `vless`, not `vless-reality`).

---

## Examples

- Adding a mutation with revision bump: see `internal/repo/mutations.go` (`SetUserNodes`, user/node/server update/delete).
- Adding an admin endpoint: `internal/web/users.go` + DTO in `dto.go` + route in `web.go`.
- Adding an agent endpoint: `internal/web/agent.go` (auth derives server_id from token) + client method in `internal/agentclient/client.go`.
