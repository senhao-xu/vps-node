# 参考 Xboard 风格重做 Web 面板样式与布局

## Goal

将 `web/` 管理面板的视觉风格整体重做，参考公开的 Xboard 管理后台（shadcn-admin 风格：极简黑白灰、近黑主色、白卡片 + 浅灰填充、可折叠侧边栏），覆盖全部页面与整体布局，同时保持现有 Vue3 + 自定义 CSS 变量的技术栈不变。

## Background

- 当前样式为蓝色主色（#2563EB）+ 浅灰蓝背景的传统 admin 风格，用户不满意。
- 研究结果（`research/xboard-admin-style.md`）：Xboard admin 基于 shadcn-admin 模板（React + shadcn/ui + Tailwind），核心特征：
  - 主色为近黑 `#0F172A`（dark 下反转为近白），无品牌彩色
  - 页面白底，卡片白底 + 1px border（#E2E8F0）+ 轻阴影 + 12px 圆角
  - 次级填充统一浅灰 `#F1F5F9`（选中态、hover、muted 区）
  - 侧边栏 256px（折叠 56px）白底 + 右侧分隔线，菜单选中 = 浅灰填充，右缘悬浮圆形折叠钮，底部版本号
  - 顶栏 64px：左侧大号页标题，右侧搜索/主题/用户
  - 按钮主色黑底白字、hover 90% 透明度、圆角 6px、font-medium
  - 登录页 `#F8FAFC` 浅灰底居中单卡（约 420px）

## Requirements

1. **设计 tokens 重做**（`styles/tokens.css`）：按 Xboard/shadcn 中性主题重定义色板（primary 近黑、muted #F1F5F9、border #E2E8F0、destructive #EF4444 等），light/dark 两套；圆角（控件 6-8px、卡片 12px）、阴影改为中性轻阴影。
2. **布局重做**（`components/AppLayout.vue`）：
   - 侧边栏 256px 白底 + 右边线，菜单选中态为浅灰填充（去掉蓝色高亮与左侧竖条）
   - 侧边栏支持折叠到 56px（仅图标），折叠状态 localStorage 持久化，右缘悬浮圆形折叠按钮
   - 侧边栏底部显示版本号：使用构建时注入的 git commit 短 sha（Vite `define` 注入 `__APP_VERSION__`），配合绿点展示
   - 顶栏 64px，左侧大号页面标题，右侧主题切换 + 用户信息 + 退出
   - 内容区白底，移动端响应式保持可用
3. **基础组件样式重做**（`styles/base.css`）：按钮（主按钮黑底白字 hover 90%）、输入框、卡片、表格相关基类按新 tokens 调整。
4. **全部页面适配**：Dashboard / Users / UserDetail / Servers / ServerDetail / Nodes / Settings / Login，以及所有 `components/` 下组件（DataTable、StatusBadge、ModalDialog、MetricBar 等），使其在新 token 体系下视觉一致；登录页改为浅灰底居中单卡。
5. **语义色保留**：成功绿 / 警告黄 / 危险红 / 离线灰作为点缀保留（状态徽章、图表等），不引入品牌彩色主色。

## Constraints

- 不引入 Tailwind / shadcn / 任何新 UI 依赖；沿用现有 CSS 变量 + scoped style 方案。
- 不改任何 API 调用、路由、业务逻辑；纯样式与布局结构（AppLayout DOM）调整。
- 保持 light / dark / system 三态主题切换可用，dark 配色按 shadcn dark 方案（bg #020817、卡片/边框 #1E293B、primary 反白）。
- 文案、菜单结构不变。

## Acceptance Criteria

- [ ] tokens.css light/dark 色板与 Xboard 参考值一致（primary #0F172A / dark 反白、muted #F1F5F9、border #E2E8F0 等）
- [ ] 侧边栏 256px/56px 可折叠，选中项浅灰填充，折叠状态刷新后保持
- [ ] 顶栏 64px 大号页标题；内容区白底卡片 12px 圆角轻阴影
- [ ] 主按钮黑底白字、hover 不透明度变化；登录页居中单卡浅灰底
- [ ] 全部 8 个页面在 light 与 dark 下无样式破损（无残留蓝色主色、对比度可读）
- [ ] `npm run build` 通过，既有 lint/typecheck 命令通过

## Notes

- 研究依据：`.trellis/tasks/09-21-xboard-style-revamp/research/xboard-admin-style.md`
- 技术设计见 `design.md`，执行计划见 `implement.md`
