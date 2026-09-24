# vps-node

轻量级多节点用户管理面板：Panel（唯一业务数据源）+ Agent（内嵌 sing-box 的节点运行时）。

## Architecture

```text
Browser --HTTPS/Admin session--> Panel API + Web UI --SQLite
                                      ^
                                      | HTTPS REST polling + Agent Bearer Token
                                      v
                               Agent (one per Server)
                                      │
                                      v
                    embedded sing-box (in-process, many Nodes)
```

- **Panel** owns users, servers, nodes, authorizations, traffic totals and online devices. It is the single source of truth.
- **Server** is a physical VPS with exactly **one** Agent; it may host **many** protocol Nodes.
- **Node** is one sing-box inbound (Shadowsocks 2022 / VLESS Reality / Hysteria2 / AnyTLS) with a port and protocol settings. AnyTLS requires sing-box ≥ 1.12; the embedded runtime is pinned to sing-box ≥ 1.13 (the agent image is built with `-tags with_quic,with_utls`).
- **User ⇄ Node** is N:N (`user_nodes`). A user has one global UUID; VLESS/Hysteria2/AnyTLS use it directly, Shadowsocks credentials are derived by the Panel per node+UUID.
- **Agent** polls the Panel for versioned, server-scoped config, loads it into an **in-process embedded sing-box** instance, and reports per-user traffic and online devices. The Agent never stores business data.

Control flow (design contract):

1. Admin change commits → the affected Server revision increments monotonically.
2. Agent heartbeats carry metrics; the heartbeat response returns the current revision.
3. Agent polls `GET /api/agent/config?version=<applied>`; unchanged revision → `{"status":"current"}` (no payload).
4. On `updated`, the Agent rebuilds its embedded sing-box instance from the rendered config (stops the previous instance, starts the new one) and registers a per-user connection tracker.
5. Any parse/start failure keeps the last applied revision; the failure is reported via heartbeat (`last_apply_error`) and logs, and the Agent keeps retrying on the next sync. The Panel stays unreachable-safe: the Agent keeps the last valid config loaded forever.

## Quickstart

### Panel

```sh
cp deploy/panel.example.yaml panel.yaml   # edit admin password (app_key optional)

make panel                    # go build -o panel ./cmd/panel
PANEL_ADMIN_PASSWORD=... ./panel
```

Panel config keys (yaml or `PANEL_*` env): `listen`, `db_path`, `log_level`, `app_key` (optional 32-byte hex; encrypts protocol secrets at rest — when omitted, one is auto-generated on first start and persisted in the `settings` table), `secure_cookie`, `admin.username`, `admin.password`, `retention.aggregate_days`, `retention.sweep_interval_seconds`, `retention.max_traffic_records`. Precedence: `PANEL_APP_KEY` env > yaml > database.

### Web UI

The Vue 3 frontend lives in `web/`. By default the Panel binary serves the API only ( `/` shows a hint page). To embed the built UI into the single binary:

```sh
make build-embed              # npm ci + npm run build + go build -tags embed_ui
```

`web/dist` is copied to `internal/webui/dist` by the Makefile and embedded behind the `embed_ui` build tag; building without the tag never requires the frontend.

### Agent

1. In the Panel web UI: **Servers → Server detail → Agent Key → 生成 Agent Key**. The key is shown at any time and can be reset; only its SHA-256 hash is used for authentication.
2. On the VPS (no separate sing-box install needed — it is embedded in the agent):

```sh
PANEL_DOWNLOAD_BASE=https://example.com/downloads/vps-node \
PANEL_VERSION=20260920 \
PANEL_URL=https://panel.example.com \
SERVER_ID=1 \
AGENT_KEY=<key-from-server-detail> \
sh install-agent.sh
```

`deploy/install-agent.sh` is idempotent: detects amd64/arm64/386, installs `/usr/local/bin/panel-agent`, writes `/etc/panel-agent/agent.yaml` only if absent, creates the `panel-agent` user and enables the hardened `panel-agent.service`.

The agent is **stateless**: it keeps no local file, so the container/process can be recreated at any time with only the panel URL, server id and Agent Key. Identity, batch-sequence resume points and the applied revision all live on the panel; the agent forces a config re-apply on every start and adopts the panel-reported sequence numbers on its first heartbeat. Resetting the Agent Key on the Server detail page invalidates the old key immediately — update the node config afterwards.

### Agent config reference

