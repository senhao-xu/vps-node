# 访问站点统计

## Goal

采集用户经节点访问的目标站点（域名或 IP、端口、网络类型、来源 IP），在**用户详情页**、
**服务器详情页**与**独立访问记录页**展示，并提供全局采集开关与保留策略。方案对齐
VpsCT 的「连接日志」语义，但采集方式改为本项目已有的**进程内 sing-box
ConnectionTracker** 直接读取 `metadata.Destination`，不解析日志。

## Background

- 现状：`traffic_records` / `online_devices` 不含目标信息，Panel 无法得知用户访问了哪些站点。
- Agent 已内嵌 sing-box 并注册自研 `adapter.ConnectionTracker`
  （`internal/kernel/singbox/tracker.go`），`entry(metadata)` 已读取 `User`/`Inbound`/`Source`，
  但未读取 `metadata.Destination`。因此本功能只需扩展 tracker，无需改配置渲染。
- 当前渲染配置**未开启 sniff**（`internal/singbox/singbox.go` `Render` 无 route.rules），
  `metadata.Domain` 恒为空；`metadata.Destination` 携带客户端在协议头中请求的目标
  （SS/VLESS/Hysteria2/AnyTLS 均为地址头），可能为域名或 IP。
- 历史上存在过 `connection_logs` 表（无目标字段），已在 xboard-parity 重构中移除。
- 详细调研：`research/telemetry-data-path.md`（本仓库数据通路）与
  `/tmp/opencode/VpsCT`（参考实现，日志尾随 + 逐连接 + 每日聚合）。

## Scope Decisions

- **D1 采集方式**：仅用 `metadata.Destination`（Fqdn 优先，否则 IP），**不启用 sniff**，
  不改动路由语义，与 VpsCT 的「记录客户端请求目标」一致。
- **D2 数据模型**：**逐连接原始记录** + **每日域名聚合**两层，对齐 VpsCT；聚合用于
  「访问最多站点」等长周期统计。
- **D3 控制粒度**：**全局开关 + 保留设置**，不做用户级/节点级开关（本期）。
- **D4 记录内容**：`user_id/node_id/server_id/dest_host/dest_port/network/client_ip/created_at`；
  **不含上下行字节**（字节已由 traffic 通道统计，避免重复与耦合）。
- **D5 展示位置**：用户详情页、服务器详情页（节点维度）、独立访问记录页（含 top 站点）。
- **D6 存储**：与现有表同库（单 SQLite），不采用 VpsCT 的独立 `connlog.db`。
- **D7 双重开关**：Agent 本地 `collection.visits` + Panel 全局 `collection.visits`；
  Panel 关闭时仍返回成功应答以避免 Agent 无限重试。
- **D8 隐私**：记录客户端来源 IP 与目标站点，属敏感运营数据；默认开启但可全局关闭，
  原始记录短保留（默认 7 天）。

## Requirements

### R1 Agent 采集
- tracker 在连接被接纳（`admissionTracked`）时记录一条访问事件：目标 host/port、network
  （`metadata.Network` tcp/udp）、来源 IP（复用现有 `sourceAddr`）、user/node。
- `metadata.Destination` 为空（Fqdn 为空且地址无效）时**跳过**，不做推断。
- 采集为有界内存队列（上限约 10000 条，超出丢弃最旧并计数/告警），不阻塞数据面。
- 新增 `Kernel.DrainVisits()`（或等价），只在 telemetry 周期 drain；**不得**在 `syncPass`
  的快照路径上消耗事件。

### R2 Agent 上报
- 复用现有批处理/重试语义：`visit_batches` 顺序号 `batch_seq` 单调递增并持久化到 agent state；
  可重试错误冻结在途批次原样重放；不可重试错误丢弃并推进序号，靠后续快照恢复。
- 每批 ≤1000 条；单批 `{batch_seq, records[]}`，每条含 `recorded_at`（RFC3339）。
- Panel 返回 `{accepted, batch_seq, records}`。

