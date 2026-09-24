# vps-node

Lightweight multi-node proxy management panel: **Panel** (single source of truth) + **Agent** (node runtime with embedded sing-box).

轻量级多节点用户管理面板：**Panel**（唯一业务数据源）+ **Agent**（内嵌 sing-box 的节点运行时）。

```text
Browser --> Panel API + Web UI --SQLite
                  ^
                  | REST polling + Agent Bearer Token
                  v
            Agent (one per Server) --> embedded sing-box (in-process, many Nodes)
```

---

## English

### Components

- **Panel** — owns users, servers, nodes, authorizations, traffic totals and online devices.
- **Server** — one VPS with exactly one Agent; may host many protocol Nodes.
- **Node** — one sing-box inbound (Shadowsocks 2022 / VLESS Reality / Hysteria2 / AnyTLS) with a port and protocol settings.
- **Agent** — stateless; polls versioned server-scoped config, runs it in an in-process embedded sing-box, reports per-user traffic, devices and visits.

### Docker Compose (recommended)

Prerequisites: Docker + Docker Compose v2. Run all commands from `deploy/`; Compose auto-loads `deploy/.env`.

```sh
cd deploy
cp .env.example .env      # set PANEL_ADMIN_PASSWORD (required)
```

| Compose file | Starts | Notes |
| --- | --- | --- |
| `docker-compose.yml` | Panel | builds locally, UI + SQLite on `:8080` |
| `docker-compose.ghcr.yml` | Panel | pulls the prebuilt image, no local build |
| `agent.docker-compose.yml` | Agent | for a node server (credentials come from the panel) |

Start the panel first (choose one):

```sh
docker compose up -d --build                       # build locally
docker compose -f docker-compose.ghcr.yml up -d     # prebuilt image
```

Then create the agent **from the panel** — the Agent Key can only be issued there:

1. Open http://localhost:8080 and log in.
2. Create a Server, then add a Node (e.g. on `:8388`).
3. Server detail → **Agent Key → generate**, then copy the ready-to-run command
   (binary install script or `docker run`) and execute it on the node VPS.

The copied `docker run` command is self-contained: it uses `--network host` and the
`ghcr.io/senhao-xu/vps-node-agent` image. Node hosts that prefer compose can fill
`.env` with `AGENT_PANEL_URL`, `AGENT_SERVER_ID` and `AGENT_KEY`, then run:

```sh
docker compose -f agent.docker-compose.yml up -d
```

> There is intentionally no one-shot all-in-one compose: the agent needs a Server and
> an Agent Key that exist only after the panel is up, so the panel is always started first.

Prebuilt image (no local build):

```sh
docker compose -f docker-compose.ghcr.yml pull
docker compose -f docker-compose.ghcr.yml up -d
```

Notes:

- `PANEL_PORT` overrides host port 8080; `NODE_PORT` overrides the published node port (default `8388` tcp+udp).
- Publish one port pair per node, or use `network_mode: host` when exposing many ports.
- TLS terminates at a reverse proxy (Caddy example commented in `docker-compose.yml`); the panel itself serves plain HTTP.
- Back up the `panel-data` volume: it holds the SQLite DB and the `app_key` that encrypts secrets at rest.

### Configuration

Panel (`panel.example.yaml` or `PANEL_*` env): `listen`, `db_path`, `log_level`, `app_key` (optional 32-byte hex), `secure_cookie`, `admin.username`, `admin.password`, `retention.*`. Precedence: env > yaml > database.

Agent (`AGENT_*` env or `agent.yaml`): `panel_url`, `server_id`, `agent_key` (required), `log_level`, `heartbeat_interval` (30s), `sync_interval` (30s), `traffic_interval` (60s), `collection.traffic`.

### Local build & development

```sh
make build              # vet + test + build panel & panel-agent
make ui                 # build the Vue frontend (web/dist)
make build-embed        # embed the UI into the panel binary
make docker-panel       # build the vps-node-panel image
make docker-agent       # build the vps-node-agent image
make test               # go test ./...
make test-integration   # validates and starts the embedded sing-box
```

Layout: `cmd/panel`, `cmd/agent`, `internal/{web,repo,singbox,kernel,agentclient,agentruntime,agentstate,webui,e2e}`, `web/`, `deploy/`.

### Docs

- API contract: [`docs/api-contract.md`](docs/api-contract.md)
- Deploy files, systemd units and examples: [`deploy/README.md`](deploy/README.md)

