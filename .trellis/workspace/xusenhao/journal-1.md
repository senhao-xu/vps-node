# Journal - xusenhao (Part 1)

> AI development session journal
> Started: 2026-09-20

---


- 2026-09-21: 完成前端整体样式重设计（09-21-frontend-redesign）。双层 token + 暗色模式（light/dark/system），tokens.css 重构，新增 stores/theme.ts，AppLayout 导航图标+主题切换，登录页重设计，硬编码颜色清零。typecheck/lint/build 全绿，commit f1c59fc。
- 2026-09-21: 完成节点协议参数补全（09-21-node-protocol-params）。hy2 obfs/hop_ports、vless flow 三态/dest，singbox.VLESSFlow 统一解析，go test 17 包全绿，commit 6a3b8d3；另提交 UI 修复 bb687f1（Esc 关闭弹窗、form-row 溢出）。
- 2026-09-21: 完成 AnyTLS 协议支持（09-21-anytls-protocol）。复用 hy2 TLS 证书模式，0006 迁移放宽 protocol CHECK，订阅/Clash/前端全链路，commit b8ea6ef；迁移前已备份 panel.db 到 /tmp/panel.db.bak-20260921。
- 2026-09-21: 完成仪表盘用户流量明细（09-21-dashboard-user-traffic）。新 API `GET /api/dashboard/user-traffic?range=today|total`（repo 新增 SumTrafficByUser/SumTrafficByUserNode/ListUsersAll，trafficWhere 重构为 trafficWherePrefixed），前端仪表盘新增可展开的用户流量表（今日/累计切换）。6 个新后端测试，go test + 前端 typecheck/lint/build 全绿，commit a9652a1 已推送。另建占位任务 09-21-site-visit-stats（访问站点统计，待规划）。

## 2026-09-21 auto-app-key 收尾
- 任务 09-21-auto-app-key 完成并提交推送 (1fd4529):app_key 可选,首次启动自动生成存入 settings 表,优先级 env > yaml > database > auto;compose 去掉 PANEL_APP_KEY 强校验,新增 PANEL_IMAGE/AGENT_IMAGE。
- trellis-check PASS,冒烟验证 auto/database/env 三种来源。
- 顺带分析:agent 容器 docker stats 57MB 中 47MB 是 sing-box 二进制的文件页缓存(可回收),真实匿名内存 ~19MB,与 Xboard 相当。
- 后续任务候选:精简 agent 镜像(源码编译 sing-box,仅 with_quic/with_utls/with_clash_api,预期 35MB→~20MB)。

## 2026-09-22 xboard-parity 任务开题 + Phase 0 spike
- 排查流量统计恒为 0：根因是 agent 靠 Clash API 的 `inboundUser` 归因，而 sing-box
  Clash API 从不返回该字段（`experimental/clashapi/connections.go` 只给 network/type/
  sourceIP/destinationIP/sourcePort/destinationPort/host/dnsMode/processPath）。测试 fixture
  是手写的，所以从未暴露。顺带修复的 vless/hy2 用户 `name` 缺失（5cd60ff）不足够。
- 调研：现状 `research/current-model.md`；Xboard（panel+node）`research/xboard-model.md`。
  结论：Xboard-Node 把 sing-box 当 Go library 内嵌，用 `adapter.ConnectionTracker` 连接级
  按 `metadata.User` 计数——这是 per-user 统计的正解。
- 决策（D1–D8，见 prd.md）：不做套餐/订单；保留 user_nodes 逐条授权；保留 host(server)+
  node 两层；维持 4 协议；清库重建不迁移；沿用 REST+revision 下发；观测=流量+在线设备
  （移除 sessions/连接日志）；UI 只对齐功能字段。
- Phase 0 spike 通过（go）：内嵌 sing-box v1.13.2，`-tags with_quic`，真实流量触发 tracker
  拿到 `metadata.User=u1`。产物：`research/embed-spike.md` + `research/embed-spike/{main.go,go.mod}`。
- 本期未改任何生产代码；Phase 1–6 待新会话继续（implement.md 有完整清单与验证命令）。

## 2026-09-22 xboard-parity 实现完成（Phase 0–6）
- 用 trellis-implement / trellis-check 子代理逐阶段实现，全部完成：
  - Phase 0 spike（go）：内嵌 sing-box v1.13.2，`-tags with_quic,with_utls`，真实流量触发
    ConnectionTracker 拿到 metadata.User。
  - Phase 1–2：agent 进程内嵌入 sing-box（`internal/kernel/singbox`），删除外部 sing-box/
    `singbox-reload`/Clash API 采集/Applier/Checker；设备数限制 gate 实现。
  - Phase 3：schema 重建（D5，单 0001_init.sql）：users transfer_enable/u/d/speed_limit/
    device_limit/online_count/last_online_at；nodes protocol_settings/rate/tags；traffic_records
    u/d；新增 online_devices(.online)/device_batches；删 sessions/connection_logs。
  - Phase 4：节点 rate/tags API + protocol_settings 分节重构；渲染器去 clash_api；契约更新。
  - Phase 5：前端 Vue3 对齐（节点表单分节、用户字段、在线设备/连接数页、删 sessions·logs）。
  - Phase 6：rate 入库生效；online_count=DISTINCT ip；online 连接数；文档/spec 更新。
