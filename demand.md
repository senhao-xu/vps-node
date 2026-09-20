# 轻量级多节点用户管理面板

## 1. 项目概述

### 1.1 项目定位

这是一个轻量级的多节点用户管理系统，用于统一管理：

* 用户
* 节点
* 用户与节点之间的权限关系
* 用户流量
* 用户在线连接
* 节点状态
* 节点 Agent

项目不定位为完整的 VPS 管理面板，也不实现 Xboard 类似的支付、订单、商城、工单等复杂业务。

核心架构：

```text
┌──────────────────────────────┐
│            Panel             │
│                              │
│  User      Node      System  │
└──────────────┬───────────────┘
               │
          HTTPS / WebSocket
               │
       ┌───────┼────────┐
       ▼       ▼        ▼
   ┌──────┐ ┌──────┐ ┌──────┐
   │Agent │ │Agent │ │Agent │
   │ HK01 │ │ JP01 │ │ US01 │
   └──┬───┘ └──┬───┘ └──┬───┘
      │        │        │
      ▼        ▼        ▼
    Node     Node     Node
```

---

# 2. 核心目标

第一阶段只实现以下能力：

1. 管理用户
2. 管理节点
3. 管理用户可访问的节点
4. 管理用户流量额度
5. 管理用户有效期
6. 查看用户当前连接
7. 查看节点状态
8. Agent 注册
9. Agent 心跳
10. Panel 向 Agent 同步用户配置
11. Agent 向 Panel 上报流量和连接状态

---

# 3. 非目标

第一阶段明确不实现：

* VPS 开关机
* VPS 重装系统
* SSH 管理
* 文件管理
* Docker 管理
* 服务部署
* 服务器监控告警系统
* 支付
* 订单
* 商城
* 套餐购买
* 优惠券
* 工单
* 邀请返利
* 复杂 RBAC
* 多租户
* AI 功能

项目重点是：

> **User → Node → Agent**

---

# 4. 系统架构

## 4.1 Panel

Panel 是整个系统的控制中心。

负责：

* 用户管理
* 节点管理
* 用户节点权限
* 用户流量
* 用户有效期
* 用户连接记录
* Agent 管理
* Agent 状态
* 配置同步

Panel 不直接管理底层节点程序。

---

## 4.2 Agent

Agent 安装在实际节点服务器上。

负责：

* 向 Panel 注册
* 保持心跳
* 获取节点配置
* 获取用户配置
* 将用户配置应用到节点服务
* 上报流量
* 上报在线连接
* 上报节点状态

Agent 不保存业务数据。

Agent 本地只保存：

* Agent ID
* Agent Token
* Panel 地址
* 节点基础配置
* 当前运行状态

---

## 4.3 User

用户是系统中的访问主体。

用户拥有：

* UUID
* Token
* 流量额度
* 流量使用量
* 有效期
* 节点权限
* 当前连接

---

## 4.4 Node

Node 表示一个实际可以提供服务的节点。

例如：

```text
香港 01
日本 01
美国 01
德国 01
```

Node 与 Agent 是一一对应关系。

```text
Node
  │
  └── Agent
```

---

# 5. Panel 功能

## 5.1 Dashboard

首页只提供必要统计。

显示：

```text
用户总数
在线用户
节点总数
在线节点
今日流量
当前连接数
```

示例：

```text
┌────────────┬────────────┬────────────┬────────────┐
│ 用户       │ 在线用户   │ 节点       │ 在线节点   │
│ 1,280      │ 123        │ 18         │ 17         │
└────────────┴────────────┴────────────┴────────────┘
```

不做复杂监控图表。

---

# 6. 用户管理

## 6.1 用户列表

支持：

* 用户搜索
* UUID 搜索
* Token 搜索
* 状态筛选
* 到期状态筛选
* 分页

列表显示：

| 字段   | 说明        |
| ---- | --------- |
| ID   | 用户 ID     |
| UUID | 用户 UUID   |
| 状态   | 正常 / 禁用   |
| 流量   | 已用 / 总量   |
| 到期时间 | 用户有效期     |
| 节点数  | 用户拥有的节点数量 |
| 在线   | 当前连接数     |
| 创建时间 | 创建时间      |

---

# 7. 用户详情

用户详情是整个 Panel 的核心页面。

## 7.1 基本信息

显示：

```text
用户 ID
UUID
Token
状态
创建时间
更新时间
```

支持：

* 修改状态
* 重置 Token
* 删除用户

---

## 7.2 流量信息

显示：

```text
总流量
已使用
剩余流量
使用百分比
```

例如：