---

## 中文

### 组成

- **Panel** — 管理用户、服务器、节点、授权、流量总量与在线设备，是唯一业务数据源。
- **Server** — 一台 VPS 对应恰好一个 Agent，可承载多个协议节点。
- **Node** — 一个 sing-box 入站（Shadowsocks 2022 / VLESS Reality / Hysteria2 / AnyTLS），含端口与协议参数。
- **Agent** — 无状态；轮询按服务器版本化的配置，在进程内嵌 sing-box 运行，上报各用户流量、设备与访问记录。

### Docker Compose（推荐）

前置条件：Docker + Docker Compose v2。命令均在 `deploy/` 下执行，Compose 自动加载 `deploy/.env`。

```sh
cd deploy
cp .env.example .env      # 设置 PANEL_ADMIN_PASSWORD（必填）
```

| Compose 文件 | 启动 | 说明 |
| --- | --- | --- |
| `docker-compose.yml` | Panel | 本地构建，UI + SQLite 于 `:8080` |
| `docker-compose.ghcr.yml` | Panel | 拉取预构建镜像，不本地构建 |
| `agent.docker-compose.yml` | Agent | 用于节点服务器（凭证由面板下发） |

先启动面板（二选一）：

```sh
docker compose up -d --build                      # 本地构建
docker compose -f docker-compose.ghcr.yml up -d    # 预构建镜像
```

然后**从面板创建 agent** —— Agent Key 只能由面板签发：

1. 打开 http://localhost:8080 并登录。
2. 创建 Server，再添加一个 Node（例如 `:8388`）。
3. 进入 Server 详情 → **Agent Key → 生成**，复制生成好的命令（安装脚本或 `docker run`），
   在节点 VPS 上执行。

复制出的 `docker run` 命令是自包含的：使用 `--network host` 与
`ghcr.io/senhao-xu/vps-node-agent` 镜像。节点侧若偏好 compose，可把
`AGENT_PANEL_URL`、`AGENT_SERVER_ID`、`AGENT_KEY` 填入 `.env` 后执行：

```sh
docker compose -f agent.docker-compose.yml up -d
```

> 刻意不提供一键 all-in-one compose：agent 依赖的 Server 与 Agent Key 只有在面板启动后
> 才会存在，因此必须先启动面板。

使用预构建镜像（不本地构建）：

```sh
docker compose -f docker-compose.ghcr.yml pull
docker compose -f docker-compose.ghcr.yml up -d
```

说明：

- `PANEL_PORT` 覆盖宿主 8080 端口；`NODE_PORT` 覆盖发布节点端口（默认 `8388` tcp+udp）。
- 每个节点发布一组端口；节点较多时可改用 `network_mode: host`。
- TLS 由反向代理终止（`docker-compose.yml` 内附 Caddy 示例注释）；面板本身只提供 HTTP。
- 备份 `panel-data` 卷：其中包含 SQLite 数据库，以及加密静态密钥的 `app_key`。

### 配置

Panel（`panel.example.yaml` 或 `PANEL_*` 环境变量）：`listen`、`db_path`、`log_level`、`app_key`（可选 32 字节 hex）、`secure_cookie`、`admin.username`、`admin.password`、`retention.*`。优先级：env > yaml > 数据库。

Agent（`AGENT_*` 环境变量或 `agent.yaml`）：`panel_url`、`server_id`、`agent_key`（必填）、`log_level`、`heartbeat_interval`（30s）、`sync_interval`（30s）、`traffic_interval`（60s）、`collection.traffic`。

### 本地构建与开发

```sh
make build              # vet + test + 构建 panel 与 panel-agent
make ui                 # 构建 Vue 前端（web/dist）
make build-embed        # 将 UI 内嵌进 panel 二进制
make docker-panel       # 构建 vps-node-panel 镜像
make docker-agent       # 构建 vps-node-agent 镜像
make test               # go test ./...
make test-integration   # 校验并启动内嵌 sing-box
```

目录：`cmd/panel`、`cmd/agent`、`internal/{web,repo,singbox,kernel,agentclient,agentruntime,agentstate,webui,e2e}`、`web/`、`deploy/`。

### 文档

- API 契约：[`docs/api-contract.md`](docs/api-contract.md)
- 部署文件、systemd 单元与示例：[`deploy/README.md`](deploy/README.md)
