# 执行计划：agent 无状态部署

> 前置：`prd.md` + `design.md` 已评审通过，且用户明确批准最终规划摘要后才可 `task.py start`。
> 建议顺序：先 DB+后端协议，再 agent 端，再前端，最后部署/文档/测试；每阶段跑一次验证。

## 0. 准备

- [ ] `git status` 确认工作区；新建分支（如 `task/stateless-agent`）。
- [ ] 备份本地 `panel-data`（如存在）以便迁移验证：`cp data/panel.db /tmp/panel.db.bak`。

## 1. 后端：身份与 DB

- [ ] `internal/db/migrations/0005_stateless_agent.sql`：`agents.token_hash`→`key_hash`；新增 `key_enc BLOB`；删除 `servers.register_token_hash`/`register_token_expires_at` 及唯一索引。参考迁移器约束（`internal/db/migrations.go:77-115`）。
- [ ] `internal/repo/agents.go`：`token_hash` 列名更新；`GetAgentByTokenHash`→`GetAgentByKeyHash`；新增 `UpsertAgentKey(ctx, serverID, keyHash string, keyEnc []byte)`（insert-or-update by server_id，保留 version/last_seen）。
- [ ] `internal/repo/servers.go`：删除 `GetServerByRegisterTokenHash`、`SetServerRegisterTokenHash`、`Server.RegisterTokenExpiresAt` 及 scan 列。
- [ ] `internal/repo/agent_ops.go`：删除 `RegisterAgent`。
- [ ] `internal/repo/telemetry.go`：新增 `LastTrafficSeq/LastDeviceSeq/LastVisitSeq(ctx, agentID)`（`COALESCE(MAX(seq),0)`）；`IngestVisitBatch` 所在文件同步。批次表 PK 已是索引，无需新索引。
- [ ] `internal/web/agent.go`：`requireAgent` 改名查找；删除 `handleAgentRegister`；`handleAgentHeartbeat` 响应加 `server_id`、`traffic_seq`、`device_seq`、`visit_seq`。
- [ ] `internal/web/servers.go`：删除 `handleServerRegisterToken`、`handleServerAgentToken`；新增 `handleServerAgentKeyGet`（Decrypt 回显 / 无 key_enc→409）、`handleServerAgentKeyGenerate`（NewToken+HashToken+Encrypt+Upsert）。`handleServerGet` 的 agent DTO 去掉 register-token 相关。
- [ ] `internal/web/web.go`：路由替换（删 register/agent-token，加 GET/POST agent-key）。

验证：`go build ./... && go vet ./...`；跑迁移测试 `go test ./internal/db/...`。

## 2. Agent 端

- [ ] `internal/config/config.go`：`Agent` 去 `Token/RegisterToken/StatePath`，加 `AgentKey`；`agentFile` 与 `AGENT_KEY` env；校验更新。
- [ ] `internal/agentclient/client.go`：删 `Register` 及相关类型；`SetToken`→`SetKey`；`HeartbeatResponse` 加字段。
- [ ] `internal/agentruntime/loop.go`：`EnsureIdentity` 改为仅设置 key + 断言 `server_id`；删除文件持久化（`saveState`/`statePath`）；心跳对账 `max(local, resume)`；`LoopOptions` 去 `StatePath`。
- [ ] `cmd/agent/main.go`：去 `agentstate.Load`，使用内存 state。
- [ ] `internal/agentstate`：保留内存结构或瘦身；若完全移除需清理引用与测试。

验证：`go build -tags with_quic,with_utls ./...`；`go test ./internal/agentruntime/... ./internal/agentclient/...`。

## 3. 前端

- [ ] `web/src/api/types.ts`：删 `RegisterTokenResult`/`AgentTokenResult`，加 `AgentKeyResult`。
- [ ] `web/src/api/servers.ts`：`createRegisterToken`/`rotateAgentToken` → `getAgentKey`/`generateAgentKey`。
- [ ] `web/src/pages/ServerDetailPage.vue`：替换 token 区块为 Agent Key 查看/复制/重置（重置二次确认）；更新安装提示。
- [ ] `web/src/utils/installCommands.ts`：占位符与 `AGENT_KEY` 更新。

验证：`cd web && npm ci && npm run build`（或 `npx vue-tsc --noEmit` 若有类型检查）。

## 4. 部署与文档

- [ ] `deploy/Dockerfile.agent`：删 `AGENT_STATE_PATH`、`VOLUME`。
- [ ] `deploy/agent.docker-compose.yml`、`deploy/docker-compose.all-in-one.yml`：删 `agent-state` 卷、`AGENT_REGISTER_TOKEN`；加 `AGENT_KEY`。
- [ ] `deploy/install-agent.sh`：`agent.yaml` 写 `agent_key`；删 `STATE_DIR` 相关。
- [ ] `deploy/panel-agent.service`：去 state 目录 `ReadWritePaths`。
- [ ] `deploy/agent.example.yaml`、`deploy/.env.example`、`deploy/README.md`。
- [ ] `README.md`：agent 部署、配置表、升级/回滚、状态相关段落。
- [ ] `docs/api-contract.md`：agent 认证、register 删除、heartbeat 续传字段、server agent-key 接口。

## 5. 测试

- [ ] 更新/删除：`internal/web/agent_test.go`（register）、`internal/web/auth_test.go`、`internal/web/servers_nodes_test.go`（register-token）、`internal/repo/repo_test.go`（agent token rotate）、`internal/web/testutil_test.go:271`（移除 `register_token_hash`）。
- [ ] 新增：
  - 管理端 agent-key 生成/查看/重置，重置后旧 key 立即 401。
  - 无 key_enc 时查看返回 409。
  - heartbeat 返回续传点，`(agent_id, seq)` 去重仍生效。
  - agent 无状态：hermetic loop 测试中重启后从面板续传点继续、不产生本地文件。
- [ ] `internal/e2e`：确保配置渲染/启动不受影响，去掉 state 依赖。

验证：`make test`；`make test-race`；必要时 `make test-integration`。

## 6. 收尾

- [ ] `make vet` 全绿。
- [ ] 手动冒烟：本地起面板→建 server→生成 agent key→起 agent（无卷）→看到心跳/遥测；删除并重建 agent 容器→身份与续传自动恢复。
- [ ] 迁移冒烟：用带旧数据的 `panel.db` 跑一次迁移，确认删列/改名成功、agent key 需重置。
- [ ] 提交前 `git diff` 审阅；提交信息说明干净切换与迁移要求。

## 风险文件 / 回滚点

- 高风险：`0005_stateless_agent.sql`（不可逆）、`agentruntime/loop.go`（续传对账）、`web/servers.go`（key 加解密）。
- 回滚：任一阶段失败可回退代码分支；DB 迁移一旦执行需用 `panel-data` 备份恢复。
