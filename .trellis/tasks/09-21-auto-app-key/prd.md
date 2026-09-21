# PRD: app_key 首次启动自动生成，免除部署配置

## 背景

当前 `app_key`(32 字节 hex）是 panel 的必填配置，用于 AES-256-GCM 加密数据库中的协议密钥、派生 SS 用户密码。Docker 部署时用户必须手动 `openssl rand -hex 32` 并通过 env/yaml 注入，compose 中带 `:?` 强校验，部署步骤繁琐。

参考 Xboard 的做法：密钥在安装/首次启动时自动生成并持久化，用户全程无感知。

## 需求

- R1. `app_key` 从必填改为可选。优先级:`PANEL_APP_KEY` env > yaml `app_key` > 数据库 `settings` 表持久化值 > 首次启动自动生成。
- R2. 自动生成使用 `crypto/rand` 产生 32 字节，hex 编码后写入 `settings` 表（key = `app_key`)，后续重启复用。
- R3. 生成/加载全程不在日志、API、UI 中输出 key 本身；启动日志仅记录来源（env / file / auto)。
- R4. 不提供 UI 配置入口，无前端改动，无 API 改动。
- R5. compose 文件中 `PANEL_APP_KEY` 的 `:?` 强校验移除，变为可选；同时给 panel 服务增加 `PANEL_IMAGE` 变量（与 agent 的 `AGENT_IMAGE` 对齐），支持直接使用 GHCR 预构建镜像。
- R6. 文档更新:`deploy/.env.example`、`deploy/README.md`、`README.md` 说明 app_key 可选、首次启动自动生成并持久化在数据库中；注明安全降级（key 与数据库同处一卷，备份数据库即包含 key)。

## 安全说明（已确认的权衡）

key 存入数据库后，数据库泄露即等于密钥泄露，静态加密不再能抵御库文件被拷贝。该降级已获用户确认，文档需明示。

## 验收标准

- AC1. 全新环境（空数据库、无 env/yaml key）启动 panel，成功监听且 `settings` 表出现 `app_key`；重启后 key 不变，加密数据可正常解密。
- AC2. 设置 `PANEL_APP_KEY` 时优先于数据库值生效；env 缺失但 yaml 提供时 yaml 生效。
- AC3. 非法的 env/yaml key（非 hex、长度错误）仍在启动时报错退出。
- AC4. 日志、设置 API 响应中不出现 key 明文。
- AC5. `docker compose up -d`（无 PANEL_APP_KEY）面板正常启动；`PANEL_IMAGE=ghcr.io/...` 时 panel 使用预构建镜像。
- AC6. `go test ./...` 与 `go vet ./...` 通过；现有依赖 app_key 的测试全部适配。

## 非目标

- 不提供 key 轮换/重加密迁移。
- 不在设置页展示 key。
- 不改动 agent 侧配置。
