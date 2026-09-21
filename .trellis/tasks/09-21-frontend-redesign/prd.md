# 前端整体样式重设计（现代简约 + 暗色模式）

## 背景

当前前端（web/，Vue 3 + 手写 CSS）视觉风格存在不足：配色与层次平淡、侧栏/顶栏质感一般、无暗色模式。用户要求全站统一重设计，方向为「现代简约（浅色为主）」，并支持暗色模式切换。

## 目标

1. 全站视觉升级为现代简约风格：更精致的配色层级、阴影、圆角、间距、排版与微交互。
2. 支持亮 / 暗主题切换，并跟随系统偏好（light / dark / system 三档），选择持久化到 localStorage。
3. 不引入任何 UI 框架或新的运行时依赖，沿用现有 tokens.css + base.css 的设计 token 体系。

## 范围

- `web/src/styles/tokens.css`、`base.css`：token 体系重构 + 暗色变体。
- `web/src/components/AppLayout.vue`：侧栏、顶栏、品牌区重设计；顶栏加主题切换控件。
- `web/src/pages/LoginPage.vue`：登录页重设计。
- 所有页面与共享组件：消除硬编码颜色（DashboardPage、ServerDetailPage、ErrorBanner、ModalDialog、StatusBadge、NodeFormDialog、AppLayout、LoginPage），改用语义 token。
- 共享组件视觉打磨：卡片、按钮、表格、对话框、徽章、分页器等。

## 非目标

- 不改动任何 API 调用、数据流、路由与业务逻辑。
- 不引入 UI 组件库、图标库、CSS 框架（图标使用内联 SVG）。
- 不改后端。

## 验收标准

1. `npm run typecheck`、`npm run lint`、`npm run build` 全部通过（web/ 目录）。
2. 顶栏提供主题切换（亮/暗/跟随系统），刷新后保持选择；暗色模式下全站无刺眼的未适配颜色（无残留硬编码亮色）。
3. `grep -rn "#[0-9a-fA-F]{3,6}" web/src --include="*.vue"` 不再出现业务颜色字面量（SVG 图标 currentColor 除外）。
4. 所有页面（登录、仪表盘、用户、用户详情、服务器、服务器详情、节点、设置）在亮/暗两种模式下视觉一致、层级清晰。
5. 移动端窄屏布局不回归（侧栏折叠为顶部导航的现有行为保留）。
6. 对话框可访问性属性（role/aria-modal/aria-labelledby）保持不变。
