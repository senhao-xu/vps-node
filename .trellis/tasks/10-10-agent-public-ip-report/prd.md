# PRD: Agent 公网 IP 探测上报

## 背景

上一轮修复后，面板只在「直连对端为公网」或「经反代带 X-Forwarded-For」时能记录服务器公网 IP；agent 与面板同宿主机 docker 部署（无反代）时，`observed_ip` 只能存空，UI 显示 `—`。用户要求**主动拿到真实公网 IP** 而不是不显示。

## 目标

Agent 主动探测自己的出口公网 IP 并随心跳上报；面板优先采纳上报值。docker 部署下 agent 出网经宿主机 NAT，探测结果即宿主机公网 IP，天然覆盖所有部署形态。

## 需求

1. agent 启动后通过 HTTP 服务（默认 `https://api.ipify.org`，可配置）探测出口公网 IP；结果缓存 30 分钟；失败时静默沿用旧值或跳过，绝不影响心跳与业务。
2. agent 配置支持静态指定 `public_ip`（文件 `public_ip` / 环境变量 `AGENT_PUBLIC_IP`），静态值优先于自动探测；探测端点可配置（`public_ip_url` / `AGENT_PUBLIC_IP_URL`）。
3. 心跳请求新增可选字段 `public_ip`；面板仅采纳「合法且公网」的值，非法/私网值静默忽略（不使心跳失败）。
4. 面板 `observed_ip` 优先级：agent 上报公网 IP > 代理头解析（XFF/X-Real-IP）> 直连对端 > 空（上一轮私网过滤逻辑保留为兜底）。
5. `docs/api-contract.md`（绑定契约）同步 heartbeat 请求字段。

## 非目标

- 不做 IPv6 单独探测（`servers.ipv6` 仍为手动管理）。
- 不做 STUN 探测（HTTP 端点已够用，可配置替换）。
- 不改变 agent 身份/所有权语义：`public_ip` 仅用于展示，不参与鉴权与所有权判定。

## 验收标准

1. agent 上报合法公网 `public_ip` → 面板 `GET /api/servers/{id}` 的 `observed_ip` 等于该值（优先于 XFF 与对端地址）。
2. agent 上报私网或非法 `public_ip` → 面板回退到 XFF/对端解析，心跳返回 200。
3. 探测端点不可达 → agent 心跳照常（不带该字段或带旧缓存值），无崩溃无阻塞超时（单次 5s 上限）。
4. 静态 `AGENT_PUBLIC_IP` 配置优先生效，无需请求外部服务。
5. `go vet ./... && go test ./...`、web 检查、`make build` 全部通过；agent 镜像内嵌 sing-box 正常构建。
