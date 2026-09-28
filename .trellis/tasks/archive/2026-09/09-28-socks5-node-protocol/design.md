# Design: SOCKS5 托管节点协议

## 1. 范围

新增 `socks` 作为托管节点协议，端到端打通：创建/编辑/筛选 → sing-box 入站下发 → 用户订阅（general + clash-meta）→ 按用户鉴权与流量/设备归属 → 可作链式代理出口。无协议设置字段。

决策（用户确认）：
- 凭据：每用户 `username = u-<用户ID>`、`password = 用户 UUID`（模型 A）。
- 支持 socks 作为链式出口。
- 无 settings。

## 2. 凭据与身份

- sing-box `auth.User` 只有 `username`/`password`（无 `name`）；SOCKS 入站把 `metadata.User` 设为 **username**。所以入站 username 必须是 `u-<id>`，运行时 `internal/kernel/singbox/tracker.go` 才能归属（`buildTable` 用 `userName(u.ID)`）。
- 新增导出助手 `internal/singbox.NameForUser(id int64) string` 返回 `u-<id>`，供入站渲染与订阅渲染共用（`internal/singbox` 包已被 `subscription`、`web` 引用）。
- 订阅渲染目前只拿 `userUUID`，socks 需要数字 ID。引入：

```go
// internal/subscription/render.go
type User struct {
    ID   int64
    UUID string
}
```

并把订阅渲染入口签名从 `userUUID string` 改为 `user User`：`RenderGeneral`、`RenderGeneralLinks`、`RenderGeneralLinksMerged`、`RenderClashFiltered`、`RenderClashFilteredMerged` 及内部 `renderURI`/`renderProxy`。调用方 `internal/web/subscriptions.go:221,231` 传 `subscription.User{ID: u.ID, UUID: u.UUID}`。测试里的 `"user-uuid"` 字面量改为 `User{UUID: "user-uuid"}`（机械改动）。

## 3. sing-box 渲染（`internal/singbox/singbox.go`）

- 常量 `ProtocolSocks = "socks"`（与其它协议同处）。
- `renderInbound` 增加 `case ProtocolSocks: return renderSOCKS(n)`：

```go
func renderSOCKS(n Node) (map[string]any, error) {
    users := make([]map[string]any, 0, len(n.Users)+len(n.Relays))
    for _, u := range n.Users {
        users = append(users, map[string]any{"username": NameForUser(u.ID), "password": u.UUID})
    }
    for _, r := range n.Relays {
        users = append(users, map[string]any{"username": RelayUserName(r.EntryServerID), "password": DeriveRelayPassword(appKey, n.ID, r.EntryServerID)})
    }
    return map[string]any{"type": ProtocolSocks, "tag": inboundTag(n), "listen": "::", "listen_port": n.Port, "users": users}, nil
}
```

  （relay 用户名的既有写法以 hysteria2/anytls 的 `relay-<sid>` 为准，实现时对齐。）注意 `renderSOCKS` 需要 `appKey` 以派生 relay 密码，签名与其它 renderer 一致。
- `renderChainOutbound` 增加 `case ProtocolSocks: return renderSOCKSOutbound(appKey, c, tag)`：

```go
return map[string]any{"type": ProtocolSocks, "tag": tag, "server": c.DialAddress, "server_port": c.Exit.Port,
    "username": RelayUserName(c.EntryServerID), "password": DeriveRelayPassword(appKey, c.Exit.ID, c.EntryServerID)}, nil
```

- 无 TLS、无 settings 读取。

## 4. 协议设置校验（`internal/web/node_settings.go`）

- `nodeSettingsSchemas` 增加 `repo.ProtocolSocks: {}`（空 schema：接受空 settings，拒绝任何未知字段）。
- `validateProtocolSettings` 增加 `case repo.ProtocolSocks: return nil`。
- `nodes.protocol_settings` 恒为 `{}`，`secret_enc` 可保持 NULL。

## 5. DB 迁移（`internal/db/migrations/0011_nodes_socks_protocol.sql`）

全表重建以放开 `nodes.protocol` 的 CHECK（沿用 0003 的 `nodes_new` + rename 模式，runner 已关闭 FK）：

- 新表列与 live 一致：`id, server_id, address, name, protocol, port, protocol_settings, rate, tags, secret_enc, status, created_at, updated_at, ipv6_enabled, ipv6_address, chain_node_id`。
- CHECK 改为 `('shadowsocks','vless','hysteria2','anytls','socks')`；保留 port/status CHECK、`server_id REFERENCES servers ON DELETE CASCADE`、`chain_node_id INTEGER REFERENCES nodes(id)`。
- 复制全部列后 `DROP TABLE nodes; ALTER TABLE nodes_new RENAME TO nodes;`。
- 重建索引：`idx_nodes_server (server_id)`、`idx_nodes_port_active (server_id, port) WHERE status='active'`、`idx_nodes_chain_node (chain_node_id)`。不重建 0001 的 `UNIQUE(server_id,name/port)`（0003 已移除）。
- 不修改已应用的 0001/0003（禁止改已应用迁移语义）。
- `internal/db/db_test.go` 迁移计数 10 → 11。
- 新增 `migration_socks_test.go`：升级后插入各协议含 `socks` 成功、`snell` 被拒、既有行与 `user_nodes`/`online_devices` 存活、`PRAGMA foreign_key_check` 干净、索引存在、幂等。
- `migration_anytls_test.go:23` 协议列表加 `"socks"`。

