# Design: 访问站点统计

> 前置：`prd.md`（D1–D8、R1–R7、A1–A2）。复杂任务，`task.py start` 前需本文件 + `implement.md`。
> 调研输入：`research/telemetry-data-path.md`；参考实现 `/tmp/opencode/VpsCT`。

## 1. 总览与数据流

```
in-process sing-box router
  └─ ConnTracker.RoutedConnection/RoutedPacketConnection
       └─ entry(metadata): 现有 user/node/ip 归因 + 新增 metadata.Destination/Network
            └─ visits ring（有界，按连接一条）
Runtime.DrainVisits()                                  kernel/singbox/runtime.go
  └─ Loop.telemetryPass: drain →（Collection.Visits 时）buildVisitBatches → client.Visits
       └─ state.VisitBatchSeq 持久化                    agentstate

POST /api/agent/visits（Bearer，requireAgent 推导 server_id）
  └─ 校验（scope/长度/时间窗口）→ repo.IngestVisitBatch（单事务幂等）
       ├─ visit_records（逐连接原始）
       └─ visit_daily_domains（(day,user,node,host) 命中数 upsert）

Admin: GET /api/users/{id}/visits · /api/servers/{id}/visits · /api/visits · /api/visits/top
Janitor: retention.visit_days / retention.visit_aggregate_days 清理。
```

与现有 `traffic`/`devices` 通路**同构**：同一 ticker、同一幂等模式、同一 DTO 三处镜像约定。

## 2. Agent 侧设计

### 2.1 内核采集（`internal/kernel/singbox`）

`types.go` 新增：

```go
type Visit struct {
    UserID   int64
    NodeID   int64
    DestHost string
    DestPort int
    Network  string // tcp|udp
    ClientIP string
    At       time.Time
}
```

`tracker.go`：
- `ConnTracker` 新增字段：`visitMu sync.Mutex`、`visits []Visit`、`visitsDropped int64`；常量
  `maxVisits = 10000`。
- 在 `entry(metadata)` 内、`admissionTracked` 分支创建 `connEntry` 之后调用
  `t.recordVisit(metadata, userID, nodeID, ip)`：
  - `host := strings.TrimSpace(metadata.Destination.Fqdn)`；若为空且
    `metadata.Destination.Addr.IsValid()` → `metadata.Destination.Addr.Unmap().String()`；
    仍为空则**返回，不记录**。
  - host 小写、截断 253；`port := int(metadata.Destination.Port)`；
    `network := metadata.Network`（空则 `""`）。
  - 追加到 `visits`；满 `maxVisits` 时丢弃最旧（`t.visits = t.visits[1:]` 后 append），
    并 `visitsDropped++`。
- 新增 `drainVisits() []Visit`：加锁取出全部并清空，返回副本。
- 新增 `droppedVisits() int64`（供日志/诊断）。
- `release()` **不改**：不记录结束时间/字节（D4）。

`runtime.go`：新增 `DrainVisits() []Visit` 委托 tracker；无 tracker 时返回 nil。
`Snapshot()` 保持只返回 Traffic/Devices，**不**消耗 visits。

### 2.2 上报循环（`internal/agentruntime/loop.go`）

- `Kernel` 接口新增 `DrainVisits() []kernelsingbox.Visit`。`stubKernel`（`internal/e2e`）需实现。
- `Loop` 新增字段：`visitInflight []agentclient.VisitBatch`、`visitInflightIdx int`、
  `pendingVisits []kernelsingbox.Visit`（有界积压，`maxPendingVisits=10000`，溢出丢最旧并告警）。
- `Loop.telemetryPass`：
  ```go
  visits := l.kernel.DrainVisits()
  if l.cfg.Collection.Visits { l.flushVisits(ctx, visits) }
  // 关闭时直接丢弃，保持队列有界
  ```
  （`collectDeltas`/`flushTraffic`/`flushDevices` 保持不动。）
