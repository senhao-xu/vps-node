# Design: 自定义/订阅线路作为链式出口

## 1. 范围与决策

托管**入口节点**的链式出口除托管节点外，可指向某个自定义节点来源内的**单条线路**（external entry）。决策（用户确认）：

- 协议覆盖（Q1=A）：ss / vless / trojan / vmess / hysteria2 / anytls / socks5；不支持的类型在选择器置灰。
- 数据可用性（Q2=A）：config 构建不联网；订阅来源无缓存 → 该出口不可用，入口回退直连。
- 生命周期（Q3=A）：删除被引用来源→`409`；来源更新/刷新→bump 引用它的入口节点所属服务器；条目失效→回退直连。

外部出口不可注入 relay 用户（外部服务器不受控）：入口 Agent 用条目自带凭据直连外部服务器。入口侧归属照旧；出口跳无面板侧统计。

## 2. 存储模型（migration `0012_nodes_chain_custom_exit.sql`）

`nodes` 增加两列（简单 `ALTER TABLE ADD COLUMN`，同 0007 模式，不做全表重建）：

```sql
ALTER TABLE nodes ADD COLUMN chain_custom_node_id INTEGER REFERENCES custom_nodes (id);
ALTER TABLE nodes ADD COLUMN chain_custom_entry_key TEXT NOT NULL DEFAULT '';
CREATE INDEX idx_nodes_chain_custom_node ON nodes (chain_custom_node_id);
```

- 与 `chain_node_id` **互斥**：应用层校验（二者不能同时非空），不新增 CHECK（避免全表重建）。
- 删除自定义节点用应用层保护（`409`），FK 默认 NO ACTION 作兜底。

`internal/repo/nodes.go`：`Node`/`NewNode` 增加 `ChainCustomNodeID *int64`、`ChainCustomEntryKey string`；按 spec §9 把所有显式列清单补上（`nodeSelect`、`nodeSelectWithServer`、`insertNodeExec`、`UpdateNodeSpec`、`UpdateNodeAndBump` 两个分支、`scanNode`、`scanNodeWithServerName`、`ListSubscriptionNodes`）。新增：

```go
func (r *Repo) ListNodesByCustomChainTarget(ctx, customNodeID int64) ([]Node, error) // 删除保护
func (r *Repo) ListServersChainingCustomNode(ctx, customNodeID int64) ([]int64, error) // 更新/刷新 bump
```

## 3. 出站转换器（`internal/singbox/outbound.go`，新增）

`internal/singbox` 不能 import `internal/subscription`（后者已 import 前者）。转换器只吃 Clash proxy `map[string]any`，由 web 层负责把链接解析成 proxy：

```go
// ProxyToOutbound 把 Clash Meta proxy map 转成 sing-box outbound map（不含 tag）。
func ProxyToOutbound(proxy map[string]any) (map[string]any, error)
// OutboundSupported 按 Clash type 判断是否支持（供 API 置灰）。
func OutboundSupported(clashType string) bool
```

映射覆盖（读 Clash 键 → sing-box 键）：

| Clash type | 关键字段 → sing-box |
|---|---|
| `ss` | `cipher→method`, `password`, `server/server_port` |
| `vless` | `uuid`, `flow`, `network`, `tls`/`servername`, `reality-opts{public-key,short-id}→tls.reality`, `ws-opts`/`grpc-opts` |
| `trojan` | `password`, `sni→tls.server_name`, `skip-cert-verify→tls.insecure`, `ws-opts`/`grpc-opts` |
| `vmess` | `uuid`, `alterId→alter_id`, `cipher→security`, `network`, `tls`, `ws-opts`/`grpc-opts` |
| `hysteria2` | `password`, `sni→tls.server_name`, `skip-cert-verify→tls.insecure`, `obfs`/`obfs-password` |
| `anytls` | `password`, `sni`, `skip-cert-verify→tls.insecure` |
| `socks5` | `username`/`password`（sing-box type 为 `socks`） |

不支持/缺必填 → error（入口回退直连）。`tag` 由 `Render` 注入 `chain-<entryNodeID>`。

## 4. 链式渲染（`internal/singbox/singbox.go`）

`ChainExit` 增加预构建出站：

```go
type ChainExit struct {
    EntryNodeID   int64
    EntryServerID int64
    Exit          Node              // 托管出口
    DialAddress   string
    Outbound      map[string]any    // 非空=外部线路，直接用（Render 注入 tag）
}
```

`Render`：`if c.Outbound != nil { clone, 设 tag, 走 route rule }` 否则走现有 `renderChainOutbound`。route rule 与 tag 逻辑不变。

## 5. 配置构建（`internal/web/agent_config.go`）

`chainExits` 扩展：
- 托管出口逻辑不变。
- 对 `ChainCustomNodeID != nil` 的 active 节点：`repo.GetCustomNode` → 必须 `active`；解析内容（**不联网**）：
  - `links`：`secrets.Decrypt(ContentEnc)` → `SplitLinkLines`；
  - `subscription`：`CachedContent`（为空则该出口不可用，回退直连）。
