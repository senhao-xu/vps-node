# 执行计划：自定义节点查看与更新订阅

> 复杂任务：按顺序执行；每步完成后跑验证命令。禁止在 `task.py start` 前改产品代码。

## 0. 前置

- [ ] 阅读 `prd.md`、`design.md`；确认 `implement.jsonl` / `check.jsonl` 有真实条目。
- [ ] `python3 ./.trellis/scripts/task.py start .trellis/tasks/09-28-custom-node-refresh`

## 1. 解析层（`internal/subscription`）

- [ ] 新增 `summary.go`：`NodeSummary` + `Summarize(links, proxies)`。
- [ ] 复用 `ParseShareURI`、`looseInt`；跳过项返回原始行/`proxy #i` 标识。
- [ ] 新增 `summary_test.go`：覆盖
  - 分享链接正常解析（ss/vless/vmess 至少各一）
  - 非法链接 → skipped
  - Clash proxies（端口为 int/string）→ 正常
  - proxies 缺 name/server/type → skipped
- [ ] 验证：`go test ./internal/subscription/...`

## 2. 后端接口（`internal/web`）

- [ ] `custom_nodes.go`：加 `customNodeEntryDTO`、`customNodeEntriesResponse`，helper `customNodeEntries(cn)`（links=解密+SplitLinkLines；subscription=NormalizeFetchedContent）。
- [ ] `handleCustomNodeNodesGet`：`pathID` → `GetCustomNode`（404）→ 组装 → 200。
- [ ] `handleCustomNodeRefresh`：`GetCustomNode` → 非 subscription → 422；否则 `FetchSubscription` 失败 → 500（不写缓存）；成功 → `UpdateCustomNodeCache` → 组装 200。
- [ ] `web.go`：注册
  - `GET /api/custom-nodes/{id}/nodes`
  - `POST /api/custom-nodes/{id}/refresh`
- [ ] 测试（`internal/web`）：view links / view subscription 有缓存 / view 无缓存不联网 / refresh subscription 成功（断言 fetched_at 更新 + entries）/ refresh links 422 / refresh 上游失败保留旧缓存 / 未知 id 404。
- [ ] 验证：`go test ./internal/web/... && go vet ./...`

## 3. 前端 API 与类型

- [ ] `web/src/api/types.ts`：`CustomNodeEntry`、`CustomNodeEntries`。
- [ ] `web/src/api/customNodes.ts`：`getCustomNodeEntries`、`refreshCustomNode`。
- [ ] 验证：`npm run typecheck`

## 4. 前端 UI

- [ ] 新增 `web/src/components/CustomNodeNodesDialog.vue`：
  - props: `open`、`node: CustomNode | null`
  - 打开时拉取 entries；展示表格（名称/协议/服务器/端口），`skipped` 用 warning 提示
  - subscription 显示“更新订阅”按钮；刷新中禁用；失败用 ErrorBanner
  - 空缓存提示“未拉取，请先更新订阅”
- [ ] `CustomNodesPage.vue`：行操作加“查看节点”；`subscription` 加“更新订阅”；挂载弹窗；刷新成功后 `load()` 回写缓存时间。
- [ ] 验证：`npm run typecheck && npm run lint`

## 5. 收尾验证

- [ ] `make build`（vet + test + build）通过。
- [ ] 手动核对 `prd.md` Acceptance Criteria。
- [ ] 更新 spec（如新增了可复用契约/约定）。

## 风险点 / 回滚

- 主要风险：`summary.go` 的端口类型转换、refresh 失败错误码语义。
- 回滚：删除新路由/handler/组件/api 方法即可，无 migration。
- 若 `task.py start` 因 session 身份报错，按提示设置 `TRELLIS_CONTEXT_ID` 后重试。
