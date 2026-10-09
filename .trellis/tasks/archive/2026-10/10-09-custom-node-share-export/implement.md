# Implement — 自定义节点分享导出与编辑回显

## Ordered Checklist

1. **subscription 渲染抽取（无行为变更）**
   - 在 `internal/subscription/render.go` 抽出 `RenderCustomProxies(source CustomSource, onSkip func(int64, string, error)) []map[string]any`，`RenderClashFilteredMerged` 改为调用它。
   - 跑 `go test ./internal/subscription/...` 确认 `TestRenderClashFilteredWithoutCustomMatchesLegacy`、`TestRenderClashFilteredMergedCustomSources` 通过（输出字节不变）。

2. **分享渲染 + 内容读取（web 层）**
   - 在 `internal/web/custom_nodes.go` 增加分享 handler：
     - `repo.GetCustomNode` → 取内容：
       - `links`：`secrets.Decrypt` + `SplitLinkLines`；
       - `subscription`：`fetchCustomNodeContent`（`subscriptions.go`）+ `NormalizeFetchedContent`。
     - `subscription.RenderCustomProxies(source, onSkip)` → `RenderProxiesFragment` 得 `clash`。
     - 组装 `customNodeShareResponse{source_type, has_cache, fetched_at, clash, links, skipped}`。
   - 增加 `GET /api/custom-nodes/{id}/content` handler：解密返回 `{content}`。
   - `skipped` 收集沿用 `ParseShareURI` 错误信息（与 `linkWarnings` 口径一致）。

3. **路由注册**
   - `internal/web/web.go` 在 custom-nodes 路由组加：
     - `GET /api/custom-nodes/{id}/share`
     - `GET /api/custom-nodes/{id}/content`
   - 均在 `requireAdmin` 下。

4. **后端测试**
   - 新文件 `internal/web/custom_nodes_share_test.go`（参考 `custom_nodes_test.go` / `nodes_share_test.go` 的 e2e 辅助）：
     - links 来源：share 返回 proxies 片段 + 明文链接；含不可解析行进 `skipped`。
     - subscription 来源：无缓存 → 懒拉取（用 httptest 上游）后 `has_cache=true`，clash/links 非空；纯 Clash 上游 → `links` 为空、`clash` 非空。
     - content 接口：links 返回原文、subscription 返回 URL。
     - 两接口未登录 `401`、未知 id `404`。
   - 确保普通 `GET /api/custom-nodes` 响应不含 `content`（现有 JSON-sweep/契约断言）。

5. **契约文档**
   - `docs/api-contract.md` 在 Custom Nodes 段追加 `GET /api/custom-nodes/:id/share` 与 `GET /api/custom-nodes/:id/content` 契约（请求/响应/错误码）；更新 `GET /api/custom-nodes` 说明为"列表不返回 content，回显走单节点接口"。这是显式最小契约 diff，需在完成报告中列出。

6. **前端 API 层**
   - `web/src/api/types.ts` 增加 `CustomNodeShare`、`CustomNodeContent`。
   - `web/src/api/customNodes.ts` 增加 `getCustomNodeShare`、`getCustomNodeContent`。

7. **前端分享弹窗**
   - 新增 `web/src/components/CustomNodeShareDialog.vue`（`ModalDialog` + 两段格式 + 复制 + skipped + 空状态 + loading/error）。

8. **前端接入**
   - `CustomNodesPage.vue`：`shareTarget` 状态、`rowActions` 增「分享」、渲染弹窗。
   - `CustomNodeFormDialog.vue`：编辑时拉取并回显 content；更新文案；保持空内容=沿用旧值。

9. **前端校验**
   - `cd web && npm run typecheck && npm run build && npm run lint`。

## Validation Commands

```bash
go build ./... && go vet ./... && gofmt -l .
go test -count=1 ./...
go test -race -count=1 ./internal/web/... ./internal/subscription/...
go build -tags embed_ui ./...
cd web && npm run typecheck && npm run build && npm run lint
```

## Risky Files / Rollback Points

- `internal/subscription/render.go`：抽取时最易引入输出漂移 → 以现有测试为闸门；如失败优先还原为内联实现。
- `internal/web/custom_nodes.go` + `web.go`：新增路由；回滚移除即可。
- `CustomNodeFormDialog.vue`：回显与"留空保持"语义要同时成立，避免误清空。

## Follow-ups Before task.py start

- 确认 `customNodeSourceContent` 对 `subscription` 不返回链接时的空态在弹窗可读。
- 确认 `clash` 空片段的具体输出（`proxies: []`）在契约文档中固定下来。
