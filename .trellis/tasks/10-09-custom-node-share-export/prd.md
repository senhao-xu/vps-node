# 自定义节点分享导出与编辑回显

## Goal

管理员在「自定义节点」页面无需选择用户，即可一键复制某个自定义节点的内容，并提供 **Clash（仅 proxies 片段）** 与 **V2 原生（明文逐行链接）** 两种格式；同时编辑自定义节点时，把管理员当初输入的内容**原样回显**到表单，便于查看和修改。

## Background / Confirmed Facts

- 「自定义节点」有 `links`（分享链接，逐行一条）与 `subscription`（上游订阅 URL）两种来源；内容 AES 加密存储在 `custom_nodes.content_enc`（`internal/secrets`）。
- 创建/编辑走 `web/src/components/CustomNodeFormDialog.vue`；编辑时故意**不回显**内容（`CustomNodeFormDialog.vue:164,240`），DTO（`internal/web/custom_nodes.go:19`）也不返回内容。
- 订阅渲染已具备复用能力：`subscription.RenderClashFilteredMerged` 内部把 `CustomSource.Links` 逐条 `ParseShareURI` 转成 Clash proxy，并用 `uniqueProxyName` 去重命名（`internal/subscription/render.go:296`）。
- `subscription.NormalizeFetchedContent(content)` 把上游内容拆成 `links` + `proxies`（`internal/web/subscriptions.go:294`）。
- 普通节点的「节点分享」`NodeShareDialog.vue` 需要用用户凭据、必须选用户；本需求**不选用户**，是不同形态。
- 自定义节点是第三方来源，链接中的凭据属于上游，与面板用户无关，因此分享不需要用户身份。

## Requirements

### R1 分享 / 导出（管理员直接复制）

- 自定义节点列表操作菜单新增「分享」入口，打开弹窗。
- 弹窗提供两种格式的文本，各自带复制按钮：
  - **Clash**：仅 `proxies` 配置片段（`proxies:` 顶层键 + 链接转换出的代理列表）。
  - **V2 原生**：明文逐行链接（原始链接，不做 base64 编码）。
- 不需要选择用户，不依赖面板用户凭据。
- 覆盖 `links` 与 `subscription` 两种来源。
- `subscription` 来源复用渲染路径的懒拉取（含 TTL 缓存与失败回退旧缓存）；无任何内容时给出明确空状态，不报错。
- 无法转换为 Clash 的条目（如不可解析链接、缺 name 的上游 proxy）在弹窗中列出为 skipped 提示，不影响其余条目。

### R2 编辑回显

- 打开编辑弹窗时，把已存内容解密后**原样回显**到内容输入框。
- `links` 与 `subscription` 两种来源都回显。
- 回显内容只通过**单节点**接口返回；列表接口 `GET /api/custom-nodes` 不返回明文。
- 保持现有更新语义：提交时内容为空则沿用旧值（不因回显误清空）；非空则替换。

## Out of Scope

- 选择用户、用用户凭据生成分享链接（普通节点 `NodeShareDialog` 的能力）。
- 修改链接/订阅来源的抓取、缓存、User-Agent、TLS 策略。
- 新增对外无需鉴权的公开分享 URL / 托管。
- 权限模型变化；分享与回显仍仅管理员可访问。
- 把上游 Clash proxy 反向转成分享链接（无法转换的条目在 V2 格式下不出现，仅计入提示）。

## Acceptance Criteria

- [ ] `GET /api/custom-nodes/{id}/share` 返回 `clash`（proxies 片段）、`links`（明文逐行链接）、`source_type`、`has_cache`、`fetched_at`、`skipped`。
- [ ] `GET /api/custom-nodes/{id}/content` 返回 `content`（解密后原文）；`links` 返回原始链接文本，`subscription` 返回原始订阅 URL。
- [ ] 两个新接口：未登录 `401 unauthorized`，未知 id `404 not_found`。
- [ ] 自定义节点列表操作菜单有「分享」入口，点击打开弹窗。
- [ ] `links` 来源：弹窗展示 proxies 片段与明文链接，均可复制并有一致成功反馈。
- [ ] `subscription` 来源：弹窗基于懒拉取/缓存结果展示两种格式；无内容时显示空状态而非报错。
- [ ] 编辑 `links` 来源：打开编辑弹窗即回显原始链接文本。
- [ ] 编辑 `subscription` 来源：打开编辑弹窗即回显原始订阅 URL。
- [ ] 编辑时留空提交仍保持原内容不变。
- [ ] 列表接口不返回内容明文。
- [ ] `docs/api-contract.md` 补齐两个新接口契约。
- [ ] Go：`go build ./... && go vet ./... && gofmt -l .`、`go test -count=1 ./...` 通过；新增接口单测覆盖成功/401/404/skipped。
- [ ] 前端：`cd web && npm run typecheck && npm run build && npm run lint` 通过。
