# 改进节点创建体验

## Goal

参考 XBoard 的协议化配置体验，降低管理员创建 Shadowsocks、VLESS Reality 和 Hysteria2 节点时理解底层 sing-box 字段及手工准备密钥的成本，同时保持本项目 Panel、Server、Node、Agent 的现有职责边界。

## Background

- 当前入口是 `web/src/components/NodeFormDialog.vue`，创建时要求管理员手工填写名称、端口和协议设置。
- 当前支持的协议固定为 `shadowsocks | vless | hysteria2`；协议和服务器的关系为一个物理 Server 可包含多个 Node。
- XBoard 将节点连接信息、后端服务信息和协议配置集中在同一模型中；本项目不会照搬该数据模型，只参考其按协议展示字段、Reality 参数成组配置和表单提示方式。
- 协议秘密由 Panel 加密保存在 `nodes.secret_enc`，通用节点 DTO 不得回显秘密。
- VLESS Reality 运行配置要求私钥、至少一个 Server Name；Short ID 可选。Shadowsocks 2022 服务端密码已有安全派生兜底，Hysteria2 入站当前实际使用用户 UUID，不读取表单中的节点密码。

## Requirements

- R1. 重组节点对话框，使基础信息与协议配置清晰分区，并为每个受支持协议提供用途说明、字段提示和合理默认值。
- R2. 创建模式必须仅要求实际运行所需的信息，不要求管理员填写运行时不会使用的字段。
- R3. VLESS Reality 配置必须作为一个完整配置组处理，明确展示 Server Name、Short ID、私钥和公钥之间的关系；管理员可明确触发 Panel 生成兼容 sing-box 的 X25519 密钥对和随机 Short ID，无需执行外部命令。
- R4. 前后端校验必须与 sing-box 渲染要求一致，避免成功创建但 Agent 无法应用的节点。
- R5. 编辑模式必须保持秘密不回显；空秘密字段表示保留现值，任何生成或替换秘密的操作必须由管理员明确触发。
- R6. 桌面和移动端均可完成创建操作，协议切换时不得误提交上一协议的字段。
- R7. 保持现有 API 错误信封、节点修订号递增、密钥加密存储和通用 DTO 不含秘密等约束。
- R8. Reality 生成结果只存在于生成响应和当前表单状态；公钥供管理员即时复制但本次不持久化，私钥随创建请求提交后按现有机制加密保存。

## Out of Scope

- 新增 XBoard 支持但本项目尚未支持的协议、传输层、混淆、倍率、权限组、路由组和父节点。
- 合并 Server 与 Node 数据模型，或改变 Agent 配置同步架构。
- 用户订阅、客户端分享链接和客户端 Reality 公钥分发。
- 自动探测远端端口占用或修改 VPS 防火墙。

## Acceptance Criteria

- [ ] 管理员可在一个清晰的协议化表单中创建现有三种节点，切换协议只提交当前协议字段。
- [ ] Shadowsocks 创建使用支持的 2022 加密方式，并且无需手工构造不必要的服务端密码。
- [ ] VLESS Reality 创建时具备运行所需的私钥、Server Name，Short ID 的格式得到前后端一致校验。
- [ ] 管理员点击生成后可看到并复制匹配的 Reality 公钥，表单自动填入私钥和 Short ID；重新打开表单不会回显上次生成结果。
- [ ] Reality 密钥使用 X25519 生成并采用 sing-box 接受的无填充 URL-safe Base64 编码，Short ID 为 8 字节小写十六进制。
- [ ] Hysteria2 表单只展示当前运行配置实际使用的选项，带宽值为非负整数。
- [ ] 缺少必需协议字段或字段格式错误时，保存前有中文提示，直接调用 API 也得到 `422 validation`。
- [ ] 编辑现有节点不暴露已存秘密，未明确替换时秘密保持不变。
- [ ] 创建或修改后继续触发所属 Server revision 递增，Agent 配置渲染测试通过。
- [ ] 节点 API 回归测试、Go 全量测试和 Web 类型检查、构建、lint 通过。

## Constraints

- Go 标准库优先；不引入前端 UI 框架。
- 不在日志、错误、列表或详情 DTO 中暴露协议秘密。
- 前端 API 类型继续集中在 `web/src/api/types.ts`，API 合约变化同步更新 `docs/api-contract.md`。

## Key Decisions

- Reality 密钥对和 Short ID 由受管理员会话保护的 Panel API 使用加密安全随机源生成，不依赖浏览器对 X25519 的支持。
- 生成是管理员显式操作，不在打开表单、切换协议或保存时静默轮换密钥。
- 公钥不进入 Node DTO 或数据库；客户端订阅和 Reality 公钥长期分发另行设计。
- Shadowsocks 创建界面不再要求服务端密码，沿用按节点派生服务端密码；Hysteria2 创建界面移除当前运行配置未使用的节点密码。
