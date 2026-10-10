# PRD: Agent 探测公网 IPv6

## 背景

上一任务（agent-public-ip-report）只自动探测 IPv4。`servers.ipv6` 仍是纯手动库存字段，服务器有公网 IPv6 时面板不知道，UI 需要手工维护。

## 目标

Agent 双栈探测出口公网 IP：v4 沿用现有端点，v6 走 IPv6-only 端点；心跳上报 `public_ipv6`；面板新增 `observed_ipv6` 列存储（不覆盖手动 `ipv6`），前端显示回退。

## 需求

1. Agent 探测器双栈化：v6 走 IPv6-only 端点（默认 `https://api6.ipify.org`，可配置 `public_ipv6_url` / `AGENT_PUBLIC_IPV6_URL`），缓存/TTL/失败语义与 v4 相同；无 v6 连通性时探测失败 → 上报为空，绝不影响心跳。
2. 静态配置 `public_ipv6`（yaml + env `AGENT_PUBLIC_IPV6`）优先于自动探测。
3. 心跳请求新增可选字段 `public_ipv6`；面板仅采纳「合法且公网」的 IPv6（拒绝私网 ULA fc00::/7、链路本地 fe80::/10、回环、组播、未指定），非法值静默忽略。
4. 新迁移 `0018_server_observed_ipv6.sql`：`servers.observed_ipv6 TEXT NOT NULL DEFAULT ''`；心跳写入。
5. 显示回退：手动 `ipv6` 优先，其次 `observed_ipv6`（服务器列表 IP 列、节点列表服务器地址族芯片）；前端类型同步。
6. 契约文档补 `public_ipv6` 字段。

## 非目标

- 不用探测值覆盖/回写手动 `ipv6` 字段。
- 不改动订阅渲染（节点级 `ipv6_address` 逻辑不变）。
- 不做 NAT64/DNS 探测等花式方案。

## 验收标准

1. agent 上报合法公网 v6 → `observed_ipv6` 存储并在 UI 回退显示中生效；手动 `ipv6` 非空时仍优先显示手动值。
2. 上报私网/非法 v6 → 静默忽略，心跳 200。
3. 无 v6 环境的 agent 心跳正常（`public_ipv6` 为空）。
4. 迁移幂等、旧库升级默认 `''`；`go vet ./... && go test ./...`（含迁移测试）、web 检查通过。
