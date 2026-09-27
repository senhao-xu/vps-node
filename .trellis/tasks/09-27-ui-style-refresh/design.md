# Design: 参考 HHUB 风格的全局 UI 样式焕新

## Architecture & Boundaries

纯样式改造，分三层，按依赖顺序推进：

1. **Token 层** `web/src/styles/tokens.css`
   - 私有 palette 换为中性灰阶（参考 Tailwind zinc/neutral 系），主色改为近黑（light: `#18181b` 系；dark 反转）。
   - 圆角放大：xs 4px / sm 6px / md 10px / 新增 lg 14px / dialog 16-20px；卡片用 lg。
   - 阴影更柔和弥散：`--shadow-card` 改为低透明大模糊（如 `0 1px 2px rgba(0,0,0,.04), 0 8px 32px rgba(0,0,0,.06)`）。
   - 新增徽章语义 token：`--color-badge-purple*`（协议徽章）、保留 success 绿色胶囊。
   - dark 主题同步映射：近黑主色反转为近白，背景/表面/shell 全套重写为中性深灰（去掉 teal 倾向）。
   - shell 系列 token（`--color-shell*`）改为浅色 sidebar 语义（light: 白/浅灰；dark: 深灰），消费方 AppLayout 无需改类名。

2. **基础组件层** `web/src/styles/base.css`
   - `.card`：圆角 lg、padding 20-24px、柔和阴影、去掉 hover 边框加深（改为无或更轻）。
   - `.btn`：主按钮近黑实心、圆角加大（8-10px）；`.btn.secondary/.outline` 白底灰边。
   - `.chip` / 输入框 / `.toggle-row` / `.theme-switch` 等圆角同步。
   - `.status-dot` 不变。

3. **布局壳 + 重点页面**
   - `AppLayout.vue`：sidebar 浅色化（token 驱动，改少量结构样式：nav-item 激活态改为浅灰圆角块、去掉左侧 3px 指示条或改为近黑小圆条）；`brand-mark` 改为近黑方块白字；toolbar 保持毛玻璃但边框更轻。
   - `StatusBadge.vue`：徽章圆角改 full（胶囊），新增 purple tone 供协议徽章使用。
   - `DashboardPage.vue`：统计区参考截图改为"大数字卡片 + 底部彩色进度条"风格（评估是否扩展 MetricStrip 或页内实现；优先扩展 MetricStrip 保持 spec 约束）。
   - `NodesPage.vue`：协议列徽章使用新 purple tone；状态列保持。
   - `NodeFormDialog.vue`：分段选择器（直连/中转）视觉对齐参考图；弹窗圆角加大（token 驱动）。

## Data Flow / Contracts

无 API/路由/逻辑改动。CSS token 名保持不变（只改值），新增 token 需同步 dark 覆盖。

## Trade-offs

- 改 token 值而非批量改组件：覆盖面最大、回归风险最低；少数硬编码颜色（AppLayout `#2dd4bf`、`#5eead4`）需替换为 token。
- MetricStrip 扩展 vs 页内新组件：spec 明确"不要在页面重建 metric 卡片"，故扩展 MetricStrip（可选 progress/footer slot），保持 single-owner。
- dark 主题重写 palette 而非微调：当前 dark 偏 teal，与新的中性 light 不配套，一次重写更一致。

## Compatibility & Rollback

- 无持久化数据/接口变更；用户 localStorage 主题键 `vps-node-theme` 不受影响。
- 回滚 = git revert 单个 commit（建议按层分 commit：tokens → base → layout/pages）。
- 风险点：全站视觉回归（暗色主题对比度、表格密度）；验证靠 typecheck/lint + 人工浏览主要页面。