| Key | Default | Meaning |
| --- | --- | --- |
| `panel_url` | – (required) | Panel base URL; use HTTPS in production |
| `agent_key` | – (required) | long-lived per-server Agent Key (Server detail page) |
| `server_id` | – (required) | must match the server the key belongs to (cross-check only; the Panel derives ownership from the key) |
| `log_level` | `info` | `debug`…`error` |
| `heartbeat_interval` | `30s` | metrics + liveness; panel response can retune it |
| `sync_interval` | `30s` | config poll cadence (a newer-revision heartbeat triggers an extra poll) |
| `traffic_interval` | `60s` | traffic/device report cadence |
| `collection.traffic` | `true` | agent-side traffic collection switch |

All keys have `AGENT_*` environment equivalents (e.g. `AGENT_PANEL_URL`, `AGENT_KEY`). Per-user traffic is counted in-process by the embedded sing-box connection tracker; there is no external sing-box binary or config file to manage.

## Docker deployment

Docker is the second supported deployment path next to systemd. Two images are built from the repo root:

| Image | Contents | Base |
| --- | --- | --- |
| `vps-node-panel` | Panel binary with the web UI embedded (`embed_ui`) + `/panel-healthcheck` helper | `scratch` |
| `vps-node-agent` | `panel-agent` with embedded sing-box (`-tags with_quic,with_utls`) + CA certs | `scratch` |

The agent image is self-contained: sing-box is compiled into the `panel-agent` binary and runs in-process, so there is no separate sing-box binary, config file or reload helper to manage. All binaries are static (CGO off), so the runtime base is `FROM scratch`.

### Build

```sh
make docker-panel                                        # vps-node-panel:latest
make docker-agent                                        # vps-node-agent:latest
```

### Run: Panel only

```sh
cd deploy
cp .env.example .env      # then fill in PANEL_ADMIN_PASSWORD
docker compose up -d      # panel on :8080, SQLite in volume panel-data
```

Compose auto-loads `deploy/.env`; prefer it over shell exports so variables survive new shells and reboots (`PANEL_ADMIN_PASSWORD` is hard-required on every `up`). Set `PANEL_PORT` to publish a host port other than 8080 (e.g. when 8080 is taken). To pull a prebuilt image instead of building locally, set `PANEL_IMAGE` (e.g. `ghcr.io/<owner>/vps-node-panel:latest`) and run `docker compose pull` first.

No local build at all: `deploy/docker-compose.ghcr.yml` pulls the prebuilt image from GitHub Container Registry directly:

```sh
cd deploy
cp .env.example .env      # then fill in PANEL_ADMIN_PASSWORD
docker compose -f docker-compose.ghcr.yml pull
docker compose -f docker-compose.ghcr.yml up -d
```

It defaults to `ghcr.io/senhao-xu/vps-node-panel:latest` (override with `PANEL_IMAGE`). If the package is private, log in first with a PAT that has `read:packages`: `echo $GITHUB_TOKEN | docker login ghcr.io -u <username> --password-stdin`.

`PANEL_APP_KEY` is optional: without it the panel generates a random key on first boot and stores it in the database (`settings` table). Note the security trade-off: the key then lives next to the data it protects, so a stolen database can be decrypted; set an explicit key if that matters, and either way back up the `panel-data` volume — the key encrypts node secrets at rest. TLS terminates in a reverse proxy; a commented Caddy example is included in `deploy/docker-compose.yml` (the panel itself serves plain HTTP on :8080 only).

### Run: Agent on a node server

```sh
cd deploy
cp .env.example .env      # set AGENT_PANEL_URL, AGENT_SERVER_ID and AGENT_KEY
docker compose -f agent.docker-compose.yml up -d
```

The agent is stateless and mounts **no volume**: recreate the container at any time with the same env and it reconnects. It forces a config re-apply at startup, so a restart reloads the Panel-rendered config into the embedded sing-box instance and brings the nodes back up without any manual step — there is no separate sing-box container, binary, config volume, or state volume. Publish one port pair (`tcp`+`udp`) per node, e.g. `8388:8388/tcp` and `8388:8388/udp` (override the host side with `NODE_PORT`); with many nodes consider `network_mode: host`. Ports ≤1024 additionally require root (the container runs as root by default).

### Run: All-in-one (panel + agent on one host)

```sh
cd deploy
cp .env.example .env      # fill in the panel vars; up -d panel first, then the agent vars
docker compose -f docker-compose.all-in-one.yml up -d panel
# create the server (note its id N), a node on port 8388, then an Agent Key in the UI
# add AGENT_SERVER_ID=N and AGENT_KEY=<key> to .env
docker compose -f docker-compose.all-in-one.yml up -d panel-agent
```

Panel↔agent traffic stays on the compose-internal network (`http://panel:8080`); only the admin port and node ports are published.

### Measured footprint

