# Implement: 节点维度批量授权用户

执行顺序与验证命令。每步完成后勾选。

## 后端

- [ ] 1. Repo：`internal/repo/mutations.go` 新增 `SetNodeUsersAndBump(ctx, nodeID int64, userIDs []int64) ([]int64, error)`
  - 事务内 diff 更新 `user_nodes`（删除 removed、插入 added），有变更才 `bumpRevisionExec(node.server_id)`
  - 复用 `dedupeInt64` / `nodeServerIDExec` 等现有 helper；错误统一 `mapErr`
- [ ] 2. Handler：新建 `internal/web/node_users.go`
  - `handleNodeUsersGet`：node 存在校验 → `ListUserIDsByNode` + `ListUsersAll` + `CountNodesByUserIDs` → `nodeUsersResponse`
  - `handleNodeUsersPut`：decode + `user_ids` nil 校验 → 逐个 `GetUser` 校验 → `SetNodeUsersAndBump` → 返回同 GET 结构
- [ ] 3. 路由：`internal/web/web.go` 注册 `GET/PUT /api/nodes/{id}/users`（`requireAdmin`）
- [ ] 4. 后端测试：`internal/web/node_users_test.go`
  - GET 空态 / PUT 覆盖 / 撤销后订阅不含 / 400 缺字段 / 400 用户不存在 / 404 节点
  - 验证：`go test ./internal/web/ -run NodeUsers -v`

## 前端

- [ ] 5. 类型 + API：`web/src/api/types.ts` 加 `NodeUsersResult`；`web/src/api/nodes.ts` 加 `getNodeUsers` / `putNodeUsers`
- [ ] 6. 组件：`web/src/components/NodeUserAuthDialog.vue`
  - ModalDialog 骨架，open 时拉取数据；全选(indeterminate) + 行 checkbox + dirty + 保存
  - 样式对齐 UserNodeAuth.vue / CustomNodeEntryPickerDialog.vue
- [ ] 7. 入口：`web/src/pages/NodesPage.vue` 行操作加 `Users` 图标按钮 + 挂载弹窗

## 验证

- [ ] 8. 全量检查：
  - `go vet ./... && go test ./...`
  - `cd web && npm run typecheck && npm run lint && npm test && npm run build`

## 回滚点

- 全部为增量文件/路由/按钮，单 commit revert 即可；无迁移、无数据变更。
