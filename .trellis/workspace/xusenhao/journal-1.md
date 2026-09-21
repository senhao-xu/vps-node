# Journal - xusenhao (Part 1)

> AI development session journal
> Started: 2026-09-20

---


- 2026-09-21: 完成前端整体样式重设计（09-21-frontend-redesign）。双层 token + 暗色模式（light/dark/system），tokens.css 重构，新增 stores/theme.ts，AppLayout 导航图标+主题切换，登录页重设计，硬编码颜色清零。typecheck/lint/build 全绿，commit f1c59fc。
- 2026-09-21: 完成节点协议参数补全（09-21-node-protocol-params）。hy2 obfs/hop_ports、vless flow 三态/dest，singbox.VLESSFlow 统一解析，go test 17 包全绿，commit 6a3b8d3；另提交 UI 修复 bb687f1（Esc 关闭弹窗、form-row 溢出）。
- 2026-09-21: 完成 AnyTLS 协议支持（09-21-anytls-protocol）。复用 hy2 TLS 证书模式，0006 迁移放宽 protocol CHECK，订阅/Clash/前端全链路，commit b8ea6ef；迁移前已备份 panel.db 到 /tmp/panel.db.bak-20260921。
