# Design: 对齐 Xboard 实体字段 + 嵌入式 sing-box 节点

> 关联 PRD：`prd.md`（D1–D8）。调研：`research/current-model.md`、`research/xboard-model.md`。

## 1. 架构与边界

现状：Panel(Go/SQLite) → 每 server 一个 agent → 外部 `sing-box` 进程；agent 轮询
Clash API 采集（已证伪）。

目标：

```
Panel (Go/SQLite, 渲染 sing-box 配置 JSON, REST+revision)
   ▲  /api/agent/config (revision)          │ POST /api/agent/traffic, /api/agent/devices
   │                                        ▼
Agent (Go, 进程内嵌入 sing-box)
   ├── 加载面板下发的 sing-box 配置 JSON → box.New + Start
   ├── 注册 ConnectionTracker（router.AppendTracker）
   │     └── 连接包装器：按 (userID, inboundTag) 累加 u/d；维护 per-user alive IP 与连接数
   ├── 设备数限制：连接建立时 gate-keep
   ├── 周期采集累计值 → 计算增量 → 上报（失败保留增量重放）
   └── heartbeat / sync（沿用现有 loop/state）
```

边界：
- 面板仍是配置渲染与用户/节点数据的唯一来源；agent 不再 shell 外部进程。
- 采集从「连接级快照 + metadata 归因」改为「连接包装器直接计数」。

## 2. 依赖与构建变更

- `go.mod` 新增 `github.com/sagernet/sing-box`（**定为 `v1.13.2`**，与 Xboard-Node 一致；
  v1.14.x 需 Go ≥1.25.5 且 `ConnectionTracker` 多一个 `RoutedFlow` 方法，本期不用）。
- 构建 tag：**`with_quic,with_utls` 必需**（hysteria2 为 QUIC inbound；VLESS Reality 需要
  uTLS——Phase 1 实测：仅 `with_quic` 时 Reality inbound 初始化报
  `uTLS ... is not included in this build`）。`CGO_ENABLED=0` 可静态构建。
- 体积：嵌入后 agent 二进制约 **38MB**（spike 实测）。
- Spike 结论见 `research/embed-spike.md`；Go 1.25 可直接编译，Dockerfile 无需 bump。
- `deploy/Dockerfile.agent`：
  - 删除下载 sing-box tarball 的 stage（`deploy/Dockerfile.agent:27-46`）。
  - 不再需要 `singbox-reload` 与 `AGENT_SINGBOX_*` 路径/命令 env；追加 `CGO_ENABLED=0` 已有。
  - 保留 `AGENT_STATE_PATH` 等。
- 删除 `cmd/singbox-reload`；`/etc/sing-box` 配置卷不再必需（可保留用于落盘调试）。

## 3. 数据模型变更（SQLite，新建 schema，不迁移）

面板 `internal/db/migrations/`：

- `nodes`：新增 `rate REAL NOT NULL DEFAULT 1`、`tags TEXT NOT NULL DEFAULT '[]'`；
  `settings` 重命名/重塑为 Xboard 风格 `protocol_settings`（见 §5）；保留 `secret_enc`(AES-GCM)
  存私密材料（私钥/证书/密码）；保留 `status`（enabled 等价）。可选：`transfer_enable`、`u`、`d`。
- `users`：
  - 对齐字段：`transfer_enable INTEGER`（0=无限）、`u INTEGER`、`d INTEGER`、
    `speed_limit INTEGER`、`device_limit INTEGER NOT NULL DEFAULT 0`（0=不限）、
    `online_count INTEGER NOT NULL DEFAULT 0`、`last_online_at INTEGER`。
  - `quota_bytes`→`transfer_enable`、`used_bytes`→(`u`+`d`) 口径统一（重建，无迁移）。
  - 保留 `uuid`、`username`、`status`、`expires_at`、`token_hash`。
- 新增 `online_devices`：`user_id`、`node_id`、`ip`、`last_seen_at`，
  唯一 `(user_id, node_id, ip)`；用于「在线设备」查询（替代 Redis）。
- `traffic_records`：列名对齐 `(user_id, node_id, server_id, u, d, created_at)`。
- **删除**：`sessions`、`connection_logs`、`connection_log_batches`，及其 repo/web 代码。
- `agents`/`server_revisions`/`traffic_batches`/`settings`/`admins`/`user_subscriptions` 保留。
- `user_nodes` 保留（D2）。

## 4. Panel ↔ Agent 契约

保留 `GET /api/agent/config?version=<rev>`（revision 语义不变），载荷调整：

```jsonc
{
  "status": "updated", "revision": 12, "renderer_version": "singbox-embed-v1",
  "config": { "singbox": { /* 完整 sing-box 配置，含 inbounds/users */ } },
  "nodes": [ { "id": 1, "protocol": "vless", "tag": "vless-1" } ],
  "users": [ { "id": 1, "uuid": "...", "device_limit": 3, "speed_limit": 0 } ]
}
```

- 上报改为：
  - `POST /api/agent/traffic`：`{ batch_seq, recorded_at, records: [{user_id,node_id,u,d}] }`
    （增量；面板按 `(agent_id,batch_seq)` 幂等，累加 `users.u/d` 并写 `traffic_records`）。
  - `POST /api/agent/devices`：`{ batch_seq, recorded_at, devices: [{user_id,node_id,ips:[...],online}] }`
    （面板 replace 该 server 的 `online_devices` 并更新 `users.online_count/last_online_at`）。
  - **移除** `/api/agent/sessions`、`/api/agent/connection-logs`。
