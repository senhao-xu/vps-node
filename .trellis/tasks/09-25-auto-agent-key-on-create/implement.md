# Implement — 创建服务器时自动生成 Agent Key

## 执行清单

1. **Repo**：`internal/repo/servers.go` 新增
   `CreateServerWithAgentKey(ctx, name, status, keyHash string, keyEnc []byte) (int64, error)`，
   在 `repo.Tx` 内 `INSERT servers` 后 `INSERT agents(server_id,key_hash,key_enc,version='')`。
2. **DTO**：`internal/web/dto.go` 新增
   ```go
   type serverCreateDTO struct {
       serverDTO
       AgentKey string `json:"agent_key"`
   }
   ```
3. **Handler**：`internal/web/servers.go` `handleServerCreate`
   - `adminauth.NewToken()` → `secrets.Encrypt(h.appKey, ...)` → `repo.CreateServerWithAgentKey`；
   - 返回 `201` + `serverCreateDTO{toServerDTO(s,0,0), key}`。
4. **文档**：`docs/api-contract.md` `POST /api/servers` 段：响应含 `agent_key`，说明自动签发、不 bump revision、可被 `GET agent-key` 读回。
5. **后端测试**：`internal/web/servers_nodes_test.go` 新增 `TestServerCreateGeneratesAgentKey`
   - HTTP `POST /api/servers` → 201，`agent_key` 非空；
   - `GET .../agent-key` → 200 同一值；
   - 用该 key `POST /api/agent/heartbeat` → 200；
   - revision 未 bump（创建后读 server revision 为 0/初始值）。
6. **前端类型/API**：`types.ts` 加 `CreateServerResult`；`api/servers.ts` `createServer` 返回它。
7. **前端对话框**：`ServerFormDialog.vue` 创建分支取回 result，emit `created(server.id)`（编辑分支仍 `saved`）。
8. **前端页面**：`ServersPage.vue` 监听 `@created` → `router.push('/servers/' + id)`。
9. **回归**：确认 `TestServerAgentKeyLifecycle` 与 `TestAgentKeyGetConflictsWithoutStoredKey` 无需改动即通过。

## 验证命令

```sh
go build ./...
go vet ./...
go test ./internal/web/ ./internal/e2e/
go test ./...
cd web && npm run typecheck && npm run lint && npm run build
```

## 风险文件 / 回滚点

- `internal/web/servers.go`：创建路径核心，回滚即恢复"不签发"。
- `internal/repo/servers.go`：新增方法，不改 `CreateServer`；`seedServer` 语义不变。
- `web/src/pages/ServersPage.vue`：新增导航，若路由有误仅影响创建后跳转，可单独回退。

## start 前检查

- [ ] `prd.md` / `design.md` / `implement.md` 评审通过（用户已在后续消息明确批准最终规划摘要）。
- [ ] `implement.jsonl` / `check.jsonl` 含真实条目（若走 sub-agent 分发）。
