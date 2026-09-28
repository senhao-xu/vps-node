# 自定义节点：用户详情按单条线路授权

## Goal

在「用户详情」页的自定义节点区块，管理员在整源授权之外，还能授权某个自定义节点来源内的**单条线路（entry）**。某用户的订阅只输出被授权的线路，未授权线路对该用户不可见。

## Background / Confirmed Facts（代码证据）

- 自定义节点是管理员维护的外部来源（`links` / `subscription`），只并入用户订阅输出，不参与 Agent 配置/流量/设备统计。
- 授权模型为逐用户整源授权：`user_custom_nodes(user_id, custom_node_id)`（`internal/db/migrations/0006_custom_nodes.sql:13-18`）；读路径 `repo.ListActiveCustomNodesByUser`（`internal/repo/custom_nodes.go:98-110`）。
- 订阅渲染入口 `customSourcesForUser`（`internal/web/subscriptions.go:248-287`）把已授权来源解析为 `subscription.CustomSource{ID,Name,Links,Proxies}`；`RenderGeneralLinksMerged`（`render.go:223`）、`RenderClashFilteredMerged`（`render.go:253`）负责合并。
- 来源内部条目**不持久化**，渲染时动态解析：`subscription.Summarize`（`summary.go:16-41`）；条目展示 API `GET /api/custom-nodes/{id}/nodes`（`internal/web/custom_nodes.go:124-141`）只读缓存、不联网，DTO 无稳定标识。
- 用户授权 API：`GET/PUT /api/users/{id}/custom-nodes`（`internal/web/custom_nodes.go:431-491`），整集替换、无 revision bump。
- 用户详情 UI：`UserDetailPage.vue` → `UserNodeAuth.vue:120-144`，每来源一个复选框，无条目级交互。

## Requirements

- R1（模型 A）：保留来源整源开关作为可见性门。开启后可再选具体线路作为**白名单**；白名单为空 = 整源全部线路（兼容既有授权）；白名单非空时只输出命中的线路。
- R2：为每个可解析条目提供**稳定标识（entry key）**：`HMAC-SHA256(appKey, canonicalJSON(proxy 去掉 name))`，与显示名解耦；不可解析的 general 行无 key。
- R3：新增 `user_custom_node_entries(user_id, custom_node_id, entry_key)` 持久化，级联删除；既有 `user_custom_nodes` 数据无需迁移（无白名单 = 全部）。
- R4：订阅渲染时按白名单过滤来源内条目，general 与 clash-meta 两种格式一致；白名单为空时输出与现状逐字节一致。
- R5：用户详情自定义节点区块：来源行提供「选择线路」入口，弹窗逐条勾选（按 key 去重），支持「全部线路 / 仅所选线路」，保存与整源授权一并生效。
- R6：未授权条目对用户不可见；来源停用/移除时其白名单一并失效。

## Acceptance Criteria

- [ ] 管理员对某用户只勾选来源内线路 A、C 后，其 general 订阅只含 A、C 的分享链接；clash-meta 只含 A、C 的 proxy 条目。
- [ ] 来源开启且白名单为空时，该来源全部线路输出，行为与改动前一致。
- [ ] 上游仅重命名线路（连接参数不变）时，已授权线路对新名称仍然可见（key 与 name 无关）。
- [ ] 来源未授权/关闭时，其任何线路都不出现在订阅；重新授权后按白名单恢复。
- [ ] `PUT /api/users/{id}/custom-nodes` 对非法输入返回 `422 validation`：`custom_node_id` 不在 `custom_node_ids` 内、`entry_key` 非 64 位十六进制。
- [ ] `GET /api/users/{id}/custom-nodes` 回显非空白名单；`GET /api/custom-nodes/{id}/nodes` 每条 entry 带 `key`。
- [ ] 既有整源授权数据（无白名单）在升级后仍输出全部线路；删除用户/自定义节点级联清理白名单。
- [ ] `go vet ./...`、`go test ./...`、`cd web && npm run typecheck && npm run lint && npm run build` 全部通过。

## Key Decisions

- 授权粒度模型 A：来源开关 + 可选白名单，空白名单 = 全部（用户选定）。
- 交互：来源行 + 「选择线路」弹窗（用户选定）；弹窗顶部「全部线路 / 仅所选线路」范围开关避免空白名单歧义。
- entry key 排除 `name` 且以 appKey 做 HMAC（稳定性 + 避免凭据裸 hash 落库）。
- 过滤在 `customSourcesForUser` 构建 source 时完成，render 包保持纯函数与两格式一致。

## Out of Scope

- 终端用户自行选择线路
- 条目级流量/设备统计
- 套餐/分组授权
- 清理上游变更后失效的白名单 key

## Known Limitations

- 无法解析的 general 行不可单独授权，白名单非空时被过滤。
- subscription 来源未拉取缓存时选择弹窗为空，需先「更新订阅」。
- 连接参数相同、仅 name 不同的重复条目折叠为同一授权单位。
