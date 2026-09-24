# Design — 创建服务器时自动生成 Agent Key

## 边界与触点

| 层 | 文件 | 改动 |
| --- | --- | --- |
| Backend HTTP | `internal/web/servers.go` | `handleServerCreate` 生成并落库 key，返回 `agent_key` |
| Backend DTO | `internal/web/dto.go` | 新增 `serverCreateDTO`（内嵌 `serverDTO` + `agent_key`） |
| Repo | `internal/repo/servers.go` | 新增 `CreateServerWithAgentKey`，单事务写 servers + agents |
| Frontend 类型 | `web/src/api/types.ts` | 新增 `CreateServerResult = Server & { agent_key: string }` |
| Frontend API | `web/src/api/servers.ts` | `createServer` 返回 `CreateServerResult` |
| Frontend 交互 | `web/src/components/ServerFormDialog.vue` | 创建成功 emit `created(id)` |
| Frontend 路由 | `web/src/pages/ServersPage.vue` | 处理 `created` → `router.push('/servers/{id}')` |
| 文档 | `docs/api-contract.md` | 更新 `POST /api/servers` 响应 |
| 测试 | `internal/web/servers_nodes_test.go` | 新增创建即签发 key 的用例 |

## 数据流

```
POST /api/servers {"name":"HK-01"}
  -> validateServerName
  -> key   := adminauth.NewToken()                     // 32B hex
  -> keyEnc:= secrets.Encrypt(h.appKey, []byte(key))
  -> id    := repo.CreateServerWithAgentKey(ctx, name, active,
                 adminauth.HashToken(key), keyEnc)     // 单事务
  -> s     := repo.GetServer(id)
  -> 201 { ...serverDTO, "agent_key": key }
```

## 契约

- `POST /api/servers` 的 `201` 响应在现有 server DTO 基础上**新增** `agent_key`（附加字段，向后兼容）。`docs/api-contract.md:265` 同步。
- `GET /api/servers/{id}/agent-key`、`POST /api/servers/{id}/agent-key` 语义不变；对"repo 直插/迁移 agent 无 `key_enc`"仍返回 409。
- 不 bump server revision（`docs/api-contract.md:299`）。

## 关键决策

1. **签名放在 HTTP handler 层，而非 `repo.CreateServer`**：web 测试的 `seedServer` 直接调 `repo.CreateServer`（`testutil_test.go:209`），保留它"无 key"的语义可让既有 409 断言（`servers_nodes_test.go:467`）继续有效。
2. **单事务写 server + agent**：新增 `CreateServerWithAgentKey` 用 `repo.Tx`（`repo.go:25`）保证 R5——key 生成/落库失败时 server 不残留。
3. **前端跳转到详情页 Agent 区**（用户选定方案 a）：详情页已加载并展示 Agent Key 与安装命令，无需新增 UI 组件。创建响应里的 `agent_key` 仍返回，供 CLI/自动化使用，但 UI 不依赖它。

## 兼容与风险

- e2e 只读响应 `id`（`internal/e2e/env_test.go:128`、`integration_test.go:29`），新增字段安全。
- 前端 `CreateServer` 返回类型由 `Server` 放宽为 `Server & { agent_key }`，无破坏。
- 若 `secrets.Encrypt` 失败（`app_key` 缺失，实际不会发生），请求 500 且事务回滚，不产生半成品。

## 回滚

- 无 schema 迁移。回滚代码后，已自动签发的 key 仍可用（管理员可在详情页重置）。
