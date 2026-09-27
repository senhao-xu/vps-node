# PRD: 仪表盘流量排行卡片——节点消耗 + 用户消耗 Top 榜

## Goal

参考截图「监控大盘」样式，在仪表盘（`web/src/pages/DashboardPage.vue`）新增两张排行卡片：**节点流量消耗排行** 与 **用户消耗排行 Top 榜**，展示序号、名称、流量、占比进度条，跟随现有「今日/累计」切换。

## Background / Confirmed Facts

- 后端 `GET /api/dashboard/user-traffic?range=today|total` 已返回全部用户的流量（`items[]`：username/status/total_bytes）及按用户×节点的明细（`nodes[]`：node_id/node_name/server_name/total_bytes），**无需改后端**。
- `DashboardPage.vue` 已有 `trafficRange`（today/total）切换与数据获取逻辑；排行直接复用同一份响应做前端聚合。
- 复用组件：`ProgressBar.vue`（占比条）；新增共享 `RankList` 组件（components/ 层）承载排行列表渲染。
- 视觉遵循中性主题：卡片 lg 圆角、序号圆徽（前 3 名可用金/银/铜色或 primary 强调）、名称超长省略。

## Requirements

- R1: 新增 `components/RankList.vue`（或 ui/ 层）：props 为排序后的条目数组（名称/副标题/数值/占比），渲染序号、名称（ellipsis）、格式化流量、百分比、进度条；空数据显示 EmptyState。
- R2: DashboardPage 新增「节点流量消耗排行」卡片：按 node_id 聚合所有用户 nodes[] 的 total_bytes，降序取 Top 8，副标题显示 server_name；占比 = 该项 / 节点总和。
- R3: 新增「用户消耗排行」卡片：按 items[] total_bytes 降序 Top 8，副标题显示状态或配额；占比 = 该项 / 用户总和。
- R4: 两卡片并排（桌面两列，窄屏堆叠），放在用户流量表格上方或下方（实现时以视觉平衡为准，参考图为表格上方）；跟随 trafficRange 切换刷新；加载中骨架、空数据 EmptyState。
- R5: 不改后端、不改现有表格；全部语义 token。

## Acceptance Criteria

- A1: `cd web && npm run typecheck && npm run lint && npm run build` 通过。
- A2: 两张排行卡数据与「用户流量」表格同源一致（今日/累计切换同步）。
- A3: 占比条、序号、流量格式化（formatBytes）正确；空数据/加载态正常。
- A4: 无颜色字面量；移动端堆叠不破裂。

## Out of Scope

- 后端 API 改动
- 参考图的 24 小时走势曲线图（既有 TrafficChart 相关，不在本次范围）
- 排行点击跳转（后续可加）
