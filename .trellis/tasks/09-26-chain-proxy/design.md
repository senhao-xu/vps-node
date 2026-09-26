# 设计：链式代理（节点选择出站节点）

## 架构与边界

```
用户 ──> 入口节点 inbound (server A, sing-box)
            │ route.rules: inbound <tag-A> → outbound chain-<A.ID>
            ▼
        出站客户端 (协议客户端, 凭证=relay 派生)
            │
            ▼
        出站节点 inbound (server B) ──> direct ──> 目标
```

- 入口侧：用户认证、流量统计、设备统计全部维持现状（发生在入口 agent）。
- 出口侧：入站上追加一个伪用户 `relay-<入口serverID>`；其流量上报后 panel 按用户名前缀丢弃，不归属任何用户。

## 数据契约

```sql
-- migration 0007_node_chain.sql
ALTER TABLE nodes ADD COLUMN chain_node_id INTEGER REFERENCES nodes(id);
```

- `chain_node_id NULL` = 直连（现状）；非空 = 出站节点 id，可跨 server。
- 自引用、成环在写入时拒绝（repo 层沿 chain_node_id 遍历，步数上限 = 节点总数）。

## relay 凭证派生（无状态，不落库）

新增派生域（`internal/singbox/singbox.go`）：

| 出站协议 | 入站伪用户 | 凭证 |
| --- | --- | --- |
| shadowsocks | `relay-<A.serverID>` | `DeriveSSPassword(appKey, exitNodeID, "relay:<A.serverID>", cipher)` |
| vless | `relay-<A.serverID>` | UUID = `uuidv5(sha256, HMAC(appKey,"relay-v1:<exitNodeID>:<A.serverID>"))` |
| hysteria2 / anytls | `relay-<A.serverID>` | password 同上派生 hex |

同一出口节点被多台入口 server 引用时，每台一个伪用户，互不覆盖。

## sing-box 渲染变更（internal/singbox/singbox.go）

`Render` 签名扩展为接收链信息（每个入口节点可选的 exit 描述）：

```go
type ChainExit struct {
    EntryInboundTag string           // 入口节点 inbound tag
    Exit            Node             // 出站节点（含其 server 的拨号地址）
    RelayCredential map[string]any   // 按出站协议渲染好的凭证
}
```

- 入口 server：`outbounds` 追加 `renderOutbound(exit, relayCred)`（ss/vless/hy2/anytls 客户端出站，tag `chain-<entryNodeID>`）；`route.rules` = `[{inbound:[entryTag], outbound:"chain-<id>"}...]`，`final` 保持 `direct`。
- 出口 server：对应入站 `users` 追加 `relay-<A.serverID>` 伪用户（凭证同源派生）。
- Reality 出站公钥：复用 x25519 从私钥推导（现有逻辑在 subscription/render.go:259-270，抽到 singbox 包共用）。
- 拨号地址：出站节点 `address`（node 级）回退 server.address；端口 = 出站节点 port。
- `ContractVersion` bump → `singbox-render-v2`，agent 端无需改（配置是完整替换）。

## 配置组装变更（internal/web/agent_config.go）

`buildAgentConfigPayload` 增加：

1. 查出本 server 所有挂链节点的出站节点（跨 server join，取 address/port/protocol/settings/解密 secret）。
2. 查"以本 server 节点为出站"的入口 server 列表 → 计算 relay 伪用户并注入对应入站。
3. 任一侧变化都要 bump **两端** server revision（`internal/repo/revisions.go` 的 bump 函数在 chain 写入/删除/出站节点变更时双端调用）。

## API 与校验

- `POST/PUT /api/nodes` 请求体增加 `chain_node_id`（可空）。
- 校验链：目标存在且 active、非自身、无环、目标不是自定义节点（自定义节点不在 nodes 表，天然排除）。
- `DELETE /api/nodes/{id}`：存在 `chain_node_id = id` 的引用 → 409，body 列出引用节点名。

## 前端

- `NodeFormDialog` 增加"出站节点"下拉：按 server 分组的 active 托管节点，排除自身；保存时随节点提交。
- 节点列表新增"链路"列：`→ <server>/<node>` 或 `—`。

## 兼容性 / 回滚

- migration 仅加可空列，旧数据全 NULL = 行为不变。
- `ContractVersion` v2：旧 agent 收到 v2 配置直接应用（完整 JSON 替换），无版本协商逻辑（核实 agentruntime 是否校验 renderer_version——若校验则需同步允许 v2）。
- 回滚：解链（置 NULL）即回 direct；DB 列保留无害。

## 不做

- 多级链（A→B→C）不禁止、由环检测覆盖，但不做专门的报表/优化。
- 出站节点健康检查 / 自动切换。
