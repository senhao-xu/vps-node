# Implement: 自定义/订阅线路作为链式出口

依赖顺序：转换器 → 存储 → 配置构建 → API/生命周期 → 前端 → 文档。每步保持 `go test ./...` 通过。

## 1. sing-box 出站转换器
- [ ] 新增 `internal/singbox/outbound.go`：`ProxyToOutbound(proxy map[string]any) (map[string]any, error)`、`OutboundSupported(clashType string) bool`，覆盖 ss/vless/trojan/vmess/hysteria2/anytls/socks5（含 reality-opts/ws-opts/grpc-opts/tls/skip-cert-verify/alterId/obfs）。
- [ ] 新增 `internal/singbox/outbound_test.go`：每类型字段映射 + 未知类型 error + `OutboundSupported`。
- 验证：`go test ./internal/singbox/...`

## 2. 存储与 Repo
- [ ] 新增迁移 `internal/db/migrations/0012_nodes_chain_custom_exit.sql`（`chain_custom_node_id` FK、`chain_custom_entry_key`、索引）。
- [ ] 新增 `internal/db/migration_custom_chain_exit_test.go`（升级/幂等/索引/FK/`foreign_key_check`）。
- [ ] `internal/repo/nodes.go`：`Node`/`NewNode` 加字段；补齐所有显式列清单；新增 `ListNodesByCustomChainTarget`、`ListServersChainingCustomNode`；`internal/repo/chain_test.go` 加 round-trip 与新方法测试。
- [ ] `internal/db/db_test.go` 迁移计数 +1。
- 验证：`go test ./internal/db/... ./internal/repo/...`

## 3. 链式渲染 + 解析复用
- [ ] `internal/singbox/singbox.go`：`ChainExit` 加 `Outbound map[string]any`；`Render` 支持预构建出站（注入 `chain-<id>` tag + route rule）。
- [ ] `internal/subscription`：新增 `ResolvedEntry{Key, Proxy, Link}` 与 `ResolveEntries(appKey, links, proxies)`；`customNodeEntries`/`SummarizeEntries` 复用它，避免解析分叉。
- [ ] `internal/singbox/chain_test.go` 加 `Outbound` 路径用例。
- 验证：`go test ./internal/singbox/... ./internal/subscription/...`

## 4. Agent 配置构建
- [ ] `internal/web/agent_config.go` `chainExits`：解析 `ChainCustomNodeID` 的 active 节点，从 `content_enc`/`cached_content`（不联网）解析，按 `entry_key` 命中 → `ProxyToOutbound` → `ChainExit{Outbound}`；缺失/停用/无缓存/解析失败 → 跳过（回退直连）。
- 验证：`go test ./internal/web/...`

## 5. API 校验 + 生命周期
- [ ] `internal/web/nodes.go`：create/update 支持 `chain_custom_node_id`（三态）与 `chain_custom_entry_key`；互斥校验；来源存在且 active；key hex64；`validateChainTarget` 兼容。
- [ ] `internal/web/dto.go`：`nodeDTO` 增 `chain_custom_node_id`/`chain_custom_node_name`/`chain_custom_entry_key`；`nodeSelectWithServer` 关联 `custom_nodes`。
- [ ] `internal/web/custom_nodes.go`：`customNodeEntryDTO` 加 `chain_supported`；`handleCustomNodeDelete` 引用保护 `409`；`handleCustomNodeUpdate`/`handleCustomNodeRefresh` 后 bump `ListServersChainingCustomNode`。
- [ ] `internal/web/nodes_custom_chain_test.go`：外部出口渲染、无缓存回退、失效回退、互斥/校验矩阵、三态更新、delete 409、update/refresh bump。
- [ ] `internal/e2e/integration_chain_test.go`：外部出口嵌入式 sing-box 校验。
- 验证：`go test ./internal/web/...`

## 6. 前端
- [ ] `web/src/api/types.ts`：`CustomNodeEntry.chain_supported`；`NodeBrief`/详情 + `CreateNodeInput`/`UpdateNodeInput` 新字段。
- [ ] `web/src/components/NodeFormDialog.vue`：出口模式「直连/托管节点/自定义线路」；自定义模式先选来源后选线路（`chain_supported` 过滤/置灰），保存互斥清空。
- [ ] `web/src/pages/NodesPage.vue`：链式列表展示外部出口来源。
- 验证：`cd web && npm run typecheck && npm run lint && npm run build`

## 7. 文档与规范
- [ ] `docs/api-contract.md`（节点 create/update 字段、`chain_supported`、外部出口语义）。
- [ ] `.trellis/spec/backend/node-protocol-settings.md` 链式小节补充外部出口；`directory-structure.md` 如需。

## 8. 全量校验
- [ ] `go vet ./...`、`go test -count=1 ./...`、`go test -count=1 -tags integration,with_quic,with_utls ./...`
- [ ] `cd web && npm run typecheck && npm run lint && npm run build`

## 风险 / 回滚点
- 转换器是主要风险：字段映射必须经嵌入式 sing-box `Validate` 验证；未知/缺字段一律 error→回退直连，绝不让 config 构建失败。
- `nodes` 新列必须补齐**所有**显式列清单（scan 列数不符会运行时报错）。
- bump 语义：节点改出口 bump 入口 server（+托管旧/新出口 server）；自定义来源 update/refresh bump 引用 server；delete 保护。
- `Outbound == nil` 路径必须与现状逐字节一致（回归 `chain_test.go`/`nodes_chain_test.go`）。
