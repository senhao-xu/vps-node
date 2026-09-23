# 实施计划：节点编辑回显公开配置

## 技术方案概要

- 后端：`nodeDetailDTO` 新增 `Settings json.RawMessage \`json:"settings"\``，在 `handleNodeGet` 中把 `repo.Node.ProtocolSettings`（公开 JSON，不含秘密）原样序列化返回；为空字符串时返回 `{}`。列表 DTO `nodeDTO` 不变。
- 前端：`NodeDetail` 类型增加 `settings?: NodeSettings`（公开字段形状，复用/扩展 `NodeSettingsInput` 的只读视角）。`NodeFormDialog` 在编辑模式打开时调用 `getNode(id)` 拉取详情并回填各协议公开字段。
- 编辑提交语义调整：公开字段回显后按表单当前值提交（与创建一致）；秘密字段仍"留空保持不变"。移除公开字段的"留空保持不变"占位/提示，保留秘密字段提示。

## 执行清单

- [ ] 1. 后端：`internal/web/dto.go` 给 `nodeDetailDTO` 增加 `settings` 字段；`internal/web/nodes.go handleNodeGet` 填充公开 settings（空则 `{}`）。
- [ ] 2. 后端测试：`internal/web` 新增/扩展测试——创建含公开+秘密字段的节点，`GET /api/nodes/{id}` 断言 `settings` 含公开字段（如 `reality_settings.public_key`、`cipher`、`tls.server_name`）且不含 `private_key`/`password`/`certificate`；`GET /api/nodes` 列表项不含 `settings` 键。
- [ ] 3. 前端类型：`web/src/api/types.ts` 为 `NodeDetail` 增加 `settings`（公开只读形状）。
- [ ] 4. 前端表单：`web/src/components/NodeFormDialog.vue`
  - 编辑打开时调用 `getNode(props.node.id)`，回填 ss cipher、vless reality 五字段（含公钥展示框）、hy2/anytls 公开字段；加加载态与错误处理（拉取失败显示 ErrorBanner，仍可编辑基础信息）。
  - `settingsPayload`：编辑模式下公开字段按当前表单值提交（与创建对齐），秘密字段仅在非空时提交；allow_insecure / obfs.open / cipher 的 touched 机制可移除或保留为无害——以"回显后所见即所存"为准简化。
  - 更新文案：公钥展示框标题在编辑态不再标注"仅本次显示"；秘密字段保留"留空保持不变"；底部 tip 文案更新为"密钥类字段不会回显，留空保持不变"。
  - 更新 `normaliseServerPort` 注释（"node DTOs never echo settings" 已过时）。
- [ ] 5. 校验：`cd web && npm run build`（含 vue-tsc）与 `go test ./...`、`go vet ./...`。
- [ ] 6. 文档：更新 `docs/api-contract.md` 节点详情接口补充 `settings` 字段说明。

## 验证命令

```bash
go test ./internal/web/ -run Node -v
go test ./...
cd web && npm run build
```

## 回滚点

- 步骤 1-2 独立可回滚（后端 DTO 新增字段，前端不读即无影响）。
- 步骤 4 前端改动独立回滚，不影响后端。