- `flushVisits`：**不**照搬 `flushDevices` 的"整批快照"语义。visits 是**增量事件**，没有下一次全量
  快照可恢复，因此：
  - 先把本次 drain 的事件并入有界积压 `pendingVisits`（溢出丢最旧 + 告警），避免在途批次冻结期间
    新事件被静默丢弃（这是 check 阶段发现的 data-loss 缺陷，见 `implement.md` Progress Log）；
  - `visitInflight` 为空且积压非空时 `buildVisitBatches(pendingVisits, state.VisitBatchSeq)` 并清空积压；
  - 可重试错误冻结在途批次原样重放；不可重试错误丢弃 inflight、推进 `VisitBatchSeq`，积压留待下次；
  - 成功推进 `VisitBatchSeq` 并 `saveState()`。该语义与 `traffic` 的 `pending` 通路一致，而非 `devices`。
- `buildVisitBatches`：每条转换 `agentclient.VisitRecord`（`RecordedAt = At.UTC().Format(RFC3339)`）；
  按 `(UserID,NodeID,DestHost,DestPort,RecordedAt)` 排序；按 1000 分块，`BatchSeq = base+idx+1`。
  visits 为空时**不**产生批次（与 devices 的"空快照也发"不同——visits 是增量事件，无需心跳式空批）。
- `EnsureIdentity` 重置分支加 `l.state.VisitBatchSeq = 0`。

### 2.3 Agent 状态与配置

- `agentstate.State` 新增 `VisitBatchSeq int64 \`json:"visit_batch_seq,omitempty"\``。
- `config.Collection` 新增 `Visits bool`；`LoadAgent` 默认 `Collection{Traffic:true, Visits:true}`；
  `agentFile.Collection.Visits *bool`（yaml `collection.visits`）；env `AGENT_COLLECTION_VISITS`
  （`strconv.ParseBool`，解析失败视为错误或忽略——沿用现有 env 处理风格）。上报周期复用
  `TrafficInterval`（不新增 interval）。

### 2.4 Agent 客户端（`internal/agentclient/client.go`）

```go
type VisitRecord struct {
    UserID     int64  `json:"user_id"`
    NodeID     int64  `json:"node_id"`
    DestHost   string `json:"dest_host"`
    DestPort   int    `json:"dest_port"`
    Network    string `json:"network"`
    ClientIP   string `json:"client_ip,omitempty"`
    RecordedAt string `json:"recorded_at"`
}
type VisitBatch struct { BatchSeq int64 `json:"batch_seq"`; Records []VisitRecord `json:"records"` }
type VisitAck   struct { Accepted bool `json:"accepted"`; BatchSeq int64 `json:"batch_seq"`; Records int64 `json:"records"` }

func (c *Client) Visits(ctx context.Context, batch VisitBatch) (*VisitAck, error) // POST /api/agent/visits
```
复用 `do`/`attempt`/`Retryable`，无 gzip（与 traffic/devices 一致）。

## 3. Panel 侧设计

### 3.1 迁移（`internal/db/migrations/0002_visit_records.sql`）

```sql
CREATE TABLE visit_records (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    node_id INTEGER NOT NULL,
    server_id INTEGER NOT NULL,
    dest_host TEXT NOT NULL,
    dest_port INTEGER NOT NULL DEFAULT 0,
    network TEXT NOT NULL DEFAULT '',
    client_ip TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);
CREATE INDEX idx_visit_records_user   ON visit_records (user_id, created_at);
CREATE INDEX idx_visit_records_node   ON visit_records (node_id, created_at);
CREATE INDEX idx_visit_records_server ON visit_records (server_id, created_at);
CREATE INDEX idx_visit_records_host   ON visit_records (dest_host, created_at);

CREATE TABLE visit_daily_domains (
    day INTEGER NOT NULL,              -- UTC 当天 00:00 的 unix 秒
    user_id INTEGER NOT NULL,
    node_id INTEGER NOT NULL,
    server_id INTEGER NOT NULL,
    dest_host TEXT NOT NULL,
    hits INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (day, user_id, node_id, dest_host)
);
CREATE INDEX idx_visit_daily_user ON visit_daily_domains (user_id, day);
CREATE INDEX idx_visit_daily_node ON visit_daily_domains (node_id, day);
CREATE INDEX idx_visit_daily_host ON visit_daily_domains (dest_host, day);

CREATE TABLE visit_batches (
    agent_id INTEGER NOT NULL REFERENCES agents (id) ON DELETE CASCADE,
    seq INTEGER NOT NULL,
    received_at INTEGER NOT NULL,
    records INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (agent_id, seq)
);
```
- `visit_records` / `visit_daily_domains` **无 FK**（高写入历史表，父删除后保留至保留窗口，
  与 `traffic_records` 一致）；`visit_batches` 有 FK+CASCADE（与 `traffic_batches` 一致）。