The agent binary with the embedded sing-box runtime is about **38 MB** on linux/amd64 (built with `-tags with_quic,with_utls`, `CGO_ENABLED=0`); the panel is a few MB smaller. Idle memory: panel ≈ 6.5 MiB; agent with embedded sing-box ≈ 16 MiB, growing with active connections — size accordingly. Rebuild locally for current figures.

> The embedded sing-box build uses only the `with_quic,with_utls` tags, so the agent binary carries just the inbound protocols/transports it needs rather than the full upstream feature set.

### Upgrades

- **Panel**: `git pull && make docker-panel && docker compose up -d` — the image is replaced, the `panel-data` volume is untouched; roll back by re-deploying the previous image (no data migration either way).
- **Agent**: `git pull && make docker-agent && docker compose -f agent.docker-compose.yml up -d`. The agent keeps no local state, so the recreated container reconnects with the same Agent Key and resumes sequence numbers from the panel; traffic idempotency is preserved across upgrades. The embedded sing-box version is pinned in `go.mod`; bump it with `go get github.com/sagernet/sing-box@<version> && go mod tidy` and rebuild.

### Notes and caveats

- The runtime base is `scratch`: no shell, no busybox. Debug with direct execs of the agent binary, `docker cp`, or `docker top` (processes are visible from the host).
- The agent embeds sing-box in-process, so it forks no children and leaks no zombies; `init: true` in the compose files is harmless and remains recommended for general process hygiene.
- The agent reads host-style `/proc` metrics (`cpu`, `meminfo`, `uptime`), so reported CPU/memory percentages reflect the host, not the cgroup limit.
- The agent container has no writable state path: all persistent data (identity, sequence resume points, applied revision) lives on the panel, so only the env vars in `deploy/agent.example.yaml` are needed. Env overrides the YAML file, so bind-mounting an `agent.yaml` into this image only works for keys not pinned by these envs.

### systemd vs Docker

| | systemd | Docker |
| --- | --- | --- |
| Isolation | hardened units (`ProtectSystem=strict`, non-root, `CAP_NET_BIND_SERVICE`) | container boundary, root user by default |
| sing-box lifecycle | embedded in the agent process | embedded in the agent process |
| Upgrades | replace binary + restart | replace image + restart container |
| Data | filesystem paths | named volumes |
| Best for | long-lived dedicated VPS | fast provisioning, reproducible nodes, all-in-one test stacks |

Both paths are fully supported and produce identical panel-side behavior (same agent API). Pick systemd where the hardened non-root setup matters; pick Docker for repeatability and one-command bring-up.


## Data collection model

- **Traffic** is reported as per-interval **deltas** per (user, node), computed from the embedded sing-box connection tracker's per-`(user,node)` cumulative byte counters (read from inbound = upload, write to inbound = download). Every batch carries a monotonically increasing `batch_seq` per kind; the Panel treats `(agent, batch_seq)` as the idempotency key, so resend-on-uncertainty never double counts. Unconfirmed deltas stay pending and are retried with the same frozen payload (so a lost ack is deduplicated), while a permanently rejected batch (non-retryable 4xx, e.g. a `recorded_at` that aged out of the acceptance window during a long panel outage) is requeued under a fresh sequence instead of blocking the queue. Sequence numbers are agent-memory only: each heartbeat returns the panel's `MAX(seq)` per stream, the agent adopts `max(local, resume)`, and a restarted agent therefore resumes above every sequence the panel already saw (the panel is the single source of truth for idempotency).
- **Devices** are full-replacement snapshots per report; the Panel replaces the server's `online_devices` set transactionally and refreshes each user's `online_count`/`last_online_at`.
- **Visits** record the destination a user reached through a node: the requested host/port, `tcp`/`udp` network, and the client source IP. The agent reads sing-box `metadata.Destination` (the client-requested address; **no sniffing** is enabled, so a client that requests a bare IP is recorded as that IP), keeps a bounded in-memory queue (10000 entries, oldest dropped and counted on overflow), and reports them in the same idempotent `batch_seq` style as traffic. The Panel stores each record in `visit_records` and upserts a per-UTC-day `visit_daily_domains` aggregate used for "most visited" summaries. Visits never carry byte counters — traffic accounting stays on its own channel.
- **Attribution**: connections are attributed in-process from sing-box inbound metadata — the inbound tag (`vless-<id>` / `shadowsocks-<id>` / `hysteria2-<id>`) selects the node and the inbound user name (`u-<user_id>`, the name the Panel renders into sing-box) selects the user. Connections without reliable attribution are excluded from reports and counted in agent logs — missing fields are never inferred.

