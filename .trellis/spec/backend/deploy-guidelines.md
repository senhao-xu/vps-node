# Deployment Guidelines

> Docker + systemd deployment conventions and container gotchas.

---

## Docker (deploy/)

- Panel image: multi-stage (npm ci → `go build -tags embed_ui` CGO_ENABLED=0 → `scratch`); SQLite lives on the `/data` named volume; HEALTHCHECK uses the `cmd/panel-healthcheck` helper (scratch has no curl/wget).
- Agent image: `cmd/agent` built with `-tags with_quic,with_utls` (embedded sing-box as a Go library, pinned in `go.mod`), CGO off, `scratch` base with only CA certs. No external sing-box binary, config file, reload helper, or `SINGBOX_*` build args. Bump the embedded version with `go get github.com/sagernet/sing-box@<version> && go mod tidy`.
- Secrets never baked into images: config/tokens via env (`.env` file, gitignored + dockerignored) or mounted yaml. `.dockerignore` must cover `*.yaml` with tokens, `.env.*`, `*.db*`, `.git`, `.trellis`.
- Compose examples use required-var guards (`${VAR:?err}`) and soft defaults (`${VAR:-default}`) deliberately: panel can start before a register token exists.

## Container Process Model

- The agent embeds sing-box in-process and forks no children, so the old zombie/reload-helper gotcha no longer exists. `init: true` in compose is harmless and remains recommended for general process hygiene.
- Applying a new revision stops the previous `box.Box` and starts a fresh one under a mutex; a failed start keeps the last applied revision and retries on the next sync (reported via heartbeat `last_apply_error`).
- The embedded instance binds node listen ports directly, so the systemd unit needs `CAP_NET_BIND_SERVICE` for ports ≤1024.

## Operations

- Upgrade = rebuild/pull image + recreate container; data survives via named volumes (SQLite verified across recreate).
- Panel TLS terminates at a reverse proxy (Caddy example in `deploy/docker-compose.yml` comments); panel itself serves plain HTTP on 8080.
- The embedded sing-box version is pinned in `go.mod`; README figures (binary size ~38 MB, tags) must match the build.
- systemd remains a parallel first-class path (`deploy/*.service` + `install-agent.sh`); selection guidance in README.
