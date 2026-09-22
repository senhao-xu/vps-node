# Deploy

- `panel.service` / `panel-agent.service` — hardened systemd units (non-root users, `ProtectSystem=strict`, minimal `ReadWritePaths`). The agent unit grants `CAP_NET_BIND_SERVICE` so embedded sing-box can listen on ports ≤1024.
- `install-agent.sh` — idempotent agent installer (arch detection, release tarball download, config bootstrap, systemd enable). See the root README for usage.
- `release-agent` Makefile target builds linux amd64/arm64/386 tarballs containing `panel-agent`, `panel-agent.service` and `install-agent.sh`.

The agent embeds sing-box as a Go library and runs it in-process. There is **no external sing-box install, no config file under `/etc/sing-box`, and no reload helper**: the agent forces a config re-apply at startup and rebuilds the instance on every revision change. Agent builds must use `-tags with_quic,with_utls` (hysteria2 QUIC + VLESS Reality uTLS); the resulting binary is ~38 MB. `release-agent` and the agent Dockerfile already pass these tags.

Compose files:

- `docker-compose.yml` — panel only, builds the image locally (default).
- `docker-compose.ghcr.yml` — panel only, pulls the prebuilt image from GHCR (`ghcr.io/senhao-xu/vps-node-panel:latest`, override with `PANEL_IMAGE`); no local build.
- `docker-compose.all-in-one.yml` — panel + agent on one host, builds locally.
- `agent.docker-compose.yml` — agent only, for node servers.

Example configuration files:

- `panel.example.yaml` — copy to `panel.yaml` next to the panel binary (or configure via `PANEL_*` environment variables).
- `agent.example.yaml` — copy to `/etc/panel-agent/agent.yaml` (or configure via `AGENT_*` environment variables). Contains the full key reference and collection switches.

`app_key` is a 32-byte hex string that encrypts protocol secrets at rest. It is **optional**: when neither `PANEL_APP_KEY` nor yaml `app_key` is set, the panel generates one on first start and persists it in the database `settings` table (so the key then lives next to the data it protects). To manage it yourself:

```sh
openssl rand -hex 32
```

Never commit real `panel.yaml` / `agent.yaml`; both are git-ignored because they contain secrets.
