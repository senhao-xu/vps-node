# PRD: 新增 HTTP 代理协议节点

## Goal

面板新增一等协议 **HTTP 代理**（sing-box `http` 入站），使其与现有
shadowsocks / vless / hysteria2 / anytls / socks **完全对齐**：创建/编辑/删除/
启停、Agent 下发、订阅（general + clash-meta）、分享二维码、链路出口，
以及自定义线路 `http(s)://` 解析；支持可选 TLS。

## Background / Confirmed Facts

- 协议常量：`internal/singbox/singbox.go`、`internal/repo/nodes.go:73`；
  白名单 `internal/web/nodes.go:22`。
- 端到端接线位置（以 socks 为无设置样板、anytls 为 TLS 样板）：
  - 入站：`internal/singbox/singbox.go` `renderInbound` / `renderSOCKS:445`；
  - 凭据：`internal/web/agent_config.go:352` `agentCredential`；
  - 设置 schema：`internal/web/node_settings.go`；
  - 订阅：`internal/subscription/render.go` `renderURI:327` / `renderProxy:494`；
  - 自定义解析：`internal/subscription/import.go` `ParseShareURI`；
  - 链路出站：`internal/singbox/singbox.go` `renderChainOutbound` /
    `internal/singbox/outbound.go`；
  - 迁移：`internal/db/migrations/`（最新 `0012`）。
- sing-box `http` 入站：`type:http`，`users:[{username,password}]`，可选 `tls`。
- 认证模型 A（socks 已用）：`username = u-<id>`、`password = <uuid>`，
  `singbox.NameForUser` 为唯一来源；Agent 由 `metadata.User` 归因。
- Xboard 亦支持 `http` 类型（`tls`/`tls_settings`，`buildHttp`，
  `auth.User` 用户名/密码）。
- 分享二维码已实现 `GET /api/nodes/{id}/share`，复用 `renderURI`。

## Requirements

- **R1 协议入库与白名单**：新增 `ProtocolHTTP = "http"`；迁移 `0013` 放宽
  `nodes.protocol` CHECK（含 0012 全部列/FK/索引）；`validProtocols` 接受 `http`。
  anchors: `internal/repo/nodes.go`, `internal/db/migrations/0011_nodes_socks_protocol.sql`（模板）,
  `internal/web/nodes.go:22`。
- **R2 入站与凭据**：`renderHTTP` 产出 `type:http` 入站（用户 + relay 用户，
  有 TLS 时附 `tls`）；新凭据契约 `http-v1`。
  anchors: `internal/singbox/singbox.go:445`, `internal/web/agent_config.go:352`。
- **R3 可选 TLS**：设置 schema 支持 `tls.server_name` / `tls.allow_insecure` /
  secret `certificate` / `private_key`；「请求了 TLS」时复用
  `validateTLSMaterial`（pair 匹配/有效期/VerifyHostname），cert/key 成对提交；
  不配置则为明文 HTTP。
  anchor: `internal/web/node_settings.go`。
- **R4 订阅输出**：general 输出 `http://user:pass@host:port[?tls=1&sni=..&allowInsecure=1]#name`；
  clash-meta 输出 `{type:http, username, password, udp:true, tls?/sni?/skip-cert-verify?}`；
  模板占位符 `__HTTP_PROXIES__`。
  anchor: `internal/subscription/render.go`。
- **R5 分享二维码**：`GET /api/nodes/{id}/share` 对 http 节点返回链接（复用 R4 的
  `renderURI`），前端二维码/复制无需额外改动。
- **R6 链路出口**：http 节点可作为其它节点的 chain 出口：`renderHTTPOutbound`
  （relay 凭据 + 条件 TLS）。
  anchor: `internal/singbox/singbox.go` `renderChainOutbound`。
- **R7 自定义线路解析**：`ParseShareURI` 支持 `http`/`https`；Clash `type:http`
  可转 sing-box outbound（`OutboundSupported("http")`）。
  anchor: `internal/subscription/import.go`, `internal/singbox/outbound.go`。
- **R8 前端**：`Protocol` 联合类型、`protocolLabel('http')='HTTP'`、节点表单
  （协议项、可选 TLS 设置区、编辑回填、payload、校验）、节点页协议筛选。
  anchors: `web/src/api/types.ts`, `web/src/utils/labels.ts`,
  `web/src/components/NodeFormDialog.vue`, `web/src/pages/NodesPage.vue`。

## Key Decisions

- **D1 凭据**：复用 socks 模型 A（`u-<id>` / UUID），新增契约 `http-v1`。
- **D2 general URI 用明文 userinfo**（非 socks 的 base64url）：HTTP 代理通用
  客户端按 `http://user:pass@host` 识别，base64 userinfo 不可解析为代理；
  Clash 输出的 `type:http` 为可靠导入路径。
- **D3 `http://` 解析守卫**：要求显式端口且空路径，降低把订阅 URL 误当线路。
- **D4 可选 TLS 不可经 UI 退回明文**：与现有协议「已存 secret 无法经 UI 清空」
  一致；文档/spec 注明。
- **D5 协议串统一为 `http`**：sing-box / Clash / panel 三者同用 `http`
  （不同于 socks 的 `socks` vs `socks5`）。

## Out of Scope

- 透明/反向代理及 CONNECT 之外的 HTTP 语义。
- 其它新协议（vmess/trojan/tuic/naive/mieru 等）。
- HTTP 代理的用户级限速/并发等新能力。

## Acceptance Criteria

- [ ] 迁移后 `http` 协议节点可创建/编辑/启停/删除/复制；旧协议与数据保留；
      非法 protocol 仍被 CHECK 拒绝。
- [ ] 明文 http 节点：订阅 general 含可导入的 `http://` 条目；clash-meta 含
      `type:http` 且凭据为 `u-<id>` / UUID；`GET /api/nodes/{id}/share` 返回该链接。
- [ ] TLS http 节点：cert/key 成对校验与 anytls 行为一致；general 链接带
      `tls=1`/`sni`，clash 代理带 `tls`/`sni`；入站含内联 PEM TLS。
- [ ] http 节点作为 chain 出口时，入口 Agent 配置含 `type:http` 出站与 relay 凭据。
- [ ] 自定义线路 `http(s)://` 可解析为 `type:http` 且 `chain_supported=true`；
      订阅 URL 形态的行不被误解析（守卫生效）。
- [ ] Agent 配置含 `{contract:"http-v1"}`；`go test ./...` 与
      `make test-integration`（嵌入式 sing-box 校验/启动）通过。
- [ ] 前端 `npm run typecheck`/`lint`/`build` 通过；节点表单可选 TLS 可创建/回填。
- [ ] `docs/api-contract.md` 与 `.trellis/spec/backend/node-protocol-settings.md`
      更新到 http。

## Risks / Deferred

- `http://` 与订阅 URL 的天然歧义：以 D3 守卫缓解，仍属已知边缘情况，记入 spec。
- 旧协议订阅输出必须字节不变（新增 case 不得影响既有分支）。
- Agent sing-box 版本要求：`http` 入站长期支持，镜像 1.14.1 满足。

## Artifacts

- `prd.md`（本文件）、`design.md`、`implement.md`、`implement.jsonl`、`check.jsonl`。