```text
总流量：1 TB
已使用：320 GB
剩余：704 GB

████████░░░░░░░░ 31%
```

支持：

* 修改流量额度
* 重置流量
* 查看流量统计

---

## 7.3 有效期

显示：

```text
创建时间
开始时间
到期时间
剩余时间
```

支持：

* 修改到期时间
* 延长有效期
* 立即过期

用户过期后：

```text
用户状态 = expired
```

Agent 不再允许该用户使用节点。

---

# 8. 用户节点权限

这是系统的核心功能之一。

用户详情中直接管理节点权限。

例如：

```text
节点权限

☑ 香港 01
☑ 香港 02
☑ 日本 01
☐ 美国 01
☐ 德国 01

[保存]
```

用户只能使用被授权的节点。

数据库关系：

```text
users
  │
  │
  ▼
user_nodes
  │
  │
  ▼
nodes
```

一个用户可以拥有多个节点。

一个节点也可以拥有多个用户。

即：

```text
User N : N Node
```

---

# 9. 用户当前连接

用户详情中显示当前连接。

例如：

```text
当前连接

┌────────┬──────────────┬────────────┬────────────┐
│ 节点   │ IP           │ 流量       │ 在线时间   │
├────────┼──────────────┼────────────┼────────────┤
│ 香港01 │ 1.2.3.4      │ 1.2 GB     │ 20 分钟    │
│ 日本01 │ 5.6.7.8      │ 520 MB     │ 5 分钟     │
└────────┴──────────────┴────────────┴────────────┘
```

支持：

* IP
* 节点
* 上传
* 下载
* 连接时间
* 最后活动时间

第一阶段只显示当前连接。

历史连接可以作为后续功能。

---

# 10. 节点管理

## 10.1 节点列表

显示：

```text
节点名称
地址
状态
在线用户
CPU
内存
流量
最后心跳
Agent 版本
```

例如：

```text
香港 01     ● Online     32 用户
日本 01     ● Online     18 用户
美国 01     ● Offline     0 用户
```

支持：

* 创建节点
* 编辑节点
* 删除节点
* 禁用节点

---

# 11. 节点详情

节点详情包含：

## 基础信息

```text
节点名称
节点 ID
地址
端口
协议
状态
Agent 版本
```

## Agent 信息

```text
Agent ID
Agent 版本
最后心跳
连接时间
```

## 系统信息

```text
CPU
Memory
Disk
Network
Uptime
```

## 当前用户

显示该节点当前在线用户：

```text
用户 UUID
IP
上传
下载
连接时间
```

---

# 12. Agent 管理

Agent 启动时向 Panel 注册。

注册流程：

```text
Agent
  │
  │ POST /api/agent/register
  ▼
Panel
  │
  │ 返回 Agent Token
  ▼
Agent
```

Panel 为 Agent 分配：

```text
agent_id
agent_token
```

之后 Agent 使用 Token 与 Panel 通信。

---

# 13. Agent 心跳

Agent 定期发送：

```http
POST /api/agent/heartbeat
```

请求包含：

```json
{
  "agent_id": "agent_xxx",
  "version": "1.0.0",
  "cpu": 23,
  "memory": 52,
  "disk": 41,
  "uptime": 123456
}
```

Panel 保存：

```text
last_seen_at
cpu
memory
disk
uptime
version
```

节点状态判断：

```text
最近心跳 < 60 秒
    ↓
Online

最近心跳 >= 60 秒
    ↓
Offline
```

具体超时时间应配置化。

---

# 14. 用户配置同步

Panel 保存用户的最终状态：

```text
User
 ├── UUID
 ├── Status
 ├── Traffic Limit
 ├── Expire At
 └── Nodes
```

Agent 定期从 Panel 获取：

```http
GET /api/agent/users
```

Panel 返回该节点允许的用户：

```json
{
  "version": 1024,
  "users": [
    {
      "id": 1001,
      "uuid": "xxxxxxxx",
      "status": "active",
      "traffic_limit": 107374182400,
      "expire_at": "2026-12-01T00:00:00Z"
    }
  ]
}
```

Agent 根据配置更新本地节点服务。

---

# 15. 配置版本

Panel 对节点配置使用版本号。

例如：

```text
Node Config Version: 1024
```

Agent 当前版本：

```text
1000
```

Agent 请求：

```http
GET /api/agent/config?version=1000
```

Panel 返回最新配置：

```text
version = 1024
```

Agent 应用成功后：

```text
Agent Version = 1024
```

这样可以避免每次心跳都发送完整用户数据。

---

# 16. 流量上报

