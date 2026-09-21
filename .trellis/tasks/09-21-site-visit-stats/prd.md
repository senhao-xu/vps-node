# 访问站点统计

## Goal

采集并展示用户访问的目标站点（域名/地址）统计。（占位任务，待后续正式规划）

## Background

- 当前 `traffic_records` / `connection_logs` 不含目标信息，Panel 无法得知用户访问了哪些站点。
- 实现需要 Agent 从 sing-box 采集连接目标（如 sniff 域名 / 连接元数据）、新增上报接口与存储表、前端展示，并需考虑隐私与留存策略。

## Requirements

- TBD（规划时补充：采集范围、存储模型、保留策略、隐私合规、展示位置）

## Acceptance Criteria

- [ ] TBD

## Notes

- 从 09-21-dashboard-user-traffic 的规划中拆出；该任务只负责仪表盘用户/节点流量明细。
