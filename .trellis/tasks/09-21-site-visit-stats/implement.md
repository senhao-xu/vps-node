# Implement: 访问站点统计

> 前置：`prd.md`（D1–D8、R1–R7、A1–A2）、`design.md`。复杂任务，`task.py start` 前需本文件 +
> `implement.jsonl` / `check.jsonl` 就绪。建议按阶段串行，每阶段结束跑一次验证命令。
> 实现须用 `trellis-implement` 子代理执行；每阶段后跑 `trellis-check`（至少阶段末一次全量）。

## Progress Log

- **2026-09-22 (planning)**：完成需求澄清 + 调研（`research/telemetry-data-path.md`）+ prd/design/implement。
  代码尚未改动。决策：仅用 `metadata.Destination`（不启用 sniff）；原始+每日聚合；全局开关+保留设置；
  展示于用户详情/服务器详情/独立访问记录页。新会话接手先 `python3 ./.trellis/scripts/task.py current`，
  若未激活则 `task.py start .trellis/tasks/09-21-site-visit-stats`。
- **2026-09-22 (backend P0–P3 complete)**：实现 Agent 采集/上报、Panel 迁移/Repo/采集处理、管理端查询/设置/Janitor。
  - P1：`internal/kernel/singbox`（`Visit`、有界队列 `maxVisits=10000`、`recordVisit`/`drainVisits`、
    `Runtime.DrainVisits()`）、`internal/agentruntime/loop.go`（`Kernel.DrainVisits`、`flushVisits`/`buildVisitBatches`）、
    `internal/agentstate`（`VisitBatchSeq`）、`internal/config`（`Collection.Visits` + yaml/env）、
    `internal/agentclient`（`Visits`）。
  - P2：`internal/db/migrations/0002_visit_records.sql`（三表+索引）、`internal/repo/visits.go`（单事务幂等+每日聚合）、
    `internal/repo/cleanup.go` 三个删除、`POST /api/agent/visits`（校验矩阵 + 关闭开关 `accepted:false`）。
  - P3：`GET /api/users/{id}/visits`、`GET /api/servers/{id}/visits`、`GET /api/visits`、`GET /api/visits/top`、
    三个设置键与 DTO、Janitor 按 `retention.visit_days`(7) / `retention.visit_aggregate_days`(90) 清理。
  - 验证（全部通过）：`gofmt -l .`、`go build ./...`、`go vet ./...`、`go test -count=1 ./...`、
    `go test -race -count=1 ./...`、`go build -tags with_quic,with_utls ./...`、
    `go test -count=1 -tags integration,with_quic,with_utls ./internal/e2e/...`。
  - 未做（属 P5/check 或 P4）：`error-handling.md`/`quality-guidelines.md` 中已移除特性的 spec 残留；
    `web/` 前端（P4，独立代理执行）。
- **2026-09-22 (P4 + P5 check complete)**：前端（P4）已实现并通过 `typecheck/build/lint`；`trellis-check` 全量复审完成。
  - **修复（CRITICAL/WARNING 级）**：`internal/agentruntime/loop.go` 的 `flushVisits` 原实现直接镜像
    `flushDevices`，在「可重试错误冻结在途批次」期间，后续 telemetry 周期 drain 出的新访问事件因
    `visitInflight` 非空而被静默丢弃（visits 是增量事件，不像 devices 那样能用下一次全量快照恢复）。
    新增 `Loop.pendingVisits` 有界缓冲（`maxPendingVisits=10000`，溢出丢最旧并告警），drain 的事件先入
    pending，inflight 清空后再按新序号构建批次；冻结批次仍原样重放，不可重试错误仍推进序号并丢弃该批。
    回归测试：`internal/agentruntime/loop_test.go` `TestTelemetryVisitReplayKeepsNewVisits`。
  - **P5 spec 残留修正**：`.trellis/spec/backend/error-handling.md`（Tests Required 的
    `connection-logs`/`destination_host` 描述）与 `quality-guidelines.md`（`decodeJSONStrict` 描述）改为
    反映现实：agent payload 走 `decodeJSON`、未知字段按 binding `api-contract.md` 忽略，未声明字段不得
    用于归属或 telemetry 计费。
  - **验证（全部通过）**：`gofmt -l .`、`go build ./...`、`go vet ./...`、`go test -count=1 ./...`、
    `go test -race -count=1 ./...`、`go test -count=1 -tags integration,with_quic,with_utls ./...`、
    `go build -tags with_quic,with_utls ./...`、`go build -tags embed_ui ./...`、`make build`、
    `cd web && npm run typecheck && npm run build && npm run lint`。
  - **未修复（报告级风险）**：迁移编号与 xboard-parity 的 squash 存在升级风险（见 check 报告）。

## 阶段总览

