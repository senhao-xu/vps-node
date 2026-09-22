# Journal - xusenhao (Part 1)

> AI development session journal
> Started: 2026-09-20

---


- 2026-09-21: 完成前端整体样式重设计（09-21-frontend-redesign）。双层 token + 暗色模式（light/dark/system），tokens.css 重构，新增 stores/theme.ts，AppLayout 导航图标+主题切换，登录页重设计，硬编码颜色清零。typecheck/lint/build 全绿，commit f1c59fc。
- 2026-09-21: 完成节点协议参数补全（09-21-node-protocol-params）。hy2 obfs/hop_ports、vless flow 三态/dest，singbox.VLESSFlow 统一解析，go test 17 包全绿，commit 6a3b8d3；另提交 UI 修复 bb687f1（Esc 关闭弹窗、form-row 溢出）。
- 2026-09-21: 完成 AnyTLS 协议支持（09-21-anytls-protocol）。复用 hy2 TLS 证书模式，0006 迁移放宽 protocol CHECK，订阅/Clash/前端全链路，commit b8ea6ef；迁移前已备份 panel.db 到 /tmp/panel.db.bak-20260921。
- 2026-09-21: 完成仪表盘用户流量明细（09-21-dashboard-user-traffic）。新 API `GET /api/dashboard/user-traffic?range=today|total`（repo 新增 SumTrafficByUser/SumTrafficByUserNode/ListUsersAll，trafficWhere 重构为 trafficWherePrefixed），前端仪表盘新增可展开的用户流量表（今日/累计切换）。6 个新后端测试，go test + 前端 typecheck/lint/build 全绿，commit a9652a1 已推送。另建占位任务 09-21-site-visit-stats（访问站点统计，待规划）。

## 2026-09-21 auto-app-key 收尾
- 任务 09-21-auto-app-key 完成并提交推送 (1fd4529):app_key 可选,首次启动自动生成存入 settings 表,优先级 env > yaml > database > auto;compose 去掉 PANEL_APP_KEY 强校验,新增 PANEL_IMAGE/AGENT_IMAGE。
- trellis-check PASS,冒烟验证 auto/database/env 三种来源。
- 顺带分析:agent 容器 docker stats 57MB 中 47MB 是 sing-box 二进制的文件页缓存(可回收),真实匿名内存 ~19MB,与 Xboard 相当。
- 后续任务候选:精简 agent 镜像(源码编译 sing-box,仅 with_quic/with_utls/with_clash_api,预期 35MB→~20MB)。

## 2026-09-22 xboard-parity 任务开题 + Phase 0 spike
- 排查流量统计恒为 0：根因是 agent 靠 Clash API 的 `inboundUser` 归因，而 sing-box
  Clash API 从不返回该字段（`experimental/clashapi/connections.go` 只给 network/type/
  sourceIP/destinationIP/sourcePort/destinationPort/host/dnsMode/processPath）。测试 fixture
  是手写的，所以从未暴露。顺带修复的 vless/hy2 用户 `name` 缺失（5cd60ff）不足够。
- 调研：现状 `research/current-model.md`；Xboard（panel+node）`research/xboard-model.md`。
  结论：Xboard-Node 把 sing-box 当 Go library 内嵌，用 `adapter.ConnectionTracker` 连接级
  按 `metadata.User` 计数——这是 per-user 统计的正解。
- 决策（D1–D8，见 prd.md）：不做套餐/订单；保留 user_nodes 逐条授权；保留 host(server)+
  node 两层；维持 4 协议；清库重建不迁移；沿用 REST+revision 下发；观测=流量+在线设备
  （移除 sessions/连接日志）；UI 只对齐功能字段。
- Phase 0 spike 通过（go）：内嵌 sing-box v1.13.2，`-tags with_quic`，真实流量触发 tracker
  拿到 `metadata.User=u1`。产物：`research/embed-spike.md` + `research/embed-spike/{main.go,go.mod}`。
- 本期未改任何生产代码；Phase 1–6 待新会话继续（implement.md 有完整清单与验证命令）。

## 2026-09-22 xboard-parity 实现完成（Phase 0–6）
- 用 trellis-implement / trellis-check 子代理逐阶段实现，全部完成：
  - Phase 0 spike（go）：内嵌 sing-box v1.13.2，`-tags with_quic,with_utls`，真实流量触发
    ConnectionTracker 拿到 metadata.User。
  - Phase 1–2：agent 进程内嵌入 sing-box（`internal/kernel/singbox`），删除外部 sing-box/
    `singbox-reload`/Clash API 采集/Applier/Checker；设备数限制 gate 实现。
  - Phase 3：schema 重建（D5，单 0001_init.sql）：users transfer_enable/u/d/speed_limit/
    device_limit/online_count/last_online_at；nodes protocol_settings/rate/tags；traffic_records
    u/d；新增 online_devices(.online)/device_batches；删 sessions/connection_logs。
  - Phase 4：节点 rate/tags API + protocol_settings 分节重构；渲染器去 clash_api；契约更新。
  - Phase 5：前端 Vue3 对齐（节点表单分节、用户字段、在线设备/连接数页、删 sessions·logs）。
  - Phase 6：rate 入库生效；online_count=DISTINCT ip；online 连接数；文档/spec 更新。
- check 子代理修复的真实缺陷（多轮）：devices 分片覆盖、limits 变更不 bump revision、
  janitor online_count 失准、流量/设备重放永久 422 死锁、rate 溢出、dashboard devices 语义、
  前端 VLESS reality server_port 编辑覆盖。
- 最终校验全绿：go build/vet/gofmt/test（含 `-tags integration,with_quic,with_utls`）+ 前端
  typecheck/build/lint。AC1–AC9 全部达成（AC4 仅单节点本地，已知限制）。
- 未提交（工作区改动待用户 review）；未验证 docker build（沙箱无 Docker）；实机端到端建议手测。
