# Implement: 节点可选 IPv6 入口（订阅追加 v6 条目）

## 0. Preconditions

- `prd.md` / `design.md` 已评审通过，`task.py start` 已执行。
- 先读 `.trellis/spec/backend/database-guidelines.md`、`error-handling.md`、`quality-guidelines.md`、`.trellis/spec/frontend/type-safety.md`、`component-guidelines.md`、`.trellis/spec/guides/cross-layer-thinking-guide.md`。

## 1. DB 迁移

- [ ] 新增 `internal/db/migrations/0004_node_ipv6.sql`，两条 `ALTER TABLE nodes ADD COLUMN`（见 `design.md` §3）。
- [ ] 确认迁移号连续、runner 能 apply-twice 幂等（现有 `db_test.go`）。
- [ ] 新增迁移测试：旧库升级后 `nodes` 两列存在且为默认值，子表行数与 `PRAGMA foreign_key_check` 不变。

## 2. repo 层

- [ ] `internal/repo/nodes.go`：`Node` / `NewNode` 加 `IPv6Enabled bool` / `IPv6Address string`。
- [ ] 同步 `nodeSelect`、`nodeSelectWithServer`、`insertNodeExec`、`UpdateNodeSpec`、`scanNode`、`scanNodeWithServerName`（INTEGER↔bool）。
- [ ] `internal/repo/subscriptions.go`：`ListSubscriptionNodes` 显式列加两列并 Scan。
- [ ] `internal/repo/mutations.go`：`UpdateNodeAndBump` 两条 UPDATE 加两列。
- [ ] repo 测试：带 v6 字段 round-trip（create/get/list）。

## 3. web 层

- [ ] `internal/web/nodes.go`：`createNodeRequest` / `updateNodeRequest` 加字段（update 用指针做 partial）。
- [ ] 新增 `validateNodeIPv6(enabled bool, addr string) error`：enabled 时地址必须为 IPv6 字面量；非空地址始终校验格式。
- [ ] create / update / copy handler 传递字段；copy 逐字复制。
- [ ] `internal/web/dto.go`：`nodeDTO` + `toNodeDTO` 加 `ipv6_enabled` / `ipv6_address`（注意 `nodeDetailDTO` 内嵌自动获得）。
- [ ] `internal/web/servers_nodes_test.go`：create/update v6 字段、非法地址 `422`、DTO 回显；copy 保留字段。

## 4. 订阅层

- [ ] `internal/subscription/render.go`：`Node` 加 `IPv6Address`；新增 `expandIPv6`（见 `design.md` §5）。
- [ ] `RenderGeneral` / `RenderGeneralLinks` / `RenderClashFiltered` 改为遍历 `expandIPv6(n)` 结果；Clash 每个展开条目的 name 都加入分组 names。
- [ ] `internal/web/subscriptions.go:179` 映射 `IPv6Address`（仅当 `n.IPv6Enabled`）。
- [ ] `internal/subscription/render_test.go`：启用 v6 时 Clash 产出两条代理（name 后缀、server 不同、port/凭据相同）、general 产出两条链接；未启用/地址为空/地址等于原地址时只产出一条。

## 5. 前端

- [ ] `web/src/api/types.ts`：`NodeBrief` 加 `ipv6_enabled: boolean` / `ipv6_address: string`；`CreateNodeInput` / `UpdateNodeInput` 加可选同名字段。
- [ ] `web/src/components/NodeFormDialog.vue`：`ToggleSwitch` 控制启用 + IPv6 地址输入（启用时显示）；客户端校验 IPv6 字面量；create/update payload 带字段（关闭时 `ipv6_enabled:false`，地址按需保留）。
- [ ] 组件使用现有 `components/ui/` 原语与 CSS token，不新增图标/SVG（`component-guidelines.md`）。

## 6. 契约

- [ ] `docs/api-contract.md`：按 `design.md` §4.3 的最小 diff 更新 Nodes 相关段落；不扩大范围。

## 7. Validation Commands

```bash
go build ./... && go vet ./... && gofmt -l .
go test -count=1 ./...
go test -race -count=1 ./...
go test -count=1 -tags integration,with_quic,with_utls ./...
go build -tags with_quic,with_utls ./...
go build -tags embed_ui ./...
cd web && npm run typecheck && npm run build && npm run lint
make build
```

## 8. Risky Files / Rollback Points

- `internal/repo/nodes.go` 的列清单：漏改任一处会造成 Scan 列错位（高风险）。改完先跑 `go test ./internal/repo/...`。
- `internal/repo/mutations.go` 两条 UPDATE 必须同时改，否则带/不带 status 的更新行为分叉。
- `internal/subscription/render.go` 的 `onSkip` 与 Clash 分组：展开后必须保证 name 唯一并全部进组。
- 回滚点：每个阶段结束可独立回退；迁移为纯加列，回退代码即可。

## 9. Follow-up Checks Before Done

- [ ] 旧节点订阅输出与升级前逐字一致（回归）。
- [ ] v4/v6 两条连接的使用量在同一节点汇总（现网或集成测试确认）。
- [ ] 更新 `.trellis/spec/backend/database-guidelines.md`（新增列与迁移约定）如有新约定；必要时记录到 spec。