- check 子代理修复的真实缺陷（多轮）：devices 分片覆盖、limits 变更不 bump revision、
  janitor online_count 失准、流量/设备重放永久 422 死锁、rate 溢出、dashboard devices 语义、
  前端 VLESS reality server_port 编辑覆盖。
- 最终校验全绿：go build/vet/gofmt/test（含 `-tags integration,with_quic,with_utls`）+ 前端
  typecheck/build/lint。AC1–AC9 全部达成（AC4 仅单节点本地，已知限制）。
- 未提交（工作区改动待用户 review）；未验证 docker build（沙箱无 Docker）；实机端到端建议手测。


## Session 1: 访问站点统计：Agent 采集目标站点 + Panel 存储/展示

**Date**: 2026-09-22
**Task**: 访问站点统计：Agent 采集目标站点 + Panel 存储/展示
**Branch**: `main`

### Summary

实现 process-in sing-box ConnectionTracker 采集连接目标（metadata.Destination），Agent 增量上报 POST /api/agent/visits；Panel 迁移 0002（visit_records/visit_daily_domains/visit_batches）单事务幂等落库，管理端 /api/visits 等端点，设置 collection_visits/retention_visit_days(7)/retention_visit_aggregate_days(90) 与 janitor 清理；前端用户详情/服务器详情/独立 /visits 页。trellis-check 修复 flushVisits 冻结期丢事件的 data-loss 缺陷（pendingVisits 积压+回归测试），并更正 spec 残留（decodeJSONStrict/connection-logs）与 design.md §2.2。全量 go/web 校验通过。

### Git Commits

| Hash | Message |
|------|---------|
| `0fdb03d` | (see git log) |

### Status

[OK] **Completed**


## Session 2: 节点编辑回显公开协议配置

**Date**: 2026-09-23
**Task**: 节点编辑回显公开协议配置
**Branch**: `main`

### Summary

GET /api/nodes/{id} 详情 DTO 新增 settings 字段回显公开协议配置（含派生 Reality 公钥），秘密字段仍仅存 secret_enc；编辑表单打开时拉取详情回填四协议公开字段，秘密字段保持留空不变，详情拉取失败时不提交 settings 避免覆盖存储值；同步更新 api-contract 与 node-protocol-settings spec；新增 nodes_settings_echo_test.go 秘密字段扫描断言。全部验证通过（go test/vue-tsc/lint/make build）。

### Git Commits

| Hash | Message |
|------|---------|
| `2086dfb` | (see git log) |

### Status

[OK] **Completed**


## Session 3: 节点可选 IPv6 入口（订阅追加 v6 条目）

**Date**: 2026-09-24
**Task**: 节点可选 IPv6 入口（订阅追加 v6 条目）
**Branch**: `main`

### Summary

节点新增 ipv6_enabled/ipv6_address（迁移 0004，纯加列）。启用后订阅（Clash/base64）由 subscription.expandIPv6 追加 {name}-v6 条目，端口/凭据/参数与主条目一致；单 inbound 不变，流量按 (user,node) 合并、visit 日志按 client_ip 区分地址族，agent 无需升级。含 repo/web/订阅/前端表单改动、docs/api-contract.md 最小 diff 与 spec §9。全部验证命令通过。

### Git Commits

| Hash | Message |
|------|---------|
| `b60efbd` | (see git log) |

### Status

[OK] **Completed**


## Session 4: Stateless agent: panel-issued agent key + panel-owned batch seq

**Date**: 2026-09-24
**Task**: Stateless agent: panel-issued agent key + panel-owned batch seq
**Branch**: `main`

### Summary

Made the panel agent stateless: replaced the one-time register_token flow with a long-lived per-server agent key (key_hash auth + key_enc app_key-encrypted reveal) via GET/POST /api/servers/{id}/agent-key. Agents keep no local state; heartbeat returns server_id and traffic/device/visit resume seqs (COALESCE(MAX(seq),0)) and the agent adopts max(local,resume) so restarts never collide with the (agent_id,seq) idempotency key. Removed /api/agent/register and both server token routes; drop the state volume/AGENT_STATE_PATH in favor of AGENT_KEY. Migration 0005 renames agents.token_hash->key_hash, adds key_enc, drops servers register-token columns. Clean break: back up panel-data before upgrading.

### Git Commits

| Hash | Message |
|------|---------|
| `ef10dc3` | (see git log) |

### Status

[OK] **Completed**
