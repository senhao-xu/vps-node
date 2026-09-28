# Design: 自定义节点按单条线路授权

## 1. 目标与边界

在模型 A 下，自定义节点来源的授权分为两层：

1. **来源开关**（已有）：`user_custom_nodes(user_id, custom_node_id)`。关闭 = 该来源对该用户完全不输出。
2. **线路白名单**（新增）：`user_custom_node_entries(user_id, custom_node_id, entry_key)`。
   - 来源开启且白名单为空 → 输出该来源全部可解析线路（兼容既有行为）。
   - 来源开启且白名单非空 → 只输出 `entry_key` 命中的线路。

不变式：自定义节点只影响订阅输出，不参与 Agent 配置、不 bump server revision、不参与流量/设备统计（沿用 `internal/web/custom_nodes.go:480` 的既有约定）。

## 2. Entry Key（线路稳定标识）

新增 `internal/subscription/entrykey.go`：

```go
// EntryKey = HMAC-SHA256(appKey, canonicalJSON(proxy with "name" removed)) 的 hex。
func EntryKey(appKey []byte, proxy map[string]any) (string, error)
// LinkEntryKey 解析分享链接后计算 key；链接无法解析时 ok=false。
func LinkEntryKey(appKey []byte, link string) (key string, ok bool)

type EntrySummary struct {
    Key string
    Name, Type, Server string
    Port int
}
// SummarizeEntries 在 Summarize 基础上为每个条目附带 Key。
func SummarizeEntries(appKey []byte, links []string, proxies []map[string]any) ([]EntrySummary, []string)
```

规则：

- **规范化**：复制 proxy map 后删除 `"name"`，用 `encoding/json.Marshal`（Go 对 map key 排序，确定性）。允许嵌套 map（ws-opts 等）。
- **排除 name**：上游重命名线路时授权不丢失；代价是「连接参数完全相同、仅 name 不同」的重复条目会折叠为同一 key（罕见，可接受）。
- **HMAC(appKey)**：避免把凭据的裸 hash 落库后被离线爆破（与节点密钥用 appKey 加密的既有安全模型一致）。
- **无法解析的 general 行**：无 key，不可单独授权；白名单非空时会被过滤掉（已知限制，写入文档与 UI 提示）。

## 3. 数据模型与迁移

新增迁移 `internal/db/migrations/0010_user_custom_node_entries.sql`：

```sql
CREATE TABLE user_custom_node_entries (
  user_id        INTEGER NOT NULL REFERENCES users(id)        ON DELETE CASCADE,
  custom_node_id INTEGER NOT NULL REFERENCES custom_nodes(id) ON DELETE CASCADE,
  entry_key      TEXT    NOT NULL,
  created_at     INTEGER NOT NULL,
  PRIMARY KEY (user_id, custom_node_id, entry_key)
);
CREATE INDEX idx_user_custom_node_entries_user ON user_custom_node_entries(user_id);
```

- 既有 `user_custom_nodes` 数据**不需要迁移**：有源、无白名自行 = 全部线路。
- 删除 custom node / user 级联清理白名单。
- 内容变更后失效的 key 保留但**惰性无效**（渲染取交集）；不主动清理。

`internal/repo/custom_nodes.go` 新增：

```go
// 返回 custom_node_id -> 已授权 key 集合；仅含非空白名单。
func (r *Repo) ListCustomNodeEntryKeysByUser(ctx, userID) (map[int64]map[string]struct{}, error)
// 单事务整集替换来源授权 + 线路白名单（替代 handler 对 SetUserCustomNodes 的直接调用）。
func (r *Repo) SetUserCustomNodesAndEntries(ctx, userID int64, customNodeIDs []int64, entries map[int64][]string) error
```

`SetUserCustomNodesAndEntries`：同一 `Tx` 内先 `DELETE` 两张表的该用户行，再插入来源 + 非空白名单；沿用 `dedupeInt64`。既有 `SetUserCustomNodes` / `AuthorizeUserCustomNode` / `RevokeUserCustomNode` 保留（测试与潜在调用方），handler 改用新方法。

## 4. API 契约

### 4.1 `GET /api/custom-nodes/{id}/nodes`
每条 entry 增加 `key string`（`customNodeEntryDTO` 新增字段；`customNodeEntries` 改用 `SummarizeEntries`）。UI 端对同 key 条目去重展示。

### 4.2 `GET /api/users/{id}/custom-nodes`
响应新增：

```json
{
  "custom_node_ids": [1,2],
  "custom_nodes": [ ... ],
  "custom_node_entries": [
    { "custom_node_id": 1, "entry_keys": ["<hex64>", "..."] }
  ]
}
```

`custom_node_entries` 只包含白名单非空的来源；无白名单的来源省略。

### 4.3 `PUT /api/users/{id}/custom-nodes`
请求体新增可选 `custom_node_entries`（同上结构）。

