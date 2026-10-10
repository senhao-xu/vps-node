# Design: Agent 公网 IP 探测上报

## 数据流

```
agent 启动 → PublicIPProvider.Get() ─(缓存未过期直接返回)
                └─(过期/首用)→ GET https://api.ipify.org (5s 超时, 64B 上限)
                                → netip 校验为公网 IP → 缓存 30min
heartbeat body: { ..., "public_ip": "203.0.113.7" }
panel: validate public → 优先级链 → RecordHeartbeat(observed_ip)
```

## Agent 侧

### 新组件 `internal/agentruntime/publicip.go`

```go
type PublicIPProvider struct { staticIP, url string; client *http.Client; mu, cached, fetchedAt }
func NewPublicIPProvider(staticIP, url string) *PublicIPProvider
func (p *PublicIPProvider) Get(ctx context.Context) string
```

- `staticIP != ""` → 恒返回静态值，零网络请求
- 缓存 TTL 30 分钟；过期则拉取：`GET url`，5s 超时，`io.LimitReader` 64B，trim 后 `netip.ParseAddr` + 非私网/回环/链路本地/未指定校验
- 拉取失败 → 返回旧缓存（允许过期值），无缓存返回 `""`；不重试、不打日志噪音（面板端还有校验兜底）
- `client` 由外部注入以便测试（`http.Client{Timeout: 5s}` 默认）

### 配置 `internal/config/config.go`

- `Agent` 增 `PublicIP string`、`PublicIPURL string`
- `agentFile` 增 yaml 标签 `public_ip` / `public_ip_url`；默认 URL 常量 `https://api.ipify.org`
- env：`AGENT_PUBLIC_IP`、`AGENT_PUBLIC_IP_URL`（沿用现有 env 覆盖模式）
- `deploy/agent.example.yaml` 补注释示例

### 心跳装配 `internal/agentruntime/loop.go`

- `LoopOptions` 增 `PublicIP *PublicIPProvider`（nil 时行为不变，兼容现有测试）
- `heartbeatPass`：`PublicIP: l.publicIP.Get(ctx)` 填入请求

### `cmd/agent/main.go`

构造 provider 并注入 LoopOptions。

## Panel 侧

### `internal/web/agent.go`

- `heartbeatRequest` 增 `PublicIP string \`json:"public_ip"\``
- handler 采纳逻辑（**非法/私网静默忽略**，返回 200——展示型字段不应让心跳失败）：

```go
ip := observedClientIP(r)
if addr, err := netip.ParseAddr(strings.TrimSpace(req.PublicIP)); err == nil && !isPrivateIP(addr.String()) && addr.IsValid() {
    ip = req.PublicIP
}
```

（`isPrivateIP`/`observedClientIP` 为上一轮已有 helper，直接复用）

## 契约 `docs/api-contract.md`

heartbeat 请求节补：`public_ip` 为可选扩展字段，agent 探测的出口公网 IP；面板只采纳合法公网值，非法/私网值被忽略；优先级高于代理头与对端地址。响应无变化。

## 测试

- `internal/agentruntime/publicip_test.go`：httptest 端点 → 探测成功；端点 500 → 沿用缓存；静态值零请求（计数）；64B 截断/私网响应被拒。
- `internal/agentruntime/loop_test.go`：注入 fake provider，断言心跳 body 带 `public_ip`（看现有 fake client 模式）。
- `internal/web/observed_ip_test.go` 扩展：上报公网 → 采纳；上报私网 → 回退 XFF；上报乱串 → 回退。
- `internal/config`：yaml/env 解析两个新键（如已有 config 测试文件则跟随其模式）。

## 回滚

- 单 commit revert；无 DB 迁移；契约变更是纯可选字段追加，旧面板忽略该字段、旧 agent 不发该字段，双向兼容。
