# Implement - 仪表盘用户流量明细

## Checklist

1. 读取 spec：`.trellis/spec/backend/database-guidelines.md`、`.trellis/spec/backend/error-handling.md`、`.trellis/spec/frontend/directory-structure.md`、`.trellis/spec/frontend/type-safety.md`。
2. repo 层：`internal/repo/stats.go` 新增 `SumTrafficByUser` / `SumTrafficByUserNode`（含 nodes/servers join 取名称）；`internal/repo/users.go` 确认或新增全量轻量用户列表方法。
3. web 层：`internal/web/dto.go` 新 DTO；`internal/web/dashboard.go` 新 handler；`internal/web/web.go` 注册路由。
4. 后端测试：新增 dashboard user-traffic handler 测试（401/400/today/total/排序/无流量用户）。
5. 前端：`types.ts`、`api/dashboard.ts`、`DashboardPage.vue`（范围切换 + 可展开表格 + 30s 刷新联动）。
6. 运行验证命令（见下），全部通过后进入 check。

## Validation

- `go build ./... && go test ./...`
- `cd web && npm run typecheck && npm run lint && npm run build`

## Review Gates

- range 参数非法值返回 400；未认证 401。
- total 模式用户合计 = users.used_bytes；today 模式 = 当日 traffic_records。
- 前端无硬编码颜色，沿用 tokens；不修改 DataTable 组件。

## Rollback

- 单 commit；回滚即 revert 该 commit，无数据库迁移、无配置变更。
