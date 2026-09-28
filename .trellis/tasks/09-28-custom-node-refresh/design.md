# 设计：自定义节点查看与更新订阅

## 架构与边界

- 全部为管理端接口（`requireAdmin`），只读查看 + 显式刷新两类操作。
- 解析逻辑下沉到 `internal/subscription` 包（纯函数、可单测），web 层只做取数/组装/缓存。
- 不触碰订阅渲染路径（`customSourcesForUser` / `fetchCustomNodeContent`）与 Agent 配置；无 DB migration。
- 刷新使用 `subscription.FetchSubscription`（已有超时/大小/重定向限制）。

## 接口契约

### `GET /api/custom-nodes/{id}/nodes`

- 200 响应：
  ```json
  {
    "source_type": "subscription",
    "has_cache": true,
    "fetched_at": "2026-09-28T10:00:00Z",
    "entries": [ { "name": "HK-1", "type": "vless", "server": "1.2.3.4", "port": 443 } ],
    "skipped": ["unsupported share link scheme \"foo\""]
  }
  ```
- 未知 id → 404 `not_found`。
- `links` 类型：解析加密内容，`has_cache` 不适用（返回 `has_cache=false`、`fetched_at=null`、entries 来自链接文本）。
- `subscription` 无缓存：`entries=[]`、`has_cache=false`，不联网。

### `POST /api/custom-nodes/{id}/refresh`

- 仅 `subscription`：强制 `FetchSubscription` → `repo.UpdateCustomNodeCache` → 200，响应与上面同构（`has_cache=true`、最新 `fetched_at`）。
- `links` 类型 → 422 `validation`（"refresh is only supported for subscription sources"）。
- 上游失败 → 500 `internal` + message（"failed to fetch upstream subscription: ..."），**不调用** `UpdateCustomNodeCache`，旧缓存保留。
- 未知 id → 404 `not_found`。

## 解析设计（新文件 `internal/subscription/summary.go`）

```go
type NodeSummary struct {
    Name   string
    Type   string
    Server string
    Port   int
}

// Summarize parses share links and/or Clash proxy maps into display summaries.
// Unparseable/incomplete entries are returned in skipped (raw line or name/index).
func Summarize(links []string, proxies []map[string]any) (summaries []NodeSummary, skipped []string)
```

- `links`：逐条 `ParseShareURI`；失败 → `skipped = append(skipped, line)`。
- `proxies`：读 `name`/`type`/`server`/`port`；缺 `name|server|type` 或端口非法 → skipped（用 name 或 `proxy #i` 标识）。
- 端口统一走 `looseInt`（已有）。

## 数据流

- links 查看：`GetCustomNode` → `secrets.Decrypt` → `SplitLinkLines` → `Summarize(lines, nil)`。
- subscription 查看：`GetCustomNode` → `CachedContent` → `NormalizeFetchedContent` → `Summarize(links, proxies)`。
- subscription 刷新：`GetCustomNode` → `FetchSubscription` → `UpdateCustomNodeCache` → `NormalizeFetchedContent` → `Summarize`。

## Web 层改动

- `internal/web/custom_nodes.go`：
  - 新增 `customNodeEntryDTO` / `customNodeEntriesResponse`。
  - 新增 `handleCustomNodeNodesGet`、`handleCustomNodeRefresh`。
  - 抽 helper `customNodeEntries(cn repo.CustomNode) customNodeEntriesResponse`（基于已解密/缓存内容组装）。
- `internal/web/web.go`：注册两条路由（紧跟现有 custom-nodes 路由）。

## 前端改动

- `web/src/api/types.ts`：新增 `CustomNodeEntry`、`CustomNodeEntries`。
- `web/src/api/customNodes.ts`：`getCustomNodeEntries(id)`、`refreshCustomNode(id)`。
- 新组件 `web/src/components/CustomNodeNodesDialog.vue`：ModalDialog 内展示 entries 列表 + `skipped` 提示；`subscription` 显示“更新订阅”按钮，刷新中禁用并回显错误。
- `web/src/pages/CustomNodesPage.vue`：行操作增加“查看节点”；`subscription` 追加“更新订阅”；弹窗打开/刷新后回写 `has_cache`/`fetched_at`（重新 `load()`）。

## 复用

- 复用 `secrets.Decrypt`、`subscription.FetchSubscription`/`NormalizeFetchedContent`/`SplitLinkLines`/`ParseShareURI`、`repo.GetCustomNode`/`UpdateCustomNodeCache`。
- 前端复用 `ModalDialog`、`ErrorBanner`、`LoadingSpinner`、`StatusBadge`/`chip` 现有组件与样式令牌。

## 权衡与决策

- **查看不联网**（用户已决定）：无缓存提示“未拉取，请先更新订阅”，保持查看只读。
- **刷新仅 subscription**（用户已决定）：links 本地解析无需刷新。
- **刷新失败用 500 `internal`**：契约无 502，且为 admin-only，message 可读；不改 envelope codes。
- **解析可在渲染路径复用**：`Summarize` 为纯函数，未来若渲染也要校验可复用；本期不改渲染。

## 兼容与回滚

- 纯新增接口/组件，无 schema 变更。回滚 = 删除路由、handler、组件与 api 方法；已有数据不受影响。
- 不改 `custom_nodes` 表结构，`UpdateCustomNodeCache` 复用现有列。

## 风险

- 上游响应上限 1 MiB、超时 5s、最多 3 次重定向（已存在），refresh 同步等待，最长约 5s。
- Clash proxy 端口类型不统一 → `looseInt` 兜底。
- 大订阅条目多，弹窗列表需设最大高度滚动。