- 需同步更新 `internal/db/db_test.go`：`COUNT(schema_migrations)` 1→2，表清单加 3 张新表。
- day 边界统一 `time.Unix(t,0).UTC().Truncate(24*time.Hour).Unix()`，与 dashboard 的 UTC day 口径一致。

### 3.2 Repo（`internal/repo/visits.go` 新文件 + `cleanup.go`）

```go
type NewVisitRecord struct {
    UserID, NodeID, ServerID int64
    DestHost string
    DestPort int
    Network  string
    ClientIP string
    CreatedAt time.Time
}
type VisitRecord struct { /* 全字段 + ID */ }        // 含 JOIN 出的 username/node_name/server_name
type VisitFilter struct {
    UserID, ServerID, NodeID int64   // 0 = 不过滤
    Host, ClientIP           string  // 子串，小写
    From, To                 *time.Time
    Page, PageSize           int
}
type TopHost struct { DestHost string; Hits int64 }

func (r *Repo) IngestVisitBatch(ctx, agentID, seq, serverID int64, records []NewVisitRecord) (int64, bool, error)
func (r *Repo) VisitBatchCount(ctx, agentID, seq int64) (int64, bool, error)
func (r *Repo) ListVisits(ctx, f VisitFilter) ([]VisitRecord, int, error)   // items + total
func (r *Repo) TopVisitHosts(ctx, f VisitFilter, day int64, limit int) ([]TopHost, error)
```
`IngestVisitBatch` 单事务，镜像 `IngestTrafficBatch`：
1. `SELECT records FROM visit_batches WHERE agent_id=? AND seq=?` → 命中即 `duplicate=true` 返回；
2. 逐条 `INSERT INTO visit_records(...)`；
3. 逐条聚合 upsert：
   `INSERT INTO visit_daily_domains(day,user_id,node_id,server_id,dest_host,hits) VALUES(?,?,?,?,?,1)
    ON CONFLICT (day,user_id,node_id,dest_host) DO UPDATE SET hits = hits + 1`；
4. `INSERT INTO visit_batches(agent_id,seq,received_at,records)`；
5. `count = len(records)`。

`cleanup.go` 新增：
`DeleteVisitRecordsBatch(cutoff,limit)`、`DeleteVisitDailyDomainsBatch(cutoff,limit)`、
`DeleteVisitBatchesBatch(cutoff,limit)`（均 `rowid IN (SELECT ... LIMIT ?)`）。

### 3.3 采集处理（`internal/web/agent_telemetry.go` + `web.go`）

- `web.go` `registerAgentRoutes` 增加
  `mux.HandleFunc("POST /api/agent/visits", h.requireAgent(h.handleAgentVisits))`。
- 新增常量 `maxVisitHostLen = 253`（复用 `maxAgentBatchRecords`、`agentTimestampWindow`、`maxIPStrLen`）。
- `handleAgentVisits`：解码 `{batch_seq, records}` → `batch_seq≥1` → 条数 ≤1000 → 读设置
  `collection.visits`；关闭则直接 `{accepted:false, batch_seq, records:0}`（不校验、不落库）→
  `agentScopes` → 逐条校验（user/node≥1；node∈nodeSet；pair∈pairSet；`dest_host`
  trim+lower 后 1–253；`dest_port` 0–65535；`network` ∈ {tcp,udp,``}；`client_ip` ≤64；
  `recorded_at` ±24h）→ `IngestVisitBatch` → `ErrConflict` 时 `VisitBatchCount` →
  `{accepted:true, batch_seq, records}`。
