# Design: 完全复刻 Xboard admin 视觉与布局

> 关联 PRD：`prd.md`（R1–R5、AC1–AC8、方案 B）。
> 权威参考：`.trellis/tasks/09-21-xboard-style-revamp/research/xboard-admin-style.md`。

## 1. 总体策略

保持现有技术栈（Vue3 + 原生 CSS 变量 + scoped style），自底向上分层重做，让全站复用同一套
token 与基类，从而「改一处、全站对齐」：

```
L0 tokens.css       精确对齐 Shadcn/ Xboard 色板、圆角、阴影、字号、控件高度
L1 base.css         按钮 variants / 输入框 / 卡片 / 表格 / field / kbd / chip 基类
L2 ui 组件          StatCard / PageHeader / SegmentedControl / Dialog / Badge / DataTable ...
L3 AppLayout        分组可折叠侧栏 + 顶栏 + 内容区骨架
L4 pages            列表/详情/表单/登录 页面模式逐一适配
```

不引入 Tailwind / shadcn / 新 UI 库；不改 API、路由、业务逻辑。

## 2. L0 设计 token（`web/src/styles/tokens.css`）

### 取值对齐（light，逐项与参考表一致）

| 语义 | 现变量 | 目标值 |
|---|---|---|
| 背景/卡片 | `--color-bg` / `--color-surface` | `#FFFFFF` |
| 次级背景 | `--color-surface-muted` | `#F8FAFC` |
| 前景文字 | `--color-text` | `#020817` |
| 次文字 | `--color-text-secondary` | `#64748B` |
| 主色 | `--color-primary` | `#0F172A` |
| 主色前景 | `--color-on-primary` | `#F8FAFC` |
| muted 填充 | `--color-primary-soft` / `--color-muted-soft` | `#F1F5F9` |
| 边框 | `--color-border` | `#E2E8F0` |
| 危险 | `--color-danger` | `#EF4444` |
| focus ring | `--color-focus-ring` | `rgba(2,8,23,.10)` |
| 侧栏选中 | `--color-shell-active` | `#F1F5F9` |

dark（`[data-theme='dark']`）按参考表：`bg #020817`、`card/border/muted #1E293B`、
`text #F8FAFC`、`muted-fg #94A3B8`、`primary #F8FAFC`（反白）、`destructive #7F1D1D`、
`ring #CBD5E1`。

### 非颜色刻度校准

- 圆角：`--radius-sm: 6px`（按钮/输入，等价 `rounded-md`）、`--radius-md: 12px`（卡片，等价
  `rounded-xl`）、`--radius-lg: 12px`、`--radius-xl: 16px`；新增 `--radius-full: 999px`。
- 控件高度：`--control-height: 36px`（`h-9`）。
- 字号：新增 `--font-size-xs: 12px`；正文 `md=14`；卡标题 `lg=18`→保持；页面标题 `xl=24`
  （等价 `text-2xl`）；统计数字使用 `2xl=24` 粗体。
- 阴影：`--shadow-card: 0 1px 2px rgba(2,8,23,.06)`；新增 `--shadow-sm`/`--shadow`；
  去掉彩色阴影。
- 字体：`--font-family` 已是系统栈，与 Tailwind `font-sans` 一致，保持。

> 策略：**保留现有变量名**（避免大面积改类名），仅校准取值并按需新增别名。

## 3. L1 基类（`web/src/styles/base.css`）

- `.btn`：`rounded-md`(6px)、`h-9`、`text-sm`、`font-medium`、`shadow-sm`、
  `focus-visible: ring-1`；hover `opacity:.9`。变体：`.btn.secondary`（浅灰填充）、
  `.btn.outline`（白底 + 边框，新增）、`.btn.ghost`（透明 hover 浅灰，新增）、
  `.btn.danger`、`.btn.small`（`h-8`）、`.btn.icon`（方形 36×36，新增）。
- 输入框/select/textarea：`rounded-md`、`h-9`、边框 `#E2E8F0`、focus `ring-1` 中性。
- `.card`：`rounded-xl`(12px) + 1px border + `shadow-sm`；`.card-title` 对齐 Shadcn
  `text-lg font-semibold tracking-tight`。
- 表格基类对齐 shadcn：表头 muted 小字、行 `hover:bg-muted/50`、底部 border。
- `.kbd`、`.chip`、`.skeleton`、`.field` 微调。

## 4. L2 通用组件

