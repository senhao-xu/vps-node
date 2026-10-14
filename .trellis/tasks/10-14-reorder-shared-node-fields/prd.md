# 调整分享节点字段顺序

## Goal

让“自定义节点 > 分享 > Clash”生成的 `proxies:` 片段使用稳定、易读的单行字段顺序，与用户给出的 VLESS 示例一致，便于直接复制和核对节点配置。

## Background

- `internal/web/custom_nodes.go:232` 使用 `subscription.RenderProxiesFragment` 生成分享内容。
- `internal/subscription/render.go:347-365` 已将每个代理输出为单行 flow-style YAML，但输入是 `map[string]any`，当前编码顺序不表达协议字段的期望顺序。
- 普通托管节点的分享接口 `GET /api/nodes/{id}/share` 只返回原生 URI，不生成用户示例中的 Clash 节点行。

## Requirements

1. 自定义节点 Clash 分享片段中的每个代理继续输出为 `- { ... }` 单行 YAML。
2. 顶层代理字段按以下优先顺序输出；不存在的字段跳过，不新增或删除字段：
   `name`, `type`, `server`, `port`, `uuid`, `alterId`, `cipher`, `udp`, `flow`, `encryption`, `tls`, `skip-cert-verify`, `servername`, `reality-opts`, `client-fingerprint`, `network`。
3. 未列入优先顺序的协议专属或上游扩展字段必须保留，并在已知字段之后稳定输出，避免分享过程中丢失配置。
4. `reality-opts` 内部字段按 `public-key`, `short-id` 的顺序输出；其他嵌套字段保留。
5. 改动仅影响自定义节点 Clash 分享片段的文本排列，不改变节点解析、字段值、V2 原生链接、完整 Clash 订阅或运行时代理配置。

## Acceptance Criteria

- [x] VLESS Reality 分享行中存在的字段顺序与用户示例一致。
- [x] 缺少某些优先字段时不会生成空字段，其余字段保持相对顺序。
- [x] 未知或协议专属字段仍出现在输出中，并具有确定性顺序。
- [x] `reality-opts` 按 `public-key`、`short-id` 输出。
- [x] 输出仍可被 YAML 解析，且解析后的数据与输入代理 map 等价。
- [x] 空代理列表仍输出 `proxies: []`。
- [x] 相关订阅单元测试通过。

## Out of Scope

- 不调整节点列表本身的排列顺序。
- 不给 VLESS 或其他协议补充原输入中不存在的字段。
- 不修改托管节点的 URI 分享顺序或完整 Clash 订阅的 YAML 排列。

## Technical Notes

- 这是轻量、局部的序列化调整，任务采用 PRD-only。
- 实现应在 `RenderProxiesFragment` 的 YAML node 构造边界显式排列键，避免依赖 Go map 遍历或 YAML 库的隐式排序。
