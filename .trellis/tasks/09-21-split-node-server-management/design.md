# Design — 节点与服务器管理界面拆分

## 改动边界

| 层 | 文件 | 改动 |
|---|---|---|
| 后端 repo | `internal/repo/nodes.go` | `ListNodesPage` 增加 protocol/status/q 过滤；JOIN servers 带出服务器名 |
| 后端 web | `internal/web/nodes.go`、`internal/web/dto.go` | 解析新查询参数；`nodeDTO` 增加 `server` 引用字段 |
| 文档 | `docs/api-contract.md` | 同步 `GET /api/nodes` 契约 |
| 前端 api | `web/src/api/types.ts`、`web/src/api/nodes.ts` | `NodeBrief` 增加 `server: NodeRef`；`listNodes` 增加过滤参数 |
| 前端页面 | `web/src/pages/NodesPage.vue`（新增） | 跨服务器节点管理页 |
| 前端路由/导航 | `web/src/router/index.ts`、`web/src/components/AppLayout.vue` | 新增 `/nodes` 路由与「节点」导航项 |
| 前端组件 | `web/src/components/NodeFormDialog.vue` | `serverId` 变为可选；缺省时渲染服务器下拉框 |
| 前端页面 | `web/src/pages/ServerDetailPage.vue` | 节点区块工具栏增加「在节点页查看」链接 |

## 后端设计

### 1. `GET /api/nodes` 扩展

查询参数（全部可选，可组合）：

- `server_id`（已有）：精确过滤
- `protocol`：`shadowsocks|vless|hysteria2`，非法值返回 400
- `status`：`active|disabled`，非法值返回 400
- `q`：节点名称模糊匹配（`LIKE '%' || ? || '%'`，`%`/`_` 转义）
- `page` / `page_size`（已有）

`ListNodesPage` 改为动态 WHERE 拼接（参数化占位符，不拼接用户输入），JOIN `servers` 取 `s.name AS server_name`。`repo.Node` 增加 `ServerName string` 字段（仅列表查询填充）。

DTO：`nodeDTO` 增加 `Server nodeRefDTO \`json:"server"\``（`{id, name}`），`nodeDetailDTO` 原有 `server` 字段不受影响。该变更是**向后兼容的纯增量**：旧前端忽略新字段。

### 2. 不变的部分

- 创建/更新/删除节点仍走 `CreateNodeAndBump` / `UpdateNodeAndBump` / `DeleteNodeAndBump`（`internal/repo/mutations.go:271-361`），revision 同步机制零改动。
- 节点与服务器归属不可变更：更新请求不接受 `server_id`（沿用现状）。

## 前端设计

### 1. `NodesPage.vue`

布局参照 `ServersPage.vue` / `UsersPage.vue` 的既有模式：

- 顶部工具栏：名称搜索框（防抖 ~300ms）、服务器下拉（`GET /api/servers` 全量，附加「全部」项）、协议下拉、状态下拉、「新建节点」按钮。
- 表格（`DataTable.vue`）：名称 / 协议 / 端口 / 所属服务器（`RouterLink` → `/servers/:id`）/ 状态（`StatusBadge`）/ 创建时间 / 操作（编辑、启停、删除）。
- 行勾选（复选框列）+ 工具栏批量「启用」「禁用」按钮：逐个 `updateNode({status})`，`Promise.allSettled` 汇总，失败项在 `ErrorBanner` 提示。
- 分页用 `TablePaginator.vue`，所有筛选状态同步到 `route.query`（`server_id/protocol/status/q/page`），onMounted 从 query 还原——这同时支撑服务器详情页深链 `/nodes?server_id=<id>`。
- 任一筛选变化时重置 `page=1` 并重新请求。

### 2. `NodeFormDialog.vue` 改造

- props 由 `serverId: number` 改为 `serverId?: number`。
- 创建模式且未传 `serverId` 时，对话框内拉取服务器列表渲染必选下拉框，提交时以选中值作为 `server_id`。
- 编辑模式不展示服务器选择（归属不可变）。
- `ServerDetailPage.vue` 现有调用传了 `serverId`，行为完全不变。

### 3. 路由与导航

- `router/index.ts` 新增 `{ path: '/nodes', name: 'nodes', component: () => import('@/pages/NodesPage.vue') }`。
- `AppLayout.vue` `navItems` 在「服务器」后插入 `{ to: '/nodes', label: '节点', exact: false }`。

### 4. 类型

```ts
// types.ts
export type NodeBrief = {
  id: number
  server_id: number
  name: string
  protocol: Protocol
  port: number
  status: NodeStatus
  created_at: string
  server: NodeRef        // 新增
}
```

`NodeRef` 已存在（`types.ts:55`），`NodeDetail.server` 类型不变。

## 兼容性 / 风险

- DTO 纯增量、查询参数纯增量：旧前端与其他调用方（`NodeChecklist`、用户详情授权弹窗等使用 `listNodes` 的地方）无需改动，但要确认这些调用方在类型增加必填 `server` 字段后仍编译通过（后端一定返回该字段）。
- `ListNodesPage` 现有唯一调用方是 `handleNodeList`，签名变更影响面小；`repo/repo_test.go` 若有覆盖需同步更新并补过滤用例。
- 动态 WHERE 必须全部参数化，禁止字符串拼接用户输入。
