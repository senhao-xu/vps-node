# 实现计划：链式代理

## 顺序清单

1. **migration**：`internal/db/migrations/0007_node_chain.sql` — `ALTER TABLE nodes ADD COLUMN chain_node_id`
2. **singbox 渲染**：`internal/singbox/singbox.go`
   - relay 凭证派生函数（`relayCredDomain`）
   - `Render` 扩展 ChainExit 入参 + 4 种协议客户端出站渲染 + route.rules
   - Reality 公钥推导从 subscription 包抽到 singbox 包共用
   - ContractVersion → v2；先核实 `internal/agentruntime` 是否校验 renderer_version
3. **repo**：nodes.go 读写 chain_node_id；环检测 `ValidateNodeChain`；`ListChainExitsByServer` / `ListChainEntriesByExitServer`；删除前引用检查
4. **revision 双端联动**：chain 写入/更新/删除 + 出站节点变更时 bump 两端 server revision
5. **配置组装**：`agent_config.go` buildAgentConfigPayload 注入出站描述与 relay 伪用户
6. **web API**：节点创建/更新接受 chain_node_id；删除返回 409 + 引用列表；流量上报处丢弃 `relay-*` 用户名
7. **前端**：NodeFormDialog 出站节点下拉（分组、排除自身）、节点列表链路列、删除报错提示
8. **自测**：`make test && make test-integration`；双 server 起 agent 手工验证落地 IP

## 验证命令

```sh
make test
make test-integration     # 内嵌 sing-box 真启动，验证链式配置可加载
make build && make ui
# 手工：入口节点客户端连接后，curl ifconfig.me 应显示出站 server IP
```

## 风险文件 / 回滚点

| 文件 | 风险 | 回滚 |
| --- | --- | --- |
| internal/singbox/singbox.go | 渲染契约变更影响所有节点 | ChainExit 为空时输出与 v1 等价；ContractVersion 保留旧常量对照 |
| internal/web/agent_config.go | 配置组装主路径 | 无链节点时渲染路径不变 |
| internal/repo/revisions.go 调用点 | 漏 bump 导致一端配置过期 | 单测覆盖双端 bump |
| 流量上报 | relay-* 误计入 | panel 侧按前缀过滤 + 用例 |

## 审查门禁

- 环检测：自引用、A→B→A、A→B→C→A 各有用例
- 删除被引用出站节点返回 409 且列出引用名
- 双端 revision bump 有 repo 层用例
- relay 凭证派生确定性（同输入同输出、跨节点隔离）有用例
- 出站节点作为普通入站仍可直接连接（集成测试）
