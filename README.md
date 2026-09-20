# vps-node

轻量级多节点用户管理面板：Panel（唯一业务数据源）+ Agent（节点运行时）+ sing-box（数据面）。

## Architecture

```text
Browser --HTTPS/Admin session--> Panel API + Web UI --SQLite
                                      ^
                                      | HTTPS REST polling + Agent Bearer Token
                                      v
                               Agent (one per Server)
                                      |
                                      v
                         sing-box (one process, many Nodes)
```

- **Panel** owns users, servers, nodes, authorizations, traffic totals, sessions and connection logs. It is the single source of truth.
- **Server** is a physical VPS with exactly **one** Agent; it may host **many** protocol Nodes.
- **Node** is one sing-box inbound (Shadowsocks 2022 / VLESS Reality / Hysteria2) with a port and protocol settings.
- **User ⇄ Node** is N:N (`user_nodes`). A user has one global UUID; VLESS/Hysteria2 use it directly, Shadowsocks credentials are derived by the Panel per node+UUID.
- **Agent** polls the Panel for versioned, server-scoped config, validates it with `sing-box check`, atomically replaces the config file, and reports traffic, sessions and connection logs. The Agent never stores business data.

Control flow (design contract):

1. Admin change commits → the affected Server revision increments monotonically.
2. Agent heartbeats carry metrics; the heartbeat response returns the current revision.
3. Agent polls `GET /api/agent/config?version=<applied>`; unchanged revision → `{"status":"current"}` (no payload).
4. On `updated`, the Agent writes a temp file (0600), runs `sing-box check -c`, then atomically renames it over the active config and (optionally) runs the configured reload command.
5. Any validation/reload failure keeps the previous file and the old revision; the failure is reported via heartbeat (`last_apply_error`) and logs. The Panel stays unreachable-safe: the Agent retries with backoff and keeps the last valid config forever.

## Quickstart

### Panel

```sh
openssl rand -hex 32          # app key
cp deploy/panel.example.yaml panel.yaml   # edit app_key + admin password

make panel                    # go build -o panel ./cmd/panel
PANEL_ADMIN_PASSWORD=... ./panel
```

Panel config keys (yaml or `PANEL_*` env): `listen`, `db_path`, `log_level`, `app_key` (32-byte hex, required; encrypts protocol secrets at rest), `secure_cookie`, `admin.username`, `admin.password`, `retention.raw_log_days`, `retention.aggregate_days`, `retention.sweep_interval_seconds`, `retention.max_connection_logs`, `retention.max_traffic_records`.

### Web UI

The Vue 3 frontend lives in `web/`. By default the Panel binary serves the API only ( `/` shows a hint page). To embed the built UI into the single binary:

```sh
make build-embed              # npm ci + npm run build + go build -tags embed_ui
```

`web/dist` is copied to `internal/webui/dist` by the Makefile and embedded behind the `embed_ui` build tag; building without the tag never requires the frontend.

### Agent

1. In the Panel web UI: **Servers → Server detail → Generate register token** (one-time, hashed at rest).
2. On the VPS (with sing-box installed):

```sh
PANEL_DOWNLOAD_BASE=https://example.com/downloads/vps-node \
PANEL_VERSION=20260920 \
PANEL_URL=https://panel.example.com \
SERVER_ID=1 \
REGISTER_TOKEN=<token-from-server-detail> \
sh install-agent.sh
```

`deploy/install-agent.sh` is idempotent: detects amd64/arm64/386, installs `/usr/local/bin/panel-agent`, writes `/etc/panel-agent/agent.yaml` only if absent, creates the `panel-agent` user and enables the hardened `panel-agent.service`. Registration happens on first start; the returned agent token (plaintext once) is persisted to the state file with mode 0600 — delete the state file to re-register.

Re-registration with a fresh register token rotates the agent token; the old one stops working immediately. Rotating later without re-registering is the **Agent token** action on the Server detail page.

### Agent config reference

| Key | Default | Meaning |
| --- | --- | --- |
| `panel_url` | – (required) | Panel base URL; use HTTPS in production |
| `token` / `register_token` | – (one required) | existing agent token, or one-time registration token |
| `server_id` | – (required) | must match the server the token belongs to (cross-check only; the Panel derives ownership from the token) |
| `state_path` | `agent_state.json` | identity + applied revision + batch sequences (0600, atomic replace) |
| `log_level` | `info` | `debug`…`error` |
| `heartbeat_interval` | `30s` | metrics + liveness; panel response can retune it |
| `sync_interval` | `30s` | config poll cadence (a newer-revision heartbeat triggers an extra poll) |
| `traffic_interval` | `60s` | traffic/session/log report cadence |
| `singbox.config_path` | `/etc/sing-box/config.json` | active config the agent atomically replaces |
| `singbox.check_bin` | `sing-box` | binary used for `check -c <file>` |
| `singbox.reload_command` | *(empty)* | optional **single binary + fixed args** (e.g. `/bin/systemctl restart sing-box`); empty = no automatic reload (logged) |
| `collection.traffic` / `collection.sessions` / `collection.connection_logs` | `true` | agent-side collection switches |

