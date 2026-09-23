# 节点编辑回显公开配置

## Goal

打开节点编辑对话框时，把该节点已保存的协议公开配置回显到表单中，包括由私钥自动派生的 Reality 公钥，让管理员能看到当前生效的配置并基于其修改。

## Background

- 当前 `GET /api/nodes/{id}` 返回的 `nodeDetailDTO` 不含协议配置，编辑表单只回填基础信息（名称/端口/协议/状态/倍率/标签），协议字段全部为空、标注"留空保持不变"。
- 节点公开配置（`nodes.protocol_settings`）与加密密钥（`nodes.secret_enc`）分离存储；公开配置本身不含私钥/密码/PEM 证书。
- VLESS 节点的 `reality_settings.public_key` 在保存时已由 `private_key` 派生并写入公开配置，但编辑时无法查看。

## Requirements

- R1. `GET /api/nodes/{id}` 在详情 DTO 中新增 `settings` 字段，内容为该节点 `protocol_settings` 的公开 JSON（管理员可见，无需解密 `secret_enc`）。无配置时返回空对象 `{}`。
- R2. 编辑对话框打开时，若传入节点则调用详情接口并按协议回填公开字段：
  - Shadowsocks：`cipher`。
  - VLESS：`reality_settings.server_name / server_port / short_id / allow_insecure / public_key`（公钥显示在公钥展示框中，可复制）。
  - Hysteria2：`tls.server_name / allow_insecure`、`bandwidth.up / down`、`obfs.open / type / password`、`hop_interval`。
  - AnyTLS：`tls.server_name / allow_insecure`、`padding_scheme`。
- R3. 敏感字段（VLESS `private_key`、Hysteria2/AnyTLS `password` / `certificate` / `private_key`、Shadowsocks `password`）不回显，保持"留空保持不变"语义。
- R4. 回显后表单展示即当前存储值：管理员修改公开字段后提交，提交内容为表单当前值；秘密字段仅在重新填写时提交。编辑时"留空保持不变"的提示文案仅保留给秘密字段。
- R5. 详情接口在节点列表/用户侧接口中不暴露 `settings`（仅管理员详情接口）。

## Constraints

- 不得把 `secret_enc` 解密内容或任何密钥字段加入 DTO。
- 保持现有部分更新（partial update）语义：未提交的公开字段保持原值。
- Hysteria2 的 `obfs.password` 按现有 schema 属于公开配置（非 `secret`），会随 `settings` 一并回显，属预期行为。

## Acceptance Criteria

- [ ] 创建 VLESS 节点后打开编辑对话框，Server Name、握手端口、Short ID、allow_insecure 开关与派生公钥正确回显，公钥展示框可复制。
- [ ] 创建 Shadowsocks 节点后打开编辑，加密方式下拉框显示当前存储值。
- [ ] 创建 Hysteria2 / AnyTLS 节点后打开编辑，TLS Server Name、带宽、混淆开关与密码、hop_interval / padding_scheme 正确回显；PEM 证书与私钥输入框为空。
- [ ] 编辑保存时未触碰的秘密字段保持原值（现有后端测试不回归）。
- [ ] `GET /api/nodes/{id}` 响应中的 `settings` 不包含 `private_key` / `password` / `certificate` 等秘密字段（Go 测试断言）。
- [ ] 前端 `npm run build`（或项目既有校验命令）与后端 `go test ./...` 通过。
