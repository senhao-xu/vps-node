# 自定义/订阅线路作为链式出口

## Goal

托管**入口节点**的链式出口除托管节点外，可指向一个自定义节点来源内的**单条线路**（external entry）：入口 Agent 用该线路自带凭据直连外部服务器作为出口；条目不可用（缺失/停用/无缓存/无法解析）时回退直连。

## Background / Confirmed Facts（代码证据）

- 现有链式代理是"托管节点→托管节点"：`nodes.chain_node_id` 自引用 `nodes`（`internal/db/migrations/0007_node_chain.sql`）；出站由 `renderChainOutbound` 按托管协议渲染，凭据由 appKey 从 `Exit.ID`+`EntryServerID` 派生（`internal/singbox/singbox.go:124-144,447-587`）；出口侧入站注入 `relay-<entryServerID>` 伪用户（`internal/web/agent_config.go:239-272`）。
- 自定义来源 `custom_nodes` 无 `server_id`，是全局管理员资源，按用户授权（`user_custom_nodes`/`user_custom_node_entries`）；条目**不持久化**，渲染时解析（`internal/web/custom_nodes.go:93-124`、`internal/subscription/entrykey.go`）。
- **不存在** Clash proxy/分享链接 → sing-box **出站** 的转换器；`import.go` 只产出 Clash proxy，`renderProxy` 是反向。链式渲染无 trojan/vmess 出站。
- 外部条目凭据来自链接/上游 proxy（`password`/`uuid`/`reality-opts`/`ws-opts`/`skip-cert-verify`/`alterId` 等）。
- 外部出口不是面板可控 Agent：**无法注入 relay 用户**，只能直连；出口跳无面板侧统计。
- Agent config 构建当前**不做网络 IO**；订阅来源内容仅在 `cached_content`（5min TTL），链接来源在 `content_enc`。
- spec §9 要求：给 `nodes` 加列必须补齐所有显式列清单（`nodeSelect`/`nodeSelectWithServer`/`insertNodeExec`/`UpdateNodeSpec`/`UpdateNodeAndBump` 两分支/`scanNode`/`scanNodeWithServerName`/`ListSubscriptionNodes`）。

## Requirements

- R1：托管入口节点可选某自定义来源的单条线路作为出口（`chain_custom_node_id` + `chain_custom_entry_key`），与 `chain_node_id` 互斥。
- R2：新增 Clash proxy → sing-box **出站** 转换器，覆盖 ss / vless / trojan / vmess / hysteria2 / anytls / socks5；不支持类型不可选。
- R3：入口 Agent 用条目自带凭据直连；**不注入 relay 用户**。
- R4：条目缺失/停用/来源无缓存/无法解析 → 该入口回退直连，不阻塞 config。
- R5：解析在 config 构建时从已存储内容读取（links=`content_enc`、subscription=`cached_content`），**不新增网络 IO**。
- R6：删除被引用来源 → `409`；来源更新/刷新 → bump 引用它的入口节点所属服务器。
- R7：前端节点表单出口模式「直连/托管节点/自定义线路」，自定义模式下按来源+线路选择，`chain_supported=false` 置灰。
- R8：文档与 Trellis 规范同步。

## Acceptance Criteria

- [ ] 入口节点设置外部出口后，其 agent config 含指向外部服务器的出站（type/凭据正确）与 `route.rules` 指向 `chain-<entryNodeID>`，并经嵌入式 sing-box `Validate` 通过。
- [ ] `chain_node_id` 与 `chain_custom_node_id` 同时非空 → `422`；来源不存在/停用 → `422`；`chain_custom_entry_key` 非 64 位小写 hex → `422`。
- [ ] 订阅来源无缓存、条目被移除/改参、来源停用、转换失败 → 该入口 config 无该 chain 出站与 route rule（回退直连），config 构建不报错。
- [ ] 删除被引用的自定义节点 → `409 conflict` 且提示引用它的入口节点；更新/刷新被引用来源 → 引用它的入口节点所属服务器 revision bump。
- [ ] 托管出口路径与旧数据输出逐字节不变（`Outbound==nil`）。
- [ ] 前端可选择外部线路、不支持类型置灰、编辑回显、保存后互斥清空；typecheck/lint/build 通过。
- [ ] `go vet ./...`、`go test -count=1 ./...`、`go test -tags integration,with_quic,with_utls ./...` 全绿。

## Key Decisions

- 协议覆盖 ss/vless/trojan/vmess/hysteria2/anytls/socks5（用户选定 A）。
- config 构建不联网，无缓存→回退直连（用户选定 A）。
- 生命周期：删除 `409`、更新/刷新 bump 引用 server、条目失效回退直连（用户选定 A）。
- 存储：`nodes` 加 `chain_custom_node_id` + `chain_custom_entry_key`（互斥，应用层校验）；选择器在节点表单内（模式切换）。
- 出站转换器放在 `internal/singbox`（避免与 `subscription` 循环依赖），入参为 Clash proxy map。

## Out of Scope

- 出口跳的流量/设备/访问统计
- 供入口节点之外使用该外部出口
- 为外部出口注入面板 relay 鉴权

## Known Limitations

- 订阅来源需先"更新订阅"才有缓存可作出口。
- `entry_key` 只哈希连接参数（排除 name）：上游改参数即失效回退，同连接不同名折叠。
- 仅支持第 R2 列出的类型。
