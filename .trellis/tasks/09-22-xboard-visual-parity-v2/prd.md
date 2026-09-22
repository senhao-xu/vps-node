# 完全复刻 Xboard admin 视觉与布局（Vue 内 Shadcn 外观）

## Goal

让 `web/` 管理后台的视觉与交互外观与 Xboard admin（基于 satnaing/shadcn-admin 的
React + shadcn/ui + Tailwind 界面）达到高保真一致：在**保持现有 Vue3 + 原生 CSS 变量 +
scoped style 技术栈不变**的前提下，按 Xboard dist 提取的精确 token、布局骨架与组件样式，
重做应用外壳与全部通用组件、页面。

用户价值：后台外观与用户认可的 Xboard 参考界面一致，消除「像但不像」的落差。

## Background

- 精确设计参考已存在：`.trellis/tasks/09-21-xboard-style-revamp/research/xboard-admin-style.md`
  （从 `cedar2025/xboard-admin-dist` 编译产物逆向出的 token / 布局 / 组件类名）。
- 参考截图：`https://github.com/cedar2025/Xboard/blob/master/docs/images/admin.png`。
- 前两轮尝试（`09-21-xboard-style-revamp`、`09-21-frontend-ui-refresh`）只做到
  「token 换色 + 局部微调」，用户明确表示仍不满意，要求「完全照抄 Xboard」。
- 技术路线已由用户确认：**Vue 内复刻 Shadcn 视觉**（不重写为 React，不引入 Tailwind/shadcn 依赖）。
- 本任务取代 `09-21-xboard-style-revamp` 的视觉目标，作为唯一权威的视觉对齐任务。

## Confirmed Facts（来自代码与逆向研究）

- Xboard 布局骨架：`aside` 侧栏 256px（折叠 56px）+ 右侧 `border-r-2 border-r-muted`；
  `main` 顶栏 `--header-height: 4rem`（64px）；内容区 `px-4 py-6 md:px-8`。
- Xboard token（light）：background `#FFFFFF`、foreground `#020817`、primary `#0F172A`、
  primary-foreground `#F8FAFC`、secondary/muted/accent `#F1F5F9`、muted-foreground `#64748B`、
  destructive `#EF4444`、border/input `#E2E8F0`、ring `#020817`、radius `0.5rem`。
- Xboard token（dark）：background `#020817`、foreground `#F8FAFC`、primary `#F8FAFC`（反白）、
  secondary/muted/border `#1E293B`、muted-foreground `#94A3B8`、destructive `#7F1D1D`、ring `#CBD5E1`。
- 组件规格：按钮 `rounded-md`(6px) + `text-sm font-medium` + `focus-visible:ring-1`；
  卡片 `rounded-xl`(12px) + 1px border + `shadow`；表格行 `hover:bg-muted/50`、表头 muted 小字；
  侧栏菜单项 `h-12 rounded-none px-6`，选中态 = `secondary`（浅灰填充）；版本区绿点 + `vX`。
- 字体：Tailwind 默认系统字体栈（无自定义 webfont）。
- 当前仓库已应用上述大部分 token（`web/src/styles/tokens.css`），但布局结构与组件外观
  （分组侧栏、顶栏布局、卡片/统计卡/图表/表格/表单细节）与 Xboard 仍有明显差距。

## Requirements

### R1 基础层（tokens + base）
- 校准 `web/src/styles/tokens.css` 使 light/dark 取值与 Xboard 精确一致；补充 Tailwind
  `font-sans` 等价系统字体栈、`text-xs/sm/base/lg/xl/2xl` 字号刻度、`rounded-md/xl/full`、
  `shadow-sm/shadow` 等语义别名，便于后续组件统一消费。
- 校准 `web/src/styles/base.css`：按钮 variants（default/secondary/ghost/outline/destructive）
  与尺寸（default/sm/icon）、输入框/select/textarea、kbd、卡片、表格、field 等基类对齐
  Shadcn 外观与 `focus-visible:ring-1` 行为。

### R2 应用外壳（AppLayout）
- 侧栏：Xboard 骨架（256px / 折叠 56px、右侧分隔线、底部绿点 + 版本、右缘悬浮圆形折叠钮），
  菜单项规格对齐。
