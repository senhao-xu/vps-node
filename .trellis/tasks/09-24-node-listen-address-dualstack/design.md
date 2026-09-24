# Design: 节点可选 IPv6 入口（订阅追加 v6 条目）

## 1. Summary

为节点增加两个字段：`ipv6_enabled`（是否对外发布 v6 入口）与 `ipv6_address`（v6 连接地址）。订阅渲染时，对启用了 v6 的节点，在主条目之外追加一条 server 为 v6 地址、端口/协议参数/凭据完全一致的条目。服务端不新增 inbound、不改 `listen`、不改端口唯一索引、不改 agent 契约；因此 v4/v6 仍命中同一 inbound，流量按 `Pair{User,Node}` 汇总、visit 日志按 `ClientIP` 可区分。

## 2. Architecture & Boundaries

```
node form ──POST/PUT──► internal/web/nodes.go ──repo──► nodes.ipv6_enabled/ipv6_address
                                                              │
GET /api/subscribe ◄── internal/web/subscriptions.go ◄────────┘ (ListSubscriptionNodes)
        │
        └── internal/subscription/render.go: expandIPv6(Node) → 主条目 + "{name}-v6" 条目
```

- **No agent/kernel change**: `internal/kernel/singbox`、`internal/agentclient`、`internal/singbox`（监听渲染）保持不变。
- **No uniqueness/index change**: `idx_nodes_port_active (server_id, port)` 不动。
- Owner of subscription shape = `internal/subscription`；web 层只做 repo→subscription 映射。

## 3. Data Model & Migration

新增迁移 `internal/db/migrations/0004_node_ipv6.sql`（下一序号，现有 0001–0003）：

```sql
ALTER TABLE nodes ADD COLUMN ipv6_enabled INTEGER NOT NULL DEFAULT 0;
ALTER TABLE nodes ADD COLUMN ipv6_address TEXT NOT NULL DEFAULT '';
```

- 用 `ALTER TABLE ADD COLUMN`，不重建表；避免 `database-guidelines.md` 里“父表重建导致子表级联”的坑。
- 默认值保证向后兼容：现有节点 v6 关闭。
- Go 侧 `repo.Node` / `NewNode` 增加 `IPv6Enabled bool`、`IPv6Address string`（INTEGER 0/1 ↔ bool）。

## 4. Contracts

### 4.1 节点 DTO（新增两个字段，其余不变）

```json
{ "id": 1, "server_id": 1, "address": "hk01.example.com",
  "ipv6_enabled": false, "ipv6_address": "",
  "name": "HK-SS", "protocol": "shadowsocks", "port": 8388, ... }
```

- `ipv6_enabled`：boolean，默认 `false`。
- `ipv6_address`：string，默认 `""`。非空时必须是一个 IPv6 字面量（`net.ParseIP(addr) != nil && To4() == nil`）。
- 校验：`ipv6_enabled == true` 时 `ipv6_address` 必须非空且合法，否则 `422 validation`；`ipv6_enabled == false` 时地址可保留但不参与订阅。
- `POST /api/nodes`、`PUT /api/nodes/:id`（partial）接受这两个字段；`POST /api/nodes/:id/copy` 逐字复制两个字段。
- `PUT` 语义：与现有 partial 更新一致，未提供则不修改。

### 4.2 订阅行为（binding behavior）

- Clash 与 general(base64) 两种格式：启用 v6 时，为该节点输出两条；v6 条目 `server = ipv6_address`、`port` 不变、协议参数/凭据不变、`name = "{原name}-v6"`。
- `ipv6_address` 为空、或 `ipv6_enabled=false`、或 `ipv6_address == address`（原地址）时，不追加条目（保持与升级前一致）。
- v6 条目的失败处理与主条目一致：走同一个 `onSkip` 回调，`node_id` 相同。

### 4.3 契约变更清单（`docs/api-contract.md` 是 binding，需同步）

