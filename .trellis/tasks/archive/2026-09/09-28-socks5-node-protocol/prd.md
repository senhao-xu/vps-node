# SOCKS5 托管节点协议

## Goal

新增 `socks` 作为**托管节点协议**（sing-box `socks` 入站），与 `shadowsocks / vless / hysteria2 / anytls` 并列：可在面板创建/编辑/筛选、下发给 Agent、并入用户订阅（general 与 clash-meta），按用户独立鉴权并正确归属流量/设备，且可作为链式代理出口。

## Background / Confirmed Facts（代码证据）

现有托管协议只有 4 种（`trojan`/`vmess` 仅存在于自定义节点分享链接解析）。新增协议必须触碰：

- 协议常量 `internal/repo/nodes.go:65-73`；允许集 `internal/web/nodes.go:22-27`（错误文案 `:44,:227`）。
- DB CHECK `internal/db/migrations/0001_init.sql:66`、`0003_node_address.sql:16`（`CHECK (protocol IN (...))`）；live `nodes` 列为 `id, server_id, address, name, protocol, port, protocol_settings, rate, tags, secret_enc, status, created_at, updated_at, ipv6_enabled, ipv6_address, chain_node_id`；索引 `idx_nodes_server`、`idx_nodes_port_active`（partial）、`idx_nodes_chain_node`。
- sing-box 入站 `internal/singbox/singbox.go:215-228` `renderInbound`，`inboundTag = protocol + "-" + id`；链式出站 `:414-428` `renderChainOutbound`。
- 协议设置注册表/校验 `internal/web/node_settings.go:32-82`、`:234-266`。
- 订阅渲染 `internal/subscription/render.go:319-375` `renderURI`、`:483-533` `renderProxy`、`:589` 占位符 map；Clash 中 SOCKS 类型名为 `socks5`。
- Agent 配置 `internal/web/agent_config.go:301-317` `agentCredential`（漏分支会导致整个 config 构建失败）。
- 运行时归属 `internal/kernel/singbox/tracker.go:69-105`：以入站 user **name** `u-<id>` 建表；sing-box `auth.User` 无 `name`，SOCKS 把 `metadata.User` 设为 **username**，故入站 username 必须是 `u-<id>`。
- 前端 `web/src/api/types.ts:4`、`web/src/utils/labels.ts:58-71`、`web/src/components/NodeFormDialog.vue`、`web/src/pages/NodesPage.vue:66-72,273-276`、`web/src/pages/SettingsPage.vue:270`。
- 文档 `README.md:23,113`、`docs/api-contract.md`、`.trellis/spec/backend/node-protocol-settings.md`。
- 测试枚举点 `internal/db/migration_anytls_test.go:23`、`internal/web/servers_nodes_test.go:200-227`、`internal/singbox/{singbox_test.go,chain_test.go:201}`、`internal/subscription/render_test.go:34,45,199-219`、`internal/e2e/{integration_test.go:43-78,integration_chain_test.go:39}`；模板 `internal/web/nodes_anytls_test.go`。

## Requirements

- R1：新增 `ProtocolSocks="socks"` 并加入所有允许集/开关（常量、`validProtocols`、错误文案、设置 schema/校验、agent credential）。
- R2：DB 迁移（0011）放开 `nodes.protocol` CHECK 以含 `socks`，保留全部现有列与索引，幂等可升级。
- R3：sing-box 入站 `socks`（`listen:"::"`、`listen_port`、每用户 `username=u-<id>`/`password=uuid`、relay 用户），tag `socks-<id>`；链式出站 socks 同理。
- R4：socks 节点无协议设置，`protocol_settings` 恒为 `{}`。
- R5：订阅渲染：general 输出 `socks://base64url(u-<id>:uuid)@host:port#name`；clash-meta 输出 `type: socks5` + `username`/`password` + `udp: true`；新增 `__SOCKS_PROXIES__` 占位符。
- R6：按用户鉴权与流量/设备归属正确（入站 username 与 tracker 的 `u-<id>` 一致）；`agentCredential` 契约 `socks-v1`。
- R7：前端可创建/编辑/筛选 socks 节点，协议标签 `SOCKS5`，无设置表单。
- R8：文档与 Trellis 规范同步。

## Acceptance Criteria

- [ ] 创建 `socks` 节点（空 settings）返回 201；带未知 settings 字段返回 `422 validation`。
- [ ] Agent config 含 `type=socks`、`tag=socks-<id>`、`users[].username==u-<id>`、`password==<该用户 uuid>`；credential 契约 `socks-v1`。
- [ ] general 订阅含 `socks://<base64url(u-<id>:uuid)>@host:port#name`；clash-meta 含 `type: socks5`、`username`、`password`、`udp: true`。
- [ ] `__SOCKS_PROXIES__` 正确展开为该用户全部 socks proxy 名。
- [ ] 既有 4 种协议的订阅输出与 agent config 逐字节不变（回归测试通过）。
- [ ] socks 节点可被选为链式出口并渲出 socks 出站。
- [ ] 迁移升级后既有节点/授权/设备数据存活，`PRAGMA foreign_key_check` 干净，重复执行幂等；`snell` 等未知协议仍被拒。
- [ ] `go vet ./...`、`go test -count=1 ./...`、`go test -tags integration,with_quic,with_utls ./...`、`web` typecheck/lint/build 全部通过。

## Key Decisions

- 凭据模型 A：每用户 `username=u-<用户ID>`、`password=用户 UUID`；订阅渲染入口改为接收 `subscription.User{ID,UUID}`（用户选定）。
- socks 支持作为链式代理出口（用户选定）。
- socks 无协议设置字段（用户选定）。
- general 分享链接用 SIP002 风格 base64url(user:pass)；Clash 类型名 `socks5`。

## Out of Scope

- 自定义节点分享链接解析 `socks://`（仅托管节点）
- 匿名/免鉴权 SOCKS
- socks 专用协议设置（udp 开关等）

## Known Limitations

- general 的 socks 链接为 base64url 形式；若目标客户端需要明文 `socks5://user:pass@`，后续再调整。
