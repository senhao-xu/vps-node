# PRD: 前端页面优化（对标 xboard）

## Goal

将 vps-node 管理后台 8 个页面整体改造为 xboard 风格（用户已提供目标截图）：更清晰的页面层级、统一的表格工具栏与单元格语言、完善的加载/空/错误态、移动端可用，消灭当前"很多地方不友好"的问题。

## Background / Confirmed Facts

完整调研见 `research/frontend-survey.md`。要点：

- 栈： Vue 3.5 + TS strict + Vite 7 + Pinia，无 UI 库，纯 CSS token 体系 + 暗色模式已就绪；主 chunk 112K
- 现有 palette 已是 Xboard/shadcn neutral 方向（`--color-primary` 近黑，spec `directory-structure.md:55`），与目标风格天然契合
- 已有组件基础： DataTable/ModalDialog/ConfirmDialog/StatusBadge/ProgressBar 等
- 关键缺陷（file:line 见 research）： Dashboard 手写表格未复用 DataTable；DataTable 用 index 作 key；无 spinner/空态/统一错误处理；Modal 无 focus trap；移动端三标题堆叠；window.confirm 残留；样式类跨 4-7 文件复制粘贴
- 目标风格（截图提炼）： 大标题+副标题、统计卡片带图标、表格工具栏（黑按钮+搜索+筛选 chips+可排序表头）、ID chip/toggle/圆点+名称/标签 chips/流量细进度条/"..."菜单、分页栏"已选择 N 项"、柔和 pill 徽章、设置页左子导航、弹窗大圆角+副标题+彩点自定义下拉+toggle 卡片

## Requirements

### R1 设计系统与共享组件
- 引入 `lucide-vue-next`（唯一新依赖，tree-shaken ~20K），替换 inline SVG 图标
- 新增共享组件： PageHeader（标题+副标题+操作区）、StatCard（标签+大数字+图标）、ToggleSwitch、AppSelect（自定义下拉，支持彩色圆点选项+✓）、FilterChip、OverflowMenu（"..."）、LoadingSpinner/骨架屏、EmptyState
- 消灭复制粘贴： 提取后删除各页 .eyebrow/.toolbar-label/.info-grid/.card-head/.actions 重复定义
- 全部样式只用语义 token（spec: directory-structure.md:46），新增 token 必须同步暗色覆盖

### R2 布局与导航
- 顶栏： 增加菜单搜索入口（⌘K 样式，覆盖 8 页菜单项）；主题切换改为带文字/清晰图标的控件
- 详情页（/servers/:id, /users/:id）： 正确标题 + 返回上级链接
- 移动端： 消除双标题堆叠（顶栏+横向 nav 合并为一个头部），保持 spec 约束——表格仍是滚动容器内语义表格，不做卡片化（directory-structure.md:51）
- 侧边栏： 激活项浅灰 pill 样式（已有基础上对齐 xboard 视觉）
- 新增 404 页替换 catch-all 重定向

### R3 表格体系
- DataTable v2： 行 key 改为必填 `rowKey` prop（修 index key 缺陷）；支持选择列（checkbox+全选）、可排序表头（↕）、工具栏插槽（搜索+FilterChip）、底部"已选择 N 项，共 M 项"
- TablePaginator 对齐 xboard： 每页显示 + 页码 + 首末页
- DashboardPage 迁移到 DataTable，删除 ~130 行重复表格 CSS
- 单元格语言统一： ID 描边 chip、启用 ToggleSwitch、圆点+名称、标签 chips、流量（数值+%+细进度条）、操作列 OverflowMenu
- TrafficChart 轻量增强： 坐标轴线 + 刻度标签（不引图表库）

### R4 交互细节
- 加载态： spinner/骨架屏替换全部裸文本"加载中…"
- 空态： EmptyState 统一（图标+文案+可选操作）
- 错误： 统一 ErrorBanner，删除散装 text-danger 块；ServerDetailPage 双 banner 合并
- 确认： window.confirm → ConfirmDialog（UserDetailBasic.vue:58）
- 按钮： loading 态统一 spinner+禁用
- ModalDialog： focus trap、body scroll-lock、唯一 title id、副标题 slot、大圆角对齐截图
- 表单： helper 文本、开关卡片行（label+说明+toggle）

## Acceptance Criteria

- A1 8 个页面全部使用新 PageHeader（标题+副标题），无 .eyebrow 残留
- A2 全部数据表格（含 Dashboard）使用 DataTable v2，行 key 为稳定 id；Nodes 批量选择、排序可用
- A3 所有列表/详情加载为 spinner 或骨架屏，空数据为 EmptyState，错误为 ErrorBanner —— 全局 grep 无 "加载中…" 裸文本、无 window.confirm
- A4 移动端（<900px）单头部，各页面可用；表格在滚动容器内横滑
- A5 弹窗具备 focus trap + scroll-lock，嵌套两个 dialog 时 id 不冲突
- A6 暗色模式全页面无未覆盖的新 token（视觉检查 light/dark）
- A7 `cd web && npm run typecheck && npm run lint && npm run build` 全绿；构建产物主 chunk < 200K
- A8 视觉对齐截图： 节点/服务器/用户列表页、设置页、新建节点/用户/服务器弹窗与 xboard 截图结构一致（人工核对）

## Out of Scope

- 后端 API 任何改动
- i18n 框架（保持中文硬编码）
- UI 组件库（Naive UI/Element Plus 等）、图表库
- 新功能（如真实的全局数据搜索、权限组管理等 xboard 有而本项目没有的功能）
- NodeFormDialog 的 765 行业务逻辑重构（仅做视觉外壳对齐）

## Key Decisions

- D1 视觉： 对标 xboard 截图（单色基调 + 黑主按钮 + 绿 toggle + 柔和徽章）——与现有 token 一致，不改品牌色
- D2 技术： 零 UI 库；唯一新依赖 `lucide-vue-next`；自研 ToggleSwitch/AppSelect/FilterChip/OverflowMenu 等小组件
- D3 范围： 一次全量改版 8 页

## Risks / Deferred

- AppSelect/OverflowMenu 的键盘导航与定位翻转需自研，质量风险中 —— 用 check 阶段专项验证
- TrafficChart 仅加轴线刻度，不重写；如需完整图表体验另立任务
- 移动端表格卡片化被 spec 明确禁止（directory-structure.md:51），不做