- `## Nodes` 示例 DTO 增加 `ipv6_enabled` / `ipv6_address`。
- `### GET /api/nodes`、`POST /api/nodes`、`GET /api/nodes/:id`、`PUT /api/nodes/:id`、`POST /api/nodes/:id/copy` 段落补充字段语义与校验。
- 订阅契约段说明“启用 v6 时追加 `{name}-v6` 条目”。
- 这是本任务对 binding contract 的全部改动，实施时如需额外改动必须回到本设计。

## 5. Data Flow / Implementation Anchors

- repo 列清单（必须同步加两列，避免列错位）：
  - `internal/repo/nodes.go`：`nodeSelect`(59)、`nodeSelectWithServer`(61)、`insertNodeExec`(64,79)、`UpdateNodeSpec`(175,177)、`scanNode`(218)、`scanNodeWithServerName`(232)。
  - `internal/repo/subscriptions.go`：`ListSubscriptionNodes`(89,91) 显式列（不是 `scanNode`）。
  - `internal/repo/mutations.go`：`UpdateNodeAndBump`(300,304,314) 的两条 UPDATE。
- web 层：`internal/web/nodes.go`（`createNodeRequest`、`updateNodeRequest`、create/update/copy handler、`validateNodeSpec` 旁新增 `validateNodeIPv6`）；`internal/web/dto.go`（`nodeDTO`、`toNodeDTO`）。
- 订阅层：`internal/subscription/render.go` 增加 `Node.IPv6Address` 与 `expandIPv6(n Node) []Node`；`RenderGeneral`(167)、`RenderGeneralLinks`(182)、`RenderClashFiltered`(199) 改为遍历展开后的节点；`renderProxy`(385)/`renderURI`(217) 无需改动（用传入 Node 的 Address/Name）。
- 映射：`internal/web/subscriptions.go:179` 设置 `IPv6Address`（仅当 `n.IPv6Enabled`）。
- 前端：`web/src/api/types.ts`（`NodeBrief`/`CreateNodeInput`/`UpdateNodeInput`）；`web/src/components/NodeFormDialog.vue`（`ToggleSwitch` + 地址输入 + 校验 + create/update payload）。

`expandIPv6` 约定：

```go
func expandIPv6(n Node) []Node {
    if n.IPv6Address == "" || n.IPv6Address == n.Address {
        return []Node{n}
    }
    v6 := n
    v6.Address = n.IPv6Address
    v6.Name = n.Name + "-v6"
    v6.IPv6Address = "" // 防止二次展开
    return []Node{n, v6}
}
```

## 6. Trade-offs / Decisions

- **合并统计**（用户确认）：v4/v6 汇总到同一节点，不新增 per-family 计数。若要拆分需改 `internal/kernel/singbox/tracker.go` 的 `Pair`，属本任务 out of scope。
- **两个字段而非一个**：`ipv6_enabled` + `ipv6_address`，支持临时关闭但保留地址；缺点是契约多一个字段。UI 的开关关掉时不丢地址。
- **只接受 IPv6 字面量**：与“填入 V6 地址”一致，不做域名解析；域名场景可继续用主 `address`。
- **不改监听/唯一约束**：单 inbound `::` 已双栈，无需方案 C 的复杂绑定。

## 7. Compatibility / Rollback

- 升级：仅需部署 Panel（含新迁移）；agent 无需升级，旧 agent 继续正常同步。
- 旧节点：`ipv6_enabled=0`，订阅输出与升级前逐字一致。
- 回滚：回退 Panel 代码即可；新增列保留无副作用（订阅不再读取）。

## 8. Operational Notes

- 需要服务器本身具备可用的全局 IPv6，且容器/sing-box 能监听 `::`（当前默认即是）；由运维保证。
- 面板不校验 v6 可达性；启用前建议运维自行 `ping6` / 用 v6 客户端实测。
