# 节点与服务器管理界面拆分

## Goal

在后台管理中把「节点」从「服务器详情页」中拆出来，提供一个独立的跨服务器节点管理页面，使管理员可以不进入具体服务器即可统一查看、筛选、创建、编辑、启停、删除所有节点。

## Background

- 数据模型层面 Server 与 Node 已经分离：`servers`（物理机 + 唯一 Agent）1:N `nodes`（协议 inbound），见 `internal/db/migrations/0001_init.sql:25-71`。
- 但管理界面层面没有拆开：节点的创建/编辑/删除只能在 `ServerDetailPage.vue` 内进行（`web/src/pages/ServerDetailPage.vue:490`），没有独立的节点管理入口。
- 后端列表 API 已支持跨服务器分页查询：`GET /api/nodes` 的 `server_id` 参数可省略（`internal/web/nodes.go:35-54` → `internal/repo/nodes.go:79`），但返回的列表 DTO 不含服务器名称（`internal/web/dto.go:77-85`），无法直接支撑跨服务器列表展示。
- 本任务只做「管理界面拆分」。节点独立地址、节点跨服务器迁移、节点级状态上报、倍率/分组等均不在范围内（数据模型不动）。

## Requirements

- 新增独立路由页面 `/nodes`（`NodesPage.vue`），并在主导航（`AppLayout.vue`）中增加「节点」入口。
- 节点列表跨服务器展示全部节点，每行显示：名称、协议、端口、所属服务器（名称，可点击跳转服务器详情）、状态、创建时间。
- 支持按服务器、协议、状态筛选，按名称模糊搜索；筛选与分页在服务端生效（分页时客户端筛选不可行）。
- 在节点页面可直接创建节点：创建时通过下拉框选择目标服务器；编辑节点时服务器不可变更（沿用现有约束）。
- 在节点页面可直接编辑、启用/禁用、删除节点，交互复用现有 `NodeFormDialog.vue` 与 `ConfirmDialog.vue`。
- 支持勾选多个节点后批量启用/禁用（客户端逐个调用现有更新 API，不新增批量接口）。
- `ServerDetailPage.vue` 内保留现有节点管理区块作为「单服务器视角」，并在其工具栏提供「在节点页查看」链接（跳转到 `/nodes?server_id=<id>` 并自动应用筛选）。
- 后端节点列表响应补充所属服务器信息（id + name），并支持 `protocol`、`status`、`q`（名称模糊）查询参数。
- 界面文案保持 zh-CN，遵循现有前端目录/组件/类型规范（`.trellis/spec/frontend/`）。

## Out of Scope

- 节点独立连接地址、中转/多入口、节点跨服务器迁移。
- 节点级状态监控/上报、倍率、排序权重、权限分组。
- 改动 `server_revisions`、telemetry 三元组、Agent 配置同步粒度。
- 用户侧订阅逻辑的任何变化。

## Acceptance Criteria

- [x] 主导航出现「节点」入口，`/nodes` 页面展示跨服务器节点列表，行内包含所属服务器名称。
- [x] 服务器/协议/状态筛选与名称搜索在服务端生效，翻页后筛选不丢失；URL query 可还原筛选状态（`?server_id=` 等）。
- [x] 可在 `/nodes` 完成节点创建（选服务器）、编辑、启停、删除，且改动后对应服务器配置正常同步（现有 revision 机制不变）。
- [x] 批量勾选后可批量启用/禁用节点。
- [x] `ServerDetailPage.vue` 节点区块功能不回退，并提供跳转 `/nodes?server_id=<id>` 的链接。
- [x] `docs/api-contract.md` 同步更新节点列表的响应字段与查询参数。
- [x] 后端 `go test ./...`、前端 lint 与 type-check/build 通过。

## Constraints

- 工作区存在其他任务的未提交改动，属于基线，不得回退。
- 不新增 UI 组件库；复用 `DataTable.vue`、`TablePaginator.vue`、`StatusBadge.vue`、`ModalDialog.vue` 等现有组件。
