# 链式代理：节点选择出站节点

## Goal

托管节点可配置一个"出站节点"（另一个托管节点，可跨 server）。用户连接入口节点后，流量经 sing-box 客户端出站转发到出站节点再落地。链路对用户订阅透明（订阅中只有入口节点）。

## Confirmed Facts（代码证据）

- sing-box 配置渲染：`internal/singbox/singbox.go:83` `Render()`，当前 outbounds 仅 `direct`、`route.final = direct`（singbox.go:95-96）。
- 节点表：`nodes`（0001_init.sql:62），server_id + protocol + port + protocol_settings + secret_enc；address/ipv6 由 0003/0004 增加。
- 配置下发：按 server 版本化轮询，`internal/web/agent_config.go:144` 调 `singbox.Render`；revision 机制在 `internal/repo/revisions.go`。
- 用户凭证派生：`DeriveSSPassword(appKey, nodeID, userUUID, method)` 等（singbox.go:60-66），HMAC 无状态派生。
- Reality 公钥可从私钥推导：`internal/subscription/render.go:259-270`（x25519，ecdh 标准库）。
- 流量按入站用户名在 agent 侧统计上报（kernel/singbox/tracker.go）。

## Requirements

- R1：`nodes` 增加 `chain_node_id INTEGER NULL REFERENCES nodes(id)`（migration 0006）。
- R2：入口 server 渲染：每个挂链节点生成一个协议客户端出站（tag `chain-<nodeID>`，server=出站节点 address，port=出站节点 port，凭证=relay 派生凭证）；`route.rules` 增加 `{"inbound":["<入站tag>"],"outbound":"chain-<nodeID>"}`，置于 final 之前。
- R3：relay 凭证：出站节点入站追加伪用户 `relay-<入口serverID>`，密码由 appKey 派生（如 SS 用 `DeriveSSPassword(appKey, exitNodeID, "relay:<serverID>")`，VLESS/Hy2/AnyTLS 用派生 UUID/密码），不落库。
- R4：四种协议均支持作为出站端（sing-box 均有对应 outbound）；入站↔出站协议可异构。
- R5：保存/改链/解链时同时 bump 入口与出站两端 server 的 revision。
- R6：环检测：保存时拒绝 A→B→A（含多级）。
- R7：删除保护：被引用的出站节点拒绝删除，返回可读错误提示先解链。
- R8：流量口径：入口侧按真实用户统计（现状不变）；出站侧 agent 上报的 `relay-*` 用户名由 panel 忽略，不计入任何用户。
- R9：管理 UI：节点表单增加"出站节点"下拉（跨 server 分组、只列 active 托管节点、排除自身及会形成环的节点）；节点列表显示链路标识。

## Acceptance Criteria

- [ ] 节点 A 挂到节点 B 后，两端 agent 拿到新配置；经 A 连接的流量实际从 B 的出口 IP 落地
- [ ] 链路中用户流量只计入一次（入口侧），relay-* 不出现在任何用户的流量里
- [ ] 配置 A→B→A 被保存接口拒绝并返回可读错误
- [ ] 出站节点 B 同时仍可被用户直接连接（入站不受链影响）
- [ ] 解链/删链后两端配置回到 direct 出站

## Out of Scope

- 自定义节点作为出站端
- 多出口负载均衡 / 链式节点池
- 链路的独立流量报表