All keys have `AGENT_*` environment equivalents (e.g. `AGENT_PANEL_URL`, `AGENT_REGISTER_TOKEN`, `AGENT_SINGBOX_RELOAD_COMMAND`).

Reload privileges: `systemctl restart sing-box` as the non-root `panel-agent` user needs polkit/sudo. Prefer a privilege-free strategy (sing-box watchdog or signal-based reload via a small helper) — the agent only ever executes the one allowlisted command from its own config file, never anything from the Panel.

## Docker deployment

Docker is the second supported deployment path next to systemd. Two images are built from the repo root:

| Image | Contents | Base |
| --- | --- | --- |
| `vps-node-panel` | Panel binary with the web UI embedded (`embed_ui`) + `/panel-healthcheck` helper | `scratch` |
| `vps-node-agent` | `panel-agent` + pinned `sing-box` + `singbox-reload` helper + CA certs | `scratch` |

The agent image is self-contained: sing-box ships inside it, and `singbox-reload` (a tiny Go binary, no shell) starts/stops sing-box via the pid file `/run/singbox/singbox.pid` whenever the agent applies a new config. All binaries are static (CGO off), so the runtime base is `FROM scratch`.

### Build

```sh
make docker-panel                                        # vps-node-panel:latest
make docker-agent                                        # vps-node-agent:latest, sing-box at the pinned default below
make docker-agent SINGBOX_VERSION=1.13.7 \
    SINGBOX_SHA256_AMD64=<sha256> SINGBOX_SHA256_ARM64=<sha256>   # bump sing-box (checksums from its GitHub release)
```

sing-box is downloaded at build time from the official GitHub release (`ARG SINGBOX_VERSION`, the static `*-musl` build, amd64/arm64 via `TARGETARCH`) and verified against pinned sha256 checksums (`SINGBOX_SHA256_AMD64`/`SINGBOX_SHA256_ARM64`, matching the default version). When you bump `SINGBOX_VERSION`, pass the matching checksums for the two `*-musl.tar.gz` files — a mismatch fails the build. Set `SINGBOX_DOWNLOAD_URL` to a mirror if GitHub is unreachable from your builder (the mirror must serve byte-identical tarballs for the checksums to pass).

### Run: Panel only

```sh
cd deploy
cp .env.example .env      # then fill in PANEL_APP_KEY and PANEL_ADMIN_PASSWORD
docker compose up -d      # panel on :8080, SQLite in volume panel-data
```

Compose auto-loads `deploy/.env`; prefer it over shell exports so variables survive new shells and reboots (`PANEL_APP_KEY`/`PANEL_ADMIN_PASSWORD` are hard-required on every `up`). Set `PANEL_PORT` to publish a host port other than 8080 (e.g. when 8080 is taken).

Data lives in the `panel-data` volume (`/data/panel.db` inside the container). Back up the volume together with `PANEL_APP_KEY` — the key encrypts node secrets at rest. TLS terminates in a reverse proxy; a commented Caddy example is included in `deploy/docker-compose.yml` (the panel itself serves plain HTTP on :8080 only).

### Run: Agent on a node server

```sh
cd deploy
cp .env.example .env      # set AGENT_PANEL_URL, AGENT_SERVER_ID and one of the tokens
docker compose -f agent.docker-compose.yml up -d
```

The agent registers on first start and stores its identity in the `agent-state` volume (`/var/lib/panel-agent/state.json`); delete that volume to re-register. After the first config apply the agent spawns sing-box itself via `singbox-reload` — there is no separate sing-box container. Publish one port pair (`tcp`+`udp`) per node, e.g. `8388:8388/tcp` and `8388:8388/udp` (override the host side with `NODE_PORT`); with many nodes consider `network_mode: host`. Ports ≤1024 additionally require root (the container runs as root by default).

### Run: All-in-one (panel + agent on one host)

```sh
cd deploy
cp .env.example .env      # fill in the panel vars; up -d panel first, then the agent vars
docker compose -f docker-compose.all-in-one.yml up -d panel
# create the server (note its id N), a node on port 8388, then a register token in the UI
# add AGENT_SERVER_ID=N and AGENT_REGISTER_TOKEN=<token> to .env
docker compose -f docker-compose.all-in-one.yml up -d panel-agent
```

