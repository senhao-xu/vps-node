# 实施计划：AnyTLS 协议支持

1. 后端协议骨架：repo 常量 → web/nodes.go 校验（抽取 TLS 校验 helper）→ singbox.go 渲染 → agent_config.go 凭据。
2. 订阅：renderURI / renderProxy / `__ANYTLS_PROXIES__` 占位符 / subscriptions.go 材料缺失跳过。
3. 前端：types.ts、labels.ts、NodeFormDialog.vue（hy2/anytls 共用 TLS 区块）。
4. 测试：singbox / web / subscription 各包用例 + 存量回归。
5. 全量验证：`go test ./... -count=1` + gofmt + go vet + web typecheck/lint/build。

## 质量门槛

- 后端先行，测试全绿后再做前端。
- 样式沿用现有 token 体系，不新增样式。
- 不改 API 路由与 secret 加密机制。

## 回滚点

- 纯新增协议，回滚 = revert 单个 commit；不影响存量协议。