- 复用现有 `decodeJSON`（保持 `api-contract.md:11`「未知字段忽略」口径；**不**引入
  `decodeJSONStrict`，并保留旧名 `destination_host` 不再出现的历史约束——本特性字段名为
  `dest_host`。见 §6 契约矛盾处理）。

### 3.4 管理端查询与 DTO

- `web.go` `registerAdminRoutes` 增加：
  ```
  GET /api/users/{id}/visits     → handleUserVisits      (pathID + GetUser 404 guard)
  GET /api/servers/{id}/visits   → handleServerVisits    (pathID + GetServer 404 guard)
  GET /api/visits                → handleVisitList
  GET /api/visits/top            → handleVisitTop
  ```
- 查询参数：`from`/`to`（RFC3339 或 `2006-01-02`，复用 `parseTimeParam`）、`host`、`user_id`、
  `server_id`、`node_id`、`page`/`page_size`（复用 `parsePageQuery`/`writePage`）；`top` 另有
  `days`（默认 7，clamp 1–365）、`limit`（默认 20，clamp ≤200）。
- `dto.go` 新增 `visitDTO`（含 `username`/`node_name`/`server_name`）与 `topHostDTO`；
  `toVisitDTO`。前端类型在 `web/src/api/types.ts` 镜像。

### 3.5 设置与 Janitor

- `web.go` 新增键常量：
  `settingCollectionVisits = "collection.visits"`、
  `settingRetentionVisit = "retention.visit_days"`、
  `settingRetentionVisitAggregate = "retention.visit_aggregate_days"`。
- `settingsDTO` 加 `CollectionVisits bool`、`RetentionVisitDays int`、`RetentionVisitAggregateDays int`；
  `settingsFromMap` 默认 `true / 7 / 90`；`handleSettingsPut` 接受三者的可选更新并校验范围
  （days ≥1，bool）。新增 `visitsCollectionEnabled(ctx) bool` 供采集处理读取。
- `janitor.Sweep`：读取 `retention.visit_days`（默认 7）清 `visit_records`；读取
  `retention.visit_aggregate_days`（默认 90）清 `visit_daily_domains` 与 `visit_batches`；
  复用 `drain` + 500 行批。janitor 新增 `daysSetting` 调用（已存在）。

## 4. Web UI 设计

- **契约类型**（`web/src/api/types.ts`）：
  ```ts
  export type Visit = { id:number; user_id:number; username:string; node_id:number; node_name:string;
    server_id:number; server_name:string; dest_host:string; dest_port:number; network:string;
    client_ip:string; created_at:string }
  export type Visits = Paged<Visit>
  export type TopHost = { dest_host:string; hits:number }
  ```
- **API 模块** `web/src/api/visits.ts`：`listVisits`、`getUserVisits`、`getServerVisits`、`getTopHosts`
  （薄封装 `request<T>`，遵循 `directory-structure.md`：`api/` 独占类型与解码）。
- **用户详情页** `components/user/UserVisits.vue`：结构对齐 `UserDevices.vue`（card + card-head +
  ErrorBanner + DataTable `bordered=false`），列：时间、节点、目标(`host:port`)、网络、来源 IP；
  `watch(userId)` 重载。在 `UserDetailPage.vue` 挂载。
- **服务器详情页** `ServerDetailPage.vue`：新增 `.card` 区块「访问站点」，列：时间、用户、节点、
  目标、来源 IP；加载该 server 的 visits（最新一页，含分页控件）。
- **独立页** `pages/VisitsPage.vue` + 路由 `/visits` + `AppLayout.vue` `navItems` 新增
  `{ to:'/visits', label:'访问记录', icon: 现有 lucide 图标 }`：`PageHeader` + 筛选条
  （用户/服务器/节点/目标/起止时间）+ `DataTable` + `TablePaginator` + Top 站点卡片/表格。
