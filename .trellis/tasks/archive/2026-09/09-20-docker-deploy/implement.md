# Implementation Plan

## Ordered Work

1. `.dockerignore` + `deploy/Dockerfile.panel`（多阶段：web build → embed_ui go build → scratch/alpine），本地构建并跑通 `/healthz` + 数据卷持久化手测。
2. `cmd/singbox-reload`（Go 重载辅助二进制：pid 文件、SIGTERM 旧进程、fork/exec 新 sing-box、退出码语义）+ 单元测试（fake 进程）。
3. `deploy/Dockerfile.agent`（agent + helper + pinned sing-box 双架构），本地构建验证体积。
4. 容器级验证：compose 起 Panel → 注册 token → Agent 容器走完 register/heartbeat/config/check/应用/重载 → 面板显示在线 + 流量幂等批次（fake 或真实 sing-box）。
5. `deploy/docker-compose.yml` + `deploy/agent.docker-compose.yml` + Makefile 目标（`docker-panel`/`docker-agent`）。
6. README Docker 章节（快速上手、升级、体积/内存实测数据、与 systemd 选型对照、端口/权限注意项）。
7. 全量回归：go build/vet/test/-race、web 三件套、make build；PRD 验收标准逐条核对。

## Validation Commands

```bash
docker build -f deploy/Dockerfile.panel -t vps-node-panel .
docker build -f deploy/Dockerfile.agent --build-arg SINGBOX_VERSION=<pinned> -t vps-node-agent .
docker compose -f deploy/docker-compose.yml up -d && curl -f localhost:8080/healthz
docker run --rm vps-node-agent --help   # config 校验路径
go test ./... && (cd web && npm run typecheck && npm run build && npm run lint)
docker images | grep vps-node   # 体积实测
```

## Review Gates

- Agent 镜像内不得包含 shell 依赖（若 scratch 成立）；构建产物体积写入验收记录。
- reload helper 必须满足既有 applier 校验（单二进制无参）——`internal/agentruntime/applier.go:35` 的校验用例覆盖。
- 容器改动不得触碰业务代码路径；现有测试全绿为准入条件。
- sing-box 版本必须 pin 且与 README 一致。

## Rollback Points

- Dockerfile 独立于现有部署物，任何失败可直接删除新增文件回滚，systemd 路径不受影响。
- scratch 受阻降级 alpine 是单点改动（base image 行）。

## Deferred Follow-Ups

- GHCR 发布流水线、多 tag 策略。
- Kubernetes manifest。
- sing-box 计量 pinned 验证（独立任务）。
