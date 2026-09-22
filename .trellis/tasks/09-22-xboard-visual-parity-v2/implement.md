# Implement: 完全复刻 Xboard admin 视觉与布局

> 前置：`prd.md`（R1–R5 / AC1–AC8 / 方案 B）、`design.md`。复杂任务，`task.py start` 前需本
> 文件 + `implement.jsonl`/`check.jsonl` 就绪。
> 按阶段串行；每阶段结束跑一次验证命令。

## Progress Log

- **2026-09-22**：规划完成（PRD/design/implement）。尚未改动任何产品代码。
- **2026-09-22（实现）**：Phase 1–5 全部完成。
  - L0/L1：`tokens.css`（圆角 6/12/8、控件 36px、字号 xs/lg=20、中性阴影、`--radius-dialog/full`）、
    `base.css`（页面全宽 `24px 32px`、按钮新增 outline/ghost/icon 变体与 `shadow-sm`、卡片标题 tracking）。
  - L2：`StatCard` 重写为 Shadcn 统计卡（标题 + 右上图标 + 大数字 + hint，新增 `hint` prop）；
    新增 `ui/SegmentedControl.vue`；`PageHeader` 增加 `backTo/backTitle` 且移动端不再隐藏标题；
    `ModalDialog` 改 `radius-dialog` 并去掉头/尾分隔线与页脚底色；`DataTable` 表头去底色/吸顶、
    行高缩小，非多选表不再重复显示总数。
  - L3：`AppLayout` 重写为分组可折叠侧栏（仪表盘 / 用户管理 / 节点管理 / 系统管理），
    分组展开态 `localStorage['sidebar-groups']` 持久化 + 路由命中自动展开；折叠 56px 时
    分组图标弹出子项浮层（修复 `.nav` `overflow-y:auto` 裁剪浮层的问题）；顶栏移除页面标题
    （改由 `PageHeader` 呈现），详情页返回链接迁入 `PageHeader`。
  - L4：`LoginPage` 改为浅灰底 + 标题区 + 居中单卡 + 右上主题切换；`DashboardPage` 统计卡
    4 列栅格 + hint + `SegmentedControl`；`UserDetailPage`/`ServerDetailPage` 增加返回链接。
  - 验证：`npm run typecheck` / `lint` / `build` 全通过；`rg` 无旧蓝色残留；
    Playwright 截图核对 light/dark × 登录/仪表盘/用户/服务器/节点/设置/详情/弹窗/折叠浮层。
  - 备注：面板镜像内嵌 UI 需 `make build-embed` 后重建镜像方可在 Docker 中生效（本任务未改 Go）。
- **2026-09-22（trellis-check）**：校验 13 个文件，修复 7 项（1 MAJOR + 6 INFO）：
  ① `.theme-switch/.theme-option` 在 AppLayout 与 LoginPage 重复且漂移 → 上收 `base.css`；
  ② 删除 `DataTable` 不可达页脚分支；③ `SegmentedControl` 改泛型消除 Dashboard 的 `as` 强转；
  ④ 折叠态分组 `aria-expanded` 反映 popover 开合 + Esc 关闭；⑤ 硬编码值改 token
  （`--radius-full`/spacing/`btn.small` 32px）；⑥ 删除死 token `--content-max-width`；
  ⑦ `PageHeader` 补 `RouterLink` 显式导入。复跑 typecheck/lint/build 全过；
  Playwright 复截登录/仪表盘/用户确认无回归。AC1–AC8 全部满足。
- **2026-09-22（本机 Docker 生效）**：`APP_VERSION=$(git rev-parse --short HEAD) docker compose
  -f deploy/docker-compose.all-in-one.yml build panel` 重建 panel 镜像并 `up -d panel` 重建容器；
  agent 镜像无改动未重建。面板 healthy、`/healthz` 200，Playwright 直连 :8080 截图确认新 UI
  已随内嵌 dist 生效。
- **2026-09-22（导航回退为扁平）**：用户实看后要求「左侧菜单不用多一层」，`AppLayout.vue`
  移除分组/子项/popover 逻辑与样式，恢复扁平 5 项（保留 Xboard 视觉规格：通栏项、选中浅灰
  填充、折叠 56px + title）；同步修订 `prd.md` R2/AC2 与 `design.md` §5。typecheck/lint/build
  通过，重建 panel 镜像并 `up -d panel`，截图确认扁平侧栏已生效。
