# Spike：嵌入 sing-box 可行性（Phase 0 结论）

- 日期：2026-09-22
- 工作目录：`/tmp/opencode/sbspike`（临时验证程序，不属于仓库）
- 结论：**可行（go）**。

## 验证方法

最小 spike 程序：
1. `ctx := include.Context(context.Background())`
2. `opts, _ := singJSON.UnmarshalExtendedContext[option.Options](ctx, cfgJSON)`
3. `inst, _ := box.New(box.Options{Context: ctx, Options: opts})`
4. `router := service.FromContext[adapter.Router](ctx); router.AppendTracker(tracker)`
5. `inst.Start()`
6. 用带用户名密码的 `socks` inbound，手工 SOCKS5 握手把一个 payload 经 direct 出站
   回环到本地 echo 服务。
7. tracker 的 `RoutedConnection` 打印 `metadata.User`。

运行输出：

```
[spike] sing-box embedded instance started
[tracker] TCP user="u1" inbound="socks-in" inboundType="socks" src=127.0.0.1
[spike] tracker calls=1 user=u1
```

→ 连接级能拿到 `metadata.User`，即 **per-user 归因在进程内可得**，从根上解决
Clash API 无 `inboundUser` 的问题。

## 关键结论（决定设计）

1. **版本定为 `github.com/sagernet/sing-box v1.13.2`**（与 Xboard-Node 一致）。
   - 本地 Go 1.25.0 可直接编译；`v1.14.1` 要求 Go ≥ 1.25.5（需切 toolchain），
     且其 `ConnectionTracker` 多了 `RoutedFlow` 方法（接口差异）。
   - v1.13.2 的 `adapter.ConnectionTracker` 只有
     `RoutedConnection` / `RoutedPacketConnection` 两个方法。
2. **构建 tag：`with_quic` 必需**（hysteria2 为 QUIC inbound）。`with_utls/with_wireguard/
   with_clash_api` 本期不需要。`CGO_ENABLED=0` 可静态构建。
3. **二进制体积**：嵌入后 spike 约 **38 MB**（含 with_quic）。
4. **注入 tracker 的位置**：`box.New` 内部创建 router 并注册进 ctx
   （`service.MustRegister[adapter.Router](ctx, router)`），因此既可用
   `inst.Router().AppendTracker(t)`，也可用 `service.FromContext[adapter.Router](ctx)`。
   在 `Start()` 之前 append 即可。
5. `box.New` 的 ctx 必须来自 `include.Context(...)`（注册全部 protocol registry），
   否则 `missing inbound registry`。

## 对设计与实现的影响

- `design.md` §2/§6：pin `v1.13.2`；tag `with_quic`；agent 侧只需 38MB 增量；
  tracker 注入方式如上。
- 面板渲染的 sing-box 配置需按 **1.13.x** 语义（当前渲染为 1.14 风格，v1.13 兼容性需在
  Phase 2 校验；主要差异在 DNS/outbound 类型，本项目仅用 inbound + direct，风险低）。
- 环境注意：本机访问 `proxy.golang.org` 的 IPv6 超时，需 `GOPROXY=https://goproxy.cn,direct`；
  CI/Docker 构建环境需确保模块可拉取。
- Go 版本：无需 bump（Dockerfile 现有 `GO_VERSION=1.25` 可用）。

## 复现命令

```bash
cd /tmp/opencode/sbspike
GOPROXY=https://goproxy.cn,direct GOSUMDB=off GOTOOLCHAIN=local \
  CGO_ENABLED=0 go build -tags with_quic -o sbspike .
./sbspike
```
