# Design - 仪表盘用户流量明细

## API

`GET /api/dashboard/user-traffic?range=today|total`（requireAdmin）

- `range` 缺省为 `today`；其他值返回 400 invalid。
- 响应：

```json
{
  "range": "today",
  "items": [
    {
      "user_id": 1,
      "username": "alice",
      "status": "active",
      "quota_bytes": 107374182400,
      "upload_bytes": 100,
      "download_bytes": 200,
      "total_bytes": 300,
      "nodes": [
        {
          "node_id": 3,
          "node_name": "hy2-443",
          "server_id": 1,
          "server_name": "HK01",
          "upload_bytes": 60,
          "download_bytes": 120,
          "total_bytes": 180
        }
      ]
    }
  ]
}
```

- `items` 包含全部用户（无流量的用户 upload/download/total 为 0、nodes 为空数组），按 `total_bytes` 降序、其次 user_id 升序。
- `quota_bytes` 始终返回，便于前端展示配额进度。

## 数据口径

| 范围 | 用户合计 | 节点明细 |
|------|----------|----------|
| today | traffic_records where created_at >= 今日 UTC 0 点，group by user_id | 同上 group by user_id, node_id |
| total | users.used_bytes（与用户列表口径一致，流量重置后同步归零） | traffic_records 全量留存 group by user_id, node_id |

已知差异：total 模式节点明细受 `retention.aggregate_days` 清理影响，且不受流量重置影响，故节点明细之和可能不等于用户合计；PRD 已声明为可接受口径。

今日起点复用 dashboard 现有口径：`time.Now().UTC().Truncate(24 * time.Hour)`。

## Backend

- `internal/repo/stats.go` 新增：
  - `type UserTrafficSum struct { UserID, UploadBytes, DownloadBytes int64 }`
  - `SumTrafficByUser(ctx, f TrafficFilter) ([]UserTrafficSum, error)` — `SELECT user_id, SUM(upload), SUM(download) FROM traffic_records WHERE ... GROUP BY user_id`，复用 `trafficWhere`。
  - `type UserNodeTrafficSum struct { UserID, NodeID int64; NodeName, ServerName string; ServerID int64; UploadBytes, DownloadBytes int64 }`
  - `SumTrafficByUserNode(ctx, f TrafficFilter) ([]UserNodeTrafficSum, error)` — traffic_records JOIN nodes ON node_id，nodes 含 server_name 字段则直接取；否则 JOIN servers。group by user_id, node_id。
- `internal/repo/users.go` 已有 `ListUsers`，但带分页/筛选；dashboard 需要全量轻量列表，新增 `ListUserTrafficBase(ctx) ([]User, error)`（仅查 id/username/status/quota_bytes/used_bytes，按 id 升序）或复用 `ListUsers` 传大 page size——优先新增轻量方法，避免分页上限耦合。
- `internal/web/dto.go` 新增 `dashboardUserTrafficDTO`、`dashboardUserNodeTrafficDTO`。
- `internal/web/dashboard.go` 新增 `handleDashboardUserTraffic`：解析 range → today 时构造 `TrafficFilter{From: &todayStart}`，分别取 byUser / byUserNode 聚合；total 时用户合计改用 `u.UsedBytes`；组装并按 total desc, user_id asc 排序。
- `internal/web/web.go` 注册 `GET /api/dashboard/user-traffic`（requireAdmin）。
- 测试：在 `internal/web/` 新增/扩展测试覆盖 401、400、today/total 两种口径与排序；repo 层测试可选（复用 repo_test 模式）。

## Frontend

- `web/src/api/types.ts`：`DashboardUserNodeTraffic`、`DashboardUserTrafficItem`、`DashboardUserTraffic`（`range` + `items`）。
- `web/src/api/dashboard.ts`：`getDashboardUserTraffic(range: 'today' | 'total')`。
- `web/src/pages/DashboardPage.vue`：
  - 统计卡片下方新增「用户流量」section：标题 + 范围切换（两个按钮/ segmented，使用 token 样式）。
  - 自定义表格（DataTable 不支持展开行，不扩展它以免回归）：列 = 用户（username + #id）、状态（StatusBadge）、上传、下载、合计、配额（MetricBar/ProgressBar 或 `已用 / 配额` 文本）、展开箭头。
  - 展开行：嵌套小表格，列 = 节点、所属服务器、上传、下载、合计；`nodes` 为空时显示「该范围内无流量」。
  - `formatBytes` 复用；`formatBytes(0)` 显示 `0 B`。
  - 状态：`trafficRange` ref、`traffic` ref、`trafficLoading`；`load()` 同时拉取 dashboard 与新接口；切换 range 时单独重新拉取新接口；30s 定时器沿用。
- 排序在前端不再处理（后端已排序）。

## Tradeoffs

- 不扩展 DataTable 组件：展开行为一次性需求，避免影响 Users/Servers/Nodes 页。
- today 口径使用 UTC 与现有「今日流量」卡片一致，避免同页两种口径。
- 全量返回用户（轻量级面板，用户量小）；如未来用户量大再加分页。
