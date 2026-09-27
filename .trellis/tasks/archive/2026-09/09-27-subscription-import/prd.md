# PRD: 订阅链接区升级——扫码导入 + 多客户端快捷导入

## Goal

将用户详情页「基本信息」卡片中的订阅链接区（`web/src/components/user/UserDetailBasic.vue:156`）升级为参考图「一键聚合订阅」风格：订阅链接 + 复制订阅 + 扫码导入 + 轮换按钮，下方一排客户端快捷导入卡片（Clash/Verge、Shadowrocket、Sing-box、v2rayN/Nekobox）。

## Background / Confirmed Facts

- 现状：订阅链接行仅有 `CopyText` + 「轮换」按钮（`UserDetailBasic.vue:156-187`）；订阅数据经 `getUserSubscription/createUserSubscription/rotateUserSubscription`（`api/users.ts`），类型 `Subscription { configured, url }`。
- 二维码库：新增依赖 `qrcode`（+ `@types/qrcode`），生成 dataURL 图片，无需后端改动。
- 客户端深链（业界标准 scheme）：
  - Clash / Clash Verge: `clash://install-config?url=<urlencoded>`
  - Shadowrocket: `shadowrocket://add/sub://<base64(url)>`
  - Sing-box: `sing-box://import-remote-profile?url=<urlencoded>#<urlencoded 名称>`
  - v2rayN / Nekobox: 无可靠深链 → 按钮行为为「复制订阅链接」（与参考图一致）
- 项目已有 `ModalDialog.vue`（a11y 完备）用于二维码弹窗；样式须用语义 token，遵循刚焕新的中性主题。
- 视觉规范见 `.trellis/spec/frontend/`（胶囊徽章、lg 圆角卡片、近黑主按钮）。

## Requirements

- R1: 订阅链接存在时显示：链接（CopyText 复制）+「扫码导入」按钮 +「轮换」按钮；扫码导入打开 ModalDialog 展示订阅链接二维码（qrcode 生成 dataURL）+ 链接文本。
- R2: 客户端快捷导入卡片区（订阅链接存在时显示）：Clash/Verge（导入→clash:// 深链）、小火箭 Shadowrocket（导入→shadowrocket:// 深链）、Sing-box（导入→sing-box:// 深链）、v2rayN/Nekobox（复制→复制链接）。每卡含图标（lucide）、名称、简述、操作按钮。
- R3: 未生成订阅时保持现有「生成订阅链接」按钮；轮换后二维码/深链自动用新 URL。
- R4: 不改后端；深链仅前端拼接；移动端布局不破裂（卡片自适应换行）。

## Acceptance Criteria

- A1: `cd web && npm run typecheck && npm run lint && npm run build` 通过。
- A2: 扫码导入弹窗展示可扫的二维码（内容为订阅 URL），Esc/点击外部可关。
- A3: 四个客户端卡片深链格式正确（urlencode/base64 正确），v2rayN 卡复制到剪贴板。
- A4: 无颜色字面量，复用 ModalDialog/CopyText 等既有组件与 token。

## Out of Scope

- 后端订阅 API 改动
- 参考图2 的流量配额卡片、节点列表卡片（属用户中心，本项目无此角色页面）
- 更多客户端（Loon/Stash 等，后续可加）

## Key Decisions

- D1: 二维码用 `qrcode` npm 库（用户已确认）。
- D2: v2rayN/Nekobox 无深链，行为为复制（与参考图一致）。
