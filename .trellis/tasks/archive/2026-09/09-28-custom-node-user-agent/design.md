# 设计：自定义节点上游 User-Agent

## 架构与边界

- 新增能力集中在“上游订阅抓取”这一条链路：`custom_nodes.user_agent` → `FetchSubscription` 的请求头。
- 两条抓取路径共用同一 UA：渲染懒拉取（`fetchCustomNodeContent`）与手动刷新（`handleCustomNodeRefresh`）。
- 仅 `subscription` 类型有 UA；`links` 强制空。
- 改动面：migration + repo + subscription 包 + web DTO/handler + 前端表单/类型。无 Agent/渲染输出结构变更。

## 数据模型

`internal/db/migrations/0008_custom_nodes_user_agent.sql`：

```sql
ALTER TABLE custom_nodes ADD COLUMN user_agent TEXT NOT NULL DEFAULT '';
```

## subscription 包

```go
// DefaultUserAgent is sent when a custom node leaves its User-Agent unset.
const DefaultUserAgent = "clash-verge/v2.0.0"

// FetchSubscription downloads an upstream subscription document with the given
// User-Agent; an empty userAgent falls back to DefaultUserAgent.
func FetchSubscription(ctx context.Context, rawURL, userAgent string) (string, error)
```

- `fetch.go:57` 的 `req.Header.Set("User-Agent", ...)` 改为使用入参/默认值。
- 其余超时/大小/重定向/SSRF 行为不变。

## repo（`internal/repo/custom_nodes.go`）

- `CustomNode` 增加 `UserAgent string`；`NewCustomNode` 增加 `UserAgent string`。
- `customNodeSelect` 末尾追加 `user_agent`；`ListActiveCustomNodesByUser` 的显式列表追加 `c.user_agent`；`scanCustomNode` 增加一列扫描。
- `CreateCustomNode` INSERT 增加 `user_agent`。
- `UpdateCustomNode` 增加 `userAgent string` 与 `invalidateCache bool` 参数：
  - 始终更新 `name`/`user_agent`/`status`。
  - `contentEnc != nil` 时更新 `content_enc`；`contentEnc != nil || invalidateCache` 时清空 `cached_content`/`fetched_at`。
- 兼容：default '' 的旧行天然回退默认 UA。

## web（`internal/web/custom_nodes.go`）

- `customNodeDTO` 增加 `UserAgent string \`json:"user_agent"\``，`toCustomNodeDTO` 映射。
- `createCustomNodeRequest` / `customNodeRequest` 增加 `UserAgent *string \`json:"user_agent"\``。
- 新增校验：
  ```go
  func validateUserAgent(ua string) error {
      if len(ua) > 255 { return errValidation("user_agent must be at most 255 characters") }
      for _, r := range ua {
          if r < 0x20 || r == 0x7f { return errValidation("user_agent must not contain control characters") }
      }
      return nil
  }
  ```
- create：仅当 `source_type == subscription` 时读取/校验 `user_agent`（links 强制空）。
- update：`existing.SourceType == subscription` 时应用 `user_agent`（提供即覆盖，空串合法 = 默认），links 保持空；`invalidateCache = contentEnc != nil || (req.UserAgent != nil && *req.UserAgent != existing.UserAgent)`。
- `fetchCustomNodeContent`（`subscriptions.go`）：`FetchSubscription(ctx, url, cn.UserAgent)`。
- `handleCustomNodeRefresh`：`FetchSubscription(ctx, url, cn.UserAgent)`。

## 前端

- `web/src/api/types.ts`：`CustomNode` 增 `user_agent: string`；`CreateCustomNodeInput`/`UpdateCustomNodeInput` 增 `user_agent?: string`。
- 预设常量：`web/src/utils/customNodeUserAgents.ts`（或放入 `labels.ts` 风格的单点），导出 `USER_AGENT_PRESETS`（含 value/label）与默认值 `clash-verge/v2.0.0`。
- `CustomNodeFormDialog.vue`：
  - 仅 `sourceType === 'subscription'` 显示 UA 字段。
  - 用 `AppSelect` 选择预设 + “自定义…”；选自定义时显示文本输入。
  - 编辑时按 `node.user_agent` 预填：命中预设 → 选中该项；空 → 默认预设；其它 → 自定义并回填输入。
  - 创建 `subscription` 默认选中 `clash-verge/v2.0.0`。
  - 文案说明“留空使用默认 Clash UA”。
- `CustomNodesPage.vue`：无需改动（UA 不在列表展示；如需可选展示，本期不做）。

## 权衡与决策

- **默认 UA = `clash-verge/v2.0.0`**（用户已决定），删除 `vps-node-panel` 预设。
- **留空 = 用默认**（用户已决定）：UA 非密钥可回显，表单预填，清空即回退。
- **UA 变更清空缓存**：避免旧 UA 的缓存继续服务最长 5 分钟；与“替换内容清缓存”一致。
- **links 不使用 UA**：links 无网络请求。

## 兼容与回滚

- 新增列有默认值，旧行读取为 `''`，行为等同默认 UA；无数据迁移。
- 回滚：删除 migration 需重建表（SQLite 不支持 DROP COLUMN 老版本），实际回滚以代码回退为主；UA 列可保留不用。

## 风险

- Header 注入：校验控制字符为硬性要求。
- 预设字符串会随上游策略变化；预设只是快捷填充，最终以存储值为准。
- 请求 UA 变化后缓存已在保存时清空，下一次渲染或手动刷新即用新 UA 拉取；无需额外操作。
