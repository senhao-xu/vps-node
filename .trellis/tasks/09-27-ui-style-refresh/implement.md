# Implement: 参考 HHUB 风格的全局 UI 样式焕新

## Ordered Checklist

1. **tokens.css 焕新**
   - [ ] palette 换为中性灰阶；`--color-primary` 系改近黑（light）/近白（dark）
   - [ ] 圆角体系放大（新增 `--radius-lg`，卡片/弹窗引用）
   - [ ] 阴影柔和化（`--shadow-card`、`--shadow-dialog`）
   - [ ] 新增 purple 徽章 token（含 dark 覆盖）
   - [ ] `--color-shell*` 改浅色 sidebar 语义（light 白/浅灰，dark 深灰），dark 全套同步
   - [ ] `--color-focus-ring` 改为中性黑透明度
2. **base.css 基础组件**
   - [ ] `.card`：lg 圆角、20-24px padding、柔和阴影、移除 hover 边框加深
   - [ ] `.btn` 系：主按钮近黑、圆角 8-10px；outline/secondary 白底灰边
   - [ ] 输入框/select/`.chip`/`.toggle-row`/`.theme-switch`/`.kbd` 圆角与边框同步
   - [ ] `.section-kicker` 颜色改中性（原 teal）
3. **AppLayout.vue 浅色 sidebar**
   - [ ] 移除硬编码 `#2dd4bf`/`#5eead4`，brand-mark 改近黑白字
   - [ ] nav-item 激活态：浅灰圆角块 + 近黑文字；侧边指示条改近黑或移除
   - [ ] sidebar-status 文字色适配浅色背景
   - [ ] 验证移动端 drawer 正常
4. **StatusBadge + 徽章体系**
   - [ ] 徽章圆角改胶囊（`--radius-full`），padding 微调
   - [ ] 新增 `purple` tone（协议徽章），更新 `utils/labels.ts` 的 Tone 类型
5. **重点页面精调**
   - [ ] DashboardPage：MetricStrip 扩展或卡片化统计区（大数字 + 底部彩色条，参考截图）
   - [ ] NodesPage：协议徽章用 purple tone；检查表格工具栏视觉
   - [ ] NodeFormDialog：分段选择器/弹窗圆角对齐参考图
6. **回归检查**
   - [ ] 其余页面（Users/Servers/Settings/Login/Detail 页）浏览无破裂
   - [ ] dark 主题逐页检查对比度

## Validation Commands

```bash
cd web && npm run typecheck && npm run lint && npm run build
```

## Risky Files / Rollback Points

- `tokens.css`：全站影响，单独 commit
- `AppLayout.vue`：导航壳，注意移动端 drawer 与 a11y 属性不破坏
- 建议 commit 粒度：tokens → base → layout → pages，便于单点 revert

## Spec Constraints (from .trellis/spec/frontend)

- 组件只引用语义 token，禁止 `.vue` 内字面量颜色；新 token 必须带 dark 覆盖
- 共享 CSS 原语只放 base.css，禁止 scoped 重复定义
- MetricStrip 是唯一的指标带组件，禁止页内重建 metric 卡片
- Icons 只用 lucide-vue-next
