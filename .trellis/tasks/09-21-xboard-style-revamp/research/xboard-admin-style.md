# Research: Xboard 管理后台（admin）前端视觉风格

- **Query**: 研究 cedar2025/Xboard 管理后台前端的视觉风格，为样式重做提供设计参考
- **Scope**: external（GitHub 仓库 + 编译产物 CSS/JS bundle 逆向分析 + README 截图）
- **Date**: 2026-09-21

## 0. 关键结论：源码不公开，只有编译产物

- 主仓库 [cedar2025/Xboard](https://github.com/cedar2025/Xboard) 的 `.gitmodules` 声明：
  ```
  [submodule "public/assets/admin"]
      path = public/assets/admin
      url = https://github.com/cedar2025/xboard-admin-dist.git
  ```
- **admin 前端源码未开源**，公开的是编译产物仓库 [cedar2025/xboard-admin-dist](https://github.com/cedar2025/xboard-admin-dist)（main 分支，40 commits）。本报告的所有布局/样式细节均从该 dist 的 `assets/index-*.css`（283KB）与 `assets/index-*.js`（6.5MB）中逆向提取，并结合 README 截图 `docs/images/admin.png` 交叉验证。
- `index.html` 的 meta description 为 "Admin Dashboard UI built with Shadcn and Vite."——与开源模板 [satnaing/shadcn-admin](https://github.com/satnaing/shadcn-admin) 的描述逐字一致；且布局代码（`--header-height`、`Layout/LayoutHeader/LayoutBody` 组件、可折叠 sidebar `md:w-14/md:w-64`、右侧悬浮折叠按钮）与该模板高度吻合。**Xboard admin 极可能是基于 satnaing/shadcn-admin 模板二次开发**。

## 1. 技术栈

| 项 | 结论 | 证据 |
|---|---|---|
| 框架 | React 18 + Vite（SPA，`#root` 挂载，React Router lazy routes） | index.html / JS bundle |
| UI 方案 | shadcn/ui（Radix UI primitives + class-variance-authority 风格 variants） | CSS 中 `@keyframes slideDown/slideUp`（radix-collapsible）、CVA variant 类串 |
| CSS | **Tailwind CSS v3**（HSL CSS 变量主题 `hsl(var(--x))`、`.dark` 类切换、tailwind preflight） | `index-DiYa-_z_.css` |
| 其他 | TanStack Query（`queryKey`/`isStale` 等）、Monaco Editor（codicon/vscode 变量，用于 JSON 编辑）、i18n（en-US/zh-CN/ru-RU，locales/*.js 全局脚本加载） | bundle / index.html |
| 配置注入 | `window.settings`（title/description/version），通过 `/settings.js`、`/settings.local.js` 注入 | index.html |

用户端前端是另一套：Vue3 + TypeScript + NaiveUI（独立仓库 cedar2025/xboard-user），不在本研究范围。

## 2. 布局结构

### 整体骨架（源码证据）

```jsx
<div class="relative h-full overflow-hidden bg-background">
  <aside class="fixed left-0 right-0 top-0 z-50 flex h-auto flex-col
                border-r-2 border-r-muted transition-[width]
                md:bottom-0 md:right-auto md:h-svh md:w-64|md:w-14">…</aside>
  <main id="content" class="overflow-x-hidden pt-16 transition-[margin]
                md:overflow-y-hidden md:pt-0 md:ml-64|md:ml-14 h-full">…</main>
</div>
```

### 侧边栏 Sidebar

- **宽度**：展开 `md:w-64`（256px），折叠 `md:w-14`（56px），`transition-[width]` 动画；折叠状态持久化（key `collapsed-sidebar`）。
- **颜色**：浅色风格。无独立背景色类（继承 `bg-background` 白色），右侧 `border-r-2 border-r-muted` 浅灰分隔线。（注意：README 老截图中侧栏呈浅灰底，当前 dist 代码侧栏为白底+灰分隔线。）
- **Logo 区**：顶部 `sticky top-0 justify-between px-4 py-3 shadow`；左侧为两条粗线组成的 "X" SVG（viewBox 256×256，`stroke="currentColor"`，strokeWidth 16），展开 `h-8 w-8`、折叠 `h-6 w-6`；旁边是 `window.settings.title`（"Xboard"，`font-medium`）。
- **菜单项**：shadcn Button 改写为 NavLink——`h-12 justify-start text-wrap rounded-none px-6`；**选中态 = variant `secondary`**（浅灰底 `hsl(210 40% 96.1%)`），未选中 = `ghost`（透明，hover 出浅灰）；折叠态为 `h-12 w-12` 图标按钮 + Tooltip/Popover 展开子菜单。子菜单项 `h-10 w-full border-l border-l-slate-500 px-2`（左侧竖线缩进）。
- **分组**：顶级菜单（Dashboard / System Management / Node Management / Subscription / User Management 等）带折叠 chevron，子项缩进排列（见截图）。
- **底部**：`border-t border-border/50`，绿色圆点（`h-1.5 w-1.5 rounded-full bg-green-500`）+ 版本号 `v1.0.0`（`text-xs text-muted-foreground`）。
- **折叠按钮**：悬浮在侧栏右缘 `absolute -right-5 top-1/2 rounded-full` 的 outline 圆形图标按钮（«/» 旋转切换）。
- **移动端**：侧栏变为顶部抽屉，遮罩 `bg-black opacity-50`。

### 顶栏 LayoutHeader

- 组件类名：`flex h-[var(--header-height)] flex-none items-center gap-4 bg-background p-4 md:px-8`
- **高度 `--header-height: 4rem`（64px）**，白底 `bg-background`。
- 内容（截图自上而下验证）：左侧为大号页面标题（如 "Dashboard"，`text-2xl/3xl font-bold tracking-tight` 风格）；右侧依次为：搜索框（placeholder "Search menus and functions..."，带 `⌘K` kbd 徽章，触发 cmdk 命令面板）、主题切换（月亮/太阳图标，切换 `.dark` 类）、语言切换（国旗 + "EN"）、用户头像（截图中为亮绿色圆形头像）。

### 内容区 LayoutBody

- `flex-1 overflow-hidden px-4 py-6 md:px-8`，白底。
- 页面由白卡片组成：`rounded-xl border bg-card text-card-foreground shadow`（shadcn Card 默认）；统计卡片变体 `rounded-xl border bg-card px-3 py-2.5 shadow-sm` 与 `rounded-xl border bg-card/50 p-4`。
- Dashboard（截图）：2 行 × 4 列统计卡（大数字 `text-2xl font-bold` + muted 说明 + 彩色小图标 + 绿色涨幅文字如 "+15.8% vs Yesterday"），下方大卡为 Revenue Overview 面积图（黑线+灰色渐变填充，Recharts 风格），右上角分段切换 "Amount | Count"（选中段为黑底白字 pill）。

## 3. 设计 Tokens（从 dist CSS `:root` 原样提取）

shadcn/ui 经典中性（zinc/slate 系）主题，HSL 分量写法，`--radius: 0.5rem`：

### Light `:root`

| Token | HSL | 约合 HEX |
|---|---|---|
| `--background` | `0 0% 100%` | `#FFFFFF` |
| `--foreground` | `222.2 84% 4.9%` | `#020817` |
| `--card` / `--popover` | `0 0% 100%` | `#FFFFFF` |
| **`--primary`** | **`222.2 47.4% 11.2%`** | **`#0F172A`（近黑的深 slate）** |
| `--primary-foreground` | `210 40% 98%` | `#F8FAFC` |
| `--secondary` / `--muted` / `--accent` | `210 40% 96.1%` | `#F1F5F9` |
| `--muted-foreground` | `215.4 16.3% 46.9%` | `#64748B` |
| `--destructive` | `0 84.2% 60.2%` | `#EF4444` |
| `--border` / `--input` | `214.3 31.8% 91.4%` | `#E2E8F0` |
| `--ring` | `222.2 84% 4.9%` | `#020817` |
| `--radius` | `0.5rem` | 8px |

### Dark `.dark`

| Token | HSL | 约合 HEX |
|---|---|---|
| `--background` | `222.2 84% 4.9%` | `#020817` |
| `--foreground` | `210 40% 98%` | `#F8FAFC` |
| `--primary` | `210 40% 98%`（反转为近白） | `#F8FAFC` |
| `--secondary`/`--muted`/`--accent`/`--border`/`--input` | `217.2 32.6% 17.5%` | `#1E293B` |
| `--muted-foreground` | `215 20.2% 65.1%` | `#94A3B8` |
| `--destructive` | `0 62.8% 30.6%` | `#7F1D1D` |
| `--ring` | `212.7 26.8% 83.9%` | `#CBD5E1` |

### 其他 tokens

- **圆角**：`--radius: .5rem`；按钮/输入 `rounded-md`（6px 等效 `calc(.5rem - 2px)`）、卡片 `rounded-xl`（12px）、头像/折叠钮 `rounded-full`。
- **阴影**：卡片 `shadow`/`shadow-sm`；按钮 `shadow`；侧栏 Logo 区 `shadow`。无彩色阴影。
- **字体**：Tailwind 默认 `font-sans` 系统字体栈（无自定义 webfont）；代码 `ui-monospace, SFMono-Regular, Menlo, Consolas…`。正文字号 `text-sm`（14px）为主，辅助文字 `text-xs`，页标题 `text-2xl~3xl font-bold`，卡标题 `text-xl font-semibold tracking-tight`。
- **辅助/语义色（散见于组件）**：成功绿 `bg-green-500`（版本点、涨幅文字绿色）、`border-l-slate-500`（子菜单竖线）；统计卡图标使用绿/蓝/橙等点缀色。**无品牌彩色 primary——主色就是"近黑"**，整体为极简黑白灰 + 少量语义色点缀。
- 背景层级：页面 `bg-background`（白）→ 卡片 `bg-card`（白 + border + shadow 区分）→ 次级填充 `bg-muted`/`bg-muted/50`/`bg-muted/20`（`#F1F5F9` 系灰）。

## 4. 组件样式特点

### 表格 / 列表

- shadcn Table：行 `border-b transition-colors hover:bg-muted/50 data-[state=selected]:bg-muted`；表脚 `border-t bg-muted/50 font-medium`。
- 表头为 muted 小字；列表页普遍顶部搜索输入（placeholder "搜索…"/"Search..."）+ 右上主按钮（黑底白字）。
- 统计/概览区用白卡片栅格，数字大而粗，辅助信息 `text-muted-foreground`。

### 按钮（shadcn Button variants）

- 基类：`inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:ring-1 focus-visible:ring-ring disabled:opacity-50`。
- **default（主按钮）：`bg-primary text-primary-foreground shadow hover:bg-primary/90`** → 深近黑底（#0F172A）白字，深色主题下反转为白底黑字。
- secondary：`bg-secondary` 浅灰底；ghost：透明 hover 浅灰；outline：边框式（用于侧栏折叠圆钮等）；destructive：红 `#EF4444`。
- 尺寸：`sm`/`default`/`icon`；侧栏菜单项复用 `secondary`/`ghost` variant 表达选中/未选中。

### 登录页（路由 `/sign-in`）

- 居中单列卡片布局：`container relative flex min-h-svh flex-col items-center justify-center bg-primary-foreground px-4 py-8`（背景为 `#F8FAFC` 极浅灰）。
- 卡片容器 `mx-auto flex w-full flex-col justify-center space-y-6 sm:w-[350px] md:w-[420px]`。
- 卡上方居中：站点标题（`text-2xl font-bold sm:text-3xl`，取自 `window.settings.title`）+ muted 副标题。
- 白卡片 `p-4 sm:p-6` 内：左对齐 "Sign In"（`text-xl font-semibold tracking-tight`）+ 描述（"Enter your email and password to sign in"）+ 邮箱/密码表单 + 忘记密码弹窗。
- 右上角悬浮主题/语言切换（`absolute right-4 top-4`）。

## 5. 来源与引用

- 主仓库：https://github.com/cedar2025/Xboard （README：Tech Stack 明确 "Admin Panel: React + Shadcn UI + TailwindCSS"）
- admin 编译产物（本研究主要证据源）：https://github.com/cedar2025/xboard-admin-dist
  - CSS tokens：https://raw.githubusercontent.com/cedar2025/xboard-admin-dist/main/assets/index-DiYa-_z_.css
  - JS bundle：https://raw.githubusercontent.com/cedar2025/xboard-admin-dist/main/assets/index-CEIYH7i8.js
  - 入口 HTML（meta description 指向 shadcn-admin 模板）：https://raw.githubusercontent.com/cedar2025/xboard-admin-dist/main/index.html
  - i18n（菜单/登录文案）：https://raw.githubusercontent.com/cedar2025/xboard-admin-dist/main/locales/zh-CN.js 、en-US.js
- admin 界面截图：https://github.com/cedar2025/Xboard/blob/master/docs/images/admin.png
- 疑似上游模板（高度吻合，供进一步参考布局细节）：https://github.com/satnaing/shadcn-admin
- 用户端前端（Vue3 + NaiveUI，另一套风格）：https://github.com/cedar2025/xboard-user

## 6. Caveats / Not Found

- **admin 源码未公开**，所有实现细节来自 minified bundle 逆向，组件名为编译后混淆名（本文已还原语义）；不排除个别细节与运行时渲染有出入。
- README 截图（admin.png）可能滞后于当前 dist：截图中侧栏为浅灰底，当前代码侧栏为白底 + `border-r-2`；以代码为准。
- 统计卡图标的具体彩色色值（绿/蓝/橙）未逐一提取，截图可见为 Tailwind 语义色级别（green/blue/orange-500 系）。
- bundle 内 `#007acc` 等高频 hex 均属 Monaco Editor 主题色，与 admin UI 无关。
