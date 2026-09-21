# AnyTLS 协议支持

## 背景

用户要求新增 AnyTLS 协议（sing-box 1.12+ 支持，当前 agent 镜像固定 sing-box 1.14.1，满足要求）。TLS 采用与 Hysteria2 相同的自定义证书模式（SNI + PEM 证书链/私钥），padding scheme 不暴露（用 sing-box 默认）。

## 需求

1. 节点协议新增 `anytls`，创建/编辑表单字段：Server Name、PEM 证书链、PEM 私钥（与 hy2 相同的必填与校验规则：证书私钥成对、证书覆盖 server_name、有效期内）。
2. 用户认证：用户 UUID 直接作为 anytls password（与 hy2 一致）。
3. sing-box inbound：`type: anytls`，users [{name, password}]，tls 内联证书（certificate/key 按行拆分，同 hy2）。
4. 订阅：
   - 通用订阅 URI：`anytls://<uuid>@host:port/?sni=<server_name>#<name>`
   - Clash Meta：`type: anytls`，`password`、`sni`、`skip-cert-verify: false`、`udp: true`
   - 模板占位符新增 `__ANYTLS_PROXIES__`
5. 节点无 TLS 材料时订阅跳过该节点（与 hy2 的 subscriptions.go 检查一致）。
6. 前端：协议下拉新增 AnyTLS；labels.ts 中文标签「AnyTLS」；表单复用 hy2 的 TLS 字段布局。

## 非目标

- 不支持 padding scheme 自定义、不支持 Reality/ACME 证书模式。
- 不改 agent 镜像的 sing-box 版本（1.14.1 已支持）。

## 验收标准

1. `go test ./... -count=1` 全绿；web typecheck/lint/build 全绿。
2. 创建 anytls 节点（SNI + 证书/私钥）成功；缺任一项 422 validation；证书不覆盖 SNI 422。
3. agent 下发配置的 inbound 为 `type: anytls`，users 密码为用户 UUID，tls 内联证书。
4. 通用订阅含 `anytls://` 链接（sni 参数正确）；Clash Meta 含 anytls 代理且 `__ANYTLS_PROXIES__` 占位符可展开。
5. 前端可创建/编辑 anytls 节点，编辑态留空保持不变语义与现有协议一致。