| 阶段 | 内容 | 主要文件 | 门禁 |
| --- | --- | --- | --- |
| P0 | 冻结契约 | `docs/api-contract.md`、`README.md` | 端点/字段经人工确认 |
| P1 | 内核采集 + Agent 上报 | `internal/kernel/singbox/*`、`internal/agentruntime/loop.go`、`internal/agentclient/client.go`、`internal/agentstate/state.go`、`internal/config/config.go`、`internal/e2e/*` | `go build ./...`、`go test ./internal/kernel/... ./internal/agentruntime/... ./internal/e2e/` |
| P2 | Panel 迁移 + Repo + 采集处理 | `internal/db/migrations/0002_visit_records.sql`、`internal/db/db_test.go`、`internal/repo/visits.go`、`internal/repo/cleanup.go`、`internal/web/agent_telemetry.go`、`internal/web/web.go` | `go test ./internal/db/... ./internal/repo/... ./internal/web/...` |
| P3 | 管理端查询 + 设置 + Janitor | `internal/web/{dto,user_data,servers,settings}.go`、`internal/janitor/janitor.go` | 同上 + `go test ./...` |
| P4 | Web UI | `web/src/api/types.ts`、`web/src/api/visits.ts`、`web/src/components/user/UserVisits.vue`、`web/src/pages/{UserDetail,ServerDetail,Visits,Settings}Page.vue`、`web/src/router/index.ts`、`web/src/components/AppLayout.vue` | `cd web && npm run typecheck && npm run build && npm run lint` |
| P5 | 全量校验 + 规格更新 | `.trellis/spec/**`、进度记录 | 见"全量验证" |

---

## P0 冻结契约（先于代码）

- [ ] 在 `docs/api-contract.md` 写入 agent `POST /api/agent/visits` 与 4 个管理端端点、
      `collection.visits` / `retention.visit_days` / `retention.visit_aggregate_days` 设置键；
      明确未知字段按现状忽略、字段上限（host 253、ip 64、batch 1000、时间 ±24h）。
- [ ] `README.md`「Data collection model」补 visits 段与隐私/保留说明。
- [ ] 复核 A1：确认「节点详情页」由服务器详情页承载；若需独立节点页，先回到 `prd.md` 调整。

**验证**：人工通读，确认端点/字段/上限与 `design.md` 一致。

## P1 内核采集 + Agent 上报

- [ ] `internal/kernel/singbox/types.go`：新增 `Visit` 结构。
- [ ] `internal/kernel/singbox/tracker.go`：
  - [ ] 新增 `visits []Visit`、`visitsDropped`、`maxVisits=10000`、`recordVisit`、`drainVisits`。
  - [ ] `entry` 在 `admissionTracked` 分支记录（`Destination.Fqdn` 优先，否则 `Addr.Unmap().String()`；
        空则跳过；host trim+lower+截断 253；`Network` 取 `metadata.Network`）。
  - [ ] 队列满丢最旧 + 计数。
  - [ ] 补 `tracker_test.go`：正常记录、空目标跳过、小写/截断、队列上限丢最旧。
- [ ] `internal/kernel/singbox/runtime.go`：新增 `DrainVisits()`；`Snapshot()` 不变。
- [ ] `internal/agentruntime/loop.go`：
  - [ ] `Kernel` 接口加 `DrainVisits()`；`Loop` 加 `visitInflight`/`visitInflightIdx`。
  - [ ] `telemetryPass` drain +（`Collection.Visits` 时）`flushVisits`；关闭时丢弃。
  - [ ] `flushVisits`（镜像 `flushDevices`）+ `buildVisitBatches`（排序/1000 分块/空不发）+ `run` 段。
  - [ ] `EnsureIdentity` 重置 `VisitBatchSeq`。
- [ ] `internal/agentstate/state.go`：`VisitBatchSeq`。
- [ ] `internal/config/config.go`：`Collection.Visits`（默认 true）+ yaml `collection.visits` +
      env `AGENT_COLLECTION_VISITS`。
- [ ] `internal/agentclient/client.go`：`VisitRecord`/`VisitBatch`/`VisitAck` + `Visits()`。
- [ ] `internal/e2e/agent_flow_test.go`：`stubKernel` 实现 `DrainVisits`，新增一次 visits 上报断言。

**验证**：
```
go build ./... && go vet ./... && gofmt -l .
go test -count=1 ./internal/kernel/... ./internal/agentruntime/... ./internal/e2e/...
go test -race -count=1 ./internal/kernel/... ./internal/agentruntime/...
```
**Rollback point**：P1 独立可回退（无 schema/接口依赖）。

## P2 Panel 迁移 + Repo + 采集处理

- [ ] `internal/db/migrations/0002_visit_records.sql`：三张表 + 索引（见 `design.md` §3.1）。
- [ ] `internal/db/db_test.go`：迁移数 1→2；表清单加 `visit_records`/`visit_daily_domains`/`visit_batches`。
- [ ] `internal/repo/visits.go`：`NewVisitRecord`/`VisitRecord`/`VisitFilter`/`TopHost` +
      `IngestVisitBatch`（单事务：去重 → 插原始 → upsert 聚合 → 写标记）、`VisitBatchCount`、
      `ListVisits`（JOIN 名称 + 过滤 + 分页）、`TopVisitHosts`。
