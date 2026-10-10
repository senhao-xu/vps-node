# Design: 节点维度批量授权用户

## 边界

- 数据模型不变：仍写 `user_nodes`（user_id, node_id）。节点维度只是同一数据的另一个管理视图，不新增表/迁移。
- 后端新增 1 个 repo 方法 + 1 组 handler + 2 条路由；前端新增 1 个弹窗组件 + 2 个 API client 函数 + 节点列表行操作入口。

## 后端

### Repo: `SetNodeUsersAndBump` (`internal/repo/mutations.go`)

节点维度 diff 更新（**不能**循环复用 `SetUserNodesAndBump`，那是用户维度全量覆盖，会清掉用户在其他节点的授权）：

```
Tx:
  1. 取 node（校验存在）→ server_id（复用现有 node 查询 / nodeServerIDExec 模式）
  2. current = SELECT user_id FROM user_nodes WHERE node_id = ?
  3. added = req - current; removed = current - req
  4. DELETE FROM user_nodes WHERE node_id=? AND user_id IN removed
  5. INSERT added 批量
  6. 若 added∪removed 非空：bumpRevisionExec(server_id)
返回最终 user_ids（dedupe）
```

### Handlers: `internal/web/node_users.go`（新文件）

- `GET /api/nodes/{id}/users`：
  - 校验 node 存在（`h.repo.GetNode`）
  - `ListUserIDsByNode(nodeID)` → authorized set
  - `ListUsersAll()` → 全量用户；`CountNodesByUserIDs(ids)` 批量取 node_count（避免 N+1，模式同 `handleUserList` users.go:116-129）
  - 响应 `nodeUsersResponse{ UserIDs []int64; Users []userDTO }`，users 全量返回，前端用 `user_ids` 标记勾选
- `PUT /api/nodes/{id}/users`：
  - body `{user_ids: []}`；`nil` → 400 `errInvalid`（对齐 user_nodes.go:50-53）
  - 校验所有 user_id 存在（复用 `h.repo.GetUser` 循环或轻量批量；量级小可接受）
  - `SetNodeUsersAndBump` → 返回与 GET 相同结构

### 路由 (`internal/web/web.go`)

```go
mux.HandleFunc("GET /api/nodes/{id}/users", h.requireAdmin(h.handleNodeUsersGet))
mux.HandleFunc("PUT /api/nodes/{id}/users", h.requireAdmin(h.handleNodeUsersPut))
```

### 语义决策

- **disabled 节点可编辑授权**（与用户侧「禁用节点不可勾选」不对称，有意为之）：预配置场景合法，下发时 `subscriptions.go`/`agent_config.go` 本就过滤非 active。
- **disabled/expired 用户可被授权**：写入无副作用，下发时 `ListEligibleUsersByServer` 已过滤；用户重新启用后自动生效。
- 链式节点：只 bump 本节点 `server_id` 的 revision。授权变更影响的是本 server inbound 的 users 列表，出口 server 配置不变。

## 前端

### API (`web/src/api/nodes.ts` + `web/src/api/types.ts`)

```ts
export type NodeUsersResult = { user_ids: number[]; users: User[] }
export function getNodeUsers(nodeId: number): Promise<NodeUsersResult>
export function putNodeUsers(nodeId: number, userIds: number[]): Promise<NodeUsersResult>
```

### 组件 `web/src/components/NodeUserAuthDialog.vue`

仿 `CustomNodeEntryPickerDialog` 的弹窗骨架（ModalDialog + LoadingSpinner + EmptyState + ErrorBanner）：

- props: `open: boolean; node: NodeBrief | null`；emit: `close`、`saved(nodeIds 里不再需要→ saved(userId count))`——实际 emit `(e: 'saved')` 供父级刷新即可
- `watch(open)` 时拉取 `getNodeUsers`，`selected = [...user_ids]`
- 列表行：checkbox + username + StatusBadge + expires_at + node_count chip
- 头部：全选 checkbox（indeterminate 状态）、`已选 x / y`、dirty 检测、保存按钮（对齐 UserNodeAuth.vue 的 saved-tip 模式）
- `sameIds` 排序比较 dirty（复用 UserNodeAuth.vue:77-79 模式）

### 入口 `web/src/pages/NodesPage.vue`

- 行操作区（现有 分享/复制/编辑/删除 四个 icon-action）新增「用户」按钮（lucide `Users` 图标），`@click="authTarget = row"`
- 挂载 `<NodeUserAuthDialog :open="!!authTarget" :node="authTarget" @close="authTarget = null" />`

## 测试

- 后端：`internal/web/node_users_test.go`——GET 空态/有数据、PUT 全量覆盖、PUT 后撤销生效（订阅不含该节点）、参数校验 400、节点 404。
- 前端：现有 `npm test` 范围不含组件测试，靠 typecheck/lint/build 兜底。

## 回滚

纯增量改动（新文件 + 新路由 + 行内新增按钮），revert 即可完全回滚，无数据迁移。
