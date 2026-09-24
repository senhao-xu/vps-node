# 创建服务器时自动生成 Agent Key

## Goal

创建 Server 时自动签发该 Server 的 Agent Key，使管理员创建后立即拿到可直接执行的 Agent 安装命令，免去"创建后再手动点一次生成 Key"的步骤。

## Background（已核实事实）

- 现状：`POST /api/servers` 只插入 servers 行（`internal/web/servers.go:56` → `repo.CreateServer`，`internal/repo/servers.go:38`），不生成 key。
- key 由独立接口 `POST /api/servers/{id}/agent-key` 生成/重置（`internal/web/servers.go:277`）：`adminauth.NewToken()` 生成明文，存 SHA-256 hash 用于鉴权、并用 panel `app_key` 加密密文用于回显（`internal/repo/agents.go:49` `UpsertAgentKey`）。
- 未生成时 `GET /api/servers/{id}/agent-key` 返回 409 `no agent key for this server; generate one first`（`internal/web/servers.go:256`）。
- Agent 侧 `EnsureIdentity` 在 `agent_key` 为空时报错（`internal/agentruntime/loop.go:153`）——Agent 必须持有面板签发的 key 才能启动。
- 契约：生成/重置 key 不 bump server revision（`docs/api-contract.md:299`，identity 非运行时配置）。
- 前端：Server 详情页已展示 Agent Key 与安装命令（`web/src/pages/ServerDetailPage.vue:495`、`web/src/utils/installCommands.ts`）；创建对话框提交后仅 `@saved` 刷新列表，不跳转（`web/src/pages/ServersPage.vue:227`）。
- 测试现状：web 测试的 `seedServer` 直接调 `repo.CreateServer`（`internal/web/testutil_test.go:209`），不走 HTTP handler；`TestServerAgentKeyLifecycle` 依赖"生成前 GET=409"（`internal/web/servers_nodes_test.go:467`）。e2e 走 HTTP `POST /api/servers` 但只读 `id`（`internal/e2e/env_test.go:128`、`internal/e2e/integration_test.go:29`）。

## Requirements

- R1: `POST /api/servers` 成功（201）时，除 server DTO 外返回新生成的明文 `agent_key`。
- R2: 该 key 与 `POST /api/servers/{id}/agent-key` 使用同一机制（NewToken + HashToken + Encrypt），可直接用于 Agent 鉴权。
- R3: 不改变 `GET` / `POST .../agent-key` 现有语义与针对"未生成 / 无 `key_enc`"的 409 行为。
- R4: 不 bump server revision。
- R5: key 生成失败不得导致"server 已创建但接口报错"的残留状态（失败策略见 design）。
- R6: 前端创建成功后跳转到该 Server 详情页（Agent 区已展示并复制安装命令）。

## Acceptance Criteria

- [ ] AC1: `POST /api/servers {"name":...}` → 201，body 含非空 `agent_key`，且该 key 能通过 `POST /api/agent/heartbeat` 鉴权。
- [ ] AC2: 创建后 `GET /api/servers/{id}/agent-key` → 200，返回同一 key。
- [ ] AC3: 不影响 `seedServer`（repo 直插）路径：repo 直插、未生成 key 的 server，GET 仍 409。
- [ ] AC4: 重置 key 语义不变（旧 key 立即失效）。
- [ ] AC5: 前端创建成功后跳转到 `/servers/{id}`，详情页 Agent 区展示该 key 及安装命令，可复制。
- [ ] AC6: `go test ./...` 与前端 typecheck/build 通过；`docs/api-contract.md` 同步。

## Out of Scope

- Agent 自动部署 / 自动注册（系统无自注册流程）。
- 修改 Agent 侧鉴权实现。

## 已定决策

- 创建成功后呈现方式：**跳转到 Server 详情页 Agent 区**（复用现成命令展示，不新增 UI）。见 R6 / AC5。
- 签发放在 HTTP handler 层（非 `repo.CreateServer`），以保留 `seedServer` repo 直插路径的 409 语义。详见 `design.md`。

无阻塞性开放问题。
