# 自定义节点：上游订阅自定义 User-Agent

## Goal

部分上游订阅会按请求的 `User-Agent` 返回不同内容（例如只对 Clash/Mihomo 客户端返回完整节点）。当前 `subscription.FetchSubscription` 对所有自定义节点硬编码 `User-Agent: vps-node-panel`，管理员无法指定。本任务让管理员为 `subscription` 类型自定义节点持久化一个 User-Agent，并用于：

1. 渲染用户订阅时的上游懒拉取（`fetchCustomNodeContent`）。
2. 手动“更新订阅”（`POST /api/custom-nodes/{id}/refresh`）。

提供一组常用 UA 预设（默认 Clash），并允许自定义输入。

## Background / Confirmed Facts（代码证据）

- 抓取实现：`internal/subscription/fetch.go:42` `FetchSubscription(ctx, rawURL)` 第 57 行硬编码 `req.Header.Set("User-Agent", "vps-node-panel")`。
- 两个调用点：
  - 渲染懒拉取 `internal/web/subscriptions.go:296` `fetchCustomNodeContent`（带 5 分钟 `CacheTTL`，失败降级旧缓存）。
  - 手动刷新 `internal/web/custom_nodes.go:162` `handleCustomNodeRefresh`（上一任务 `09-28-custom-node-refresh` 新增）。
- 数据模型：`custom_nodes`（migration `0006_custom_nodes.sql`）无 UA 字段；repo `internal/repo/custom_nodes.go` 的 `CustomNode`/`NewCustomNode`/`customNodeSelect`/`ListActiveCustomNodesByUser`（显式列）/`scanCustomNode`/`UpdateCustomNode` 均为显式列。
- DTO：`internal/web/custom_nodes.go:15` `customNodeDTO`（list/create/update 共用）。
- 用户订阅请求的 UA 仅用于选择输出格式（`subscriptions.go:188`），与上游抓取 UA 无关。
- 迁移约定：`internal/db/migrations/NNNN_name.sql` 顺序执行、幂等；`ALTER TABLE ... ADD COLUMN` 是既有做法（见 `0007_node_chain.sql`）。
- links 类型不发网络请求，UA 无意义。

## Requirements

- R1：`custom_nodes` 新增列 `user_agent TEXT NOT NULL DEFAULT ''`（migration `0008_custom_nodes_user_agent.sql`）。空字符串表示使用默认 UA。
- R2：`subscription.FetchSubscription` 增加 UA 入参（`FetchSubscription(ctx, rawURL, userAgent string)`）；`userAgent` 为空时回退默认常量 `DefaultUserAgent = "clash-verge/v2.0.0"`。两个调用点都传入所属自定义节点的 UA。
- R3：创建/编辑自定义节点时，`subscription` 类型可设置 UA：常用预设下拉 + 自定义输入；`links` 类型不显示该字段（不存储、不使用，强制空）。
- R4：UA 校验：长度上限 255；禁止 `\r`/`\n` 及 ASCII 控制字符（防 header 注入）；不合法返回 `422 validation`。
- R5：`customNodeDTO` 增加 `user_agent`，list/`GET`/create/update 响应都返回；repo 读写全链路补齐（`customNodeSelect`、`ListActiveCustomNodesByUser`、`scanCustomNode`、`NewCustomNode`、`UpdateCustomNode`）。
- R6：默认预设（去掉 `vps-node-panel`；默认 = `clash-verge/v2.0.0`），均可自定义：
  - `clash-verge/v2.0.0`（默认）
  - `Mihomo/1.18.0`
  - `sing-box/1.10.0`
  - `v2rayN/6.60`
  - `Shadowrocket/2.2.30`
- R7：编辑表单预填当前 UA（UA 非密钥，可回显）；留空 = 使用默认 UA（存空串）。
- R8：不改变 links 行为、订阅输出结构、自定义节点授权语义与 Agent 配置；仅 `custom_nodes` 加一列。

## Acceptance Criteria

- [x] 新建/编辑 `subscription` 自定义节点可设置 UA；列表与 create/update 响应包含 `user_agent`。
- [x] 手动“更新订阅”时上游收到的 `User-Agent` 等于节点配置值；留空/未配置时发送 `clash-verge/v2.0.0`。
- [x] 渲染用户订阅触发的上游懒拉取同样携带该节点配置的 UA。
- [x] `links` 类型表单不显示 UA 字段；其请求行为不变。
- [x] UA 含 `\r`/`\n`/控制字符或超过 255 字符 → `422 validation`。
- [x] 修改 UA 会清空该节点的上游缓存（下次渲染/刷新用新 UA 拉取），与“替换内容清缓存”一致。
- [x] migration 幂等；已存在的自定义节点 `user_agent` 为空且行为回退默认。
- [x] 既有自定义节点/订阅测试不回归；`make build` 通过。

## Out of Scope

- 全局或按用户的 UA 覆盖。
- 每次刷新时临时指定 UA（以节点持久化值为准）。
- `links` 类型的 UA。
- 按 UA 对上游内容做特殊解析。

## Technical Notes

- **Header 注入**：UA 进入 HTTP 头，写入前必须拒绝 `\r`/`\n`/控制字符；长度上限 255。
- **默认常量单一来源**：`DefaultUserAgent` 放 `internal/subscription`，web 层与测试引用，避免多处硬编码。
- **缓存失效**：UA 变更视为内容变更（清空 `cached_content`/`fetched_at`），需要 repo 更新方法感知 UA 是否变化。
- **迁移**：`ALTER TABLE custom_nodes ADD COLUMN user_agent TEXT NOT NULL DEFAULT '';`；幂等由迁移 runner 的 `schema_migrations` 保证。
