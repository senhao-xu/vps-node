# 设计：前端样式重设计 + 暗色模式

## 总体思路

以现有设计 token 体系为基础做「双层 token」重构：

- **Layer 1 — 原始色板（primitive）**：`tokens.css` 中定义中性灰阶、品牌蓝阶、语义色阶（success/warning/danger）的原始值，亮暗两套。
- **Layer 2 — 语义 token（semantic）**：`--color-bg`、`--color-surface`、`--color-text`、`--color-border` 等现有名字保持不变，值改为引用原始色板。组件代码零改动即可换肤。

暗色通过 `<html data-theme="dark">` 切换：`:root` 定义浅色，`[data-theme='dark']` 覆盖同名语义 token。所有组件已使用语义 token，因此暗色适配的主要工作是把残留的硬编码颜色字面量替换为 token。

## 主题切换机制

- 新增 `web/src/stores/theme.ts`（pinia）：
  - state: `mode: 'light' | 'dark' | 'system'`，初始读 `localStorage('vps-node-theme')`，缺省 `'system'`。
  - `resolved` computed：`system` 时读 `window.matchMedia('(prefers-color-scheme: dark)')`。
  - `apply()`：把解析结果写到 `document.documentElement.dataset.theme`；监听 matchMedia change（system 模式下自动跟随）。
  - 组件挂载前在 `main.ts` 里同步执行一次初始 apply，避免首屏闪烁。
- 顶栏右侧加 `ThemeToggle`（内联在 AppLayout 或独立小组件）：循环切换 light → dark → system，使用内联 SVG 图标（太阳/月亮/显示器），`currentColor` 着色。
- `color-scheme: light dark` 按主题写入 `:root`/`[data-theme]`，让原生表单控件、滚动条跟随。

## 视觉升级要点（现代简约浅色）

1. **色板**：中性色从偏蓝灰（slate 系）微调为更干净的灰阶；品牌色保持蓝色系但微调饱和度；soft 色（badge/提示底色）统一从色板派生。
2. **层次**：页面背景与卡片背景拉开明度差；卡片阴影更轻更散（双层柔和阴影）；边框色分级（默认/强调）。
3. **侧栏**：品牌区精炼；导航项加内联 SVG 图标（仪表盘/用户/服务器/节点/设置），active 态用左侧指示条 + soft 底色；悬停过渡统一。
4. **顶栏**：保留 sticky；右侧为主题切换 + 用户名 + 退出。
5. **按钮/输入**：统一 control-height；hover 微浮起改为更克制的阴影变化；focus ring 保持现有 token。
6. **表格**：表头使用 surface-muted 底色 + 小号大写字距风格；行 hover 用 token；斑马纹不加（保持简约）。
7. **登录页**：居中卡片 + 品牌标识 + 背景细微渐变/网格纹（纯 CSS，暗色下同样优雅）。
8. **暗色**：背景用近黑灰（非纯黑），surface 抬一级；soft 色在暗色下用低透明度的语义色叠加；阴影在暗色下减弱、边框增强。

## 硬编码颜色清理清单（13 处 / 8 文件）

- `pages/DashboardPage.vue`：`#fde68a`（attention pill 边框）→ 新增 `--color-warning-border` token。
- `pages/LoginPage.vue`：2 处 → token。
- `pages/ServerDetailPage.vue`：1 处 → token。
- `components/ErrorBanner.vue`、`ModalDialog.vue`、`AppLayout.vue`（`#fff` 等）、`StatusBadge.vue`、`NodeFormDialog.vue`：各 1–2 处 → token。
- `base.css` 中 `.btn` 的 `#fff` 文字、`.btn.danger:hover` 的 `#b91c1c` → 新增 `--color-danger-hover`、`--color-on-primary` token。

## 受影响文件

| 文件 | 改动 |
|---|---|
| `styles/tokens.css` | 重构为双层 token + `[data-theme='dark']` 覆盖 |
| `styles/base.css` | 用新 token 打磨按钮/输入/卡片/表格基元 |
| `stores/theme.ts` | 新增主题 store |
| `main.ts` | 启动时应用主题 |
| `components/AppLayout.vue` | 侧栏/顶栏重设计 + 主题切换 + 导航图标 |
| `components/LoginPage.vue` 等 8 个文件 | 硬编码颜色 → token |
| 其余页面/组件 | 仅视觉走查微调（必要时） |

## 兼容与回滚

- 纯 CSS + 一个 store，无 API 变化；回滚 = revert 对应文件。
- localStorage key 新增 `vps-node-theme`，读取失败时回退 `system`。
- SSR 无（纯 SPA），matchMedia 在浏览器环境可用；`main.ts` 顶层执行安全。

## 验证

- `npm run typecheck && npm run lint && npm run build`（web/）。
- 手动走查：`npm run dev`，亮/暗/system 三档切换 + 刷新持久化；8 个页面两种主题；窄屏（<900px、<600px）布局。
- grep 检查无残留硬编码颜色。
