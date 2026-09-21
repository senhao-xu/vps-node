# 设计：AnyTLS 协议支持

## 总览

完全复用 Hysteria2 的既有模式（TLS 证书校验、UUID 作密码、订阅渲染），新增第四种协议。agent 端 sing-box 1.14.1 原生支持 `anytls` inbound，无需改 agent。

## 后端改动

### 1. `internal/repo/nodes.go`
- 新增 `ProtocolAnyTLS = "anytls"` 常量。

### 2. `internal/web/nodes.go`
- `validProtocols` 增加 anytls。
- settings 白名单：`server_name`（plain）、`certificate`/`private_key`（secretFields）、`password`（secret，沿用 hy2 的保留 case 如存在）。
- `validateProtocolSettings`：anytls 复用 hy2 的 TLS 校验（server_name 必填 + X509KeyPair 匹配 + 证书有效期 + VerifyHostname）。将 hy2 的校验块抽成共享 helper（如 `validateTLSMaterial(plain, secretFields)`）供两个协议调用，避免复制。
- 编辑场景 partial 语义不变。

### 3. `internal/singbox/singbox.go`
- 新增 `ProtocolAnyTLS = "anytls"`；`renderInbound` switch 加 case。
- `renderAnyTLS`：users `[{name: "u-<id>", password: uuid}]`；tls 与 hy2 相同（server_name + 按 `\n` 拆分的 certificate/key 内联）。无带宽/obfs 字段。

### 4. `internal/web/agent_config.go`
- `agentCredential`：anytls → `{"contract": "uuid-v1", "password": userUUID}`（与 hy2 相同）。

### 5. `internal/subscription/render.go`
- `renderURI`：`anytls://` + url.PathEscape(userUUID) + `@host:port` + `?sni=<server_name>` + `#name`；server_name 缺失时报错跳过（与 hy2 一致）。
- `renderProxy`：`{type: anytls, password: uuid, sni, skip-cert-verify: false, udp: true}`。
- `expandGroups` placeholders 增加 `__ANYTLS_PROXIES__`。

### 6. `internal/web/subscriptions.go`
- 第 174 行附近的 hy2 材料缺失检查扩展为 anytls 同样适用（server_name/certificate/private_key 缺失则跳过该节点）。

### 测试
- singbox：renderAnyTLS inbound 结构断言（users 密码、tls 内联拆分）。
- web：创建校验矩阵（缺 SNI/证书/私钥 422、证书不覆盖 SNI 422、未知字段 422）、编辑 partial 保留、agent 配置断言。
- subscription：URI 参数与 Clash 字段断言、`__ANYTLS_PROXIES__` 展开。
- e2e 如已有三协议用例，补 anytls 最小用例。

## 前端改动

- `web/src/api/types.ts`：`Protocol` 联合类型加 `'anytls'`。
- `web/src/utils/labels.ts`：`protocolLabel` 加 AnyTLS。
- `NodeFormDialog.vue`：协议下拉加 AnyTLS；TLS 字段区（SNI/证书/私钥）hy2 与 anytls 共用同一模板块（v-if 条件改为 `protocol === 'hysteria2' || protocol === 'anytls'`），anytls 隐藏上下行带宽与 obfs/端口跳跃字段；校验逻辑共用 TLS 部分。
- `NodesPage.vue` 等涉及协议展示的地方确认标签正常。

## 兼容性

- 纯新增协议，存量节点/订阅行为不变。
- 旧 agent（sing-box < 1.12）无法运行 anytls inbound——镜像已固定 1.14.1，二进制安装的节点需自行保证 sing-box ≥ 1.12（install 文档/README 提及即可，不强制）。

## 验证

- `go test ./... -count=1`、`gofmt`、`go vet`
- web：typecheck / lint / build