Panel↔agent traffic stays on the compose-internal network (`http://panel:8080`); only the admin port and node ports are published.

### Measured footprint

Measured with sing-box 1.14.1 on linux/amd64 (Docker 29.7.2), idle all-in-one stack:

| Item | Unpacked (`docker images`) | Serialized (`docker save` ≈ pull size) |
| --- | --- | --- |
| `vps-node-panel` | 24.5 MB | 7.4 MB |
| `vps-node-agent` | 138 MB | 37.1 MB |

Idle memory: panel ≈ 6.5 MiB; agent container incl. running sing-box ≈ 16 MiB (sing-box grows with active connections — size it accordingly).

> The 60 MB agent-image target refers to pull/transfer size (37 MB ✓). The unpacked 138 MB is dominated by the official sing-box 1.14.1 binary itself (~92 MB unpacked; 1.13.x ≈ 68 MB — no current official release ships smaller). To shrink further, build sing-box from source with minimal feature tags (`with_quic,with_utls,with_clash_api`) in place of the release download.

### Upgrades

- **Panel**: `git pull && make docker-panel && docker compose up -d` — the image is replaced, the `panel-data` volume is untouched; roll back by re-deploying the previous image (no data migration either way).
- **Agent**: `make docker-agent SINGBOX_VERSION=<new>` then `docker compose -f agent.docker-compose.yml up -d`. The state volume (identity, applied revision, batch sequences) survives; traffic idempotency is preserved across upgrades. To change sing-box only, rebuild with a new `SINGBOX_VERSION` — the agent re-checks and re-applies its config on start.

### Notes and caveats

- The runtime base is `scratch`: no shell, no busybox. Debug with `docker exec <c> /usr/local/bin/sing-box version` style direct execs, `docker cp`, or `docker top` (processes are visible from the host).
- sing-box is a child of the agent. Each reload replaces the running sing-box: the previous process is SIGTERMed by `singbox-reload` and would linger as a zombie, because the agent (a plain Go binary) does not reap reparented orphans. The compose files therefore set `init: true`, which runs Docker's built-in init as PID 1 to reap it — keep that setting. If you run the image with bare `docker run`, pass `--init` for the same effect; without it every config apply leaks one zombie process until the container restarts. Note that `docker top` does not show defunct processes — check for zombies with `ps aux | grep defunct` on the host. `docker stop` SIGTERMs the agent; any remaining sing-box is killed when the container's PID namespace goes away.
- The agent reads host-style `/proc` metrics (`cpu`, `meminfo`, `uptime`), so reported CPU/memory percentages reflect the host, not the cgroup limit.
- Container env defaults mirror `deploy/agent.example.yaml` container paths: `AGENT_STATE_PATH=/var/lib/panel-agent/state.json`, `AGENT_SINGBOX_CONFIG_PATH=/etc/sing-box/config.json`, `AGENT_SINGBOX_CHECK_BIN=/usr/local/bin/sing-box`, `AGENT_SINGBOX_RELOAD_COMMAND=/usr/local/bin/singbox-reload`. Env overrides the YAML file, so bind-mounting an `agent.yaml` into this image only works for keys not pinned by these envs.

### systemd vs Docker

| | systemd | Docker |
| --- | --- | --- |
| Isolation | hardened units (`ProtectSystem=strict`, non-root) | container boundary, root user by default |
| sing-box lifecycle | host package + `systemctl restart` | pinned version inside the image, helper-managed |
| Upgrades | replace binary + restart | replace image + restart container |
| Data | filesystem paths | named volumes |
| Best for | long-lived dedicated VPS | fast provisioning, reproducible nodes, all-in-one test stacks |

Both paths are fully supported and produce identical panel-side behavior (same agent API). Pick systemd where the hardened non-root setup matters; pick Docker for repeatability and one-command bring-up.


## Data collection model

- **Traffic** is reported as per-interval **deltas** per (user, node), computed from sing-box Clash API `/connections` totals. Every batch carries a monotonically increasing `batch_seq` per kind; the Panel treats `(agent, batch_seq)` as the idempotency key, so resend-on-uncertainty never double counts (worst case undercounts one interval). Sequence numbers are persisted only after a confirmed ack.
- **Sessions** are full-replacement snapshots per report; the Panel replaces the server's session set transactionally. Snapshots older than `session_freshness_seconds` stop counting as online.
- **Connection logs** are emitted when a tracked connection disappears from the Clash API (status `closed`, lifetime totals). Logs never contain destination host/port or payload; the Panel strips/rejects such fields.
- **Attribution**: connections are attributed via the inbound tag (`vless-<id>` / `shadowsocks-<id>` / `hysteria2-<id>`) and the inbound user name (`u-<user_id>`, the name the Panel renders into sing-box). Connections without reliable attribution are excluded from reports and counted in agent logs — missing fields are never inferred.

