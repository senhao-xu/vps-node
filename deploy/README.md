# Deploy

- `panel.service` / `panel-agent.service` — hardened systemd units (non-root users, `ProtectSystem=strict`, minimal `ReadWritePaths`). The agent unit grants write access to `/etc/sing-box` only.
- `install-agent.sh` — idempotent agent installer (arch detection, release tarball download, config bootstrap, systemd enable). See the root README for usage.
- `release-agent` Makefile target builds linux amd64/arm64/386 tarballs containing `panel-agent`, `panel-agent.service` and `install-agent.sh`.

Example configuration files:

- `panel.example.yaml` — copy to `panel.yaml` next to the panel binary (or configure via `PANEL_*` environment variables).
- `agent.example.yaml` — copy to `/etc/panel-agent/agent.yaml` (or configure via `AGENT_*` environment variables). Contains the full key reference including `singbox.reload_command` and collection switches.

`app_key` is a 32-byte hex string that encrypts protocol secrets at rest. It is **optional**: when neither `PANEL_APP_KEY` nor yaml `app_key` is set, the panel generates one on first start and persists it in the database `settings` table (so the key then lives next to the data it protects). To manage it yourself:

```sh
openssl rand -hex 32
```

Never commit real `panel.yaml` / `agent.yaml`; both are git-ignored because they contain secrets.