| 组件 | 变更 |
|---|---|
| `ui/StatCard.vue` | 改为 Shadcn 统计卡：左上 muted 小标题、右上小彩色图标（**不再用 40px 圆角色块**）、下方大号粗体数字（`text-2xl`）、可选次要说明/涨幅行；新增 `hint?`/`delta?`/`tone` props |
| `ui/PageHeader.vue` | 标题字号/间距对齐；**移动端不再隐藏**（顶栏不再承载标题）；保留 subtitle 与 actions 插槽 |
| `ui/SegmentedControl.vue` | **新增**：通用分段控件（`items` + `v-model`），用于 Dashboard 时间范围等；样式 = 浅灰槽 + 选中黑底白字 pill |
| `DataTable.vue` | 表头/行 hover/边框对齐 shadcn；`--card-padding` 出血保持 |
| `TablePaginator.vue` / `OverflowMenu.vue` / `FilterChip.vue` / `EmptyState.vue` / `LoadingSpinner.vue` | 圆角/边框/间距/中性色校准 |
| `ModalDialog.vue` / `ConfirmDialog.vue` | dialog `rounded-lg`、遮罩、标题/描述/页脚间距对齐 shadcn |
| `StatusBadge.vue` | 语义色 + 中性 pill 边框保持一致，核对对比度 |
| `ErrorBanner.vue` / `CopyText.vue` / `OneTimeSecret.vue` / `NodeChecklist.vue` / `ToggleSwitch.vue` / `AppSelect.vue` / `MetricBar.vue` / `ProgressBar.vue` / `TrafficChart.vue` | 消费新基类与 token，去掉残留旧色 |

## 5. L3 应用外壳（`components/AppLayout.vue`）

### 5.1 导航数据模型（最终：扁平）

```ts
type NavItem = { to: string; label: string; icon: LucideIcon; exact?: boolean }

const navItems: NavItem[] = [
  { to: '/', label: '仪表盘', icon: LayoutDashboard, exact: true },
  { to: '/users', label: '用户', icon: Users },
  { to: '/servers', label: '服务器', icon: Server },
  { to: '/nodes', label: '节点', icon: Waypoints },
  { to: '/settings', label: '设置', icon: Settings },
]
```

- 扁平 5 项，无分组层级（用户实看方案 B 后明确要求「左侧菜单不用多一层」）。
- 菜单项通栏渲染（`border-radius: 0`、左右 padding 24px、高 44px），选中态浅灰填充 + 文字加深。
- 移动端 `mobile-nav` 复用同一 `navItems`。

### 5.2 折叠态（56px）

- `collapsed` + `localStorage['sidebar-collapsed']` 持久化，折叠后仅显示图标 + `title` 提示。

### 5.3 顶栏（去掉重复标题）

- 保留 64px、白底、底边框；**移除页面标题与 parent 返回链接**（改由 `PageHeader`/页面内呈现）。
- 右侧：搜索输入框样式触发器（`Search` 图标 + 占位文案 + `⌘K` kbd，点击开 `MenuSearch`）、
  主题切换（浅/深/系统）、用户入口（用户名/头像 + 退出）。
- 详情页返回：由 `PageHeader` 增加 `backTo`/`backTitle` 可选 props 承载（或页面内自行渲染）。
- 移动端：保留顶部 `mobile-nav` 横向导航（按新外观校准），标题由页面内容提供。

## 6. L4 页面模式

- **列表页**（Users/Servers/Nodes）：`PageHeader` + 工具栏（搜索输入 + 过滤 + 右侧主按钮）+
  `DataTable` + 分页；卡片包裹。
- **详情页**（UserDetail/ServerDetail）：`PageHeader`（标题 + 描述 + 操作按钮）+ 分区 `card`
  栅格；子组件（`components/user/*`）统一卡片/字段样式。
- **设置页**：分区卡片 + `toggle-row` 样式对齐。
- **登录页**：`--color-surface-muted`(#F8FAFC) 全屏底 + 居中单列卡片（约 420px）+ 右上角主题切换。
- **Dashboard**：`PageHeader`；统计卡改为 4 列栅格（Xboard 参考 2×4；本项目 6 项按
  `repeat(auto-fit,minmax(220px,1fr))`）；流量卡用 `SegmentedControl`（今日/累计）+
  对齐后的表格；`TrafficChart` 若展示则改为灰填充面积图外观。

## 7. 兼容与迁移

- 纯前端 CSS/DOM 结构变更；无 API、路由、状态契约变化。
- 路由 meta（title/parentTitle/parentPath）保留，`AppLayout` 不再消费标题；若改用
  `PageHeader` 承接返回链接，需同步各详情页。
- 旧任务 `09-21-xboard-style-revamp` / `09-21-frontend-ui-refresh` 的视觉目标由本任务取代。

## 8. 风险 / 回滚

- **风险**：折叠态子菜单 popover 交互（层级/定位/点击外部）；dark 模式对比度；移动端
  横向导航与标题并存。
- **回滚**：按文件粒度 `git checkout -- <file>`；整体 `git revert` 本任务提交。
- 实现完成后以 light/dark × 全部页面目检 + `npm run build/lint/typecheck` 作为门禁。

## 9. 不在本设计内

原生 Tailwind/shadcn、React 重写、i18n、后端与 API 变更、业务功能新增。