- 渲染配置中 `experimental.clash_api` **不再注入**（不再需要轮询）；由面板 `singbox.Render` 去除。

## 5. 节点设置结构（向 Xboard `protocol_settings` 对齐，协议仍 4 种）

- `nodes.protocol_settings`（对外 JSON，公开字段）按 Xboard 分节：
  - shadowsocks：`{ cipher, obfs:{...}, plugin, plugin_opts }`
  - vless：`{ tls, tls_settings{server_name,...}, reality_settings{server_name,public_key,short_id,...}, flow, network, network_settings, multiplex, utls }`
  - hysteria2：`{ version:2, bandwidth{up,down}, obfs{open,type,password}, tls{server_name,...}, hop_interval }`
  - anytls：`{ padding_scheme, tls{...} }`
- 私密材料继续放 `secret_enc`（`reality_settings.private_key`、`tls.key`/`certificate`、
  ss `server password`），公开 JSON 只放可公开信息（public_key/short_id/server_name 等）。
  这是与 Xboard「全明文」的有意偏离（保持加密，理由：安全，且改动可控）。
- 面板 `buildNodeSettings` 的分节校验重写；前端 `NodeFormDialog` 按分节渲染。
- `Renderer`（面板）仍产出 sing-box 配置；字段读取自 `protocol_settings`+`secret_enc`。

## 6. Agent 嵌入式运行时

新增 `internal/kernel/singbox/`（agent 侧）：

- `buildBox(cfgJSON) (*box.Box, error)`：`ctx := include.Context(bg)` →
  `singJSON.UnmarshalExtendedContext[option.Options](ctx, cfgJSON)` →
  `box.New(box.Options{Context: ctx, Options: opts})` →
  `router := service.FromContext[adapter.Router](ctx); router.AppendTracker(tracker)` →
  `Start()`。以上 API 已在 spike 中验证（见 `research/embed-spike.md`）。
- `ConnTracker`：实现 `adapter.ConnectionTracker`（v1.13.2 仅两个方法）：
  - `RoutedConnection`/`RoutedPacketConnection`：`metadata.User` 取用户 UUID（需 UUID→userID
    映射，由配置 users 建立）；按 `(userID, inboundTag)` 键累加 `atomic.Int64 u/d`；
    维护 `ips map[ip]refcount` 与连接数。
  - 字节语义：入站读=上传、入站写=下载（沿用 Xboard-Node 约定）。
  - 设备数限制：连接建立时若该用户 alive IP 数 ≥ `device_limit`（>0）且新 IP，则拒绝。
  - `Snapshot()` 返回 `map[(user,node)][2]int64` 累计 + `map[user]map[ip]bool` + 连接数。
- 移除 `clash.go`、`collector.go`、`checker.go`、`applier.go`（或改造为「加载嵌入式实例」），
  `loop.go` 的 telemetryPass 改为读 tracker 快照算增量并上报。
- 配置/用户变更：revision 变化时重建 box（`Stop` + `buildBox` + `Start`），实现简单且正确；
  可后续优化为热更新。

## 7. 增量与上报可靠性

- agent 侧维护 `lastSeen[(user,node)]`；`delta = cum - lastSeen`，负值按 0 处理（重启容错）。
- 未成功上报的增量累积在 pending，下轮合并重放（对齐 Xboard `RestoreTraffic` 思路）。
- 幂等：沿用 `(agent_id,batch_seq)`，`batch_seq` 持久化在 agentstate。

## 8. 前端（Vue3，功能/字段对齐）

- `NodeFormDialog.vue`：按 `protocol_settings` 分节（tls/network/reality/multiplex），
  协议仍 4 种；沿用现有字段组件风格。
- 用户：表单/详情新增 `transfer_enable`、`speed_limit`、`device_limit`、在线设备与在线数展示。
- 服务器：字段保持；节点列表按新字段（rate/tags/enabled）。
- 移除 `UserSessions.vue`、`UserConnectionLogs.vue` 与对应路由/API/类型。
- 新增「在线设备」区块（用户详情或服务器详情）。
- 订阅页/接口基本不变，字段随实体对齐。

## 9. 风险 / 待验证 / 回滚

- **嵌入 API**（已由 Phase 0 spike 消解）：pin `v1.13.2`，tag `with_quic`，注入方式已确认。
- **配置版本兼容**：面板当前渲染偏 1.14 风格；本项目仅用 inbound + direct，需在 Phase 2
  用 v1.13.2 的 `sing-box check`/`UnmarshalExtendedContext` 校验渲染产物兼容。
- **二进制体积/内存**：嵌入后 agent 体积约 +38MB；镜像与部署文档需更新。
- **设备数限制跨节点**：Xboard 用 Redis 全局状态；本期单 SQLite、无跨节点全局 devices，
  设备限制仅按单节点本地统计（记录为已知限制）。
- **回滚点**：内嵌运行时以新包隔离；若 spike 失败，可回退到「外部 sing-box + V2Ray API
  gRPC stats」备选（PRD 未采纳，但保留技术后路）。
- 无数据迁移，重建库；部署时清空 `panel.db` 与 agent state。

## 10. 不在本设计内

套餐/订单/支付、server_group、machine、11 协议、WebSocket、MySQL/Redis、视觉重写。