- [ ] `internal/repo/cleanup.go`：`DeleteVisitRecordsBatch`/`DeleteVisitDailyDomainsBatch`/
      `DeleteVisitBatchesBatch`。
- [ ] `internal/web/web.go`：注册 `POST /api/agent/visits`。
- [ ] `internal/web/agent_telemetry.go`：`handleAgentVisits`（校验矩阵见 `design.md` §3.3；
      读 `collection.visits` 关闭时返回 `accepted:false`）。
- [ ] `internal/web/agent_test.go`：`TestAgentVisitIngestion`（幂等/越权/非法字段/413/关闭开关）。
- [ ] `internal/repo/repo_test.go`（或新增）：`IngestVisitBatch` 幂等与聚合 upsert。

**验证**：
```
go build ./... && go vet ./... && gofmt -l .
go test -count=1 ./internal/db/... ./internal/repo/... ./internal/web/...
```

## P3 管理端查询 + 设置 + Janitor

- [ ] `internal/web/dto.go`：`visitDTO`/`topHostDTO` + `toVisitDTO`。
- [ ] `internal/web/user_data.go`：`handleUserVisits`（pathID + GetUser 404）。
- [ ] `internal/web/servers.go`：`handleServerVisits`（pathID + GetServer 404）。
- [ ] 新 `internal/web/visits.go`：`handleVisitList`、`handleVisitTop`。
- [ ] `internal/web/web.go`：注册 4 个管理端路由；新增三个设置键常量。
- [ ] `internal/web/settings.go`：`settingsDTO` 三字段 + `settingsFromMap` 默认 + `handleSettingsPut`
      校验；`visitsCollectionEnabled(ctx)` 助手。
- [ ] `internal/web/user_data.go` 复用 `parseTimeParam`；分页复用 `parsePageQuery`/`writePage`。
- [ ] `internal/janitor/janitor.go`：按 `retention.visit_days`(7) / `retention.visit_aggregate_days`(90)
      清理三表。
- [ ] 测试：列表过滤/分页、top 聚合、settings 往返、janitor 清理。

**验证**：
```
go build ./... && go vet ./... && gofmt -l .
go test -count=1 ./...
```

## P4 Web UI

- [ ] `web/src/api/types.ts`：`Visit`/`Visits`/`TopHost` + `Settings` 三字段。
- [ ] `web/src/api/visits.ts`：`listVisits`/`getUserVisits`/`getServerVisits`/`getTopHosts`。
- [ ] `web/src/components/user/UserVisits.vue`：对齐 `UserDevices.vue`；挂到 `UserDetailPage.vue`。
- [ ] `web/src/pages/ServerDetailPage.vue`：新增「访问站点」区块（含分页）。
- [ ] `web/src/pages/VisitsPage.vue`：筛选（用户/服务器/节点/目标/起止时间）+ 表格 + 分页 + Top 站点。
- [ ] `web/src/router/index.ts` + `web/src/components/AppLayout.vue`：`/visits` 路由与导航项。
- [ ] `web/src/pages/SettingsPage.vue`：采集开关 + 两个保留天数。
- [ ] 遵循 `DataTable`（`rowKey` 必填）、`TablePaginator`、`EmptyState`/`ErrorBanner`/loading 三态；
      `api/` 独占类型，禁止 `any`/裸 `fetch`。

**验证**：
```
cd web && npm run typecheck && npm run build && npm run lint
```

## P5 全量校验 + 收尾

- [ ] `trellis-check` 全量复审（对齐 `check.jsonl`）。
- [ ] 修正 spec 残留：`error-handling.md:65` / `quality-guidelines.md:11` 关于已移除
      `connection-logs` / `decodeJSONStrict` 的描述（见 `design.md` §6）。
- [ ] 更新本文件 Progress Log；如产生可复用约定（如新 telemetry 事件类型 recipe），追加到
      `.trellis/spec/backend/database-guidelines.md` 或 `directory-structure.md`。

**全量验证**：
```
go build ./... && go vet ./... && gofmt -l .
go test -count=1 ./...
go test -race -count=1 ./...
go test -count=1 -tags integration,with_quic,with_utls ./...
go build -tags with_quic,with_utls ./...
cd web && npm run typecheck && npm run build && npm run lint
make build
```

## Rollback 点

- **P1 后**：Agent 仅本地多采一份数据，不影响 Panel；可单独回退 `internal/kernel`+`agentruntime`。
- **P2 后**：新增表与端点；如需回退，删端点即可，表可留（无引用）或随迁移回退（生产不回退迁移，
  仅停用端点/关闭 `collection.visits`）。
- **紧急开关**：设置 `collection.visits=false` 立即停止落库（无需回滚代码）。

## 风险与注意

- 迁移数断言 `db_test.go:36` 必须同步，否则 `go test ./...` 红。
- 三处 DTO 镜像 + `api-contract.md` 必须同改；漏一处会漂移。
- `stubKernel` 实现了 `Kernel` 接口，接口加方法需同步实现，否则 e2e 编译失败。
- 1MB 请求体上限：1000 条 × (253 host + 64 ip + 固定字段) 需保持在 1MB 内（估算 ~400KB，安全）。
