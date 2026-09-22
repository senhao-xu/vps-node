# Implement: 前端页面优化（对标 xboard）

## Validation Commands

```bash
cd web && npm run typecheck   # vue-tsc --noEmit
cd web && npm run lint
cd web && npm run build       # 产物主 chunk < 200K
```

每个阶段完成后跑 typecheck+lint；全部完成后跑 build 并人工核对页面（对照 xboard 截图）。

## Checklist

### 阶段 1 · 基础设施
- [ ] 1.1 `cd web && npm i lucide-vue-next`
- [ ] 1.2 tokens.css 增量： 骨架屏 pulse 色、overlay 加深、必要时新 radius；同步 `[data-theme='dark']` 全覆盖
- [ ] 1.3 base.css: 按钮 loading 态（spinner）、chip/outline-badge 原语、kbd 样式
- [ ] 1.4 `components/ui/composables.ts`: useClickOutside / useFloatingPanel / useFocusTrap / useScrollLock

### 阶段 2 · UI 原语（components/ui/）
- [ ] 2.1 ToggleSwitch（role=switch，键盘可操作）
- [ ] 2.2 AppSelect（彩点选项 + ✓ + teleport 翻转定位 + 键盘导航）
- [ ] 2.3 OverflowMenu + FilterChip（复用 floating panel）
- [ ] 2.4 LoadingSpinner / EmptyState
- [ ] 2.5 PageHeader / StatCard

### 阶段 3 · 核心组件升级
- [ ] 3.1 DataTable v2: rowKey 必填、骨架行、EmptyState 内嵌、selectable、sortable 表头、#toolbar 插槽、footer"已选择 N 项"
- [ ] 3.2 TablePaginator v2: 每页显示 + 页码 + 首末页
- [ ] 3.3 ModalDialog v2: subtitle、focus trap、scroll-lock、useId title、footer 插槽右对齐、圆角/遮罩对齐截图
- [ ] 3.4 ConfirmDialog 跟随 v2；替换 UserDetailBasic.vue:58 window.confirm

### 阶段 4 · 布局
- [ ] 4.1 AppLayout: 路由 meta.title 驱动的顶栏标题 + 详情页返回链接
- [ ] 4.2 顶栏菜单搜索（⌘K command palette，静态菜单项过滤）
- [ ] 4.3 主题切换控件改为清晰三态
- [ ] 4.4 移动端单头部（合并顶栏与横向 nav，消除三标题堆叠）
- [ ] 4.5 router: 404 页 + meta.title 补全

### 阶段 5 · 页面迁移（每页： PageHeader + 去重 CSS + 加载/空/错误三态）
- [ ] 5.1 LoginPage（视觉对齐即可）
- [ ] 5.2 DashboardPage: StatCard 行 + 迁移 DataTable v2（删 ~130 行重复 CSS）+ 流量单元格进度条
- [ ] 5.3 NodesPage: 工具栏（搜索+FilterChip）+ 选择列 + 排序 + toggle 启用 + OverflowMenu；删 :deep() 发散覆写
- [ ] 5.4 ServersPage: 同上 + 统计卡片行
- [ ] 5.5 UsersPage: 同上 + 流量列（数值+%+细条）+ 到期 pill 徽章
- [ ] 5.6 ServerDetailPage: 信息网格统一组件、MetricBar 保留、节点表迁移
- [ ] 5.7 UserDetailPage + user/* 面板： 统一错误展示、CopyText 去重（合并 3 处 copy-timer）
- [ ] 5.8 SettingsPage: 左侧子导航 + 分节表单（label+input+helper）+ toggle 卡片
- [ ] 5.9 NodeFormDialog/ServerFormDialog/UserCreateDialog: 弹窗外壳对齐（副标题、toggle 卡片、AppSelect 彩点协议下拉、footer）

### 阶段 6 · 收尾
- [ ] 6.1 全局 grep 验收： 无 "加载中…"、无 window.confirm、无 .eyebrow、无 inline style 残留（除动态宽度）
- [ ] 6.2 typecheck + lint + build 全绿，主 chunk < 200K
- [ ] 6.3 明暗主题各过一遍 8 页 + 3 弹窗，对照 xboard 截图核对（A8）
- [ ] 6.4 移动端 375px 过一遍核心页（A4）

## Review Gates

- 阶段 3 完成后： DataTable/ModalDialog API 评审（破坏性签名确认）
- 阶段 5 中期（5.3 完成后）: Nodes 页作为样板核对一次 xboard 截图，再继续其余页面

## Risky Files / Rollback Points

- `components/DataTable.vue` / `ModalDialog.vue`（签名破坏）→ 阶段 3 结束即跑全量 typecheck
- `styles/tokens.css`（暗色同步）→ 每次改动后 grep 新 token 的 dark 覆盖
- `AppLayout.vue`（移动端）→ 4.4 后立刻 375px 人工核对
- 回滚： 每阶段一个 commit；整体回滚 = 恢复旧 dist 构建

## Follow-up Checks

- spec 更新（trellis-update-spec）: 新组件目录 `components/ui/`、composables、DataTable v2 约定写入 `.trellis/spec/frontend/`
