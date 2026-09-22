# Implement: 对齐 Xboard 实体字段 + 嵌入式 sing-box 节点

> 前置：`prd.md`（D1–D8）、`design.md`。复杂任务，`task.py start` 前需本文件 + jsonl 就绪。
> 建议按阶段串行，每个阶段结束跑一次验证命令。

## Progress Log

- **2026-09-22 (session 1)**：完成 Phase 0 spike（go）。内嵌 sing-box **v1.13.2** 编译运行、
  真实流量触发 tracker 并拿到 `metadata.User`。构建 tag `with_quic`，`CGO_ENABLED=0`，
  二进制约 38MB，Go 1.25 无需切 toolchain。
  - 结论与复现：`research/embed-spike.md`；可运行源码：`research/embed-spike/{main.go,go.mod}`。
  - 环境：本机 `proxy.golang.org` IPv6 超时，需 `GOPROXY=https://goproxy.cn,direct GOSUMDB=off`。
  - 影响：design 的版本由 v1.14.1 改为 **v1.13.2**（v1.14 需 Go≥1.25.5 且多 `RoutedFlow`）。
- **未开始**：Phase 1–6（依赖接入 + 内嵌运行时 + 面板 schema/接口 + 前端）。
  - 注意：本期**尚未**改任何仓库代码（仅任务 artifacts + research 归档），`go.mod`/`Dockerfile`
    等生产文件保持原样。Phase 0 的 spike 在 `/tmp/opencode/sbspike`（已归档）。
- 新会话接手：先 `python3 ./.trellis/scripts/task.py current`，若未激活则
  `task.py start .trellis/tasks/09-22-xboard-parity`。
- **2026-09-22 (session 2)**：Phase 1 + Phase 2 由 trellis-implement 子代理完成。
  - 新增 `internal/kernel/singbox`（`runtime.go`/`tracker.go`/`types.go` + 测试）；agent 改为
    进程内嵌入 sing-box，`telemetryPass` 用 tracker 快照算增量并复用现有 traffic/session 上报。
  - 删除 `cmd/singbox-reload`、`agentruntime/{applier,checker,clash,collector}.go` 及测试、
    `e2e/testdata/fake_singbox.sh`；`config.go` 去掉 `SingBox`/`connection_logs` 配置。
  - 构建 tag 实测需 **`with_quic,with_utls`**（Reality 需 uTLS，design 原判断有误，已更正）。
  - Agent 侧 `prepareConfig` 会剥离 `experimental`（clash_api），因本期 panel 仍渲染它；
    Phase 4 移除后变 no-op。
  - 未实现：设备数限制/限速（仅记录 alive IP/在线数，gate 待后续）；Docker 构建未跑（沙箱无 docker）。
  - 校验：`go build ./...`、`go build -tags with_quic,with_utls ./...`、`go vet ./...`、
    `gofmt -l .`、`go test -count=1 ./...` 全过；`-race`（kernel/agentruntime/e2e）过。
- **2026-09-22 (session 2, trellis-check)**：校验并自修复 Phase 1+2。
  - 修复 CRITICAL-1：`go test -tags integration` 失败（Reality 需 uTLS）→ 集成测试移到
    `integration_embed_test.go` 并用 `integration,with_quic,with_utls` tag；`Makefile test-integration` 同步。
  - 修复 CRITICAL-2：流量重放丢增量——失败批次与新增量合并后复用同一 `batch_seq`，panel 去重
    返回旧结果导致新增量被丢弃。改为「冻结在途批次」（`inflight`，`buildTrafficBatches`），
    同一 seq 内容永不变化；新增 dedup server 回归测试。
  - INFO（延后，非缺陷）：设备数限制/限速未实现（PRD R2 要求本期至少设备限制 → **待补**）；
    devices 上报端点未接（本期仍走 sessions，Phase 3/4 切换）；socks inbound 在 `-race`
    下触发上游 `sing` LazyConn 竞态（已用 http inbound 规避）。
  - 全量校验（含 `-tags integration,with_quic,with_utls`、`-race`）通过。
- **2026-09-22 (session 2, Phase 3 + 契约)**：trellis-implement 完成面板数据模型重建 +
  agent 契约同步（traffic u/d + devices）。
  - Schema 压缩为单个 `0001_init.sql`：users 新字段（transfer_enable/u/d/speed_limit/
    device_limit/online_count/last_online_at）、nodes `protocol_settings`+`rate`+`tags`、
    `traffic_records` u/d、新增 `online_devices`/`device_batches`；删除 sessions/
    connection_logs/connection_log_batches 及 0002–0006。
  - repo/web/agent/agentclient 全部切换到 devices；`POST /api/agent/devices`；config 载荷加
    用户 device_limit/speed_limit；渲染器去掉 `clash_api`；`docs/api-contract.md` 已更新。
  - 设备数限制 gate 已在 tracker 实现（PRD AC4）。
  - trellis-check 复查并修复 3 个缺陷：① devices 分片上报被逐片 DELETE 覆盖（改为
    upsert + 按 recorded_at 剪除）；② device_limit/speed_limit 变更未 bump revision（已纳入）；
    ③ janitor 清理设备后 `online_count` 未刷新（同一 tx 重算）。
  - INFO 待决：`online_count` 语义（COUNT(*) 按 (user,node,ip) vs DISTINCT ip，契约措辞不一致）；
    design §4 顶层 `nodes[]` 未采用（改用每用户 nodes[]，自洽）。
  - 全量校验（build/vet/gofmt/test/-race/integration/embed_ui、前端 typecheck+build+lint）通过。
