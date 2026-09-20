# Deployment Guidelines

> Docker + systemd deployment conventions and container gotchas.

---

## Docker (deploy/)

- Panel image: multi-stage (npm ci → `go build -tags embed_ui` CGO_ENABLED=0 → `scratch`); SQLite lives on the `/data` named volume; HEALTHCHECK uses the `cmd/panel-healthcheck` helper (scratch has no curl/wget).
- Agent image: agent + `cmd/singbox-reload` + pinned sing-box (`ARG SINGBOX_VERSION`, default variant must be **`-musl`** — the default release variant is glibc-dynamic and cannot run on scratch/alpine). sha256 checksums pinned via `SINGBOX_SHA256_*` ARGs; bump procedure in README.
- Secrets never baked into images: config/tokens via env (`.env` file, gitignored + dockerignored) or mounted yaml. `.dockerignore` must cover `*.yaml` with tokens, `.env.*`, `*.db*`, `.git`, `.trellis`.
- Compose examples use required-var guards (`${VAR:?err}`) and soft defaults (`${VAR:-default}`) deliberately: panel can start before a register token exists.

## Container Process Model (critical gotcha)

> **Warning**: Go programs ignore SIGCHLD by default, so any child they fork/exec and orphan (e.g. `singbox-reload` spawning sing-box, then exiting) becomes a **zombie reparented to PID 1** — and scratch-based containers have no init to reap it. `docker top` hides defunct processes; check with `ps aux | grep defunct` on the host.

- Agent containers MUST run with an init: `init: true` in compose, or `--init` for bare `docker run`.
- Verified live: without init, one `<defunct>` leaked per config reload; with `init: true`, zero.
- `singbox-reload` pid-file discipline: refuse to signal a pid it did not start (pid-reuse guard); SIGTERM → wait → SIGKILL fallback; atomic pid write. Known accepted TOCTOU window (<1ms, container-local) documented; pidfd is the follow-up if ever needed.

## Operations

- Upgrade = rebuild/pull image + recreate container; data survives via named volumes (SQLite verified across recreate).
- Panel TLS terminates at a reverse proxy (Caddy example in `deploy/docker-compose.yml` comments); panel itself serves plain HTTP on 8080.
- sing-box version is pinned in Dockerfile ARG and README must match; checksums pinned alongside.
- systemd remains a parallel first-class path (`deploy/*.service` + `install-agent.sh`); selection guidance in README.
