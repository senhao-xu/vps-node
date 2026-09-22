# Design: 前端页面优化（对标 xboard）

## Architecture / Boundaries

改动全部在 `web/` 内，零后端改动。分层：

```
styles/tokens.css + base.css   → 令牌微调 + 全局原语增强（不动两层的结构）
components/ui/ (新目录)         → 新增无业务原语： ToggleSwitch, AppSelect, OverflowMenu,
                                  FilterChip, LoadingSpinner, EmptyState, PageHeader, StatCard
components/                     → DataTable v2, TablePaginator v2, ModalDialog v2 (原地升级)
components/AppLayout.vue        → 顶栏搜索/详情页标题/移动端单头部
pages/                          → 8 页迁移到新组件，删除重复 CSS
router/index.ts                 → 新增 404 路由
```

边界规则（来自 spec，必须遵守）：

- 组件只引用语义 token，禁止字面色值/primitive（directory-structure.md:46）；每加一个 token 同步 `[data-theme='dark']`（:47）
- 表格保持"滚动容器内语义表格"，禁止卡片化、禁止父级 `overflow:hidden`（:51）
- DataTable 的 full-bleed 负边距模式沿用现有 `--card-padding` 方案（:52）
- 弹窗保留 `role="dialog"`/`aria-modal`/`aria-labelledby`（:54）
- 类型： 禁 `any`，API 类型只在 `api/types.ts`（type-safety.md）；DataTable 保持 `T extends Record<string, unknown>` 泛型 + typed slots

## New Component Contracts

- `ToggleSwitch`: `modelValue: boolean, disabled?: boolean` → `update:modelValue`；role="switch"，键盘 Space/Enter
- `AppSelect<T>`: `modelValue, options: {value, label, dot?: string}[]`；下拉 panel 用 teleport + `useClickOutside` + 翻转定位；键盘 ↑↓/Enter/Esc；选中项 ✓。协议色点通过 option.dot（语义类名如 `dot-vless`，色值映射集中在组件内 token 类）
- `OverflowMenu`: trigger "..." + teleport menu，items: `{label, danger?, onSelect}[]`，Esc/外点关闭
- `FilterChip`: `label, icon?` + 下拉 panel 插槽（复用 AppSelect 的定位 composable）
- `PageHeader`: props `title, subtitle?` + `#actions` 插槽；取代各页 .page-header + .eyebrow
- `StatCard`: `label, value, icon, tone?: default|success|warning|danger`
- `EmptyState`: `icon?, title, hint?` + `#action`
- `LoadingSpinner`: `size?`；表格骨架行由 DataTable 内部渲染

共享 composables 放 `web/src/utils/` 或 `components/ui/composables.ts`：`useClickOutside`, `useFloatingPanel`（定位+翻转）, `useFocusTrap`, `useScrollLock`。

## DataTable v2

- 新 props: `rowKey: (row: T) => string | number`（必填，修复 index key）、`selectable?: boolean`（emit `update:selected`）、`sortKey/sortDir` + emit `sort`（表头 ↕，排序逻辑留在页面/store）
- 新 slots: `#toolbar`（搜索框 + FilterChip）、保留 `cell-*`
- loading 时渲染骨架行（pulse 动画），empty 时内嵌 EmptyState
- 底部 footer 区： "已选择 N 项，共 M 项" + TablePaginator（每页显示 + 页码）
- 破坏性检查： 现仅 Nodes/Servers/Users/ServerDetail 使用 DataTable，签名变更逐一迁移

## ModalDialog v2

- `subtitle?` prop（标题下灰字）、`#footer` 右对齐按钮组
- 挂载时 `useFocusTrap` + `useScrollLock`，关闭恢复焦点
- title id 用 `useId()`（Vue 3.5）保证唯一
- 圆角/遮罩对齐截图： `--radius-lg` 提升 + overlay token 调深

## AppLayout v2

- 顶栏： 左 = 当前页标题（路由 meta.title，详情页 meta 支持动态："服务器详情"并附返回链接）；右 = 菜单搜索按钮（⌘K 样式 kbd）、主题切换（带标签的三态 segmented 或明确 icon+文字）、用户名、登出
- 菜单搜索： 本地静态菜单项（8 页）模糊过滤的 command palette，纯前端，快捷键 ⌘K/Ctrl+K
- 移动端 ≤900px： 顶栏保留（标题+菜单按钮），侧边 nav 并入顶栏下单一横排；消除第三个标题栏（页面 .page-header 在移动端只保留操作按钮）
- 侧边栏激活 pill、分组结构维持现状（仅 5-6 项，不引入折叠分组）

## Data Flow

无新数据流。所有 API 调用、类型、store 不变；纯展示层 + 少量 UI 状态（选择集、排序键、面板开合）留在页面本地。

## Compatibility / Migration

- 不新增全局常量；`__APP_VERSION__` 模式不动
- lucide-vue-next 加入 `web/package.json` dependencies；`npm ci` 后构建
- tokens.css 只做增量（radius/overlay/骨架色），不改既有语义 token 含义 → 旧页面在迁移完成前不至于崩坏
- 回滚： 单任务单分支，按 commit 回滚；无数据库/配置迁移

## Trade-offs

- 自研 AppSelect/OverflowMenu（~150 行/个）换零 UI 库依赖： 键盘导航/定位边界需 check 阶段专项验证
- 移动端不做卡片式表格（spec 禁止），接受横向滚动，但收紧 min-width 并统一去掉各页 :deep() 发散覆写
- TrafficChart 仅加轴线/刻度，不引图表库

## Rollback

纯前端静态产物。回滚 = 恢复 `web/dist` 旧构建（或重新构建旧 commit）。panel 二进制 embed 不受影响的构建（8080 静态服务）直接回退 dist 即可。
