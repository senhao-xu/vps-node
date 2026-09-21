# 节点协议参数补全（参考 Xboard）

## 背景

节点创建/编辑表单相比 Xboard 缺少若干协议参数。用户确认补齐以下 4 项：

1. **Hysteria2 obfs 混淆**：salamander 混淆密码，降低被 QoS/识别的概率。
2. **Hysteria2 端口跳跃**：客户端在端口范围内跳跃（如 30000-40000），配合服务端 NAT 端口转发使用。
3. **VLESS flow 可选**：当前写死 `xtls-rprx-vision`，支持 vision / 不启用 两种选择。
4. **VLESS dest 目标站**：Reality 回落目标（handshake server），当前写死为 `server_names[0]:443`，支持自定义。

## 范围

- 后端：`internal/web/nodes.go`（settings 校验）、`internal/singbox/singbox.go`（inbound 渲染）、`internal/subscription/render.go`（分享链接 + Clash Meta）、`internal/web/agent_config.go`（agent 凭据中的 flow）。
- 前端：`NodeFormDialog.vue` 新增对应表单项（含校验、编辑态留空保持不变的语义）。
- 测试：相关 Go 单测更新/新增。

## 详细需求

### Hysteria2 obfs
- 新增 settings 字段 `obfs_password`（明文 settings，非 secret；最长 64 字符，可空=不启用）。
- sing-box inbound：`"obfs": {"type": "salamander", "password": ...}`，仅当非空。
- 订阅：hy2 URI 增加 `obfs=salamander&obfs-password=<pwd>`；Clash Meta 增加 `obfs: salamander` + `obfs-password`。

### Hysteria2 端口跳跃
- 新增 settings 字段 `hop_ports`（字符串，格式 `start-end`，如 `30000-40000`，可空=不启用；校验 1-65535 且 start<=end）。
- **仅影响订阅渲染**：hy2 URI 增加 `mport=<hop_ports>`；Clash Meta 增加 `ports: <hop_ports>`。
- 服务端 sing-box inbound 不变（端口转发由管理员在服务器上配置 NAT/iptables），前端表单给出明确提示文案。
- 端口跳跃时订阅链接的主端口仍使用节点 port（客户端以 mport/ports 跳跃）。

### VLESS flow
- 新增 settings 字段 `flow`（枚举：`""` 不启用 或 `"xtls-rprx-vision"`；缺省/未知值按 vision 处理以保持存量节点行为不变）。
- sing-box inbound users[].flow 按设置输出（不启用时不输出 flow 字段）。
- 订阅：URI `flow` 参数与 Clash `flow` 字段同逻辑（不启用时省略）。
- agent 凭据 `agentCredential` 中 vless flow 同步改为按节点设置输出。

### VLESS dest
- 新增 settings 字段 `dest`（字符串，host 或 host:port，可空）。
- sing-box inbound reality.handshake：`dest` 非空时解析 host/port（无端口默认 443）；为空时保持现状 `server_names[0]:443`。
- 校验：host 为合法域名/IP，端口 1-65535。

## 非目标

- 不新增协议（trojan/vmess 等）。
- 不由 agent 自动配置 iptables/NAT 端口转发。
- 不改用户/流量/认证模型。

## 验收标准

1. `go test ./...` 全绿；`web/` 下 typecheck/lint/build 全绿。
2. 创建/编辑 Hysteria2 节点可填 obfs 密码与端口跳跃范围；生成订阅的 hy2 链接含 `obfs`、`obfs-password`、`mport`；Clash Meta 含对应字段。
3. 创建/编辑 VLESS 节点可选 flow（vision/不启用）与 dest；订阅 URI 的 `flow` 参数随之变化；sing-box 下发配置 handshake 使用 dest。
4. 存量节点不填新字段时行为与之前完全一致（flow 默认 vision，handshake 用 server_names[0]:443，无 obfs/mport）。
5. 非法输入（hop_ports 格式错、dest 端口越界、flow 非法值）被后端 422 拒绝并给出中文可读错误。
