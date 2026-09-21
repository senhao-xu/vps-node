# Implement: 参考 Xboard 风格重做 Web 面板样式与布局

执行顺序按依赖分层，每层完成后可运行 build 验证。

## Checklist

1. [ ] **tokens.css 重做**：按 `design.md` Token 映射表重写 light + dark 两套变量；新增 `--shell-width-collapsed: 56px`；中性 focus-ring；中性卡片阴影。
2. [ ] **base.css 基础组件**：`.btn` 主按钮黑底白字 + hover opacity .9（去彩色阴影/translateY）；secondary/danger 变体适配；输入框 focus-ring；`.card` 12px 圆角轻阴影；`.page-title`。
3. [ ] **AppLayout.vue 布局**：
   - [ ] 侧栏 256px、白底 + 右边线；菜单 h-12、选中 = 浅灰填充 + 文字 600（删左侧竖条与蓝字）
   - [ ] 折叠功能：`collapsed` ref + localStorage `sidebar-collapsed` 持久化 + 右缘悬浮圆形折叠钮；折叠态宽 56px 仅图标（label 隐藏 + title）
   - [ ] 侧栏底部版本区：绿点 + 版本号 = git commit 短 sha（`vite.config.ts` 用 `define` 注入 `__APP_VERSION__`，`execSync('git rev-parse --short HEAD')`，失败回退 `'dev'`；`src/env.d.ts` 补类型声明）
   - [ ] 顶栏 64px 大号页标题
   - [ ] 移动端 ≤900px 保持横条行为、折叠不生效
4. [ ] **LoginPage.vue**：浅灰底（surface-muted）居中单卡 ~420px。
5. [ ] **DashboardPage.vue + MetricBar/TrafficChart/ProgressBar**：图表/进度条主色近黑化；统计卡大数字粗体。
6. [ ] **DataTable.vue / TablePaginator.vue**：行 hover muted 浅灰、表头 muted 小字。
7. [ ] **列表/详情页**：Users / UserDetail(+子组件) / Servers / ServerDetail / Nodes / Settings — 替换蓝色强调引用，核对选中/focus 态。
8. [ ] **其余组件**：StatusBadge / ModalDialog / ConfirmDialog / ErrorBanner / CopyText / OneTimeSecret / NodeChecklist / App.vue — `rg "#2563EB|#1d4ed8|primary-soft"` 全仓扫描残留蓝色与误用。
9. [ ] **dark 模式全页面目检**：8 页面 × light/dark 对照验收标准。

## 验证命令

```bash
cd web && npm run build
# lint/typecheck（若 package.json 提供）
cd web && npm run lint 2>/dev/null; cd web && npm run typecheck 2>/dev/null
```

## Review Gates

- 步骤 3 完成后：布局结构（侧栏/折叠/顶栏）自查一次再继续页面适配。
- 步骤 9 为最终全量核对（对应验收标准清单）。

## Rollback Points

- 每步为独立文件集合，可单独 `git checkout -- <file>` 回滚。
- 全量回滚：`git revert` 本任务提交。

## Context Manifests

- `implement.jsonl`：frontend spec + 研究结论摘要（供 trellis-implement）。
- `check.jsonl`：prd 验收标准 + design token 映射表（供 trellis-check）。