Agent 定期上报用户流量。

例如：

```http
POST /api/agent/traffic
```

```json
{
  "node_id": 1,
  "users": [
    {
      "user_id": 1001,
      "upload": 12345678,
      "download": 98765432
    }
  ]
}
```

Panel 对流量进行累计。

最终：

```text
user.traffic_used
```

---

# 17. 在线连接上报

Agent 定期向 Panel 上报：

```http
POST /api/agent/sessions
```

数据：

```json
{
  "node_id": 1,
  "sessions": [
    {
      "user_id": 1001,
      "ip": "1.2.3.4",
      "upload": 123456,
      "download": 456789,
      "connected_at": "2026-09-20T10:00:00Z",
      "last_seen_at": "2026-09-20T10:20:00Z"
    }
  ]
}
```

Panel 根据上报数据更新当前连接。

---

# 18. 用户状态

用户状态：

```text
active
disabled
expired
```

状态规则：

### active

允许使用节点。

### disabled

禁止使用节点。

### expired

超过有效期，禁止使用节点。

Agent 在同步用户配置时只同步：

```text
status = active
expire_at > now
```

---

# 19. 节点状态

节点状态：

```text
active
disabled
offline
```

### active

正常提供服务。

### disabled

管理员主动禁用。

### offline

Agent 超过心跳时间未连接。

---

# 20. 数据库设计

## users

```sql
id
uuid
token
status
traffic_limit
traffic_used
start_at
expire_at
created_at
updated_at
```

---

## nodes

```sql
id
name
address
port
protocol
status
agent_id
agent_version
last_seen_at
created_at
updated_at
```

---

## user_nodes

```sql
user_id
node_id
created_at
```

唯一索引：

```text
(user_id, node_id)
```

---

## agents

```sql
id
node_id
token
version
last_seen_at
created_at
updated_at
```

---

## sessions

```sql
id
user_id
node_id
ip
upload
download
connected_at
last_seen_at
```

---

## traffic_records

```sql
id
user_id
node_id
upload
download
created_at
```

可以按照时间进行聚合。

---

## admins

```sql
id
username
password_hash
created_at
updated_at
```

第一阶段只需要管理员账号，不实现复杂 RBAC。

---

# 21. API 设计

## Admin API

```text
POST   /api/admin/login

GET    /api/users
POST   /api/users
GET    /api/users/:id
PUT    /api/users/:id
DELETE /api/users/:id

POST   /api/users/:id/reset-token
POST   /api/users/:id/reset-traffic

GET    /api/users/:id/nodes
PUT    /api/users/:id/nodes

GET    /api/users/:id/sessions

GET    /api/nodes
POST   /api/nodes
GET    /api/nodes/:id
PUT    /api/nodes/:id
DELETE /api/nodes/:id
```

---

## Agent API

```text
POST /api/agent/register
POST /api/agent/heartbeat

GET  /api/agent/config
GET  /api/agent/users

POST /api/agent/traffic
POST /api/agent/sessions
```

Agent API 与 Admin API 必须完全隔离。

---

# 22. Agent 安全

Agent 与 Panel 通信必须使用 HTTPS。

Agent 使用：

```text
Agent ID
Agent Token
```

进行身份认证。

例如：

```http
Authorization: Bearer <agent-token>
```

Token：

* 创建时随机生成
* 数据库只保存 Hash
* 支持重新生成
* 重新生成后旧 Token 立即失效

---

# 23. Panel 技术建议

推荐：

```text
Backend:
Go

HTTP:
Gin / Fiber

ORM:
GORM

Database:
SQLite / PostgreSQL

Cache:
可选 Redis

Frontend:
Vue 3

UI:
Element Plus

API:
REST
```

第一版：

```text
Go
+
Vue 3
+
SQLite
```

即可运行。

不强制依赖 Redis。

---

# 24. Agent 技术建议

Agent 使用 Go 开发。

原因：

* 单文件部署
* Linux 部署简单
* 内存占用低
* 适合长期运行
* 交叉编译方便

编译：

```text
linux/amd64
linux/arm64
linux/386
```

---

# 25. Agent 部署

目标：

```bash
curl -fsSL https://example.com/install.sh | sh
```

或者：

```bash
wget -O install.sh https://example.com/install.sh
bash install.sh
```

安装后：

```text
/usr/local/bin/panel-agent
/etc/panel-agent/config.yaml
```

使用 systemd：

```text
panel-agent.service
```

---

# 26. Agent 配置

```yaml
panel:
  url: https://panel.example.com
  token: xxxxxxxxx

node:
  id: 1

agent:
  heartbeat_interval: 30
  sync_interval: 30
  traffic_interval: 60
```