## sing-box verification status

> **Pending pinned-version verification.** The Panel renderer and the Agent apply path are covered by tests, but **user-level traffic measurement is NOT yet verified against a pinned sing-box release**. Specifically:
> - whether the pinned sing-box exposes per-connection inbound user (`inboundUser`) in the Clash API metadata,
> - whether Hysteria2/Reality inbound configs pass `sing-box check` exactly as rendered,
> - real throughput behavior of user-level stats.
>
> Run `make test-integration` on a host with the pinned `sing-box` in `PATH`; the integration tests print the binary version (`sing-box version`) and validate the rendered config of all three protocols with the real `sing-box check`. Until they pass on the pinned version, treat traffic/connection measurement as unverified (per the task's review gate).

## Auth model

| Scope | Mechanism | Routes |
| --- | --- | --- |
| Admin | HttpOnly session cookie `panel_session` (SameSite=Lax), bcrypt-hashed passwords | `/api/admin/*`, `/api/users/*`, `/api/servers/*`, `/api/nodes/*`, `/api/dashboard`, `/api/settings` |
| Agent | `Authorization: Bearer <agent_token>`; token stored hashed; rotation invalidates instantly | `/api/agent/*` |

The two scopes share no middleware and no token space. Agent `server_id` is always derived server-side from the token; body-supplied IDs are only validated for ownership. Plaintext tokens/registration tokens are returned exactly once at creation/rotation. Full contract: [`docs/api-contract.md`](docs/api-contract.md).

## Retention

- Raw connection logs: default **7 days**; aggregates: default **90 days** (configurable via `/api/settings` and panel config); the janitor sweeps on a configurable interval. Hard row-count caps (`retention.max_connection_logs`, default 1,000,000; `retention.max_traffic_records`, default 5,000,000; `0` disables) bound storage even if the retention window is large — the oldest rows are deleted first.
- Traffic records feed user/node/server totals transactionally and survive user/server deletion until retention cleanup.
- Heartbeat is the only liveness source; a server goes `offline` after `server_offline_after_seconds` (default 60s) without heartbeat. Traffic/session/log reports never affect online status.

## Development

```sh
make build              # vet + test + build both binaries
make test               # go test ./... (includes hermetic agent e2e with a fake sing-box)
make test-race          # go test -race ./...
make test-integration   # go test -tags integration ./... (skips when sing-box is absent)
make ui                 # build the Vue frontend (web/dist)
make build-embed        # frontend embedded into the panel binary
make release-agent      # linux amd64/arm64/386 release tarballs in dist/
make docker-panel       # build vps-node-panel image (web UI embedded)
make docker-agent       # build vps-node-agent image (agent + sing-box + reload helper)
```

Module layout:

```text
cmd/panel            panel binary (API + embedded UI)
cmd/agent            agent binary (register/heartbeat/sync/telemetry loop)
internal/web         panel HTTP handlers (admin + agent)
internal/repo        SQLite repositories and transactions
internal/singbox     config renderer + credential derivation contract
internal/agentclient typed agent→panel HTTP client (retry/backoff, idempotent batches)
internal/agentruntime agent-side runtime: config applier, metrics, Clash collectors, loop
internal/agentstate  agent state file (identity, revision, batch seqs)
internal/webui       web/dist embedding (build tag embed_ui)
internal/e2e         end-to-end smoke test + sing-box integration tests
deploy/              systemd units, example configs, installer, Dockerfiles + compose files
```

## Deploy notes

- Production Agent↔Panel traffic must use **HTTPS with certificate verification** (behind a reverse proxy for Docker, e.g. Caddy — see the Docker section).
- systemd units are hardened (`NoNewPrivileges`, `ProtectSystem=strict`, `PrivateTmp`, non-root users, minimal `ReadWritePaths`).
- SQLite backups must copy the WAL/SHM set and preserve the `app_key` (it encrypts protocol secrets at rest).
- Start with one Panel and one canary Agent; pin the sing-box version on the canary before scaling.
- Node listen ports >1024 need no container privileges; ports ≤1024 require root (or `NET_BIND_SERVICE`). Node ports are published with `ports:` one pair per node, or `network_mode: host` when exposing many.
