# 轻量级多节点用户管理面板 MVP

## Goal

为个人或小团队提供轻量级 Panel，统一管理用户、物理 VPS、代理入口和 Agent，支持按节点授权用户，并查看流量、当前连接、历史连接日志及节点状态。Panel 是唯一业务数据源，Agent 负责运行时配置应用和数据采集。

## Confirmed Decisions

- 参考 VpsCT 的 Panel + Agent、主动回连、配置应用、流量采集、连接日志和部署思路。
- 参考 Xboard 的用户 UUID、协议适配、流量额度、有效期和节点配置思路，但不引入支付、订单、套餐、邀请返利等业务。
- 第一版节点服务使用 sing-box，支持 Shadowsocks、VLESS Reality、Hysteria2，暂不支持 Snell。
- Panel 与 Agent 使用 HTTPS REST 轮询，不使用 WebSocket；Agent 主动连接 Panel。
- `Server 1 : 1 Agent`、`Server 1 : N Node`、`User N : N Node`。
- 用户使用全局 UUID；VLESS/Hysteria2 使用该 UUID，Shadowsocks 凭据按节点配置和 UUID 派生，不强制建立用户节点密码表。
- 当前连接与连接日志分开；第一版不记录目标主机、域名和目标端口，只记录来源 IP、用户、Server、Node、协议/服务、时间、上下行流量和状态。

## In Scope

- 管理员登录和单管理员基础认证，不做复杂 RBAC 或多租户。
- Dashboard：用户总数、在线用户、Server 总数、在线 Server、今日流量、当前连接数。
- 用户 CRUD、启用/禁用、Token 重置、流量额度修改/重置、有效期修改/立即过期。
- 用户详情：身份、流量、有效期、节点授权、当前连接、历史连接日志。
- Server CRUD、启用/禁用、删除；展示 Agent 心跳状态和 CPU、内存、磁盘、Uptime、版本。
- Node CRUD；Node 归属一个 Server，保存协议、端口、协议配置和状态。
- 用户详情直接维护 `user_nodes` 授权；Node/Server 详情查看用户和连接。
- 用户、Node、Server 维度的累计流量和按时间聚合统计。
- 连接日志查询、原始日志保留天数、聚合保留天数和采集开关。
- Agent 注册、Token Hash、Token 轮换、HTTPS Bearer 认证、心跳、版本化配置拉取、sing-box 校验/原子替换/回滚、流量/连接/日志上报。
- Panel 离线时 Agent 保留最后一次有效配置。

## Data Model

- `users`：UUID、Token、状态、流量额度/已用、开始时间、到期时间。
- `servers`：物理 VPS 地址、状态、系统指标、最后心跳和 Agent 关联。
- `agents`：Server 关联、Token Hash、版本、最后心跳。
- `nodes`：Server 关联、名称、协议、端口、协议配置和状态。
- `user_nodes`：用户与 Node 的唯一授权关系。
- `sessions`：当前在线连接快照。
- `connection_logs`：历史连接日志，不存传输正文和目标地址。
- `traffic_records`：按用户、Server、Node 和时间记录流量增量。
- `admins`：管理员账号和密码 Hash。

## Out Of Scope

- VPS 开关机、重装系统、SSH、文件管理、Docker、通用服务部署和完整监控告警。
- 支付、订单、商城、套餐、优惠券、工单、邀请返利、多租户、复杂 RBAC、AI。
- WebSocket、Snell、订阅链接、用户端门户、目标地址/端口和传输正文采集。
- 任意配置文件、systemd 命令或 Shell 远程执行。

## Acceptance Criteria

- [ ] 管理员可以创建、编辑、禁用、删除用户，并重置 Token、流量和有效期。
- [ ] 用户详情可以勾选并保存多个 Node 权限，重复授权不会产生重复关系。
- [ ] 只有 `active`、未过期且未超出流量额度的用户才同步到对应 Agent。
- [ ] 一个 Server 可以拥有多个协议 Node，但只有一个 Agent。
- [ ] Agent 注册、Token 认证、心跳、在线/离线状态和 Token 轮换可用。
- [ ] Agent 只能访问绑定 Server 的配置和上报接口。
- [ ] Panel 生成 sing-box 配置，版本变化可被 Agent 检测；应用失败保留旧配置并报告错误。
- [ ] 能生成并应用 Shadowsocks、VLESS Reality、Hysteria2 配置。
- [ ] 流量增量幂等累计到用户、Node、Server，不因重复批次重复计量。
- [ ] 用户和 Node/Server 详情可查看当前连接，过期快照不再显示在线。
- [ ] 连接日志包含用户、Server、Node、来源 IP、协议/服务、连接时间、结束时间、上下行流量和状态。
- [ ] 连接日志不包含目标地址、目标端口、传输正文、Token 或密码。
- [ ] 原始日志和聚合统计独立保留，后台清理受保留天数和存储上限约束。
- [ ] Admin API 与 Agent API 使用不同认证边界，生产 Agent 通信要求 HTTPS。
- [ ] MVP 提供登录、Dashboard、Users、User Detail、Servers、Server Detail、Settings 页面。

## Risks And Deferred Items

- sing-box 的用户级流量和连接采集能力必须用固定版本验证，不能把配置同步成功当作计量已验证。
- SQLite 适合 MVP 单实例；高并发、备份恢复和 PostgreSQL 迁移后置。
- 管理员密码、用户 Token、协议密钥和 Agent Token 的 Hash/加密边界必须在设计中落实。
