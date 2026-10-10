# Implement: Agent 探测公网 IPv6

- [x] 1. 迁移：`0018_server_observed_ipv6.sql` + 迁移/列枚举测试同步
- [x] 2. repo：`Server.ObservedIPv6` + select/scan + `RecordHeartbeat` 增参 + node 查询列
- [x] 3. agent 探测器：`PublicIPProvider` 双栈（Get6/静态 v6/v4-拒绝校验）+ 测试
- [x] 4. 配置：`PublicIPv6`/`PublicIPv6URL`（yaml+env）+ `agent.example.yaml` 注释
- [x] 5. 装配：`HeartbeatRequest.PublicIPv6` + loop 填充
- [x] 6. panel：`observedIPv6` 校验/回退 + heartbeat handler 接线 + DTO `observed_ipv6`
- [x] 7. 前端：types + NodesPage 地址族 + ServersPage 显示回退
- [x] 8. 契约：`docs/api-contract.md` 补 `public_ipv6`
- [x] 9. 验证：`gofmt -l . && go vet ./... && go test ./...`、`go build -tags with_quic,with_utls ./...`、`make build`、web typecheck/lint/build

## 回滚点

单 commit revert；`observed_ipv6` 列残留无害；字段双向兼容。

## 追加需求（会话内）

- [x] 10. NodeFormDialog 创建时回显：address 默认服务器 v4（ip || observed_ip），启用 IPv6 时 ipv6_address 默认服务器 v6（ipv6 || observed_ipv6）；仅在字段为空时填充，编辑模式不回显
