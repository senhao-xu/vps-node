# Technical Design

## Images

```text
Dockerfile.panel:
  stage1 node:22-alpine   → web: npm ci && npm run build (dist/)
  stage2 golang:1.25      → go build -tags embed_ui -o /out/panel ./cmd/panel
  stage3 scratch|alpine   → panel + /data volume + EXPOSE 8080 + HEALTHCHECK /healthz

Dockerfile.agent:
  stage1 golang:1.25      → agent 二进制 + reload helper（Go 编译，非 shell）
  stage2                  → pinned sing-box（ARG SINGBOX_VERSION，官方 release 二进制或 alpine 包）
  stage3 scratch|alpine   → 两个二进制 + ca-certificates，无 shell 依赖
```

- `FROM scratch` 优先：两个 Go 二进制均静态（CGO_ENABLED=0），helper 取代 shell 脚本；ca-certificates 单独 COPY。若 alpine 取舍更简单，体积差约 7 MB，两案都在 60 MB 内——实现时先试 scratch，受阻再降级 alpine 并记录原因。
- sing-box 获取方式：`ARG SINGBOX_VERSION` + 官方 GitHub release tarball（amd64/arm64 双架构，`--platform` 匹配 TARGETARCH），下载校验后仅保留二进制。

## Agent 容器内进程模型

采用 **agent 父进程 + reload helper**（保持现有代码不变）：

1. agent 照常轮询，渲染配置落到 `singbox.config_path`（容器内 `/etc/sing-box/config.json`）。
2. `reload_command` 配置为 `/usr/local/bin/singbox-reload`（Go 辅助二进制，无参数）。
3. `singbox-reload` 职责：读取 pid 文件（`/run/singbox/singbox.pid`）→ 旧进程存在则 SIGTERM 并等待退出 → `fork/exec` 启动 `sing-box run -c <config>`，写 pid 文件 → 退出码 0/非 0 即 agent 日志中的重载结果。
4. 容器 entrypoint = agent 本体；sing-box 是 agent 的受管子进程（经 helper 首次拉起于第一次配置应用后），容器内无 init 系统需求；`stopsignal=SIGTERM` 直达 agent，agent 关闭时 helper 经由 pid 文件兜底杀 sing-box（agent 退出钩子可选，MVP 用容器 SIGKILL 兜底即可，文档注明）。
5. clash_api 渲染在 127.0.0.1:随机端口，agent 与 sing-box 同容器同 netns，采集路径与 systemd 形态完全一致，零代码改动。

## Panel 容器

- 数据：`-v panel-data:/data`，配置 `db_path: /data/panel.db`（env `PANEL_DB_PATH` 或挂载 yaml）。
- `app_key` 经 `PANEL_APP_KEY` env 注入（compose 示例用 `.env` 文件，git 忽略）。
- admin 引导：`PANEL_ADMIN_USER/PANEL_ADMIN_PASSWORD` env（已支持）。
- TLS：文档明确反代终结（compose 注释附 Caddy 两行示例）；容器只暴露 8080。

## Compose 与构建

- `deploy/docker-compose.yml`：panel 服务 + named volume + healthcheck；Caddy 示例放注释块。
- `deploy/agent.docker-compose.yml`：模板，`register_token`/`token` 经 env 注入，`network_mode: host` 不需要（合并镜像无需 host 网络；节点端口直接由容器端口映射或 host 模式二选一，示例默认端口映射并注明大量端口时的 host 模式选项）。
- `deploy/docker-compose.all-in-one.yml`（同节点单机部署，已确认双容器方案）：`panel` + `agent` 两个服务共享默认网络；agent 的 `panel_url=http://panel:8080`；节点端口由 agent 服务 ports 映射；一条 `docker compose -f deploy/docker-compose.all-in-one.yml up -d` 拉起全栈。明确不做单容器 all-in-one 镜像（生命周期耦合、镜像膨胀、升级绑定）。
- `.dockerignore`：`web/node_modules`、`*.db*`、`.trellis`、`deploy/*.yaml` 等。
- Makefile：`docker-panel`、`docker-agent`（可传 `SINGBOX_VERSION`）。

## Compatibility

- 不改任何 Go/Vue 业务代码（除新增 helper `cmd/` 与可能的 Makefile 行）；`reload_command` 语义不变（单二进制无参）。
- systemd 部署物保留为并行路径；README 给出选型对照（何时用哪种）。
- 回滚：镜像升级 = 拉新镜像重启容器；数据卷独立于镜像，回滚无数据迁移。

## Risks

- scratch 无 shell/调试工具 → 文档建议调试时用 `docker cp` 或临时 alpine 覆盖。
- sing-box 多架构下载源若被墙 → Dockerfile ARG 允许替换镜像源地址。
- helper 与 agent 的 pid 文件目录权限（`/run/singbox` tmpfs 或容器内普通目录）。
