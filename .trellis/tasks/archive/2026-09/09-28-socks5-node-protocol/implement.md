# Implement: SOCKS5 托管节点协议

依赖顺序：常量/允许集 → 渲染 → 存储 → 订阅 → 前端 → 文档。每步保持 `go test ./...` 通过。

## 1. 协议常量、允许集、设置、Agent 凭据
- [ ] `internal/repo/nodes.go` 加 `ProtocolSocks = "socks"`。
- [ ] `internal/web/nodes.go` `validProtocols` 加 socks；`:44,:227` 错误文案更新。
- [ ] `internal/web/node_settings.go` `nodeSettingsSchemas` 加空 schema；`validateProtocolSettings` 加 `case ProtocolSocks: return nil`。
- [ ] `internal/web/agent_config.go` `agentCredential` 加 `userID int64` 参数与 socks 分支（`socks-v1`），更新唯一调用点 `:162`。
- 验证：`go build ./... && go vet ./...`

## 2. sing-box 渲染
- [ ] `internal/singbox/singbox.go` 加 `ProtocolSocks` 常量、`NameForUser(id)` 助手、`renderSOCKS` 入站、`renderSOCKSOutbound` 链式出站，并在 `renderInbound`/`renderChainOutbound` 注册。
- [ ] `internal/singbox/singbox_test.go` 加 `TestRenderSOCKS`；`chain_test.go:201` 循环加 socks。
- 验证：`go test ./internal/singbox/... ./internal/kernel/...`

## 3. DB 迁移
- [ ] 新增 `internal/db/migrations/0011_nodes_socks_protocol.sql`（按 design 第 5 节全表重建，CHECK 含 socks，重建三个索引）。
- [ ] 新增 `internal/db/migration_socks_test.go`（各协议含 socks 可插入、snell 拒绝、既有行/子表存活、`foreign_key_check` 干净、索引存在、幂等）。
- [ ] `internal/db/migration_anytls_test.go:23` 加 `"socks"`；`internal/db/db_test.go` 迁移计数 10 → 11。
- 验证：`go test ./internal/db/...`

## 4. 订阅渲染（含 `User` 结构改造）
- [ ] `internal/subscription/render.go` 定义 `User{ID,UUID}`，把 `RenderGeneral*`/`RenderClashFiltered*`/`renderURI`/`renderProxy` 的 `userUUID string` 改为 `user User`。
- [ ] `renderURI` 加 socks 分支（`socks://base64url(u-<id>:uuid)@host#name`）；`renderProxy` 加 socks 分支（`type=socks5`, username, password, udp=true）；占位符加 `__SOCKS_PROXIES__`。
- [ ] `internal/web/subscriptions.go:221,231` 传 `subscription.User{ID: u.ID, UUID: u.UUID}`。
- [ ] 迁移 `internal/subscription/render_test.go`/`custom_test.go` 的调用为 `User{}`；协议列表加 socks。
- 验证：`go test ./internal/subscription/... ./internal/web/...`

## 5. 前端
- [ ] `web/src/api/types.ts` Protocol 加 `'socks'`。
- [ ] `web/src/utils/labels.ts` `protocolLabel` 加 `case 'socks': return 'SOCKS5'`。
- [ ] `web/src/components/NodeFormDialog.vue`：protocolOptions、settingsPayload（socks→`{}`）、loadNodeDetail/validationMessage/changeProtocol 分支、设置区说明块。
- [ ] `web/src/pages/NodesPage.vue` 筛选项与 deep-link 解析加 socks。
- [ ] `web/src/pages/SettingsPage.vue` 占位符帮助文本加 `__SOCKS_PROXIES__`。
- 验证：`cd web && npm run typecheck && npm run lint && npm run build`

## 6. 文档与规范
- [ ] `README.md`、`docs/api-contract.md` 同步。
- [ ] `.trellis/spec/backend/node-protocol-settings.md` 增加 SOCKS 小节；`directory-structure.md`、`error-handling.md` 协议枚举同步。

## 7. 全量校验
- [ ] `go vet ./...`
- [ ] `go test -count=1 ./...`
- [ ] `go test -count=1 -tags integration,with_quic,with_utls ./...`
- [ ] `cd web && npm run typecheck && npm run lint && npm run build`

## 风险 / 回滚点
- 订阅渲染签名从 `userUUID` 改为 `User`：影响所有协议调用，必须保证既有输出逐字节不变 → 全量 `internal/subscription` 回归。
- `agentCredential` 漏 socks 分支会让含 socks 节点的 agent config 构建整体失败 → 用 `nodes_socks_test.go` 覆盖。
- 0011 迁移全表重建：务必复刻 live 列/索引，参考 `0003` 与 `migration_node_chain_test.go`。