- **设置页** `SettingsPage.vue`：采集开关 `ToggleSwitch` + 两个保留天数输入，随现有 section 保存。

## 5. 保留、隐私与性能

- 原始 `visit_records` 默认保留 **7 天**，聚合 `visit_daily_domains` 默认 **90 天**（对齐 VpsCT）。
- Agent 队列有界 10000 条，溢出丢最旧并计数；正常情况下 telemetry 周期（60s）远小于队列容量。
- Panel 每批 ≤1000 条且请求体受 `decodeJSON` 的 1MB 限制；单条 host ≤253、ip ≤64，保证 1000 条
  批次不触顶。
- 记录来源 IP 与目标站点为敏感数据：全局开关可关闭（关闭后不落库），文档在 `README` 隐私说明。
- 不记录报文内容、不做 GeoIP。

## 6. 兼容性、契约与权衡

- **向后兼容**：纯新增端点/表/设置；旧 agent 不上报 visits，无破坏。渲染配置不变（不启用
  sniff），`renderer_version` 不动。既有 traffic/devices 通路不变。
- **契约同步（三处镜像 + 文档）**：`agentclient` 类型、`web/agent_telemetry.go` handler 局部类型、
  `web/src/api/types.ts`、`docs/api-contract.md` 必须一起改（`directory-structure.md`）。
- **JSON 严格解码矛盾**：`spec/backend/error-handling.md:65` 与 `quality-guidelines.md:11` 提到
  `decodeJSONStrict` / `connection-logs`（已移除特性残留），与 `api-contract.md:11`「未知字段忽略」
  冲突。本设计遵循 binding 的 `api-contract.md`，沿用 `decodeJSON`；Phase 3 校验时同步
  修正这两处 spec 残留（不引入 `decodeJSONStrict`）。
- **权衡**：
  - 不启用 sniff → 域名完整度取决于客户端请求地址（纯 IP 客户端记 IP）；换取零路由语义变更。
  - 同库两张表（原始+聚合）→ 查询简单、事务简单；代价是原始表写入量随连接数增长，用短保留兜底。
  - 无字节/时长 → 避免与 traffic 通道重复，降低耦合；代价是无法按站点看流量。
  - 聚合含 `user_id` → 支持每用户长周期 Top 站点；代价是聚合行数高于 VpsCT（VpsCT 按 share）。

## 7. 边界与风险

- `metadata.Destination` 在个别协议/客户端下可能为空 → 跳过，不推断（不影响其它功能）。
- 连接风暴下 tracker 队列溢出 → 丢最旧、计数，日志可见；不阻塞数据面。
- 迁移数断言 `db_test.go` 必须同步，否则 CI 红。
- `agentScopes` 每次上报都会查 `nodes` + `user_nodes`；visits 上报频率与 devices 相同，已有同量级开销。
- 高频目标（如 CDN 域名）导致原始表行数大：短保留 + `(dest_host,created_at)` 索引支撑 Top 查询。

## 8. 测试策略

- 内核：tracker 记录/跳过空目标/小写截断/队列上限丢最旧（`tracker_test.go`）。
- Agent：`buildVisitBatches` 分块与排序、`flushVisits` 重试语义；e2e `stubKernel` 覆盖 `DrainVisits`。
- Panel：`TestAgentVisitIngestion` 镜像 `TestAgentTrafficIngestion`——重复 seq 不重复落库、跨 agent 独立、
  越权 node/未授权 user/host·port·network 非法/时间越界/坏 seq → 422、>1000 → 413、
  `collection.visits=false` 不落库且返回 `accepted:false`。
- Repo：`IngestVisitBatch` 幂等与聚合 upsert；`ListVisits` 过滤/分页；`TopVisitHosts` 聚合。
- 迁移：`db_test.go` 迁移数与表清单；`migration` 行为测试可选。
- Janitor：按新保留设置清理三张表。
- 前端：`npm run typecheck && npm run build && npm run lint`。
