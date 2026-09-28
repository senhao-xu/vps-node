# Design: 新增 HTTP 代理协议节点

## 1. Architecture & Boundaries

新增一等协议 `http`，贯穿：repo 常量 → 迁移（放宽 CHECK）→ Web 校验/设置 →
sing-box 入站/出站 → Agent 凭据 → 订阅（general/clash）→ 分享二维码 →
自定义线路解析/链路出口 → 前端类型/标签/表单。边界与现有 `socks` 对齐，
差异点是 **可选 TLS**（复用 Hysteria2/AnyTLS 的 TLS 证书模型）。

### 协议标识映射

| 层 | 取值 |
|---|---|
| panel / repo / agent 协议串 | `http` |
| sing-box 入站 type | `http` |
| sing-box 出站 type | `http` |
| Clash proxy type | `http` |
| general URI scheme | `http`（TLS 仍用 `http://` + `?tls=1`，不用 `https://`，避免与真实 HTTPS URL 混淆） |
| Clash 模板占位符 | `__HTTP_PROXIES__` |
| Agent 凭据契约 | `http-v1` |

## 2. Credential Model

复用 socks 的 **模型 A**（`internal/singbox/singbox.go` `NameForUser`）：

- sing-box 入站 `users[].username = u-<user id>`，`password = <user uuid>`。
- Agent 凭据：`agentCredential` 返回 `{contract:"http-v1", username, password}`。
- 中继（chain 出口）：`relay-<entry server id>` + `DeriveRelayPassword`，
  与 socks 相同。
- Agent 侧 `ConnectionTracker` 通过 `metadata.User`（HTTP 代理认证用户名）
  映射回 panel 用户，http 与 socks 同为用户名映射，无需协议特判。

## 3. Backend Contracts

### 3.1 入站渲染 `renderHTTP(appKey, n)`

```json
{
  "type": "http",
  "tag": "http-<id>",
  "listen": "::",
  "listen_port": <port>,
  "users": [{"username": "u-<id>", "password": "<uuid>"}, {"username": "relay-<sid>", "password": "..."}],
  "tls": {"enabled": true, "server_name": "...", "certificate": ["..."], "key": ["..."]}
}
```

- `tls` 仅在节点配置了 `tls.server_name` + `certificate` + `private_key` 时出现
  （复用 `renderAnyTLS` 的 PEM 按 `\n` 拆行形状）；否则为明文 HTTP 代理。
- `tls.allow_insecure` 与 hy2/anytls 一致：只存设置，不入服务端入站。

### 3.2 链路出口出站 `renderHTTPOutbound(appKey, c, tag)`

```
{type:"http", tag, server: c.DialAddress, server_port: c.Exit.Port,
 username: RelayUserName(c.EntryServerID),
 password: DeriveRelayPassword(appKey, c.Exit.ID, c.EntryServerID),
 tls?: {enabled, server_name, certificate:[...] | insecure:true}}
```

- 仅当出口节点配置了 TLS 时附 `tls`（新增条件化 helper，不能直接用
  `chainTLS`，它对 hy2/anytls 恒定启用 TLS）。

### 3.3 自定义线路（Clash proxy → sing-box outbound）

- `ProxyToOutbound` 增加 `case "http"` → `httpProxyOutbound`：
  `{type:"http", server, server_port, username, password, tls?}`（`tls`/`sni`/
  `skip-cert-verify` 来自 Clash proxy）。
- `OutboundSupported` 增加 `"http"`，使 `chain_supported=true`。

### 3.4 设置 schema 与校验（`internal/web/node_settings.go`）

`nodeSettingsSchemas[repo.ProtocolHTTP]`（全可选）：

| 字段 | 类型 | 说明 |
|---|---|---|
| `tls.server_name` | 明文，≤253 | 启用 TLS 的标记之一 |
| `tls.allow_insecure` | 明文 bool | 仅客户端语义 |
| `certificate` | secret PEM | 成对出现 |
| `private_key` | secret PEM | 成对出现 |

- `certificate`/`private_key` 成对校验：把 `repo.ProtocolHTTP` 加入现有
  hy2/anytls 的 `certSet != keySet` 检查。
- `validateProtocolSettings` 增加 `case repo.ProtocolHTTP`：
  - 若「请求了 TLS」（`tls.server_name` 非空 或 cert/key 任一存在）→ 复用
    `validateTLSMaterial(repo.ProtocolHTTP, plain, secretFields)`（key pair 匹配、
    有效期、`VerifyHostname`）。
  - 否则视为明文 HTTP，`{}` 合法。
- 空 schema 子项之外未知键仍 `422 validation`（`flattenSettings` 通用逻辑）。
- **已知限制**（与现有协议一致）：UI 无法在启用 TLS 后清空已存的 secret，
  因此启用 TLS 后不可退回明文；在文档/spec 中注明。

### 3.5 订阅渲染（`internal/subscription/render.go`）

