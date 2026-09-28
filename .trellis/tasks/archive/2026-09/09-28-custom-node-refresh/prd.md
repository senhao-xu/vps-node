# 自定义节点：查看节点与更新订阅

## Goal

自定义节点（`links` 分享链接 / `subscription` 上游订阅）目前只展示“是否已缓存”，管理员看不到其中实际包含哪些节点，也无法手动刷新上游订阅。本任务让管理员能在管理端：

1. 查看某个自定义节点解析出的节点摘要列表（名称/协议/服务器/端口）。
2. 对 `subscription` 类型执行“更新订阅”——强制拉取上游、刷新缓存与抓取时间。

## Background / Confirmed Facts（代码证据）

- 自定义节点数据模型：`internal/repo/custom_nodes.go:22` `CustomNode`（含 `ContentEnc` 加密存储、`CachedContent`+`FetchedAt` 上游缓存）。
- 上游抓取与缓存：`internal/web/subscriptions.go:289` `fetchCustomNodeContent`——渲染订阅时才调用，命中 `subscription.CacheTTL`（5 分钟，`internal/subscription/fetch.go:23`）则用缓存，失败降级旧缓存。
- 内容解析：
  - `subscription.SplitLinkLines`（`fetch.go:141`）拆分 links 文本。
  - `subscription.NormalizeFetchedContent`（`fetch.go:80`）把上游内容分类为 base64/纯文本分享链接或 Clash `proxies`。
  - `subscription.ParseShareURI`（`import.go:19`）把单条分享链接转为 Clash proxy map（含 `name`/`type`/`server`/`port`）。
- 现有自定义节点接口：`internal/web/custom_nodes.go`（list/create/update/delete），路由注册在 `internal/web/web.go:176-181`；无“查看/刷新”接口。
- 管理端页面：`web/src/pages/CustomNodesPage.vue`（列表 + 行操作：编辑/启停/删除），`web/src/components/CustomNodeFormDialog.vue`（新建/编辑）；API 封装 `web/src/api/customNodes.ts`。
- `fetched_at` 在 DTO 中为 `*string`，`has_cache` 标识是否有缓存（`custom_nodes.go:15`）。
- 后端错误契约固定（`docs/api-contract.md`、`.trellis/spec/backend/error-handling.md`），无 502 状态码。

## Requirements

- R1：新增只读接口 `GET /api/custom-nodes/{id}/nodes`，返回自定义节点解析出的节点摘要列表。每个条目包含：名称、协议/类型、服务器地址、端口。`links` 与 `subscription` 两种类型都支持查看。
- R2：`links` 类型查看时解析其加密存储的链接文本（`ParseShareURI`），不产生网络请求。
- R3：`subscription` 类型查看时解析已缓存内容（`CachedContent` → `NormalizeFetchedContent`）；**无缓存时不隐式发起网络请求**，返回空列表并以 `has_cache=false` 明确标识“未拉取”。
- R4：新增 `POST /api/custom-nodes/{id}/refresh`，仅 `subscription` 类型支持：忽略 5 分钟 TTL，强制拉取上游内容，成功后写入缓存与 `fetched_at` 并返回解析后的节点摘要列表（与 R1 同构）。
- R5：上游拉取失败时，`refresh` 返回明确错误，**保留旧缓存与 `fetched_at`**（与现有渲染降级一致）。
- R6：解析失败/被跳过的条目返回在 `skipped` 列表中，便于管理员定位坏节点；不影响其余节点展示。
- R7：管理端 UI：自定义节点行操作增加“查看节点”（两种类型都有）；`subscription` 类型额外显示“更新订阅”。查看节点用弹窗展示 R1 列表；更新订阅完成后刷新列表与弹窗内容。
- R8：不改变现有订阅渲染行为与自定义节点授权语义；不新增对托管节点/Agent 的影响；无 DB migration。

## Acceptance Criteria

- [x] 点击某自定义节点的“查看节点”，弹窗列出解析出的节点（名称/协议/服务器/端口）；`links` 类型同样可用。
- [x] `subscription` 节点无缓存时“查看节点”显示“未拉取，请先更新订阅”，且不触发上游请求。
- [x] 点击“更新订阅”后强制拉取上游、更新 `fetched_at` 与缓存，并展示解析后的最新节点列表。
- [x] 上游不可达/非 2xx 时“更新订阅”返回可读错误，旧缓存与 `fetched_at` 不被清空。
- [x] `links` 类型不显示“更新订阅”入口；对 links 调 refresh 返回 422 validation。
- [x] 解析失败的条目被计入 `skipped`，不影响其余节点展示。
- [x] `GET /api/custom-nodes/{id}/nodes`、`POST /api/custom-nodes/{id}/refresh` 均走 `requireAdmin`，未知 id 返回 404。
- [x] 既有订阅渲染（general / clash-meta）与自定义节点 CRUD 测试不回归；`make build` 通过。

## Out of Scope

- 终端用户自助查看/刷新自定义节点。
- 自定义节点的流量统计、在线设备。
- 定时自动刷新订阅（本任务仅手动触发）。
- 解析内容编辑（仍走现有编辑对话框的 URL/链接文本替换）。
- 将解析出的节点转为面板托管节点。

## Technical Notes

- 解析结果中协议字段沿用 Clash proxy `type`（`ss`/`vless`/`hysteria2`/`anytls`/`trojan`/`vmess`），前端仅展示。
- 端口在分享链接中为 `int`，在 Clash YAML 中可能为 `int`/`float64`/`string`，需统一转换（复用 `looseInt`）。
- `refresh` 失败返回 `500 internal` + 可读 message（契约无 502；admin-only 接口）。查看（R3）永不产生网络副作用。
