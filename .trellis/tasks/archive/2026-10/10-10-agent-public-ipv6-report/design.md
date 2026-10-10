# Design: Agent 探测公网 IPv6

## Agent 侧

### `internal/agentruntime/publicip.go` 双栈化

`PublicIPProvider` 扩展为同时维护 v4/v6 两组缓存（复用现有 TTL/失败语义）：

```go
type PublicIPProvider struct {
    staticIP, staticIPv6 string
    url, url6            string
    client *http.Client
    mu sync.Mutex
    cached, cached6 string
    fetchedAt, fetchedAt6 time.Time
}
func (p *PublicIPProvider) Get(ctx) string        // 现有 v4
func (p *PublicIPProvider) Get6(ctx) string       // 新增 v6
```

- v6 默认端点常量 `defaultPublicIPv6URL = "https://api6.ipify.org"`（AAAA-only 域名，无 v6 路由时连接失败 → 返回旧缓存/空）
- v6 校验：`normalizePublicIP` 之外增加 `addr.Is4()` 拒绝（v6 端点必须返回 v6；防端点误配双栈返回 v4）
- 静态值校验同上；静态 v6 直接透传（面板端还有二次校验）

### `internal/config/config.go`

- `Agent` 增 `PublicIPv6 string`、`PublicIPv6URL string`
- yaml：`public_ipv6` / `public_ipv6_url`；env：`AGENT_PUBLIC_IPV6` / `AGENT_PUBLIC_IPV6_URL`
- `deploy/agent.example.yaml` 补注释

### 心跳装配

- `agentclient.HeartbeatRequest` 增 `PublicIPv6 string \`json:"public_ipv6,omitempty"\``
- `loop.go` heartbeatPass 填入 `l.publicIP.Get6(ctx)`

## Panel 侧

### 迁移 `internal/db/migrations/0018_server_observed_ipv6.sql`

```sql
ALTER TABLE servers ADD COLUMN observed_ipv6 TEXT NOT NULL DEFAULT '';
```

同步更新引用 servers 列清单的测试（`migration_identity_accounting_test.go` 的列枚举）。

### 读写链路

- `repo/servers.go`：`Server` 增 `ObservedIPv6 string`；`serverSelect`/scan 增列
- `repo/agent_ops.go` `RecordHeartbeat` 增参写入 `observed_ipv6`
- `internal/web/web.go`：`observedIP` 旁新增 `observedIPv6(r, reported)`——v6 专用校验（`netip.ParseAddr` + `Is4()==false` + 非私网），失败静默回退（v6 没有代理头链路可回退，直接置空）
- `internal/web/agent.go`：heartbeatRequest 增 `ReportedIPv6 \`json:"public_ipv6"\``
- DTO：`serverDTO`/`nodeRefDTO` 增 `observed_ipv6`（dto.go 88/196 行区域）；`repo/nodes.go` nodeSelectWithServer 增列 + scan

### 前端

- `api/types.ts`：`ServerRef`/`Server` 增 `observed_ipv6`
- `NodesPage.vue` `serverAddressFamilies`：v6 判定加入 `node.server.observed_ipv6`
- `ServersPage.vue`：IPv6 列或 IP 列显示回退 `row.ipv6 || row.observed_ipv6 || '—'`（按现有列布局，IP 列拆两段显示或加芯片，取改动最小方式）
- `ServerFormDialog.vue` v6 placeholder 逻辑如需对齐则同步

## 契约 `docs/api-contract.md`

heartbeat 请求节补 `public_ipv6` 说明（与 `public_ip` 平行）。

## 测试

- `publicip_test.go`：v6 端点成功/失败沿用缓存/静态优先/端点返回 v4 被拒
- `observed_ip_test.go` 扩展：上报公网 v6 采纳、私网/非法忽略、`ipv6` 手动值不被覆盖（GET 响应两字段并存）
- `repo`/迁移测试：列存在、默认空、心跳写入
- web 现有列枚举断言同步（`migration_identity_accounting_test.go`、`server_details_test.go` 如涉及）

## 回滚

单 commit revert + 迁移列保留无害（SQLite 不需 drop，回滚代码即停用）；字段双向兼容。
