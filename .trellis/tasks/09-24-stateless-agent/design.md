# 技术设计：agent 无状态部署

## 1. 目标与边界

让 agent 在**不挂载任何持久卷**的前提下启动并持续工作。落地方式：把 agent 目前写入 `state.json` 的三类状态改由面板持有：

1. **身份**：由"一次性注册换 token"改为面板签发、可随时查看的长期 agent key（用户决策 D1）。
2. **批次续传序号**：由面板按 agent 回传 `MAX(seq)`，agent 启动时对账采纳。
3. **已应用配置版本**：不再持久化；启动时强制重新拉取并应用。

边界：不改 sing-box 渲染与节点协议；不改计费/结算逻辑；不引入外部组件。采用**干净切换**（D2）：面板与所有 agent 一起升级，旧注册流程整体移除。

## 2. 现状锚点（改造点）

- agent 状态文件：`internal/agentstate/agentstate.go`（Load/Save，0600 原子写）。
- agent 启动：`cmd/agent/main.go:55` 加载 state，`:86` 组装 Loop。
- 身份/注册：`internal/agentruntime/loop.go:145-192`（EnsureIdentity），`:193-197`（saveState）。
- 协议客户端：`internal/agentclient/client.go`（`Register`、`SetToken`、`Heartbeat`）。
- 面板 agent 中间件：`internal/web/agent.go:29-49`（按 `token_hash` 查 agent）。
- 面板注册/心跳：`internal/web/agent.go:70-171`。
- 管理端 token 接口：`internal/web/servers.go:247-301`（register-token、agent-token）。
- 路由：`internal/web/web.go:126-133`（agent），`:162-163`（管理端 token）。
- DB：`agents(server_id UNIQUE, token_hash, version, last_seen_at, ...)`（`0001_init.sql:52-60`）；`servers.register_token_hash/register_token_expires_at`（`:42-50`）；批次表 `traffic_batches/device_batches` PK `(agent_id, seq)`（`:126-140`，visits 见 `0002`）。
- 前端：`web/src/pages/ServerDetailPage.vue`、`web/src/api/servers.ts`、`web/src/api/types.ts`、`web/src/utils/installCommands.ts`。
- 部署：`deploy/Dockerfile.agent`、`deploy/agent.docker-compose.yml`、`deploy/docker-compose.all-in-one.yml`、`deploy/install-agent.sh`、`deploy/panel-agent.service`、`deploy/agent.example.yaml`、`deploy/.env.example`。
- 文档：`README.md`、`docs/api-contract.md`。

## 3. 身份模型（D1 + D3）

### 3.1 存储

复用 `agents` 表（每 server 一行，`server_id UNIQUE`），字段调整：

- `token_hash` → 重命名为 `key_hash`（保持 `UNIQUE`）：Bearer key 的 SHA-256，用于认证查找。
- 新增 `key_enc BLOB`：用面板 `app_key` 经 `internal/secrets.Encrypt` AES-GCM 加密的明文 key，用于"随时查看"（D3-B）。复用节点 `secret_enc` 的同一套机制。
- `version`、`last_seen_at` 不变。

> 认证只需 `key_hash` 做等值查找；`key_enc` 仅供管理端回显。可解密存储是 D3-B 明确选择的代价。

### 3.2 管理端接口（替换旧两个接口）

- `POST /api/servers/{id}/agent-key`：生成或重置。流程：`adminauth.NewToken()`（32B→64 hex）→ 计算 `HashToken` → `secrets.Encrypt(app_key, key)` → upsert `agents`（按 `server_id`）设置 `key_hash`、`key_enc`、`updated_at`。返回 `{ "agent_key": "<plaintext>" }`。重置立即让旧 key 失效。
- `GET /api/servers/{id}/agent-key`：读取 `key_enc`，`secrets.Decrypt` 后返回 `{ "agent_key": "<plaintext>" }`；若 `key_enc` 为 NULL（迁移来的旧 agent 尚未重置）返回 `409 conflict`，提示先重置。
- 移除 `POST /api/servers/{id}/register-token` 与 `POST /api/servers/{id}/agent-token`。

### 3.3 agent 端认证

- 移除 `POST /api/agent/register`。
- `requireAgent` 改为按 `key_hash` 查找（`GetAgentByTokenHash` → `GetAgentByKeyHash`，语义不变，仅换列名）。
- agent 用配置项 `agent_key` 作为 `Authorization: Bearer`，全程不落盘。

## 4. 批次续传（去本地状态的关键）

保留 `(agent_id, seq)` 去重语义不变，**不新增 seq 列**，续传点从批次表派生：

- 面板在心跳响应中返回三个续传点：`traffic_seq`、`device_seq`、`visit_seq`，取值 `SELECT COALESCE(MAX(seq),0) FROM <batch_table> WHERE agent_id = ?`（PK 即索引，O(log n)）。
- agent 侧：序号仅存内存。每次心跳成功后执行对账 `local = max(local, resume)`（单调不减），因此：
  - agent 重启（内存清零）→ 下一次心跳把 local 抬到面板当前最大值 → 后续批次 `seq = max+1`，不与历史 `(agent_id, seq)` 冲突。
  - 有 in-flight 未确认批次（local 已领先）→ `max` 保护，不会回退，重试仍用原 seq，去重幂等。
