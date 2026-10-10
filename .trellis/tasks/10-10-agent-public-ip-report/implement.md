# Implement: Agent 公网 IP 探测上报

- [ ] 1. Agent 探测器：`internal/agentruntime/publicip.go`（PublicIPProvider：静态值/缓存 TTL/拉取校验）+ `publicip_test.go`
- [ ] 2. 配置：`internal/config/config.go` 加 `PublicIP`/`PublicIPURL`（yaml `public_ip`/`public_ip_url`，env `AGENT_PUBLIC_IP`/`AGENT_PUBLIC_IP_URL`，默认端点常量）
- [ ] 3. 装配：`loop.go` LoopOptions/heartbeatPass 接入 provider；`cmd/agent/main.go` 构造注入
- [ ] 4. 面板：`internal/web/agent.go` heartbeatRequest 加 `public_ip`，采纳逻辑（公网优先、非法静默忽略）
- [ ] 5. 面板测试：扩展 `internal/web/observed_ip_test.go`（上报采纳/私网回退/非法回退）
- [ ] 6. 契约：`docs/api-contract.md` heartbeat 请求节补 `public_ip` 说明
- [ ] 7. 部署示例：`deploy/agent.example.yaml` 补两个新键注释
- [ ] 8. 验证：`gofmt -l . && go vet ./... && go test ./...`、`go build -tags with_quic,with_utls ./...`、`make build`、`cd web && npm run typecheck && npm run lint`

## 回滚点

单 commit revert；无迁移；字段双向兼容。