### R3 Panel 采集接口与幂等
- `POST /api/agent/visits`（Bearer agent 鉴权）；`server_id` 一律由凭据推导。
- 校验：`batch_seq ≥ 1`；条数 ≤1000（否则 413）；`user_id/node_id ≥ 1`；
  `node` 属于该 server 且在 `user_nodes` 授权内（否则 422，整批不落库）；
  `recorded_at` RFC3339 且在 ±24h 窗口内；`dest_host` 1–253 字符、`dest_port` 0–65535、
  `network` ∈ {`tcp`,`udp`,`''`}、`client_ip` ≤64 字符。
- 幂等键 `(agent_id, batch_seq)`：重复批次返回既有计数，不重复写入（同一事务内写标记）。

### R4 管理端查询
- `GET /api/users/{id}/visits`：该用户访问记录，支持 `from`/`to`/`host`/`node_id` 筛选 + 分页。
- `GET /api/servers/{id}/visits`：该服务器访问记录（含节点），同样支持筛选 + 分页。
- `GET /api/visits`：全局访问记录，支持 `user_id`/`server_id`/`node_id`/`host`/`from`/`to` 筛选 + 分页。
- `GET /api/visits/top`：按 `days`（默认 7）聚合的访问最多站点，支持同维度筛选。
- 响应统一分页信封 `{items,total,page,page_size}`；时间 RFC3339；错误用既有 envelope。

### R5 Web UI
- 用户详情页新增「访问站点」卡片（表格：时间、节点、目标、端口/网络、来源 IP）。
- 服务器详情页新增「访问站点」区块（表格：时间、用户、节点、目标、来源 IP）。
- 新增独立「访问记录」页（路由 + 导航项）：筛选（用户/服务器/节点/目标/时间）、分页、
  Top 站点概览。
- 复用 `DataTable` / `TablePaginator` / `EmptyState` / `ErrorBanner`；类型集中在 `api/types.ts`。

### R6 开关与保留
- 新增设置：`collection.visits`（bool，默认开）、`retention.visit_days`（原始，默认 7）、
  `retention.visit_aggregate_days`（聚合，默认 90），可在设置页读写。
- Janitor 按保留窗口批量清理 `visit_records` / `visit_daily_domains` / `visit_batches`。

### R7 文档与契约
- 更新 `docs/api-contract.md`：agent `visits` 端点、管理端端点、设置键。
- 更新 `README.md` 数据采集模型与隐私说明。

## Acceptance Criteria

- [ ] Agent 在真实/集成 sing-box 下可采集连接目标并通过 `POST /api/agent/visits` 上报。
- [ ] 重复 `(agent_id,batch_seq)` 不重复落库；跨 agent 同序号各自独立。
- [ ] 越权 node / 未授权 user / 非法 host·port·network / 过期·格式错误时间 → 422 且整批不落库。
- [ ] 超 1000 条 → 413；未知/关闭 `collection.visits` 时不产生存储增长。
- [ ] 用户详情页、服务器详情页、独立访问记录页均能展示并按筛选分页。
- [ ] Janitor 能按 `retention.visit_days` / `retention.visit_aggregate_days` 清理。
- [ ] `go build ./... && go vet ./... && gofmt -l .`、`go test -count=1 ./...`、
  `go test -race -count=1 ./...`、`cd web && npm run typecheck && npm run build && npm run lint` 通过。

## Assumptions

- **A1**：当前 UI 无独立「节点详情页」（路由仅 `/servers/:id` 展示服务器及其节点）。
  「节点详情页」需求由**服务器详情页**承载：该页展示该服务器全部节点的访问记录，
  支持按节点查看；节点级筛选在独立访问记录页也可用。若后续新增节点详情页，复用同一端点。
- **A2**：`metadata.Destination` 在本项目 4 种协议下均可获得；纯 IP 客户端会记录到 IP，
  不启用 sniff，因此不做域名回填。

## Out of scope

- sing-box `sniff`（TLS SNI / HTTP Host）与域名回填。
- 单条连接的上下行字节、连接时长、结束时间。
- 用户级/节点级采集开关、按分享/套餐维度的开关。
- 客户端 IP 地理位置、CSV 导出。
- 独立 `connlog.db`（VpsCT 方案）——本期同库。