- 启动顺序保证：`Loop.Run` 先执行 `heartbeatPass`，再 `syncPass`、`telemetryPass`（`loop.go:110-112`），所以首次上报前续传点已就绪。
- 心跳响应同时回带 `server_id`，agent 断言等于 `cfg.ServerID`，不等则报错退出，保留原有"错配即拒绝绑定"的安全语义。

`applied_revision`：不持久化也不服务端跟踪。agent 启动即 `forceApply`（`loop.go:87,247-249`）拉取全量配置并应用；运行期靠 `server_revision`/config `status` 判定是否需要重应用。代价是每次重启会重建一次 sing-box 实例（可接受，xboard 同款）。

## 5. 配置与协议变更

### 5.1 agent 配置（`internal/config`）

- `Agent` 结构：删除 `Token`、`RegisterToken`、`StatePath`；新增 `AgentKey`。保留 `ServerID`（用作错配断言）。
- yaml：`agent_key`；env：`AGENT_KEY`。删除 `AGENT_TOKEN`/`AGENT_REGISTER_TOKEN`/`AGENT_STATE_PATH`。
- 校验：`agent_key` 必填，`server_id >= 1`。

### 5.2 agent 运行时

- `cmd/agent/main.go`：不再 `agentstate.Load`；用内存 `agentstate.State`（或精简为内存结构）。
- `loop.go`：删除 `EnsureIdentity` 的注册分支与 `saveState` 文件写；`LoopOptions` 去掉 `StatePath`；保留 `EnsureIdentity` 仅做 `client.SetKey(cfg.AgentKey)`；心跳响应处理加入续传对账与 `server_id` 断言。
- `agentclient`：删 `Register`/`RegisterResponse`；`SetToken`→`SetKey`（内部仍发 Bearer）；`HeartbeatResponse` 增 `ServerID`、`TrafficSeq`、`DeviceSeq`、`VisitSeq`。

## 6. DB 迁移（`0005_stateless_agent.sql`）

1. `ALTER TABLE agents RENAME COLUMN token_hash TO key_hash;`
2. `ALTER TABLE agents ADD COLUMN key_enc BLOB;`（旧行 NULL，须重置）
3. `servers`：删除 `register_token_hash`、`register_token_expires_at` 及唯一索引（先 `DROP INDEX`，再 `ALTER TABLE ... DROP COLUMN`；若 SQLite 版本限制则按迁移框架的表重建模式处理，迁移器已 `PRAGMA foreign_keys=OFF` + 单事务）。
4. 不触碰批次表与历史数据，去重与保留策略不受影响。

迁移是不可逆的向前迁移。**升级前必须备份 `panel-data` 卷**。

## 7. 部署产物

- `Dockerfile.agent`：删除 `ENV AGENT_STATE_PATH` 与 `VOLUME /var/lib/panel-agent`。
- `agent.docker-compose.yml` / `docker-compose.all-in-one.yml`：删除 `agent-state` 卷；`AGENT_KEY` 替代 token 变量；`AGENT_SERVER_ID` 保留；`init: true` 与端口映射不变。
- `install-agent.sh`：写 `agent.yaml` 含 `agent_key`/`server_id`；删除 `STATE_DIR` 及赋权；保留用户创建与 systemd 安装。
- `panel-agent.service`：移除针对 state 目录的 `ReadWritePaths`；保留 `CAP_NET_BIND_SERVICE`。
- `agent.example.yaml`、`deploy/.env.example`、`deploy/README.md`：同步。

## 8. 前端

- `web/src/api/servers.ts`：`createRegisterToken`/`rotateAgentToken` → `getAgentKey`/`generateAgentKey`；`api/types.ts` 相应替换。
- `ServerDetailPage.vue`：以"Agent Key"卡片替换原 register token + agent token 两块；支持查看/复制/重置，重置需二次确认；安装命令用新 key。
- `utils/installCommands.ts`：占位符改为 agent key，env 名改为 `AGENT_KEY`。

## 9. 兼容 / 上线 / 回滚

- **干净切换**：面板与全部 agent 必须同版本升级。旧 agent 调 `/api/agent/register` 将 404，无法连接；升级后需在 Server 详情页为每个 server 生成 agent key 并更新节点配置。
- 回滚：恢复升级前的 `panel-data` 备份 + 回退镜像。迁移向前执行，不可在旧二进制上原地回退。

## 10. 权衡与风险

- **优点**：agent 无需持久卷、无本地敏感文件；身份/续传单一可信源在面板；DB 备份即覆盖身份与去重状态。
- **代价**：每次 agent 重启重建 sing-box；in-flight 内存批次在重启时丢失（与现状一致）；面板 DB 成为强依赖（丢失需重新签发 key）。
- **安全**：key 以 hash 认证 + `app_key` 加密回显；`app_key` 缺省自动生成时与节点密钥同款取舍（`README.md`）。重置即时失效。
- **风险点**：迁移删列/改名需在 SQLite 上小心；`testutil_test.go:271` 的敏感字段禁用列表需同步移除 `register_token_hash`；e2e/hermetic 测试与 `docs/api-contract.md`、`README.md` 需整体更新。
