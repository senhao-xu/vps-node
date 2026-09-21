# Design: 参考 Xboard 风格重做 Web 面板样式与布局

## 总体策略

不更换技术栈（Vue3 + 原生 CSS 变量 + scoped styles），把 Xboard/shadcn-admin 的设计语言**映射到现有 token 体系**：重做 `tokens.css` 色板 → 重做 `base.css` 基础组件样式 → 重做 `AppLayout.vue` 布局 → 逐页面/组件适配。由于全站样式均消费同一组 CSS 变量，大部分页面只需少量调整。

## Token 映射（styles/tokens.css）

### Light

| 现有 token | 现值 | 新值（Xboard 参考） |
|---|---|---|
| `--color-bg` | #F5F7FA | `#FFFFFF`（页面白底） |
| `--color-surface` | #FFFFFF | `#FFFFFF` |
| `--color-surface-muted` | #FAFBFD | `#F8FAFC`（登录页/次级背景） |
| `--color-border` | #E2E8F0 | `#E2E8F0`（不变） |
| `--color-text` | #172033 | `#020817` |
| `--color-text-secondary` | #64748B | `#64748B`（不变） |
| `--color-primary` | #2563EB | `#0F172A`（近黑主色） |
| `--color-primary-hover` | #1D4ED8 | 同色 + `opacity: .9`（按钮 hover 用） |
| `--color-primary-soft` | #EFF6FF | `#F1F5F9`（secondary/muted 填充） |
| `--color-primary-border` | #BFDBFE | `#E2E8F0` |
| `--color-on-primary` | #FFFFFF | `#F8FAFC` |
| `--color-danger` | #DC2626 | `#EF4444` |
| `--color-focus-ring` | 蓝色半透明 | `rgba(2,8,23,.10)`（中性） |
| `--color-shell*` | 白色系 | 白底 + `#E2E8F0` 边线；选中 `#F1F5F9` |
| `--radius-sm/md/lg` | 8/12/16 | 控件 8px（rounded-md 视觉）、卡片 12px 保持 |
| `--shadow-card` | 蓝色调阴影 | `0 1px 3px rgba(2,8,23,.06)` 中性轻阴影 |
| `--shadow-btn-primary` | 蓝色阴影 | 去掉（改透明度过渡） |
| `--shell-width` | 244px | 256px（新增 `--shell-width-collapsed: 56px`） |

成功绿 / 警告黄 / 离线灰等语义色维持现状（仅作点缀）。

### Dark（[data-theme='dark']）

按 shadcn dark：`--color-bg: #020817`、`--color-surface: #020817`、`--color-surface-muted/--color-muted-soft: #1E293B`、`--color-border: #1E293B`、`--color-text: #F8FAFC`、`--color-text-secondary: #94A3B8`、`--color-primary: #F8FAFC`（反白）、`--color-on-primary: #020817`、`--color-danger: #7F1D1D`、focus-ring `rgba(203,213,225,.25)`。shell 与内容同色，用 border 分层。

## 布局（components/AppLayout.vue）

- **侧边栏**：宽 256px，白底（dark 下同 surface），右侧 1px border；Logo 区保留现有 brand-mark。菜单项 `h-12` 通栏、圆角 8px；选中态 = `background: var(--color-primary-soft)` + 文字 `--color-text` + 600 字重，**删除左侧竖条与蓝色文字**。底部增加 `版本区`（绿点 + 版本号，从现有注入或写死 `v0.1.0`，实现时确认数据来源）。
- **折叠**：新增 `collapsed` 状态（`localStorage['sidebar-collapsed']`），折叠时宽 56px 仅显示图标（label 隐藏，加 `title`）；折叠钮为侧栏右缘悬浮圆形 outline 按钮（chevron 旋转）。宽度用 CSS 变量 + `transition: width .2s`。
- **版本号**：`vite.config.ts` 增加 `define: { __APP_VERSION__: JSON.stringify(<git rev-parse --short HEAD 结果，失败时回退 'dev'>) }`（构建时用 `child_process.execSync` 取 sha），`src/env.d.ts` 声明该全局常量；侧栏底部绿点 + `__APP_VERSION__`。
- **顶栏**：64px，左侧页面标题（`--font-size-xl`、700、tracking-tight），右侧保留主题切换 + 用户名 + 退出按钮。
- **内容区**：`--color-bg` 白底；`.page` 保持 max-width 1320 + padding。
- **移动端**（≤900px）：保留现有"侧栏变顶部横条"行为，折叠态在移动端不生效。

## 基础组件（styles/base.css）

- `.btn` 主按钮：黑底（`--color-primary`）白字、`font-weight: 500`、hover 改 `opacity: .9`（去掉彩色阴影与 translateY）；`.btn.secondary` 改为 `bg-surface + border + 文字色`，hover `bg-primary-soft`；`.btn.danger` 用新 `#EF4444`。
- 输入框/ select：保持 8px 圆角，focus ring 用中性色。
- `.card`：12px 圆角 + 1px border + 中性轻阴影。
- `.page-title`：24px/700。

## 页面与组件适配

按依赖顺序逐个核对（多数只需替换被删除/改义的变量引用）：

1. `pages/LoginPage.vue` → 浅灰底（`--color-surface-muted`）居中单卡，宽约 420px。
2. `pages/DashboardPage.vue` + `MetricBar` / `TrafficChart` / `ProgressBar` → 图表主色从蓝改近黑/灰阶，统计卡大数字粗体。
3. `components/DataTable.vue` / `TablePaginator.vue` → 行 hover `muted` 浅灰，表头 muted 小字。
4. `components/StatusBadge.vue` → 语义色不变，确认对比度。
5. `pages/Users|Servers|Nodes|Settings*Page.vue` + 三个 `*FormDialog.vue` + `UserDetail*` → 替换蓝色强调、核对 focus/选中态。
6. 其余（`ModalDialog`、`ConfirmDialog`、`ErrorBanner`、`CopyText`、`OneTimeSecret`、`NodeChecklist`、`App.vue`）→ grep 检查 `--color-primary` / 硬编码 hex。

## 兼容与回滚

- 纯 CSS/DOM 结构变更，无 API/路由变化；回滚 = `git revert`。
- 风险点：dark 模式 primary 反白后，所有"primary 底 + on-primary 字"的组合仍成立；逐个页面在 dark 下目检。
- `--color-primary-soft` 语义从"浅蓝"变"浅灰"，引用它做"信息提示底色"的地方（如 ErrorBanner/表单提示）需改为语义色或确认可接受。

## 验证

- `npm run build`（web/）；lint/typecheck 按仓库现有命令。
- 人工：light/dark × 8 页面截图核对验收标准。
