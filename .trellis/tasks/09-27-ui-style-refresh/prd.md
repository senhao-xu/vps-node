# PRD: 参考 HHUB 风格的全局 UI 样式焕新

## Goal

参考用户提供的 HHUB 面板截图，将 `web/` 前端整体视觉焕新为现代轻盈的中性色风格：浅色 sidebar、白色大圆角卡片、柔和弥散阴影、近黑主按钮、彩色徽章点缀，同时保留暗色主题可用。

## Background / Confirmed Facts

- 前端：Vue 3 + Vite + Pinia，无 UI 框架，样式全部手写；zh-CN UI。
- 全局样式集中在 `web/src/styles/tokens.css`（两层 token 体系，light + `[data-theme='dark']` 全套覆盖）与 `web/src/styles/base.css`（.card/.btn/.chip/表单等共享原语）。
- 布局壳 `web/src/components/AppLayout.vue`：深色 sidebar（`--color-shell*` token + 硬编码 `#2dd4bf`/`#5eead4`）+ sticky 毛玻璃 toolbar。
- 当前主色 teal（`--color-primary: #0f766e`），圆角小（3/5/8px）。
- 徽章：`StatusBadge.vue`（tone 类型在 `utils/labels.ts`）+ base.css `.chip`/`.status-dot`。
- 指标区：`MetricStrip.vue` 是唯一指标带组件（spec 禁止页内重建 metric 卡片）。
- 参考图风格要点：浅色 sidebar（激活项浅灰圆角块）、白卡片大圆角（~16px）柔和阴影、近黑实心主按钮、紫色协议徽章、绿色"运行中"胶囊、统计卡片大数字 + 底部彩色进度条、大圆角弹窗 + 分段选择器。
- 主题机制：`stores/theme.ts`（light/dark/system，localStorage `vps-node-theme`），dark 覆盖必须与 light 同步（spec 约束）。

## Key Decisions

- D1: 主色改黑/中性——主按钮近黑实心，页面白/灰中性色为主，彩色仅用于徽章与状态指示。
- D2: sidebar 由深色改为浅色（token 驱动，dark 主题下仍为深色）。
- D3: 暗色主题保留并全套适配新中性配色（重写 dark palette，去 teal 倾向）。

## Requirements

- R1: token 焕新（tokens.css）：中性灰 palette、近黑主色、圆角放大（新增 lg）、阴影柔和化、新增 purple 徽章 token、shell 系列改浅色语义；所有改动同步 dark 覆盖。
- R2: 基础组件焕新（base.css）：.card/.btn/输入框/.chip/.toggle-row 等圆角、阴影、配色对齐参考风格。
- R3: 布局壳焕新（AppLayout.vue）：浅色 sidebar、激活态浅灰圆角块、清除硬编码 teal 色值、brand-mark 近黑化；不破坏移动端 drawer 与 a11y 行为。
- R4: 徽章体系（StatusBadge.vue + utils/labels.ts Tone）：胶囊圆角，新增 purple tone 供协议徽章。
- R5: 重点页面精调：DashboardPage 统计区（扩展 MetricStrip 实现大数字 + 彩色条风格）、NodesPage 协议徽章 purple 化、NodeFormDialog 弹窗/分段选择器对齐参考图。
- R6: 纯样式/模板调整，不改任何功能逻辑、API、路由、组件契约（DataTable/MetricStrip props 只增不破）。

## Acceptance Criteria

- A1: `cd web && npm run typecheck && npm run lint && npm run build` 全部通过。
- A2: 亮色主题视觉与参考图风格一致：浅色 sidebar、大圆角卡片、柔和阴影、近黑主按钮、紫/绿彩色徽章。
- A3: 暗色主题全套适配，对比度与可读性正常，无 teal 残留色。
- A4: `.vue` 文件内无新增颜色字面量（语义 token only）；新增 token 均有 dark 覆盖。
- A5: 全部既有页面无布局破裂、无功能回归（移动端 drawer、dialog、表格交互正常）。

## Out of Scope

- 后端（Go）任何改动。
- 新功能或交互流程变更。
- 参考图中的"用户中心"页面（本项目无对应页面）。
- 非重点页面的像素级复刻（只做 token 驱动的自动焕新 + 破裂修复）。

## Risks / Deferred

- 全站视觉回归风险 → 分层 commit（tokens → base → layout → pages）便于单点 revert。
- DashboardPage 统计区若 MetricStrip 扩展成本过高，退化为页内卡片但需在 check 阶段评审确认合理性。