- **2026-09-22（列表页/弹窗对齐 shadcn-admin）**：参照 satnaing/shadcn-admin 的 users 表与
  弹窗结构重做三个列表页：
  - 新增 `ui/SearchInput.vue`（Search 图标 + 300ms 防抖 `search` 事件 + Enter）。
  - `DataTable`：移除 `#toolbar` slot、`.table-wrap`/`.table-toolbar` 与 card 出血；新增
    `bordered` prop（默认 `true` = `.table-box` 1px 边框 + `--radius-sm`）；表头 `h-40px` muted
    小字无底色、td `8px 12px`、行 hover/selected `--color-muted-soft`。
  - `TablePaginator`：改 shadcn-admin 布局（左侧「共 N 条」；右侧「每页显示 + 第 X/Y 页 +
    首/上/下/末 32px 图标按钮」）；移除数字分页。
  - `FilterChip`：改 shadcn outline 触发按钮（h32、`--radius-sm`、前景色、chevron 旋转）。
  - 列表页：工具栏移到表格上方、页面主体不再用 `.card` 包裹（改 `.table-card`）、移除页内
    `.search-input` 局部样式与独立「搜索」按钮、重置改 `btn ghost small` + X。
  - 卡内表格传 `:bordered="false"`（Dashboard / ServerDetail / UserDevices）。
  - trellis-check 修复 2 个 MAJOR：NodesPage 页内重复防抖（双请求）；SearchInput 对程序性
    赋值也触发防抖（破坏 `?page=` 深链、reset 双请求）。另清死代码 `.filters`。
  - 门禁 `typecheck/lint/build` 全过；重建 panel 镜像并 `up -d panel`，直连 :8080 截图确认。
- **2026-09-22（卡片对齐缺陷修复）**：用户反馈统计卡「不齐」。根因：base.css 全局
  `.card + .card { margin-top }` 作用到网格内卡片，同排卡片被下推 16px、首张被拉伸
  （实测 123px vs 107px，top 174 vs 190）。修复：base.css 增加
  `.stat-grid > .card + .card, .two-col > .card + .card { margin-top: 0 }`。复测
  仪表盘（6 卡全部 h=107、同 top）/ 服务器（5 卡全部 h=85、同 top）对齐；三门禁通过，
  重建 panel 镜像上线。spec `component-guidelines.md` 记录该约束。
- **2026-09-22（禁用态可辨识）**：用户反馈「编辑节点不能选协议了吗」。实为 `09-22-xboard-parity`
  的既有设计（`NodeFormDialog.vue:456` `:disabled="isEdit"`），但禁用控件的背景用了
  `--color-bg`（= 卡片白底），与可点击态无法区分。修复：`AppSelect` 与 `base.css` 的
  `:disabled` 改为 `--color-muted-soft` 背景 + 次要文字色，并降低 AppSelect 禁用态
  chevron/dot 透明度。**未改功能逻辑**（编辑时仍不可切换协议）。

## Phase 1 — L0 token + L1 基类（AC1、AC4 基础）

- [ ] `web/src/styles/tokens.css`：按 design §2 校准 light/dark 取值；新增
      `--radius-full`、`--shadow-sm`、`--font-size-xs`；`--radius-sm: 6px`、
      `--control-height: 36px`、卡片阴影中性化。
- [ ] `web/src/styles/base.css`：按钮基类与 variants（default/secondary/**outline**/**ghost**/
      danger）、尺寸（small/icon）；输入框/select/textarea；`.card`/`.card-title`；表格基类；
      `.kbd`/`.chip`/`.field` 按 design §3 对齐。
- 验证：`cd web && npm run typecheck && npm run lint && npm run build`。
- 回滚点：两个文件独立，可单独 `git checkout --`。

## Phase 2 — L2 通用组件（AC4、AC5）

- [ ] `ui/StatCard.vue`：改为 Shadcn 统计卡结构（标题 + 右上图标 + 大数字 + 说明/涨幅），
      新增 `hint?`/`delta?` props；保留 `tone`。