校验（`422 validation`）：
- 每个 `custom_node_id` 必须出现在 `custom_node_ids` 中；
- `entry_key` 非空、长度为 64 的十六进制（不联网校验其属于该来源，渲染时取交集兜底）；
- 去重；空白名单项可省略。

handler 调 `SetUserCustomNodesAndEntries`，响应同 GET，仍无 revision bump。

## 5. 渲染过滤

`internal/subscription/render.go`：`CustomSource` 保持原有字段（`ID/Name/Links/Proxies`），过滤在 web 层完成，render 包不感知白名单。

`internal/web/subscriptions.go` `customSourcesForUser`：
- `ListCustomNodeEntryKeysByUser` 取该用户白名单；
- 构建 source 时过滤：
  - links：`allowed` 为空 → 原样保留；否则 `LinkEntryKey(appKey, line)` 命中才保留（不可解析行丢弃）；
  - proxies：`allowed` 为空 → 保留；否则 `EntryKey(appKey, proxy)` 命中才保留。
- 过滤后 `len(Links)==0 && len(Proxies)==0` → 复用既有的 `continue` 跳过来源（`subscriptions.go:297`）。
- `RenderGeneralLinksMerged` / `RenderClashFilteredMerged` 逻辑不变，因为过滤已在构建 source 时完成（白名单为空时行为与现状字节一致，保证回归安全）。

权衡：把过滤放在「构建 source」层而非 render 层，可让 render 包保持纯函数、且 general/clash 两格式天然一致；代价是 `customSourcesForUser` 多一次解析 links 的工作（可接受，规模有限）。

## 6. 前端

- `web/src/api/types.ts`：
  - `CustomNodeEntry` 增加 `key: string`；
  - 新增 `CustomNodeEntrySelection = { custom_node_id: number; entry_keys: string[] }`；
  - `UserCustomNodes` 增加 `custom_node_entries: CustomNodeEntrySelection[]`。
- `web/src/api/customNodes.ts`：`putUserCustomNodes(userId, customNodeIds, entries)`，body 带 `custom_node_entries`。
- 新组件 `web/src/components/user/CustomNodeEntryPickerDialog.vue`（复用 `ModalDialog` + 状态三件套）：
  - 打开时 `getCustomNodeEntries(sourceId)`（不联网，缓存空则 `EmptyState` 提示先更新订阅）；
  - 顶部授权范围 `SegmentedControl`：「全部线路」/「仅所选线路」；
  - 「仅所选」时展示去重后条目的复选框，要求至少选 1 条；
  - 确认后 emit `(entry_keys: string[])`：全部 → `[]`；仅所选 → 选中的 keys。
- `UserNodeAuth.vue`：
  - 新增 `selectedCustomEntries: Record<number, string[]>`（props 同步）；
  - 每个来源行增加「选择线路」按钮（来源勾选且 active 时可用）+ 摘要 chip（白名单空 → “全部线路”，否则 “已选 N 条”）；
  - `save()` 一并提交来源与白名单。
- `UserDetailPage.vue`：把 `custom_node_entries` 从 `getUserCustomNodes` 传入 `UserNodeAuth`，`onNodesSaved` 更新新状态。

## 7. 兼容性与回滚

- 纯增量：新表 + 新增 API 字段 + 新增可选请求字段。旧数据与旧订阅输出在「白名单为空」路径下逐字节不变。
- 回滚：回退代码即可；新表残留无害（可后续手动 drop）。无 destructive migration，无需 revision bump。

## 8. 测试

- `internal/subscription/entrykey_test.go`：key 稳定性（同连接不同 name 同 key）、不同连接不同 key、appKey 不同则 key 不同、不可解析链接 ok=false。
- `internal/db/migration_user_custom_node_entries_test.go`：升级建表、幂等、FK 级联。
- `internal/repo/custom_nodes_test.go`：`SetUserCustomNodesAndEntries` 整集替换/去重/级联；`ListCustomNodeEntryKeysByUser`。
- `internal/web/custom_nodes_test.go`：entries API 返回 key；PUT 校验（未知来源 key、非法 key 格式、空白名单省略）；GET 回显。
- `internal/web/custom_nodes_test.go`（渲染）：general 与 clash 在白名单下只输出命中线路；白名单为空 = 全部（回归）；不可解析 general 行在白名单下被过滤。
- `internal/subscription/custom_test.go`：保留现有 render 回归（render 层不感知白名单，白名单过滤由 web 层测试覆盖）。

## 9. 已知限制 / 延后项

- 不可解析的 general 行无法单独授权（白名单非空时被过滤）。
- subscription 来源未拉取缓存时 picker 为空，需先「更新订阅」。
- 同连接多 name 折叠为一条授权单位。
- 不清理上游变更后失效的白名单 key。
