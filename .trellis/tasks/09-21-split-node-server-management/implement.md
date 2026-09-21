# Implement — 节点与服务器管理界面拆分

## Checklist

### 1. 后端：节点列表过滤 + 服务器名

- [ ] `internal/repo/nodes.go`：`Node` 增加 `ServerName string`；`ListNodesPage` 改为接收过滤结构体（serverID/protocol/status/q + 分页），动态参数化 WHERE，JOIN servers
- [ ] `internal/repo/nodes.go` / `repo_test.go`：补充 protocol/status/q/server 名带出 的测试用例
- [ ] `internal/web/dto.go`：`nodeDTO` 增加 `Server nodeRefDTO`，`toNodeDTO` 填充
- [ ] `internal/web/nodes.go`：`handleNodeList` 解析 `protocol`/`status`/`q`，非法枚举值返回 400
- [ ] `docs/api-contract.md`：更新 `GET /api/nodes` 查询参数与响应示例
- [ ] 验证：`go test ./... && go vet ./...`

### 2. 前端：类型与 API

- [ ] `web/src/api/types.ts`：`NodeBrief` 增加 `server: NodeRef`
- [ ] `web/src/api/nodes.ts`：`listNodes` 参数扩展 `{ serverId?, protocol?, status?, q?, page?, pageSize? }`

### 3. 前端：NodesPage

- [ ] 新建 `web/src/pages/NodesPage.vue`：工具栏筛选 + DataTable + 分页 + 行操作 + 批量启停
- [ ] 筛选状态与 `route.query` 双向同步（含 `server_id` 深链还原）
- [ ] `web/src/router/index.ts`：注册 `/nodes`
- [ ] `web/src/components/AppLayout.vue`：导航增加「节点」

### 4. 前端：组件与详情页联动

- [ ] `web/src/components/NodeFormDialog.vue`：`serverId` 可选，创建时缺省渲染服务器下拉
- [ ] `web/src/pages/ServerDetailPage.vue`：节点区块工具栏加「在节点页查看」链接
- [ ] 验证：`npm run lint`、`npm run type-check`（或 build，以 package.json 实际脚本为准）

### 5. 自查

- [ ] 手动过一遍：创建（选服务器）→ 编辑 → 批量禁用/启用 → 删除；服务器详情页节点区块无回退
- [ ] 确认改动服务器后 Agent 配置同步正常（revision 机制未动，抽查一台即可）

## Review Gates

- 第 1 步完成后：跑后端测试，确认旧前端调用 `listNodes` 无类型错误（类型改完即编译验证）
- 全部完成后：对照 `prd.md` 验收标准逐条核对，再进入 `trellis-check`

## Rollback

- 纯增量改动，回滚 = revert 本任务全部文件；DTO 新字段删除对旧数据无影响（无迁移）。