## Embedded sing-box runtime

> The agent embeds `github.com/sagernet/sing-box` (pinned in `go.mod`) and builds with `-tags with_quic,with_utls`, running one instance in-process. A custom `adapter.ConnectionTracker` wraps TCP and UDP connections and counts bytes per user as the traffic flows, so per-user accounting no longer depends on polling the Clash API. `go test -tags integration,with_quic,with_utls ./internal/e2e/` validates that the Panel-rendered config parses and starts against the pinned embedded sing-box (no external binary required).

## Auth model

| Scope | Mechanism | Routes |
| --- | --- | --- |
| Admin | HttpOnly session cookie `panel_session` (SameSite=Lax), bcrypt-hashed passwords | `/api/admin/*`, `/api/users/*`, `/api/servers/*`, `/api/nodes/*`, `/api/dashboard`, `/api/settings` |
| Agent | `Authorization: Bearer <agent_key>`; key stored hashed (`key_hash`) and encrypted at rest (`key_enc`, panel `app_key`) for reveal; reset invalidates instantly | `/api/agent/*` |

The two scopes share no middleware and no token space. Agent `server_id` is always derived server-side from the key; body-supplied IDs are only validated for ownership. The Agent Key is revealed on the Server detail page and returned by the agent-key endpoints. Full contract: [`docs/api-contract.md`](docs/api-contract.md).

## Retention

- Traffic records: default **90 days** (configurable via `/api/settings` and panel config); the janitor sweeps on a configurable interval. The `retention.max_traffic_records` cap (default 5,000,000; `0` disables) bounds storage even if the retention window is large — the oldest rows are deleted first. Stale `online_devices` rows are swept after 24h without a report.
- Traffic records feed user/node/server totals transactionally and survive user/server deletion until retention cleanup.
- Visit records: raw `visit_records` default **7 days** (`retention.visit_days`), the daily `visit_daily_domains` aggregate and `visit_batches` markers default **90 days** (`retention.visit_aggregate_days`). Visit collection can be switched off globally with `collection_visits`; the agent can also disable its local `collection.visits`. Recorded source IPs and destination hosts are sensitive operational data and never leave the Panel database.
- Heartbeat is the only liveness source; a server goes `offline` after `server_offline_after_seconds` (default 60s) without heartbeat. Traffic/device/visit reports never affect server online status.

## Development

```sh
make build              # vet + test + build both binaries
make test               # go test ./... (includes hermetic agent e2e with a stub kernel)
make test-race          # go test -race ./...
make test-integration   # go test -tags integration,with_quic,with_utls ./... (validates + starts the embedded sing-box)
make ui                 # build the Vue frontend (web/dist)
make build-embed        # frontend embedded into the panel binary
make release-agent      # linux amd64/arm64/386 release tarballs in dist/
make docker-panel       # build vps-node-panel image (web UI embedded)
make docker-agent       # build vps-node-agent image (agent with embedded sing-box)
```

Module layout:

```text
cmd/panel            panel binary (API + embedded UI)
cmd/agent            agent binary (heartbeat/sync/telemetry loop)
internal/web         panel HTTP handlers (admin + agent)
internal/repo        SQLite repositories and transactions
internal/singbox     config renderer + credential derivation contract
internal/kernel/singbox embedded sing-box runtime + per-user connection tracker (agent side)
internal/agentclient typed agent→panel HTTP client (retry/backoff, idempotent batches)
internal/agentruntime agent-side loop: config sync, telemetry deltas, metrics
internal/agentstate  in-memory agent state (batch seqs; no file IO)
internal/webui       web/dist embedding (build tag embed_ui)
internal/e2e         end-to-end smoke test + embedded sing-box integration tests
deploy/              systemd units, example configs, installer, Dockerfiles + compose files
```

## Deploy notes

- Production Agent↔Panel traffic must use **HTTPS with certificate verification** (behind a reverse proxy for Docker, e.g. Caddy — see the Docker section).
- systemd units are hardened (`NoNewPrivileges`, `ProtectSystem=strict`, `PrivateTmp`, non-root users, minimal `ReadWritePaths`).
- SQLite backups must copy the WAL/SHM set; when `app_key` is auto-generated it lives in the same database (`settings` table), so the backup already contains it — when set via env/yaml, preserve the key alongside the backup (it encrypts protocol secrets at rest).
- Start with one Panel and one canary Agent; pin the sing-box version on the canary before scaling.
- Node listen ports >1024 need no container privileges; ports ≤1024 require root (or `NET_BIND_SERVICE`). Node ports are published with `ports:` one pair per node, or `network_mode: host` when exposing many.