## 6. 订阅渲染（`internal/subscription/render.go`）

- `renderURI` 增加：

```go
case singbox.ProtocolSocks:
    credential := base64.RawURLEncoding.EncodeToString([]byte(singbox.NameForUser(user.ID) + ":" + user.UUID))
    return "socks://" + credential + "@" + host + "#" + name, nil
```

- `renderProxy` 增加：

```go
case singbox.ProtocolSocks:
    p["type"] = "socks5"           // Clash 类型名是 socks5，而非 socks
    p["username"] = singbox.NameForUser(user.ID)
    p["password"] = user.UUID
    p["udp"] = true
```

- 占位符 map（`:589`）增加 `"__SOCKS_PROXIES__": singbox.ProtocolSocks`；`names[n.Protocol]` 分组天然得到 `"socks"`。
- `internal/web/subscriptions.go` 的 hy2/anytls TLS 完整性跳过逻辑不适用于 socks。

## 7. Web 校验 / Agent 配置

- `internal/web/nodes.go:22-27` `validProtocols` 加 `repo.ProtocolSocks: true`；错误文案 `:44,:227` 更新为包含 socks（`want shadowsocks, vless, hysteria2, anytls or socks`）。
- `internal/web/agent_config.go:301` `agentCredential` 签名加 `userID int64`（唯一调用点 `:162` 有 `u.ID`），增加：

```go
case singbox.ProtocolSocks:
    return map[string]any{"contract": "socks-v1", "username": singbox.NameForUser(userID), "password": userUUID}, nil
```

  漏掉该分支会让含 socks 节点的整个 agent config 构建失败。

## 8. 前端

- `web/src/api/types.ts:4` `Protocol` 加 `'socks'`。
- `web/src/utils/labels.ts:58-71` `case 'socks': return 'SOCKS5'`。
- `web/src/components/NodeFormDialog.vue`：`protocolOptions` 加 socks；`settingsPayload` 对 socks 返回 `{}`；`loadNodeDetail`/`validationMessage`/`changeProtocol` 增加 socks 分支（无设置区块，可加一句说明文案）；模板增加 `v-else-if="protocol === 'socks'"` 的说明块。
- `web/src/pages/NodesPage.vue:66-72` 筛选项、`:273-276` deep-link 解析加 `'socks'`。
- `web/src/pages/SettingsPage.vue:270` 占位符帮助文本加 `__SOCKS_PROXIES__`。

## 9. 文档 / 规范

- `README.md:23,113` 协议列表加 SOCKS5。
- `docs/api-contract.md`：协议枚举、settings 段（socks 无设置）、创建示例、Clash proxy 类型（socks5）、agent credential 契约（`socks-v1`）、`__SOCKS_PROXIES__`。
- `.trellis/spec/backend/node-protocol-settings.md` 增加 SOCKS 小节；`directory-structure.md:58`、`error-handling.md:129` 的协议枚举同步。

## 10. 测试

- 新增 `internal/web/nodes_socks_test.go`（以 `nodes_anytls_test.go` 为模板）：创建校验（空 settings 通过、未知字段 422）、agent config 入站 `type=socks`/`tag=socks-<id>`/`users[].username=u-<id>`/`password=uuid`、credential `socks-v1`、订阅 general 含 `socks://` 与 base64(user:pass)、clash `type=socks5`+username/password/udp。
- `internal/singbox/singbox_test.go` 增加 `TestRenderSOCKS`；`chain_test.go:201` 循环加 socks。
- `internal/subscription/render_test.go:34,45,199-219,288` 协议列表加 socks；`render_test.go` 签名 `User{}` 迁移。
- `internal/web/servers_nodes_test.go:200-227` 加 socks 合法 settings 用例；`:187-197` 坏协议用例不变。
- `internal/e2e/integration_test.go:43-78`、`integration_chain_test.go:39` 加 socks（嵌入式 sing-box 会实际校验渲染）。
- `internal/db` 见第 5 节。

## 11. 兼容性与回滚

- 增量：新协议仅在 admin 主动创建 socks 节点时生效；既有 4 种协议的渲染字节不变（本次只新增 switch 分支，且订阅签名改动是纯参数封装）。
- 订阅层签名 `User` 改动影响所有协议的调用，但输出不变 → 重点回归 `internal/subscription` 全量测试。
- 回滚：回退代码；0011 迁移为可加性 CHECK 放宽，残留无害。

## 12. 已知限制 / 延后

- 自定义节点分享链接暂不解析 `socks://`（仅托管节点）。
- general 的 socks 分享链接使用 base64url(user:pass) 形式（SIP002 风格）；若目标客户端需要明文 `socks5://user:pass@`，后续可调整。
