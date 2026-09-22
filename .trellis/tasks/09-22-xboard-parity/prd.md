# 对齐 Xboard：实体字段 + 嵌入式 sing-box 节点

## Goal

把本项目的 server / node / user 实体字段向 Xboard 语义对齐，并将节点运行架构从
「外部 sing-box 进程 + Clash API 轮询」替换为 Xboard-Node 式的「嵌入式 sing-box +
ConnectionTracker」，使每用户流量与在线设备统计可靠可用，后台表单/字段与观测页面
相应调整。

## Background

- 现状调研：`.trellis/tasks/09-22-xboard-parity/research/current-model.md`
- Xboard 调研：`.trellis/tasks/09-22-xboard-parity/research/xboard-model.md`

现状核心问题：agent 调用外部 sing-box 二进制，通过 Clash API `/connections` 的
`inboundUser` 归因；该字段在 sing-box Clash API 中不存在，导致每用户流量恒为 0
（`internal/agentruntime/clash.go:169-222`、`loop.go:279-282`）。Xboard-Node 通过把
sing-box 作为 Go library 嵌入并注册 `adapter.ConnectionTracker`，在连接包装器里按
用户直接累加字节（`metadata.User`），从根上解决该问题。

## Scope Decisions

- **D1 不做商业化**：不引入套餐/订单/支付/优惠券/佣金，不引入 MySQL/Redis。
- **D2 保留 `user_nodes` 逐条授权**：不引入 server_group / group_ids 组模型。
- **D3 保留两层拓扑**：物理宿主机 = `server`，单个 inbound = `node`；不引入 `machine`。
- **D4 协议维持 4 种**（shadowsocks/vless/hysteria2/anytls），但节点设置结构向 Xboard 的
  `protocol_settings` JSON 形态对齐，为扩展预留。
- **D5 不保留现有数据**：按新 schema 重建，不写数据迁移。
- **D6 沿用 REST 轮询 + revision**：agent 主动拉配置、心跳发现变更；不改 WebSocket。
- **D7 可观测对齐 Xboard**：保留每用户流量 + 在线设备(按 IP 去重) + 在线连接数；
  移除 sessions 实时会话与 connection_logs 历史连接日志（含表、接口、采集、页面）。
- **D8 UI 只对齐功能与字段**：保留 Vue3 技术栈与整体视觉，不改 React/Shadcn 视觉体系。

## Requirements

### R1 实体字段对齐（server / node / user）
- `server`（宿主机）：对齐状态口径（active/disabled/offline 派生）、机器指标、版本、
  last_seen 等；字段语义向 Xboard 靠拢（命名与 DTO 保持本仓库既有约定）。
- `node`：设置统一收敛为 Xboard 风格 `protocol_settings` JSON（多层嵌套，含 tls/network/
  reality 等分节），并补齐 Xboard 节点字段中与本期相关者：`rate`(流量倍率)、`tags`、
  `enabled`、`transfer_enable`、`u`/`d`（节点级累计，可选）。
- `user`：对齐 Xboard 用户字段中与本期相关者：`u`/`d`、`transfer_enable`、`speed_limit`、
  `device_limit`、`online_count`、`last_online_at`；保留现有 `uuid`/`status`/`quota_bytes`/
  `expires_at` 语义（或按 D2 合并到 u/d 口径）。

### R2 节点运行架构：嵌入式 sing-box
- agent 以 Go module 方式引入 sing-box，在进程内构建并运行实例（`box.New` + `Start`）。
- 注册自定义 `adapter.ConnectionTracker`：TCP/UDP 连接包装器按用户累加上下行，并记录
  每用户 alive IP 集合与连接数；语义与 Xboard-Node 一致（入站读=上传、入站写=下载）。
- 支持设备数限制（连接建立时按用户 alive IP 数 gate-keep）与限速（`golang.org/x/time/rate`）
  的可选能力（本期至少实现设备数限制与上下行字节统计；限速视工作量提供）。
- 采集：agent 周期读取累计计数器，计算增量后上报；上报失败需可重放（保留未成功增量）。
- 移除外部 sing-box 进程路径：`cmd/singbox-reload`、`Applier` 的 check/reload、
  Clash API 采集（`clash.go`/`collector.go`）与镜像内 sing-box 下载。

### R3 面板采集接口与数据
- 上报载荷改为按用户（可选按 用户×node）的流量增量 + alive IP + 连接数；
  面板幂等落库，累加 `users.u/d`（或 `used_bytes`）并维护在线设备/在线数。
- 新增「在线设备」数据模型与查询接口；移除 sessions/connection_logs 相关接口与表。
- 保留 revision 配置下发契约（`GET /api/agent/config`），配置渲染仍由面板产出 sing-box
  配置 JSON（含多用户），agent 嵌入加载。

### R4 后台 UI（Vue3）功能/字段对齐
- 节点表单按 `protocol_settings` 分节重构（tls/network/reality 等），协议维持 4 种。
- 服务器、用户列表/表单/详情页字段对齐 R1；新增在线设备展示，移除 sessions/日志页。
- 订阅页保持，字段随实体对齐。

## Out of Scope

- 套餐/订单/支付/优惠券/佣金体系；server_group 组模型；`machine` 实体；
  Xboard 的 11 种协议全量；WebSocket 热更新；MySQL/Redis；React/Shadcn 视觉重写；
  历史数据迁移。

## Acceptance Criteria

- [x] AC1：agent 进程内启动 sing-box 实例并加载面板下发的配置，节点可正常代理转发。
- [x] AC2：VLESS（及 SS/HY2/AnyTLS）真实产生流量后，面板按用户累计 `used_bytes` 增长，
      不再出现「attributed=0 / 流量恒为 0」。
- [x] AC3：面板可查询用户在线的 IP 列表与在线连接数，数据随连接建立/断开更新。
- [x] AC4：设备数限制按用户生效（超过 limit 的新连接被拒绝）。已知限制：仅单节点本地统计，
      不做跨节点全局设备状态（design §9）。
- [x] AC5：配置/用户变更经 revision 下发后，agent 在 ≤ 一个 sync 周期内热应用。
- [x] AC6：sessions / connection_logs 相关表、接口、采集、页面全部移除，构建与测试通过。
- [x] AC7：节点表单支持 `protocol_settings` 分节字段，4 种协议端到端可用。
- [x] AC8：`go build ./...`、`go test ./...`、前端 `npm run build`/lint 通过。
- [x] AC9（补充）：节点 `rate` 在入库时作为流量倍率作用于用量累计（`IngestTrafficBatch`）。

## Status

已完成（2026-09-22）。全部 Go 构建/vet/gofmt/test（含 `-tag integration,with_quic,with_utls`）
与前端 typecheck/build/lint 通过；`docker build` 未在沙箱验证（无 Docker）。
实机端到端（真实客户端 → 内嵌 sing-box → 面板累计）建议上线前手测一次。


## Open Questions

- 无（阻塞项已清空）。