- **导航结构（最终确认）**：保持**扁平 5 项**（仪表盘 / 用户 / 服务器 / 节点 / 设置），
  不引入分组层级；菜单项按 Xboard 规格渲染（通栏、选中态浅灰填充）。
  - 折叠到 56px 时仅显示图标（含 `title` 提示）。
  - 注：曾按方案 B 实现分组可折叠，用户实看后要求「左侧菜单不用多一层」，故回退为扁平结构，
    仅保留 Xboard 视觉规格。
- 顶栏：64px；右侧搜索输入框（带 `⌘K` kbd）+ 主题切换 + 用户入口；
  **页面主标题不再放顶栏**，统一由内容区 `PageHeader` 呈现（Xboard 中 `Dashboard` 大标题位于内容区顶部）。
- 内容区：白底、`px-4 py-6 md:px-8`（或等价间距）；移动端保持可用。

### R3 通用组件
- 统一重做：Card / StatCard / Button / AppSelect / ToggleSwitch / StatusBadge / ModalDialog /
  ConfirmDialog / ErrorBanner / EmptyState / LoadingSpinner / DataTable / TablePaginator /
  OverflowMenu / FilterChip / PageHeader / MetricBar / ProgressBar / CopyText / OneTimeSecret /
  NodeChecklist，使其外观对齐 Shadcn 对应组件。
- 统计卡对齐 Xboard：白卡 + 标题（muted 小字）+ 右上彩色图标 + 大号粗体数字 + 次要说明行。
- 新增 `ui/SegmentedControl.vue`（Xboard `Amount | Count` 式 pill）；`TrafficChart` 面积图外观对齐 Xboard。

### R4 页面布局模式与全部页面
- 列表页：顶部工具栏（搜索输入 + 右侧主按钮）+ Shadcn 风格表格 + 分页。
- 详情页：页头（标题 + 描述 + 操作按钮）+ 白卡分区。
- 表单/弹窗：Shadcn Dialog/Form 外观与间距。
- 登录页：`#F8FAFC` 浅灰底 + 居中单列卡片（约 420px）+ 右上角主题切换。
- 覆盖页面：Dashboard / Users / UserDetail / Servers / ServerDetail / Nodes / Settings / Login / NotFound，
  以及 `components/` 与 `components/ui/`、`components/user/` 下全部组件。

### R5 主题与响应式
- light/dark/system 三态保持可用，dark 严格按 Xboard dark token。
- 响应式：≤900px 侧栏收起为顶部导航（沿用现有移动端策略并适配新外观）。

## Acceptance Criteria

- [ ] AC1：`tokens.css` light/dark 取值与 Xboard 参考表逐项一致（primary/muted/border/radius 等）。
- [ ] AC2：侧栏为 Xboard 骨架：256px/56px 可折叠且刷新保持、选中态浅灰填充、底部绿点 + 版本、
      右缘悬浮圆形折叠钮；导航为扁平 5 项、无分组层级。
- [ ] AC3：顶栏 64px，含搜索框（`⌘K` kbd）、主题切换、用户入口；页面大标题位于内容区顶部。
- [ ] AC4：Card/Button/Input/Table/Badge/Dialog 等通用组件外观与 Shadcn 规格一致
      （圆角、边框、阴影、hover/focus 态）。
- [ ] AC5：统计卡呈现「标题 + 右上图标 + 大数字 + 次要说明」，Dashboard 视觉对齐参考截图。
- [ ] AC6：列表页为「工具栏 + 表格 + 分页」、详情页为「页头 + 分区卡」、登录页为居中单卡浅灰底。
- [ ] AC7：全部页面在 light/dark 下无破损、无残留旧蓝色/旧填充、对比度可读。
- [ ] AC8：不改动任何 API 调用、路由、业务逻辑；`npm run build`、`npm run lint`、
      `npm run typecheck` 全部通过。

## Out of Scope

- 不引入 Tailwind / shadcn/ui / 任何第三方 UI 组件库或图标库替换。
- 不重写为 React；不改后端（Go）与 API 契约。
- 不引入 i18n / 语言切换（当前仅中文）。
- 不新增页面或业务功能；除 R2 已确认的分组导航外，不改动其余导航文案与页面信息层级。
- 不实现 Xboard 的商业化内容（Income/Tickets/Commission 等）。

## Open Questions

- 无（导航结构先按方案 B 分组实现，用户实看后改为扁平 5 项，已按最终意见落地）。

## Notes

- 设计参考：`09-21-xboard-style-revamp/research/xboard-admin-style.md`。
- 本任务为复杂任务，需 `design.md` + `implement.md`。
- 技术设计见 `design.md`，执行计划见 `implement.md`。