- **2026-09-22 (session 2, Phase 4 余项 + 5 + 6)**：全部完成。
  - Phase 4 余项：节点接口接受 `rate`/`tags`（校验/持久化/DTO/revision）；`protocol_settings`
    重构为 Xboard 分节（`internal/web/node_settings.go`），渲染器/订阅/前端同步读取。
  - Phase 5：前端对齐（types/NodeFormDialog 分节/用户字段/在线设备页/删除 sessions·logs 页）；
    check 修复 VLESS reality `server_port` 编辑被覆盖的缺陷。
  - Phase 6：`rate` 入库生效（`IngestTrafficBatch`，同一 batch tx）；`online_count` 改为
    `COUNT(DISTINCT ip)`；补 `online_devices.online` 在线连接数（补齐 AC3）；文档/spec 更新。
  - 多次 trellis-check 修复：devices 分片覆盖、limits 变更不 bump revision、janitor
    `online_count` 失准、流量/设备重放永久 422 死锁、rate 溢出、dashboard devices 语义等。
  - 最终校验全绿：`go build/vet/gofmt/test`（含 `-tags integration,with_quic,with_utls`）、
    前端 `typecheck/build/lint`。AC1–AC9 见 `prd.md`。
  - 未验证：`docker build`（沙箱无 Docker）；真实客户端端到端建议上线前手测。
- **2026-09-22 (session 2, 本机 Docker 实机验证 + 缺陷修复)**：完整跑通。
  - 重建 panel/agent 镜像（需 `with_quic,with_utls`；构建时经 goproxy.cn 拉模块），清空旧卷
    按新 schema 重建库，面板 API 完成 server/node/user/register-token 引导。
  - agent 注册→拉配置→内嵌 sing-box 起 `shadowsocks-1` inbound；用 sing-box CLI 作客户端
    经节点实际访问，面板 `used_bytes` 从 0 增长到 6025，agent 日志出现 `[u-1] inbound
    connection`，**AC1/AC2/AC5 实机通过**。
  - **发现并修复真实缺陷**：SS2022 多用户订阅/Clash 客户端密码只给了用户子密钥，sing-box
    服务端报 `shadowsocks: invalid request`；应为 `serverKey:userKey`。已在
    `internal/subscription/render.go`（`ssClientPassword`，URI+Clash 共用）+`singbox.IsSS2022`
    修复并加单测；重建 panel 后订阅链接直接可用（3 次请求 200），面板继续累加。
  - 环境备注：Docker Hub 需镜像（`docker.m.daocloud.io`）；容器内 `go mod download` 需
    `GOPROXY=https://goproxy.cn`。

## Phase 0 — Spike：嵌入 sing-box（已完成 ✅）

- [x] 版本定为 `github.com/sagernet/sing-box v1.13.2`（v1.14.x 需 Go ≥1.25.5 且多 `RoutedFlow`）。
- [x] 最小示例验证 `include.Context` + `UnmarshalExtendedContext[option.Options]` +
      `box.New` + `router.AppendTracker` + `Start`。
- [x] 构建 tag：`with_quic` 必需，`CGO_ENABLED=0` 可静态构建；二进制约 38MB。
- [x] 结论写入 `research/embed-spike.md`。
- 验证：SOCKS5 真实流量触发 tracker，`metadata.User=u1`，per-user 归因可得。

## Phase 1 — 依赖与构建

- [ ] `go.mod` 引入 `github.com/sagernet/sing-box v1.13.2`；`go mod tidy`。
- [ ] agent 构建加 `-tags with_quic`（Makefile + `deploy/Dockerfile.agent`）。
- [ ] 修改 `deploy/Dockerfile.agent`：移除 sing-box 下载 stage、`singbox-reload`、
      `AGENT_SINGBOX_*`；保留 `AGENT_STATE_PATH`。
- [ ] 删除 `cmd/singbox-reload/`；更新 `Makefile`/compose 中相关引用。
- [ ] 更新 `deploy/agent.example.yaml`、`deploy/*.yml` 与 README（去外部 sing-box 说明）。
- 验证：`go build ./...`；`docker build -f deploy/Dockerfile.agent .`。

## Phase 2 — Agent 嵌入式运行时（`internal/kernel/singbox` 或 `internal/agentruntime`）

- [ ] 新增 `ConnTracker`（`adapter.ConnectionTracker`）：按 `(userID,inboundTag)` 累加 u/d，
      维护 per-user alive IP(refcount) 与连接数；实现设备数限制 gate。
