# 实施计划：前端样式重设计 + 暗色模式

## 执行顺序

### 1. Token 层（基础，先做）
- [ ] 重构 `styles/tokens.css`：原始色板 + 语义 token 双层结构；新增 `--color-on-primary`、`--color-danger-hover`、`--color-warning-border`、`--color-success-border`、`--color-danger-border` 等缺口 token；新增 `[data-theme='dark']` 全套覆盖；写入 `color-scheme`。
- [ ] 验证：dev 下浅色外观不劣化（token 同名替换）。

### 2. 主题机制
- [ ] 新增 `stores/theme.ts`（light/dark/system + localStorage + matchMedia 监听）。
- [ ] `main.ts`：启动时同步应用主题，防首屏闪烁。
- [ ] `AppLayout.vue` 顶栏加主题切换按钮（内联 SVG，currentColor）。

### 3. 硬编码颜色清理
- [ ] 8 个文件 13 处字面量 → 语义 token（清单见 design.md）。
- [ ] `base.css` 中 `.btn` 文字色、danger hover 色 → token。
- [ ] 验证：`grep -rn "#[0-9a-fA-F]\{3,6\}" web/src --include="*.vue"` 无业务颜色残留。

### 4. 视觉升级
- [ ] `base.css`：卡片/按钮/输入/表格/焦点态打磨。
- [ ] `AppLayout.vue`：侧栏导航加 SVG 图标、active 指示条、品牌区精炼。
- [ ] `LoginPage.vue`：居中卡片 + 品牌区重设计。
- [ ] 共享组件走查微调：DataTable、StatusBadge、MetricBar、ProgressBar、ModalDialog、TablePaginator、TrafficChart（SVG 已用 token，确认暗色表现）。

### 5. 全量验证
- [ ] `npm run typecheck && npm run lint && npm run build` 全绿。
- [ ] dev 手动走查：8 页面 × 亮/暗 × 宽/窄屏；主题切换持久化；system 跟随。
- [ ] 对话框 aria 属性未回归。

## 回滚点

- 每阶段完成即可独立提交/回滚；token 层（步骤 1）与视觉层（步骤 4）解耦，token 层单独回滚不影响功能。

## 质量门槛

- typecheck / lint / build 三绿才可进入步骤 5。
- 禁止引入新依赖、禁止改动 api/ 与 stores/auth.ts。
