# 实现计划：自定义节点

## 顺序清单

1. **migration**：`internal/db/migrations/0006_custom_nodes.sql`（custom_nodes + user_custom_nodes，见 design.md 数据契约）
2. **repo**：`internal/repo/custom_nodes.go` — CRUD + `ListActiveCustomNodesByUser` + `UpdateCustomNodeCache` + 授权函数（镜像 user_nodes.go）
3. **解析器**：`internal/subscription/import.go` — `ParseShareURI`（ss/vless/hy2/anytls/trojan/vmess）+ 表驱动单测 `import_test.go`
4. **抓取**：`internal/subscription/fetch.go` — `FetchSubscription`（http/https、5s、1MiB、≤3 跳转）+ httptest 单测
5. **渲染合并**：`internal/subscription/render.go` 增加自定义节点合并入口（给现有函数加参数），名称去重加后缀
6. **web API**：`internal/web/custom_nodes.go` — CRUD 4 端点 + `PUT /api/users/{id}/custom-nodes` 授权端点 + 路由注册（web.go）
7. **订阅接入**：`handlePublicSubscription`（subscriptions.go:141）按 `ListCustomNodeIDsByUser` 过滤后并入两种格式；subscription 类型走 TTL 缓存抓取
8. **前端**：types.ts + api 函数 + 自定义节点管理页（DataTable + ModalDialog）+ 用户编辑对话框"自定义节点"授权区块 + 菜单入口
9. **自测**：`make build && make test`；手工起 panel 验证授权/未授权用户的两种格式订阅输出

## 验证命令

```sh
make test                    # go test ./...
make ui                      # 前端构建
make build                   # vet + test + build
curl -s "http://localhost:8080/<sub-path>/<token>?flag=general" | base64 -d
curl -s "http://localhost:8080/<sub-path>/<token>?flag=clash-meta"
```

## 风险文件 / 回滚点

| 文件 | 风险 | 回滚 |
| --- | --- | --- |
| internal/subscription/render.go | 合并逻辑影响既有渲染 | 合并参数为空切片时走原路径（保持旧签名包装） |
| internal/web/subscriptions.go | 订阅主路径 | 自定义节点为空时行为与现状逐字节一致 |
| 0006 migration | SQLite 迁移 | 迁移失败启动报错，不破坏旧表 |

## 审查门禁

- URI 解析器表驱动测试覆盖 6 种协议 + 畸形输入
- 抓取超时/超限/非 http(s) 各有用例
- 名称冲突后缀逻辑有用例
- 前端 lint + typecheck 通过