---

# 27. 配置同步策略

推荐采用：

```text
Panel = Source of Truth
Agent = Runtime
```

即：

```text
Panel
  │
  │ 用户配置
  ▼
Agent
  │
  │ 应用
  ▼
Node Service
```

不要让 Agent 修改 Panel 中的用户配置。

---

# 28. 故障处理

## Panel 离线

Agent：

* 保留最后一次有效配置
* 不主动删除用户
* 等待 Panel 恢复

---

## Agent 离线

Panel：

```text
Node = Offline
```

但不删除：

* 用户
* 节点
* 节点权限
* 流量数据

---

## 配置同步失败

Agent：

```text
继续使用旧配置
```

直到新配置成功应用。

---

# 29. 用户删除策略

删除用户前：

```text
删除用户节点关系
删除当前连接
停止同步该用户
```

Agent 下一次同步后删除本地用户配置。

---

# 30. 节点删除策略

删除节点前：

1. 禁止节点
2. 停止新的用户配置同步
3. 删除用户节点关系
4. 删除 Agent
5. 删除节点

历史流量记录是否删除由后续版本决定。

---

# 31. MVP 页面

第一版只需要以下页面：

```text
/login

/dashboard

/users
/users/:id

/nodes
/nodes/:id

/settings
```

其中最重要的是：

```text
/users/:id
```

用户详情页面应作为整个系统最核心的操作页面。

---

# 32. MVP 开发顺序

## Phase 1

基础框架：

```text
Panel
├── Admin Login
├── User CRUD
├── Node CRUD
└── User ↔ Node
```

---

## Phase 2

Agent：

```text
Agent Register
Agent Heartbeat
Agent Authentication
Node Online/Offline
```

---

## Phase 3

用户同步：

```text
User Config
Node Config
Config Version
Agent Sync
```

---

## Phase 4

统计：

```text
Traffic
Sessions
Online Users
Node Metrics
```

---

## Phase 5

完善：

```text
Token Reset
Traffic Reset
User Disable
Node Disable
Agent Upgrade
Audit Log
```

---

# 33. 第一版验收标准

完成以下功能即可认为 MVP 完成：

### 用户

* [ ] 创建用户
* [ ] 删除用户
* [ ] 修改用户
* [ ] 禁用用户
* [ ] 修改流量
* [ ] 修改有效期
* [ ] 重置 Token
* [ ] 查看用户详情

### 节点

* [ ] 创建节点
* [ ] 删除节点
* [ ] 修改节点
* [ ] 查看节点状态
* [ ] 查看在线用户

### 用户节点

* [ ] 给用户授权节点
* [ ] 移除节点权限
* [ ] 用户详情查看节点
* [ ] 节点详情查看用户

### Agent

* [ ] Agent 注册
* [ ] Agent Token
* [ ] Agent 心跳
* [ ] Agent 在线状态
* [ ] Agent 配置同步

### 流量

* [ ] Agent 上报流量
* [ ] Panel 累计流量
* [ ] 用户查看流量
* [ ] 节点查看流量

### Session

* [ ] Agent 上报连接
* [ ] 用户查看连接
* [ ] 节点查看连接

---

# 34. 后续可扩展功能

第一版完成以后，可以根据实际需要增加：

```text
订阅链接
多种协议
用户设备限制
速度限制
IP 限制
流量重置
流量统计图
历史连接
审计日志
Webhook
Telegram 通知
Agent 自动升级
多管理员
API Token
```

这些功能都不应该影响第一版的核心架构。

---

# 35. 最终产品结构

最终保持简单：

```text
                ┌──────────────┐
                │    Panel     │
                │              │
                │ Users        │
                │ Nodes        │
                │ Sessions     │
                │ Traffic      │
                └──────┬───────┘
                       │
                 HTTPS / WS
                       │
       ┌───────────────┼───────────────┐
       │               │               │
       ▼               ▼               ▼
    Agent HK        Agent JP        Agent US
       │               │               │
       ▼               ▼               ▼
     Node            Node            Node
```

核心数据关系：

```text
User
 │
 ├──── UserNode ──── Node
 │                     │
 │                     └── Agent
 │
 ├──── Traffic
 │
 └──── Session
```

核心原则：

> **Panel 管理状态，Agent 执行状态，Node 提供服务。**

> **Panel 是唯一的业务数据源，Agent 是节点运行时。**

> **User 与 Node 是多对多关系。**

> **第一版不做任何与“用户、节点、Agent”无关的业务。**

