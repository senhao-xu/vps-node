# 用户增加用户名字段

## Goal

为用户增加 `username`（用户名）字段，作为人类可读的唯一标识，便于管理员在列表中识别和搜索用户。UUID/Token 保持机器标识角色不变。

## Confirmed Decisions

- **必填且全局唯一**（用户已确认）：创建用户时必须提供；重复 → `409 conflict`。
- 可编辑（编辑时同样唯一校验）。
- 列表 `query` 搜索扩展为：username / uuid / token 精确匹配任一命中。
- 存量用户迁移：回填 `user-` + uuid 前 8 位（uuid 唯一 ⇒ 回填值唯一）。

## Requirements

### DB（新迁移 0004）
- `users` 增加 `username TEXT NOT NULL UNIQUE`（SQLite 重建表方式保证 NOT NULL + UNIQUE），回填存量：`'user-' || substr(uuid, 1, 8)`。
- 索引：唯一约束即索引；`query` 搜索无需额外索引（MVP 量级）。

### Admin API（契约同步更新 docs/api-contract.md）
- User DTO（list/detail/create/update）增加 `username` 字段。
- `POST /api/users`：`username` 必填，空/缺失 → `422 validation`；重复 → `409 conflict`；格式校验：1–64 字符，字母/数字/`_`/`-`/`.`。
- `PUT /api/users/:id`：支持修改 `username`，同样的唯一/格式校验。
- `GET /api/users?query=`：命中范围扩展为 username | uuid | token 精确匹配。
- 不影响 Agent API：eligible 同步载荷不含 username（Agent 只需要 uuid/凭据）。

### 前端（web/）
- 创建用户对话框：`username` 必填项。
- 用户列表：新增「用户名」列（uuid 列保留）；搜索框提示文案覆盖用户名。
- 用户详情：基本信息区显示 `username` 并支持编辑；类型定义同步 `web/src/api/types.ts`。

### 兼容性
- 现有测试更新：用户创建相关 fixture 补 `username`；新增唯一冲突/格式校验/搜索命中用例。
- Agent 侧零改动；`sing-box` 渲染不受影响。

## Acceptance Criteria

- [ ] 创建用户不传 username → 422；传重复 username → 409；合法值 → 201 且 DTO 含 username。
- [ ] PUT 修改 username 成功；改成他人已占用值 → 409；改回自己原值 → 200（不与自身冲突）。
- [ ] query=username 精确命中用户；uuid/token 搜索行为不变。
- [ ] 迁移在含存量用户的库上执行后：老用户获得 `user-xxxxxxxx` 用户名且全部唯一，迁移可重复执行（幂等）。
- [ ] 前端创建必填校验、列表列、详情编辑可见可用；typecheck/build/lint 通过。
- [ ] `go build/vet/test -count=1`、`-race` 全绿；docs/api-contract.md 与实现一致。

## Out Of Scope

- 用户名作为登录凭据（管理员登录体系不变）。
- 用户自助修改用户名、订阅链接中的用户名展示。
- 模糊搜索/排序（保持精确匹配）。

## Risks

- SQLite 重建表迁移需在事务中完成并保持外键引用有效（users 被 sessions/user_nodes/traffic 引用——重建表需 `PRAGMA foreign_keys=off` 或 `legacy_alter_table` 处理，测试覆盖迁移前后 FK 完整性）。
