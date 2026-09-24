# 节点可选 IPv6 入口（订阅追加 v6 条目）

## Goal

让管理员为一个节点配置可选的 IPv6 入口（启用开关 + IPv6 地址）；启用后，用户的订阅（Clash / 通用 base64）在为主节点输出主条目之外，追加一条使用 IPv6 地址的连接条目（端口、协议参数、凭据与主条目完全一致，仅 server 地址不同）。服务端仍只保留一个 inbound，v4/v6 客户端命中同一 inbound，因此流量与日志照常采集。

## Background / Confirmed Facts

- 渲染器已把 sing-box inbound 监听写死为 `"::"`（`internal/singbox/singbox.go:146,183,219,263`），单节点本就双栈监听；节点 `address` 只是客户端连接地址（`internal/subscription/render.go:218,386`）。
- 流量按 `Pair{UserID, NodeID}` 记账，节点归属来自 `metadata.Inbound` 的 inbound tag（`internal/kernel/singbox/tracker.go:79,90,154`）。v4/v6 命中同一 inbound，两条订阅条目仍汇总到同一节点。
- 访问日志 `Visit` 含 `ClientIP`（`internal/kernel/singbox/tracker.go:186`），日志中可区分 v4 / v6。
- 订阅流程：`internal/web/subscriptions.go:153-179` 组装 `[]subscription.Node`，再由 `RenderGeneralLinks`（`render.go:182`）/ `RenderClashFiltered`（`render.go:199`）渲染，二者直接用 `n.Address`、`n.Port`。
- Clash 代理名必须唯一（`render.go:363` 的 `assembleClash`/`expandGroups` 按 name 建组），追加条目需要可区分的名称。
- 现无任何 IPv6 相关节点字段；`docs/api-contract.md` 是 binding contract。

## Requirements

- R1 节点新增 `ipv6_enabled`（bool，默认 false）与 `ipv6_address`（string，默认空）：启用开关 + IPv6 地址。
- R2 启用 v6 时，订阅（Clash 与通用 base64）为该节点追加一条 v6 条目：`server = ipv6_address`，`port`/凭据/协议参数与主条目一致。
- R3 追加条目名称与主条目可区分（`"{name}-v6"`），保证 Clash 代理名唯一、分组正常。
- R4 不新增 inbound、不改 `listen` 渲染、不改端口唯一索引、不改 agent 契约；流量与日志继续按 `(user, node)` 采集。
- R5 迁移向后兼容：现有节点默认 `ipv6_enabled=0`，订阅输出与升级前逐字一致。
- R6 `ipv6_address` 非空时必须是 IPv6 字面量；启用时地址必填，否则 `422 validation`。

## Key Decisions

- v4 / v6 流量**合并统计**到同一节点（用户确认）；如需分地址族拆分属后续独立需求。
- 用两个字段（开关 + 地址）而非单一字段，支持临时关闭但保留地址。
- 只接受 IPv6 字面量，不做域名解析。

## Out of Scope

- 按地址族分别统计流量（需改 `internal/kernel/singbox/tracker.go` 的计量键）。
- 服务端监听地址字段、同端口多 inbound、端口唯一索引调整。
- 自动探测或校验服务器是否具备可用全局 IPv6（由运维保证）。

## Acceptance Criteria

- [ ] 节点可配置并保存启用状态与 IPv6 地址，并在编辑表单回显。
- [ ] 启用后，Clash 订阅为该节点多出一条 v6 代理，端口/凭据/参数与主条目一致、名称可区分。
- [ ] 通用 base64 订阅同样多出一条 v6 分享链接。
- [ ] 关闭、地址为空、或地址等于主地址时，订阅输出与升级前一致。
- [ ] 非法 IPv6 地址返回 `422 validation`。
- [ ] v4/v6 两条连接的使用量汇总计入同一节点，visit 日志按 `ClientIP` 可区分。
