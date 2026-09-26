# 自定义节点：管理员全局外部节点/订阅汇入

## Goal

管理员可在面板维护"自定义节点"（不受 Agent 托管的外部节点），支持两种来源：逐条分享链接、上游订阅 URL。这些节点**按用户授权**后合并进对应用户的订阅输出（general 与 clash-meta 两种格式），授权语义与托管节点一致。

## Confirmed Facts（代码证据）

- 订阅输出入口：`internal/web/subscriptions.go:141` `handlePublicSubscription`，按 `flag` 分 general（base64 分享链接）/ clash-meta（YAML）。
- general 渲染：`internal/subscription/render.go:206` `RenderGeneralLinks` 逐节点生成 URI，base64 拼接。
- clash-meta 渲染：`render.go:223` `RenderClashFiltered` 将节点渲成 proxy map，经模板 `assembleClash` 注入 `proxies` 与 proxy-groups 占位符（`__ALL_PROXIES__` 等）。
- 单个坏节点跳过机制已存在（`onSkip` 回调，subscriptions.go:198）。
- 面板已支持协议：shadowsocks / vless(reality) / hysteria2 / anytls（`internal/singbox/singbox.go:17-21`）。
- 秘密静态加密复用 `internal/secrets`（appKey），节点密钥即如此存储（agent_config.go:188-198）。

## Requirements

- R1：新表 `custom_nodes`：id、name、source_type（`links` | `subscription`）、content_enc（加密存储：links=每行一条 URI；subscription=上游 URL）、cached_content + fetched_at（订阅抓取缓存）、status、时间戳。
- R2：links 类型：`general` 格式原样追加 URI；`clash-meta` 格式经新 URI→Clash proxy 解析器（`internal/subscription/import.go`）转换，协议范围：ss / vless / hysteria2 / anytls / trojan / vmess，解析失败的条目跳过并记日志。
- R3：subscription 类型：渲染时拉取上游，带短 TTL 缓存（~5min）；拉取失败降级用旧缓存；无缓存则跳过。SSRF 防护：仅 http/https、超时、响应大小上限。
- R4：自定义节点追加在托管节点之后；名字冲突时加后缀或跳过。
- R5：管理 UI：自定义节点 CRUD（列表、新建/编辑对话框、启停、删除），入口放在节点相关页面；用户编辑对话框增加"自定义节点"授权区块（与托管节点授权并列）。
- R6：自定义节点**按用户授权**（与托管节点 user_nodes 语义一致）：新表 `user_custom_nodes(user_id, custom_node_id)`，默认无人可见，授权后出现在该用户订阅；不参与流量统计、在线设备。

## Acceptance Criteria

- [ ] 管理员添加 ss:// 等链接后，任意用户 general 订阅包含该链接原文
- [ ] 同一链接在 clash-meta 订阅中渲染为合法 proxy 条目并进入 `__ALL_PROXIES__` 组
- [ ] 添加订阅 URL 后，其内容并入两种格式输出；上游不可达时 5 分钟内用缓存，之后跳过不报错
- [ ] 停用的自定义节点不出现在任何订阅
- [ ] 未授权用户的订阅不含该自定义节点；授权后出现，撤销后消失
- [ ] 托管节点的既有订阅输出不回归（现有测试 + 新增用例）

## Out of Scope

- 终端用户私有自定义节点
- 自定义节点的流量/设备统计
- 自定义节点参与链式代理（不能作为出站节点）
