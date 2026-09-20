# Docker 部署支持（Panel + Agent）

## Goal

为 vps-node 提供 Docker 部署路径：Panel 一条命令起服务（数据持久化），Agent 在节点服务器上以合并镜像容器方式运行 sing-box + 采集上报。补齐现有 systemd 之外的第二种部署形态。

## Background（仓库证据）

- Panel：静态 Go 二进制 + SQLite（`db_path`）+ `app_key` 必需；`go build -tags embed_ui` 可嵌入 web/dist 为单二进制；当前仅 HTTP 监听（`listen`），无内置 TLS（`cmd/panel/main.go`）。
- Agent：`internal/config` 已支持 `state_path`、`singbox.config_path/check_bin/reload_command`、`AGENT_*` 环境变量覆盖；`reload_command` 限制为「单个二进制 + 固定参数」，为空则不自动重载（`internal/agentruntime/applier.go:35`）；采集走渲染配置里的 clash_api（127.0.0.1 随机端口 + secret），要求 agent 与 sing-box 同网络命名空间。
- 现有部署物：`deploy/panel.service`、`deploy/panel-agent.service`、`deploy/install-agent.sh`、`Makefile`（build/release-agent 等）。
- sing-box 用户级流量计量仍待 pinned 版本验证（与部署形态无关，保持 pending）。

## Key Decisions

- **Agent 采用合并镜像**（用户已确认）：agent + pinned sing-box 同容器；重载用 Go 编译的小型辅助进程（`cmd/singbox-reload`，符合「单二进制无参」约束）替代 shell 脚本，使镜像可用 `scratch` 底座；镜像**拉取/序列化体积 ≤ 60 MB（实测 35–37 MB）**，解包体积 ≤ 150 MB（官方 musl sing-box 单二进制即 ~92 MB，进一步缩小需源码最小 tag 构建）；空闲 RSS ≤ 80 MB（实测 ~15 MB）。
- Panel TLS 由反向代理终结（文档 + 可选 Caddy compose 示例），Panel 本体不内置 TLS。
- sing-box 版本以 Dockerfile ARG pin（默认写入一个当前稳定版，构建可覆盖）。
- 镜像发布流水线（CI 推 GHCR 等）不在本期，本地/自建 `docker build` 为主。

## Requirements

1. `deploy/Dockerfile.panel`：多阶段（npm build → `go build -tags embed_ui` → 最小运行镜像）；`PANEL_*` env 全量可用；`/data` 卷持久化 SQLite；HEALTHCHECK 用 `/healthz`。
2. `deploy/Dockerfile.agent`：包含 agent 二进制 + pinned sing-box + 重载辅助二进制；配置/状态/渲染产物路径容器化（`/etc/vps-node-agent`、`/var/lib/panel-agent`）；entrypoint 负责启动 agent（agent 以 reload_command 拉起 sing-box 子进程，或 entrypoint supervisor 方式，见 design）。
3. `deploy/docker-compose.yml`（Panel，含可选 Caddy TLS 注释示例）、`deploy/agent.docker-compose.yml`（节点侧模板）、`deploy/docker-compose.all-in-one.yml`（同节点双容器一键起：panel + agent 走 compose 内部网络，`panel_url: http://panel:8080`，用户已确认采用双容器而非单容器 all-in-one 镜像）。
4. `.dockerignore`；Makefile 增加 `docker-panel` / `docker-agent` 构建目标。
5. README 部署章节补 Docker 路径：构建、运行、升级（镜像替换 + 数据卷）、与 systemd 方式的选择建议。
6. 现有 systemd 路径与全部现有测试不受影响。

## Acceptance Criteria

- [ ] `docker build -f deploy/Dockerfile.panel .` 成功；`docker run` 后 `/healthz` 返回 ok；容器重建后 SQLite 数据仍在（卷挂载验证）。
- [ ] `docker build -f deploy/Dockerfile.agent .` 成功；容器内 agent 完成 register → heartbeat → config 拉取 → sing-box check → 配置生效 → 重载辅助进程按 reload_command 触发（可用 fake sing-box 或真实 sing-box check 验证）。
- [ ] 面板侧能看到该容器 Agent 在线，并出现真实/伪造流量批次被幂等累计。
- [ ] 合并镜像拉取/序列化体积 ≤ 60 MB（实测 35–37 MB 达标）、解包体积 ≤ 150 MB（实测 138 MB，主因 sing-box musl 二进制 92 MB）、空闲内存 ≤ 80 MB（实测值记录进 README）。
- [ ] `go build ./...`、`go test ./...`、`web` 三件套（typecheck/build/lint）全部保持通过。
- [ ] README 含 Docker 快速上手与升级说明；compose 示例可直接使用。

## Out Of Scope

- CI 镜像发布流水线、GHCR 自动推送。
- Kubernetes/Helm/Swarm、容器编排高可用。
- Panel 内置 TLS。
- sing-box 用户级流量计量的 pinned 版本验证（独立事项，README 维持 pending 声明）。

## Risks And Deferred Items

- scratch 底座无 shell：`docker exec` 调试不便；若取舍后选 alpine 则体积 +7 MB（在 60 MB 目标内）。
- sing-box 内存随连接数增长，80 MB 空闲目标不含高负载场景（README 注明）。
- 容器内 `sing-box check` 需要 CAP_NET_ADMIN 之外的权限吗——check 仅解析配置不需要特权；运行期监听低端口需注意（节点端口 >1024 时无额外要求，写入文档）。
