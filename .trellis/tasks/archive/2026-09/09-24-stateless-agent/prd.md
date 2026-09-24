# 支持 agent 无状态部署：身份与批次序号下沉到面板

## Goal

让 agent 节点像 xboard-node 一样"只需启动、无需挂载持久卷"：把 agent 目前落盘到 `state.json` 的身份、配置进度与批次序号改由面板侧持有和下发，使容器/进程重建后无任何本地状态即可继续工作。

## Background（已确认事实）

- agent 本地状态字段：`AgentID`、`AgentToken`、`ServerID`、`AppliedRevision`、`TrafficBatchSeq`、`DeviceBatchSeq`、`VisitBatchSeq`（`internal/agentstate/agentstate.go`）。
- 身份现状二选一（`internal/config/config.go:151-165`、`internal/agentruntime/loop.go:145-192`）：静态 `token`，或一次性 `register_token`（注册后换取 agent token，面板随即清空注册 token，`internal/repo/agent_ops.go:37-40`；测试 `internal/web/agent_test.go:95`）。
- 面板无"长期 agent 凭据"查看入口：注册返回的 token 只出现一次，库里只存 hash（`0001_init.sql:52-60`）。
- 遥测去重按 `(agent_id, seq)`：traffic/device/visit 各有 `*_batches` 表，命中相同 seq 视为重复不入库（`internal/repo/telemetry.go:41-95`、`internal/web/agent_telemetry.go:128,215,322`）。agent 侧 seq 必须单调递增且跨重启保持，否则重启后从 1 开始被判重丢弃。
- `agent_id` 每 server 唯一（`agents.server_id UNIQUE`），重复注册不会产生重复 agent 行。
- 配置下发已带进度：`GET /api/agent/config?version=<applied>`（`internal/agentclient/client.go:208-218`）；心跳返回 `server_revision`（`internal/web/agent.go:161-170`）。
- agent `pending` 缓冲在内存，重启即丢，状态搬到面板侧不会更差。
- 面板已有可复用的加密与令牌工具：`internal/secrets.Encrypt/Decrypt`（`app_key` AES-GCM，节点 `secret_enc` 同款）、`adminauth.NewToken/HashToken`。

## Decisions

- **D1 身份模型**：面板为每个 server 提供可查看/可重置的长期 agent key，用户填进 agent 的 env/config，agent 直接认证，取消一次性注册步骤。
- **D2 上线方式**：干净切换。面板与所有 agent 同版本升级，旧注册流程整体移除，节点需重新配置。
- **D3 key 可见性**：随时可查看。key 以 `app_key` 加密存储，管理员可在 Server 详情页回显（`internal/secrets` 机制）。

## Requirements

- R1：agent 重建后不依赖任何本地持久文件即可完成认证并继续上报（设计 §3、§5）。
- R2：批次序号不再由 agent 本地持久化，改由面板按 `(agent_id, seq)` 派生续传点并下发（设计 §4）。
- R3：保留遥测幂等：重试同一批次使用相同 seq，面板去重后不重复计数（设计 §4）。
- R4：面板提供长期 agent key 的生成/查看/重置接口与 UI，key 以 hash 认证、以 `app_key` 加密回显（设计 §3、§8）。
- R5：移除一次性注册与旧 agent-token 轮换接口/流程（设计 §3、§9）。
- R6：清理 agent 本地状态持久化（`state_path`、`agentstate` 文件 IO、`AGENT_STATE_PATH`）（设计 §5）。
- R7：部署产物（Dockerfile / compose / systemd / 安装脚本 / 示例 / 文档）更新为无持久卷即可运行（设计 §7）。
- R8：不持久化 `applied_revision`：启动强制拉取并应用配置，运行期按 revision/status 判定（设计 §4）。

## Acceptance Criteria

- [ ] AC1：删除 `agent-state` 卷并在重建后，agent 仍能认证、心跳、上报流量；不生成任何本地状态文件。
- [ ] AC2：管理端可生成、查看、重置某 server 的 agent key；重置后旧 key 立即 401。
- [ ] AC3：agent 在已上报 `seq=N` 后重启，下一次上报 `seq>N`，面板不将新批次判重丢弃，用户流量不丢不重。
- [ ] AC4：agent 重试同一 in-flight 批次使用相同 seq，面板去重后计数不变。
- [ ] AC5：`/api/agent/register`、`/api/servers/{id}/register-token`、`/api/servers/{id}/agent-token` 不再存在；新 agent 用 agent key 正常连接。
- [ ] AC6：`deploy/` 与根 `README.md` 的 agent 部署路径不再要求任何持久卷/状态目录。
- [ ] AC7：`make vet`、`make test`、前端 `npm run build`、panel/agent 构建通过。

## Out of Scope

- 不改 sing-box 渲染、节点协议与计费/结算算法。
- 不改用户侧 API 与订阅逻辑。
- 不引入外部消息队列/缓存。
- 不保留新旧 agent/面板混部兼容（D2 明确为干净切换）。

## Notes

- 迁移不可逆，升级前必须备份 `panel-data` 卷。
- 相关设计见 `design.md`，执行顺序与验证命令见 `implement.md`。
