# 实施计划：节点协议参数补全

## 执行顺序

### 1. 后端校验（internal/web/nodes.go）
- [ ] VLESS：`flow`（`""`/`xtls-rprx-vision`，其他 422）、`dest`（host[:port]，port 1-65535）。
- [ ] Hysteria2：`obfs_password`（1-64 字符）、`hop_ports`（`start-end`，1-65535，start<=end）。
- [ ] 未知字段仍 422（现有 default 分支自动覆盖）。

### 2. sing-box 渲染（internal/singbox/singbox.go）
- [ ] `renderVLESS`：flow 三态（key 缺失→vision；显式 ""→省略；vision→vision）；dest 非空时 handshake 用 dest host/port（缺省 443）。
- [ ] `renderHysteria2`：`obfs_password` 非空 → `obfs: {type: salamander, password}`。
- [ ] hop_ports 不进 inbound。

### 3. 订阅渲染（internal/subscription/render.go）
- [ ] hy2 URI：`obfs=salamander` + `obfs-password`；`mport=hop_ports`。
- [ ] hy2 Clash：`obfs`/`obfs-password`/`ports`。
- [ ] vless URI + Clash：flow 显式空时省略。
- [ ] `agentCredential` vless flow 按 settings 输出（先确认 agent 端消费方式）。

### 4. 前端表单（NodeFormDialog.vue）
- [ ] VLESS：flow 下拉（默认 vision / 不启用）、dest 输入。
- [ ] Hysteria2：obfs 密码输入、端口跳跃输入（带 NAT 提示）。
- [ ] 编辑态：留空=保持不变；校验规则与后端一致。
- [ ] `web/src/api/types.ts` 无需改（NodeSettingsInput 是 Record）。

### 5. 测试
- [ ] `internal/singbox/singbox_test.go`：obfs/flow/dest 渲染断言。
- [ ] `internal/subscription/render_test.go`：链接参数断言。
- [ ] `internal/web/servers_nodes_test.go`：合法/非法新字段 422/201 用例。
- [ ] 存量行为回归断言（无新 key 时输出不变）。

### 6. 全量验证
- [ ] `go test ./...`
- [ ] web: `npm run typecheck && npm run lint && npm run build`

## 质量门槛

- 后端遵循 .trellis/spec/backend 规范；Go 测试全绿才可进前端联调。
- 前端遵循 .trellis/spec/frontend 视觉系统约定（用语义 token，表单样式与现有一致）。
- 不改 API 路由结构；settings 新 key 向后兼容。

## 回滚点

- 步骤 1-3（后端）与步骤 4（前端）可独立回滚；后端先行部署对旧前端无影响（旧前端不会发送新 key）。
