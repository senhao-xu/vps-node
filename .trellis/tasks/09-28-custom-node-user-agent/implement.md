# 执行计划：自定义节点上游 User-Agent

> 复杂任务：按顺序执行；每步完成后跑验证命令。禁止在 `task.py start` 前改产品代码。

## 0. 前置

- [ ] 阅读 `prd.md`、`design.md`；确认 `implement.jsonl` / `check.jsonl` 有真实条目。
- [ ] （经用户批准后）`python3 ./.trellis/scripts/task.py start .trellis/tasks/09-28-custom-node-user-agent`

## 1. 数据库迁移

- [ ] 新增 `internal/db/migrations/0008_custom_nodes_user_agent.sql`：`ALTER TABLE custom_nodes ADD COLUMN user_agent TEXT NOT NULL DEFAULT '';`
- [ ] 验证：`go test ./internal/db/...`

## 2. repo 层

- [ ] `CustomNode`/`NewCustomNode` 加 `UserAgent`；`customNodeSelect`、`ListActiveCustomNodesByUser`、`scanCustomNode`、`CreateCustomNode`、`UpdateCustomNode` 全链路补齐。
- [ ] `UpdateCustomNode` 增加 `userAgent string` + `invalidateCache bool`（清缓存条件：内容变更或 UA 变更）。
- [ ] 验证：`go test ./internal/repo/...`

## 3. subscription 包

- [ ] `DefaultUserAgent = "clash-verge/v2.0.0"`；`FetchSubscription(ctx, rawURL, userAgent string)`；空回退默认。
- [ ] 更新 `fetch_test.go` 及所有调用点（web 两处）。
- [ ] 新增用例：传入 UA 时上游收到该值；空时收到 `DefaultUserAgent`。
- [ ] 验证：`go test ./internal/subscription/...`

## 4. web 层

- [ ] `customNodeDTO` 加 `user_agent`；create/update 请求加 `user_agent`。
- [ ] `validateUserAgent`（长度 ≤255、拒绝控制字符）。
- [ ] create/update 应用规则（links 强制空；update 计算 `invalidateCache`）。
- [ ] `fetchCustomNodeContent` 与 `handleCustomNodeRefresh` 传 `cn.UserAgent`。
- [ ] 测试：创建 subscription 带 UA 返回；links 忽略 UA；非法 UA → 422；更新 UA 清缓存；未设 UA 时刷新请求头为默认。
- [ ] 验证：`go test ./internal/web/... && go vet ./...`

## 5. 前端

- [ ] `types.ts`：`CustomNode.user_agent`、`Create/UpdateCustomNodeInput.user_agent?`。
- [ ] 单点预设常量文件（label/value）；默认 `clash-verge/v2.0.0`。
- [ ] `CustomNodeFormDialog.vue`：subscription 显示 UA 选择（预设 + 自定义），编辑预填、留空=默认。
- [ ] 验证：`npm run typecheck && npm run lint`

## 6. 收尾

- [ ] `make build` 通过；核对 `prd.md` Acceptance Criteria。

## 风险点 / 回滚

- Header 注入校验、UA 变更清缓存是本任务主要风险。
- 回滚以代码为主；新增列保留不影响旧代码（旧代码 select 显式列，不会因多列报错）。
