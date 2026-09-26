# 设计：自定义节点（管理员全局外部节点/订阅汇入）

## 架构与边界

新增一个"自定义节点"只读汇入通道，完全不触碰 Agent / sing-box 渲染 / 流量统计链路：

```
Admin UI ──CRUD──> /api/custom-nodes ──> repo.custom_nodes (content_enc 加密)
                                              │
订阅请求 handlePublicSubscription              │ 渲染时读取 active 自定义节点
  ├─ 托管节点渲染（现状不变）                    ▼
  ├─ general: 既有 links + 自定义 links/上游 links 原样追加
  └─ clash-meta: 既有 proxies + 解析后的自定义 proxies，名称注入 proxy-groups
```

- **解析器**：新文件 `internal/subscription/import.go`，纯函数 `ParseShareURI(raw string) (map[string]any /*clash proxy*/, error)`，支持 ss / vless / hysteria2 / anytls / trojan / vmess。无状态、无 IO，表单与渲染共用。
- **上游抓取**：`internal/subscription/fetch.go`，`FetchSubscription(ctx, url)` → 文本。仅 http/https、5s 超时、1MiB 响应上限、跟随跳转 ≤3。clash 上游提取 `proxies:` 段并入；general/base64 上游按行并入。
- **缓存**：`custom_nodes.cached_content` + `fetched_at`，TTL 5 分钟。渲染时过期则同步抓取并回写缓存；抓取失败且有旧缓存 → 用旧缓存；无缓存 → 跳过该条（等价 onSkip，记日志）。回写失败不影响响应。

## 数据契约

```sql
-- migration 0006_custom_nodes.sql
CREATE TABLE custom_nodes (
    id INTEGER PRIMARY KEY,
    name TEXT NOT NULL UNIQUE,
    source_type TEXT NOT NULL CHECK (source_type IN ('links','subscription')),
    content_enc BLOB NOT NULL,          -- links: 每行一条 URI; subscription: 上游 URL
    cached_content TEXT NOT NULL DEFAULT '',
    fetched_at INTEGER NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active','disabled')),
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE user_custom_nodes (
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    custom_node_id INTEGER NOT NULL REFERENCES custom_nodes(id) ON DELETE CASCADE,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (user_id, custom_node_id)
);
```

- 授权模型与 `user_nodes` 完全对齐：默认无人可见，repo 提供 `SetUserCustomNodes` / `AuthorizeUserCustomNode` / `RevokeUserCustomNode` / `ListCustomNodeIDsByUser`（镜像 user_nodes.go）。
- 订阅渲染时按 `ListCustomNodeIDsByUser(u.ID)` 过滤 active 自定义节点。
- 无 server_id：不属任何 agent；删除/启停只影响订阅输出。
- content 加密复用 `internal/secrets`（与节点 secret_enc 同模式）。
- 不落 revision：自定义节点不进 agent 配置，无需 bump server revision。

## 渲染合并规则

- 顺序：托管节点在前，自定义节点在后（按 custom_nodes.id 升序）。
- 名称冲突：与托管节点或先前自定义节点重名时，追加 ` <id>` 后缀保证 Clash proxy 名唯一。
- links 类型在 general 格式**逐行原样透传**（含无法解析的行）；clash-meta 中解析失败的行跳过 + Warn 日志（沿用现有 onSkip 模式，subscriptions.go:198）。
- 上游订阅的内容不做协议白名单过滤（上游自行负责），仅 clash-meta 需能转成 proxy 段。

## API

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | /api/custom-nodes | 列表（含 cached 状态，不回显明文 content） |
| POST | /api/custom-nodes | 新建 {name, source_type, content}；links 逐行校验可解析性（仅警告不拒绝） |
| PUT | /api/custom-nodes/{id} | 更新 name/content/status |
| DELETE | /api/custom-nodes/{id} | 删除（user_custom_nodes 级联清除） |
| PUT | /api/users/{id}/custom-nodes | 设置某用户的自定义节点授权（镜像 SetUserNodes 语义） |

错误包络、鉴权（admin session）沿用 `internal/web` 现有模式。

## 前端

- 新页面/区块：建议复用节点列表页加 tab 或独立菜单项 `Custom Nodes`，组件复用 DataTable + ModalDialog + form 模式（参照 NodeFormDialog）。
- 用户编辑对话框增加"自定义节点"授权区块（与托管节点授权并列，同一保存动作提交）。
- API 类型单点：`web/src/api/types.ts` 增加 `CustomNode`；请求函数放 `web/src/api/`。

## 安全与风险

- **SSRF**：订阅 URL 仅 http/https + 超时 + 大小上限；不做内网 IP 段封锁（面板部署在内网时管理员可能订阅内网地址，属管理员自觉行为），但在 PRD/文档注明。
- **内容注入**：general 透传的 URI 原文进入用户订阅 —— 仅管理员可写，与现有 Clash 模板编辑同级信任。
- **回滚**：migration 可逆（drop table）；渲染合并在无自定义节点时为零开销。

## 不做

- 终端用户自助上传（本期仅管理员维护 + 按用户授权）。
- 流量统计、在线设备、链式出站引用。