- 逐条：链接用 `subscription.ParseShareURI` 得 proxy，proxy 用 `singbox.EntryKey(h.appKey, proxy)` 求 key；上游 proxy 直接求 key；`key == ChainCustomEntryKey` 命中即 `singbox.ProxyToOutbound(proxy)`。
- 命中且转换成功 → `ChainExit{EntryNodeID: n.ID, EntryServerID: server.ID, Outbound: outbound}`；否则跳过（无 route rule → 直连）。
- 需要给 `customNodeEntries`/解析复用：在 `internal/subscription` 增加 `ResolveEntries(appKey, links, proxies) ([]ResolvedEntry, error)`（`ResolvedEntry{Key string; Proxy map[string]any; Link string}`），`customNodeEntries` 与 `chainExits` 共用，避免解析逻辑分叉。

## 6. 生命周期与 revision

- **节点 create/update/delete**：`chain_custom_node_id` 变化只影响入口节点所属服务器；托管 `chain_node_id` 的旧/新出口 server bump 逻辑保持。互斥校验：同时给 `chain_node_id` 与 `chain_custom_node_id` → `422`。update 三态：省略=保留、`null`=清除、id=设置；`chain_custom_entry_key` 随 source 一并设置/清除。
- **自定义节点 update/refresh**（`internal/web/custom_nodes.go`）：成功后对 `ListServersChainingCustomNode(id)` 的每个 server bump（源停用/内容变化会让引用它的入口重渲染）。删除自定义节点前若 `ListNodesByCustomChainTarget` 非空 → `409 conflict` 提示引用它的入口节点。
- 订阅来源的**懒加载**缓存刷新发生在订阅渲染路径（不影响 agent config）；因此链式出口只读缓存，管理端通过"更新订阅"触发上面这条 bump 规则。

## 7. API 契约

- `GET /api/custom-nodes/{id}/nodes`：`customNodeEntryDTO` 增加 `chain_supported bool`（`singbox.OutboundSupported(type)`），供选择器置灰。
- `POST /api/nodes` / `PUT /api/nodes/{id}`：新增 `chain_custom_node_id`（可空）与 `chain_custom_entry_key`（string）。校验：来源存在且 active；`entry_key` 为空表示清除，否则 64 位小写 hex；不能与 `chain_node_id` 同时非空。update 中 `chain_custom_node_id` 为三态（省略/`null`/id），`chain_custom_entry_key` 随之为空或值。
- `nodeDTO`：增加 `chain_custom_node_id`、`chain_custom_node_name`（join `custom_nodes`，仅展示）、`chain_custom_entry_key`。

## 8. 前端

- `web/src/api/types.ts`：`CustomNodeEntry` 加 `chain_supported: boolean`；`NodeBrief`/节点详情加 `chain_custom_node_id`、`chain_custom_node_name`、`chain_custom_entry_key`；`CreateNodeInput`/`UpdateNodeInput` 加对应字段。
- `web/src/components/NodeFormDialog.vue`：出口选择改为模式选择「直连 / 托管节点 / 自定义线路」。选自定义线路时：先选来源（`listCustomNodes`，active），再 `getCustomNodeEntries(sourceId)` 拉取线路，仅可选 `chain_supported` 的项；保存时设置 `chain_custom_node_id` + `chain_custom_entry_key`，并清空 `chain_node_id`（反之亦然）。编辑打开时用已存 key 在高亮匹配项。
- `web/src/pages/NodesPage.vue`：链式列对外部出口显示 `→ <来源名>`（可加线路名若能解析）。
- 避免对不支持类型选择：`chain_supported=false` 的条目 disabled 并提示。

## 9. 兼容与回滚

- 纯增量：`chain_custom_node_id` 默认 NULL，旧数据与旧 agent config 逐字节不变（`Outbound == nil` 路径不变）。
- 回滚：回退代码；0012 为加列表/索引，残留无害。

## 10. 已知限制

- 出口跳无面板侧流量/设备/访问统计；入口侧统计不变。
- 订阅来源需先"更新订阅"（缓存）才能作为出口；无缓存回退直连。
- `entry_key` 排除 name 且只哈希连接参数：上游改参数即失效回退；同连接不同名折叠。
- 仅支持第 3 节列出的类型，其余类型不可选。

## 11. 测试

- `internal/singbox/outbound_test.go`：每种 type 的字段映射、reality/ws/grpc/tls/insecure、socks5→socks、未知类型 error。
- `internal/singbox/chain_test.go`：`ChainExit.Outbound` 直用并注入 tag/route rule。
- `internal/web/nodes_custom_chain_test.go`（以 `nodes_chain_test.go` 为模板）：外部出口渲染出站、来源无缓存回退直连、条目失效回退、互斥校验、三态更新、delete 保护、update/refresh bump。
- `internal/e2e/integration_chain_test.go`：外部出口用嵌入式 sing-box `Validate` 实际校验。
- `internal/repo`：新列 round-trip、`ListNodesByCustomChainTarget`、`ListServersChainingCustomNode`。
- `internal/db/migration_custom_chain_exit_test.go`：升级/幂等/FK/索引。
- 前端：`npm run typecheck/lint/build`。