- [ ] 新增 `ui/SegmentedControl.vue`（`items` + `v-model`，浅灰槽 + 选中黑底白字）。
- [ ] `ui/PageHeader.vue`：字号/间距校准；**取消移动端隐藏**；新增可选 `backTo`/`backTitle`。
- [ ] `DataTable.vue`：表头 muted 小字、行 hover `muted/50`、边框对齐；`TablePaginator` 同步。
- [ ] `ModalDialog.vue` / `ConfirmDialog.vue`：圆角/间距/遮罩对齐 shadcn。
- [ ] `StatusBadge` / `ErrorBanner` / `EmptyState` / `LoadingSpinner` / `OverflowMenu` /
      `FilterChip` / `ToggleSwitch` / `AppSelect` / `CopyText` / `OneTimeSecret` /
      `NodeChecklist` / `MetricBar` / `ProgressBar` / `TrafficChart`：消费新 token/基类，
      清除残留旧色与硬编码 hex。
- 验证：`cd web && npm run typecheck && npm run lint && npm run build`。
- 风险文件：`DataTable.vue`（多处引用）、`ui/StatCard.vue`（Dashboard 依赖 props）。

## Phase 3 — L3 应用外壳（AC2、AC3）

- [ ] `components/AppLayout.vue`：改用 `navGroups` 分组模型（方案 B：仪表盘 / 用户管理 /
      节点管理 / 系统管理），单项分组渲染为直链。
- [ ] 分组展开/收起 + `localStorage['sidebar-groups']` 持久化 + 路由变化自动展开。
- [ ] 折叠态（56px）：单项 icon+title；多子项分组用轻量 popover 列出子项。
- [ ] 顶栏：移除标题与 parent 链接；保留搜索触发器 + 主题切换 + 用户入口；保留移动端
      `mobile-nav`。
- [ ] 详情页返回入口迁移（`PageHeader` backTo 或页面内实现）：`UserDetailPage`、
      `ServerDetailPage`。
- 验证：`cd web && npm run typecheck && npm run lint && npm run build`；light/dark 目检侧栏。
- Review Gate：外壳（侧栏分组/折叠/顶栏）自查通过后再进入页面适配。
- 风险文件：`AppLayout.vue`（结构性改动）。

## Phase 4 — L4 全部页面（AC5、AC6、AC7）

- [ ] `LoginPage.vue`：浅灰底居中单卡 ~420px + 右上主题切换。
- [ ] `DashboardPage.vue`：统计卡改 4 列栅格；流量卡用 `SegmentedControl`；清残留样式。
- [ ] 列表页：`UsersPage` / `ServersPage` / `NodesPage` 工具栏 + 表格 + 分页样式对齐。
- [ ] 详情页：`UserDetailPage` + `components/user/*`、`ServerDetailPage` 页头 + 分区卡对齐。
- [ ] `SettingsPage.vue`、`NotFoundPage.vue`：对齐。
- [ ] 表单弹窗：`UserCreateDialog` / `ServerFormDialog` / `NodeFormDialog`：
      field/按钮/间距/校验提示对齐。
- 验证：`cd web && npm run typecheck && npm run lint && npm run build`。

## Phase 5 — 收尾核对（AC7、AC8）

- [ ] `rg` 全仓扫描残留旧色/硬编码 hex：`#2563eb|#1d4ed8|#f5f7fa|#172033|#eff6ff|#bfdbfe`
      及误用的 `--color-primary-soft`。
- [ ] light/dark × 全部页面人工目检（对照参考截图）。
- [ ] 全量：`go build ./...`（确保未误伤）、`cd web && npm run typecheck && npm run lint && npm run build`。
- [ ] 更新 spec（如新增组件/约定）与 journal。

## 验证命令汇总

```bash
cd web && npm run typecheck
cd web && npm run lint
cd web && npm run build
```

## Review Gates

- Phase 3 完成后：外壳结构自查（分组展开/折叠/持久化/顶栏）再继续。
- Phase 5：最终全量核对（对应 AC1–AC8）。

## Rollback Points

- 每个 Phase 为独立文件集合，可单独 `git checkout -- <file>`。
- 全量回滚：`git revert` 本任务提交。

## task.py start 前检查

- [ ] `prd.md` 已通过收敛（无阻塞 open question；AC 可测）。
- [ ] `design.md` / `implement.md` 与 PRD 一致。
- [ ] `implement.jsonl` / `check.jsonl` 含真实 spec/research 条目。
- [ ] 用户已审阅并批准本规划。
