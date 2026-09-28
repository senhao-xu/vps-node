# Implement: 新增 HTTP 代理协议节点

> 顺序：先 DB/常量 → sing-box → Web 校验/凭据 → 订阅 → 自定义解析 → 前端 → 文档/spec。
> 每一步后跑该步的验证；全部完成后跑完整门禁。

## 0. Preconditions

- 已读：`.trellis/spec/backend/node-protocol-settings.md`、
  `internal/db/migrations/0011_nodes_socks_protocol.sql`、
  `internal/subscription/render.go`、`internal/singbox/singbox.go`。
- 参考实现：`socks`（无设置样板）、`anytls`（TLS 样板）。
- 分支：`main`（`base_branch`）。

## 1. DB 与常量

- [ ] `internal/db/migrations/0013_nodes_http_protocol.sql`：重建 `nodes`，
      CHECK 加 `'http'`，含 0012 全部列与 FK，重建 4 个索引（含
      `idx_nodes_chain_custom_node`）。
- [ ] `internal/repo/nodes.go`：加 `ProtocolHTTP = "http"`。
- [ ] `internal/db/migration_http_test.go`：仿 `migration_socks_test.go`：
      迁移后 `http` 节点可插入、旧协议与数据保留、CHECK 拒绝非法值、索引存在。
- 验证：`go test ./internal/db/`

## 2. sing-box 入站/出站

- [ ] `internal/singbox/singbox.go`：
      - 加 `ProtocolHTTP = "http"`；`renderInbound` switch 加 case。
      - `renderHTTP(appKey, n)`：socks 用户 + relay 用户；有 TLS 时附 `tls`。
      - `renderChainOutbound` switch 加 case；`renderHTTPOutbound`。
- [ ] `internal/singbox/outbound.go`：
      - `ProxyToOutbound` 加 `case "http"`；`httpProxyOutbound`。
      - `OutboundSupported` 加 `"http"`。
- [ ] 单测：`internal/singbox/singbox_test.go`/`outbound_test.go` 补 http
      入站（明文 + TLS）与 http outbound 断言。
- 验证：`go test ./internal/singbox/`

## 3. Web 校验、设置与凭据

- [ ] `internal/web/nodes.go`：`validProtocols` 加 `repo.ProtocolHTTP`；
      更新 `validateNodeSpec` 与 list 过滤的错误提示串。
- [ ] `internal/web/node_settings.go`：
      - `nodeSettingsSchemas[repo.ProtocolHTTP]`（`tls.server_name`、
        `tls.allow_insecure`、secret `certificate`、secret `private_key`）。
      - `buildNodeSettings` 的 cert/key 成对检查加入 `repo.ProtocolHTTP`。
      - `validateProtocolSettings` 加 `case repo.ProtocolHTTP`：
        请求了 TLS 才 `validateTLSMaterial`，否则明文合法。
- [ ] `internal/web/agent_config.go`：`agentCredential` 加
      `case singbox.ProtocolHTTP` → `{contract:"http-v1", username, password}`。
- [ ] `internal/web/nodes_http_test.go`：创建明文/TLS http 节点、非法设置 422、
      DTO 不回显 secret、agent config 含 `type:http` 入站与 `http-v1` 契约。
- 验证：`go test ./internal/web/`

## 4. 订阅渲染

- [ ] `internal/subscription/render.go`：
      - `renderURI` `case ProtocolHTTP`（明文 userinfo + `?tls=1/sni/allowInsecure`）。
      - `renderProxy` `case ProtocolHTTP`（Clash `type:http`）。
      - `expandGroups` 占位符加 `__HTTP_PROXIES__`。
- [ ] `internal/web/subscriptions.go`：确认 `renderableSubscriptionNode` 的
      hy2/anytls TLS-skip 规则不误伤 http（http TLS 可选，不 skip）。
- [ ] 单测：`internal/subscription/render_test.go` 补 http general/clash 输出；
      `internal/web/*subscriptions*_test.go` 补含 http 节点的 general/clash/json。
- 验证：`go test ./internal/subscription/ ./internal/web/`

## 5. 自定义线路解析

- [ ] `internal/subscription/import.go`：`ParseShareURI` 加 `http`/`https`；
      `parseHTTPURI`（显式端口 + 空路径守卫；`https`/`?tls=1` → `tls:true`）。
- [ ] 更新 `ParseShareURI` 注释 scheme 列表。
- [ ] 单测：`import_test.go` 合法/非法 http(s) 链接、守卫拒绝订阅 URL。
- 验证：`go test ./internal/subscription/`

## 6. 前端

- [ ] `web/src/api/types.ts`：`Protocol` 加 `'http'`。
- [ ] `web/src/utils/labels.ts`：`protocolLabel` 加 `http → 'HTTP'`。
- [ ] `web/src/pages/NodesPage.vue`：协议筛选项 + `restoreFromQuery` 白名单。
- [ ] `web/src/components/NodeFormDialog.vue`：协议项、编辑回填、
      payload、校验、模板设置区、`changeProtocol` 重置、`httpAllowInsecure` ref。
- 验证：`npm run typecheck && npm run lint`（workdir `web/`）

## 7. 文档与 spec

- [ ] `docs/api-contract.md`：节点协议枚举加 `http`；POST/PUT 说明可选 TLS；
      自定义节点 scheme 列表加 `http`/`https`。
- [ ] `.trellis/spec/backend/node-protocol-settings.md`：新增 `§4.4 HTTP`
      （凭据契约、可选 TLS、订阅形式、迁移 0013、UI 不可退回明文限制）。

## 8. 完整门禁

```bash
go vet ./...
go test ./...
make test-integration     # 嵌入式 sing-box 校验 + 启动（含 http 入站）
(cd web && npm run typecheck && npm run lint && npm run build)
```

- [ ] `internal/e2e`：若 e2e fixture 覆盖多协议，补一个 http 节点确保
      sing-box 接受该入站。

## 9. Risky Files / Rollback Points

高风险（协议枚举/迁移）：
- `internal/db/migrations/0013_nodes_http_protocol.sql`（回滚点：迁移前 0012）
- `internal/singbox/singbox.go`（switch 遗漏 → 节点静默 unrenderable）
- `internal/subscription/render.go`（旧协议输出必须字节不变）
- `internal/web/node_settings.go`（TLS 可选性判定）
- `internal/subscription/import.go`（`http://` 误解析风险）

回滚：代码可整体还原；0013 仅放宽 CHECK，向后兼容，可保留。

## 10. Follow-up Checks Before `task.py start`

- [ ] `implement.jsonl` / `check.jsonl` 各含至少 1 条真实 spec/research 条目
      （非 `_example`）。
- [ ] 所有 block open question 已清空；PRD 收敛。
- [ ] 最终计划摘要已呈现，并获用户后续明确批准。