- `renderURI` `case ProtocolHTTP`：
  `http://<user:pass>@<host>#<name>`，其中 userinfo 用
  `url.UserPassword(NameForUser(id), uuid)` **明文**（失败/不安全的 base64
  userinfo 不被通用客户端识别）。
  - TLS 追加 `?tls=1[&sni=<server_name>][&allowInsecure=1]`。
  - IPv6 变体经既有 `expandIPv6` 自动产出。
- `renderProxy` `case ProtocolHTTP`：
  `{name, type:"http", server, port, username, password, udp:true}` +
  `tls:true`/`sni`/`skip-cert-verify`（配置了 TLS 时）。
- `expandGroups` 占位符表增加 `"__HTTP_PROXIES__": ProtocolHTTP`。

### 3.6 自定义线路解析（`internal/subscription/import.go`）

- `ParseShareURI` 增加 `case "http", "https"` → `parseHTTPURI`。
- `parseHTTPURI` 输出 Clash `{type:"http", server, port, username, password, tls?, sni?, skip-cert-verify?}`。
- **防误解析守卫**（关键兼容点）：要求显式端口且路径为空（允许 `/`），
  以降低把订阅 URL 当线路解析的风险；`https` 或 `?tls=1` 置 `tls:true`。
- 更新 `ParseShareURI` 头部注释的 scheme 列表。

### 3.7 Web 校验白名单

- `internal/web/nodes.go` `validProtocols` 增加 `repo.ProtocolHTTP`，
  并更新 `validateNodeSpec`/list 的错误提示串。
- `internal/web/agent_config.go` `agentCredential` 增加 `case singbox.ProtocolHTTP`。

## 4. Migration

`internal/db/migrations/0013_nodes_http_protocol.sql`：沿用 0011 的
「建 `nodes_new` → copy → drop → rename → 重建索引」模式，放宽 CHECK：

```
protocol TEXT NOT NULL CHECK (protocol IN ('shadowsocks','vless','hysteria2','anytls','socks','http'))
```

必须包含截至 0012 的全部列与约束：

- 列：`id, server_id, address, name, protocol, port, protocol_settings, rate,
  tags, secret_enc, status, created_at, updated_at, ipv6_enabled, ipv6_address,
  chain_node_id, chain_custom_node_id, chain_custom_entry_key`
- 外键：`server_id → servers(id) ON DELETE CASCADE`、`chain_node_id → nodes(id)`、
  `chain_custom_node_id → custom_nodes(id)`
- 索引：`idx_nodes_server`、`idx_nodes_port_active`（partial）、
  `idx_nodes_chain_node`、`idx_nodes_chain_custom_node`

runner 已禁用外键，自引用 FK 在 rename 后仍指向 `nodes`。

## 5. Frontend

- `web/src/api/types.ts`：`Protocol` 增加 `'http'`。
- `web/src/utils/labels.ts`：`protocolLabel` 增加 `case 'http': return 'HTTP'`。
- `web/src/pages/NodesPage.vue`：协议筛选选项 + `restoreFromQuery` 白名单加 `http`。
- `web/src/components/NodeFormDialog.vue`：
  - `protocolOptions` 增 `{value:'http', label:'HTTP', dot:'muted'}`。
  - 编辑回填：`settings.tls` → `tlsServerName` / `httpAllowInsecure`。
  - payload：`tls:{server_name, allow_insecure}`（可选）+ `certificate`/`private_key`
    （填写时）；无 TLS 时返回 `{}`（非 `undefined`，与 socks 空设置区分）。
  - 校验：TLS 字段要么全空（明文），要么 server_name 合法且 cert/key 成对；
    server_name 用现有 `/^[A-Za-z0-9.-]+$/`。
  - 模板：新增 `v-else-if="protocol === 'http'"` 设置区，复用
    `tlsServerName`/`tlsCertificate`/`tlsPrivateKey` 输入 + 新增
    `httpAllowInsecure` 开关；`changeProtocol` 重置这些 ref。
- 分享二维码：`NodeShareDialog` 复用 `GET /api/nodes/{id}/share`，无需改动（
  `renderURI` 支持 http 后即生效）。

## 6. Compatibility / Trade-offs

- **general URI 用明文 userinfo**（而非 socks 的 base64url）：HTTP 代理通用
  客户端按 `http://user:pass@host` 识别；base64 userinfo 无法被解析为代理。
  Clash 输出的 `type: http` 是可靠导入路径。
- **`http://` 歧义**：自定义线路解析要求显式端口 + 空路径来抑制订阅 URL 误解析；
  但仍可能误收「带端口且无路径」的 URL，属已知边缘情况并记录在 spec。
- **迁移可回滚**：迁移仅新增 CHECK 取值，无数据丢失；回滚点 = 迁移前（0012）。
- 现有 4 种协议 + socks 的订阅输出保持字节不变（新增 case 不影响旧分支）。

## 7. Operational / Rollback

- sing-box `http` 入站长期支持，Agent 镜像 1.14.1 满足要求；
  e2e（`internal/e2e`）校验配置并启动嵌入式 sing-box 作为门禁。
- 回滚：还原代码 + 不应用 0013（或保留，因向后兼容）。
