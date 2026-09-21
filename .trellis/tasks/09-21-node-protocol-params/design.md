# 设计：节点协议参数补全

## 数据流

管理端表单 → `POST/PUT /api/nodes`（`internal/web/nodes.go` 校验，明文入 `settings` JSON）→ 面板用 `singbox.Render` 渲染完整 sing-box 配置经 `/api/agent/config` 下发 agent（`agent_config.go` → `singboxNode`）→ 订阅在 `internal/subscription/render.go` 用同一份 settings 生成分享链接/Clash Meta。

新字段全部属于**明文 settings**（非 secret）：`obfs_password`、`hop_ports`、`flow`、`dest`。私钥/证书仍走 secretFields 加密存储，不改动。

## 后端改动

### 1. `internal/web/nodes.go` — 校验

- VLESS 分支新增 case：
  - `flow`：string，允许 `""` 或 `"xtls-rprx-vision"`，其他值 422。`""` 存入表示不启用；缺省（key 不存在）视为 vision。
  - `dest`：string，可空；非空时校验 `[host][:port]`，host 限域名/IP 字符，port 1-65535（用 `net.SplitHostPort` 容错解析 + 手动补 443 逻辑）。
- Hysteria2 分支新增 case：
  - `obfs_password`：string，1-64 字符，可空。
  - `hop_ports`：string，格式 `^\d+-\d+$`，两端 1-65535 且 start<=end；可空。
- `validateProtocolSettings`：编辑场景 partial update 时，新字段均为可选，不破坏现有必填校验逻辑。
- 注意：现有逻辑对 hysteria2 `server_name` 用 `secretString`（1-256 字符）但存入 plain——新字段沿用 plain 存储即可。

### 2. `internal/singbox/singbox.go` — inbound 渲染

- `renderVLESS`：
  - `flow := SettingString(n.Settings, "flow")`；空串时 user map 不写 `flow` 键；非空（含缺省）写 `xtls-rprx-vision`。**兼容规则**：key 不存在 → vision；显式 `""` → 不写 flow。实现上用 `n.Settings` 里 key 是否存在区分：`flow, hasFlow := n.Settings["flow"]; if !hasFlow || flow=="xtls-rprx-vision" { vision } else { 省略 }`。
  - dest：解析 `SettingString(n.Settings, "dest")`，非空时拆分 host/port（无端口补 443），`handshake.server/server_port` 用 dest；否则保持 `serverNames[0]:443`。
- `renderHysteria2`：`obfs_password` 非空时 `inbound["obfs"] = map[string]any{"type": "salamander", "password": pw}`。

### 3. `internal/subscription/render.go`

- hy2 URI：`obfs_password` 非空 → `q.Set("obfs","salamander"); q.Set("obfs-password", pw)`；`hop_ports` 非空 → `q.Set("mport", hop)`。
- hy2 Clash：`obfs: salamander`、`obfs-password`、`ports: hop_ports`。
- vless URI：flow 省略逻辑同 sing-box（key 存在且为 `""` 时不输出 `flow` 参数；否则输出 vision）。dest 不影响订阅（dest 是服务端回落目标，客户端只用 sni/pbk/sid）。
- vless Clash：`flow` 同逻辑省略。

### 4. `internal/web/agent_config.go`

- `agentCredential` vless 分支：`flow` 按节点 settings 输出（缺省 vision，显式空则不带 flow 键）。先查 agent 端（`internal/agentruntime`）是否消费该字段，若仅透传则同步改。

### 测试

- `singbox_test.go`：hy2 obfs inbound 断言；vless flow 省略/vision 断言；dest handshake 断言。
- `servers_nodes_test.go`：新字段合法/非法创建用例（hop_ports 格式错 422、dest 端口越界 422、flow 非法 422）。
- `render_test.go`（subscription）：hy2 链接含 obfs/mport 参数；vless flow 省略。
- 存量行为回归：无新字段时输出与现状逐字节一致。

## 前端改动（`NodeFormDialog.vue`）

- VLESS 区块新增：
  - `flow` 下拉：`xtls-rprx-vision（默认）` / `不启用`。编辑态回显当前值（settings 明文字段可从 node detail 接口读到；确认 NodeBrief 是否返回 settings 明文——若未返回，编辑态按「保持不变」处理）。
  - `dest` 文本输入，placeholder `默认使用 Server Name:443`。
- Hysteria2 区块新增：
  - `obfs 混淆密码` 文本输入，hint「启用 Salamander 混淆，客户端需同步填写」。
  - `端口跳跃` 文本输入，placeholder `如 30000-40000`，hint「仅影响订阅客户端；需在服务器自行配置 NAT 端口转发」。
- 前端校验与后端规则一致（正则/范围），编辑态留空=保持不变。

## 兼容性

- 存量节点 settings 无新 key → 行为完全不变（vision flow、server_names[0]:443、无 obfs/mport）。
- 编辑旧节点不填新字段 → payload 不含 key → settings 保持原样（现有 partial 语义）。
- agent 端 sing-box 配置由面板整体渲染下发，新增 map 键对旧 agent 无影响（同版本发布）。

## 风险

- `flow` 的「显式空 vs 缺省」区分依赖 settings JSON 的 key 存在性，前后端都要遵守该约定（写进测试）。
- 端口跳跃只做订阅侧，若用户未配置服务端转发会导致客户端连不通——UI 提示文案必须醒目。