- [ ] 新增 `buildBox(configJSON, userMap, tracker)`：`box.New` + `Start`；`Stop` 支持重建。
- [ ] UUID→userID 映射来自配置 `users`；inbound tag = `<protocol>-<id>`（沿用面板约定）。
- [ ] `Snapshot()`：`map[(user,node)][2]int64` 累计 + `map[user]map[ip]bool` + 连接数。
- [ ] 改造 `loop.go` `telemetryPass`：累计→增量（`lastSeen`，负值置 0）、pending 重放、
      分别上报 traffic 与 devices；`syncPass` 在 revision 变化时重建 box。
- [ ] 删除/替换 `clash.go`、`collector.go`、`checker.go`、`applier.go`、`clash` 相关测试。
- [ ] `agentclient`：移除 `Sessions`/`ConnectionLogs`，新增 `Devices`；`Config` 结构加 nodes/users 限制字段。
- 验证：`go build ./...`、`go test ./internal/agentruntime/...`。
- 风险文件：`internal/agentruntime/loop.go`、`internal/agentclient/client.go`。

## Phase 3 — Panel 数据模型（SQLite 重建）

- [ ] 重写 `internal/db/migrations/0001_init.sql`（及后续）：`nodes` 加 `rate/tags`、
      `settings`→`protocol_settings`；`users` 加 `transfer_enable/u/d/speed_limit/device_limit/
      online_count/last_online_at`；新增 `online_devices`；`traffic_records` 列名对齐 `u/d`；
      删除 `sessions`/`connection_logs`/`connection_log_batches`。
- [ ] 删除 0004/0006 的 CHECK 重建类迁移中 sessions 相关部分（重建 schema 下可合并）。
- [ ] `internal/repo/`：更新类型/查询；删 sessions/connection_logs；新增 online_devices 与
      用户 u/d/limits 读写；`traffic.go` 列名。
- [ ] `internal/repo/telemetry.go`：改为 traffic(增量) + devices；删 sessions/logs 摄入。
- 验证：`go test ./internal/repo/... ./internal/db/...`。
- 风险文件：`internal/repo/telemetry.go`、`internal/repo/mutations.go`、migrations。

## Phase 4 — Panel 配置渲染与接口

- [ ] `internal/singbox/singbox.go`：设置读取改自 `protocol_settings`；去掉 `clash_api` 注入；
      保留 4 种协议渲染；`ContractVersion` 升级。
- [ ] `internal/web/agent_config.go`：载荷加 `nodes[]`、`users[]`(device_limit/speed_limit)；
      users 不再需要 credential 契约里的 clash 相关字段（保留凭证推导）。
- [ ] `internal/web/agent_telemetry.go`：改为 traffic + devices；删 sessions/connection-logs handler。
- [ ] `internal/web/web.go`：路由增删（+devices，-sessions/-connection-logs）。
- [ ] `internal/web/nodes.go`：`buildNodeSettings` 分节重写 + 校验；`users.go` 新字段。
- [ ] `internal/web/dto.go`、`subscriptions.go`：字段对齐。
- 验证：`go build ./...`、`go test ./internal/web/...`。

## Phase 5 — 前端（Vue3）

- [ ] `web/src/api/types.ts`：同步新 DTO（users 新字段、node protocol_settings、devices）。
- [ ] `NodeFormDialog.vue`：按 tls/network/reality 分节；协议仍 4 种。
- [ ] 用户表单/详情：新增 transfer_enable/speed_limit/device_limit；在线设备展示。
- [ ] 删除 `UserSessions.vue`、`UserConnectionLogs.vue` 及路由/API。
- [ ] 服务器/节点列表字段对齐（rate/tags/enabled）。
- 验证：`cd web && npm run build`（及 lint）。

## Phase 6 — 测试 / 文档 / 收尾

- [ ] 更新 `internal/e2e/` 用例；新增 tracker 单测（字节语义、设备 limit、增量重放）。
- [ ] 更新 `docs/api-contract.md`、README、部署文档。
- [ ] 全量：`go build ./... && go vet ./... && go test ./...`、`npm run build`、`docker build`。

## 验证命令汇总

```bash
go build ./...
go vet ./...
go test ./...
cd web && npm run build
docker build -f deploy/Dockerfile.agent .
```

## 风险文件 / 回滚点

- 高风险：`internal/agentruntime/*`（整体替换）、migrations、`internal/repo/telemetry.go`。
- 回滚：Phase 0 spike 是 go/no-go；运行时以新包隔离，失败可整体回退到当前外部 sing-box 路径。
- 无数据迁移，清库重建（panel.db + agent state）。

## task.py start 前检查

- [ ] Phase 0 spike 已执行并记录结论（或明确接受风险先做 Phase 1）。
- [ ] `implement.jsonl` / `check.jsonl` 已含真实 spec/research 条目。
- [ ] PRD/design/implement 三者一致，无阻塞 open question。
