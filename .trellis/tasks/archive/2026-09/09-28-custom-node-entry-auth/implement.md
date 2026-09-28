# Implement: 自定义节点按单条线路授权

依赖顺序：后端 key/存储 → 后端 API/渲染 → 文档 → 前端类型/组件 → 校验。每步保持 `go test ./...` 通过。

## 1. Entry Key（`internal/subscription`）
- [ ] 新增 `entrykey.go`：`EntryKey`（HMAC-SHA256(appKey) over canonical JSON，删 `name`）、`LinkEntryKey`、`EntrySummary`、`SummarizeEntries`。
- [ ] 新增 `entrykey_test.go`：name 无关稳定性、连接不同则不同、appKey 影响、不可解析链接 ok=false。
- 验证：`go test ./internal/subscription/...`

## 2. 迁移 + Repo（`internal/db`, `internal/repo`）
- [ ] 新增 `migrations/0010_user_custom_node_entries.sql`（表 + index + 级联 FK）。
- [ ] 新增 `migration_user_custom_node_entries_test.go`（建表、升级幂等、FK 级联）。
- [ ] `internal/repo/custom_nodes.go`：`ListCustomNodeEntryKeysByUser`、`SetUserCustomNodesAndEntries`（单事务整集替换）。
- [ ] 扩展 `custom_nodes_test.go`：整集替换、去重、空白名单省略、级联删除。
- 验证：`go test ./internal/db/... ./internal/repo/...`

## 3. 后端 API + 渲染（`internal/web`）
- [ ] `custom_nodes.go`：`customNodeEntryDTO` 增 `key`；`customNodeEntries` 改用 `SummarizeEntries`。
- [ ] `custom_nodes.go`：GET/PUT 用户自定义节点增 `custom_node_entries`；PUT 校验（来源子集、hex64、去重）；handler 改调 `SetUserCustomNodesAndEntries`。
- [ ] `subscriptions.go`：`customSourcesForUser` 读取白名单并过滤 links/proxies。
- [ ] 扩展 `custom_nodes_test.go`：entries 带 key；PUT 校验矩阵；GET 回显；general/clash 白名单过滤；空白名单=全部回归。
- 验证：`go test ./internal/web/... ./internal/subscription/...`

## 4. 契约文档（`docs/api-contract.md`）
- [ ] 更新 custom node entries 字段、用户自定义节点 GET/PUT 请求/响应、白名单语义、已知限制。
- 验证：人工通读与实现一致。

## 5. 前端类型 + API client（`web/src/api`）
- [ ] `types.ts`：`CustomNodeEntry.key`、`CustomNodeEntrySelection`、`UserCustomNodes.custom_node_entries`（Hand-mirror contract）。
- [ ] `customNodes.ts`：`putUserCustomNodes(userId, customNodeIds, entries)`。
- 验证：`cd web && npm run typecheck`

## 6. 前端组件
- [ ] 新增 `components/user/CustomNodeEntryPickerDialog.vue`（ModalDialog + 状态三件套 + SegmentedControl 授权范围；按 key 去重；仅所选需 ≥1）。
- [ ] `components/user/UserNodeAuth.vue`：白名单状态、来源行「选择线路」按钮 + 摘要 chip、保存一并提交。
- [ ] `pages/UserDetailPage.vue`：接线 `custom_node_entries`。
- 验证：`cd web && npm run typecheck && npm run lint && npm run build`

## 7. 全量校验
- [ ] `go vet ./...`
- [ ] `go test ./...`
- [ ] `cd web && npm run typecheck && npm run lint && npm run build`

## 风险 / 回滚点
- 白名单为空必须与现状字节一致 → 重点回归 `internal/subscription/custom_test.go` 与 `internal/web/custom_nodes_test.go`。
- 迁移为纯新增表，回退代码即可；无 revision bump。
- 前端保存是来源+白名单的组合请求，注意 `Promise.all` 的原子性（沿用现有模式）。
