# Server 详情页 Agent 安装命令（二进制/Docker Tab）

## Goal

管理员在 Server 详情页生成 register token 后，直接看到可复制的 Agent 安装命令，Tab 切换二进制（systemd + install 脚本）与 Docker 两种形态，做到"生成 token → 复制命令 → 节点执行"三步完成接入。

## Confirmed Decisions

- 展示位置：Server 详情页 register token 展示区旁（用户已确认）。
- token 明文仅生成时展示一次（沿用 OneTimeSecret 模式）：**生成瞬间**安装命令内嵌真实 token；此后未重新生成时命令使用 `<register_token>` 占位符并提示先生成 token。
- Panel 地址取 `window.location.origin`（部署在反代后即公网地址）。

## Requirements

### UI（web/，纯前端，后端零改动）
1. Server 详情页 Agent 信息区新增「Agent 安装」块：
   - Tab 切换：「二进制安装」/「Docker 安装」，zh-CN 标签。
   - 命令展示区：等宽字体代码块 + 一键复制按钮（复用 CopyText）。
   - 仅在「本次会话刚生成的 token 明文可见」时内嵌真实 token；否则用占位符 + 提示「先生成 Register Token」。
2. 二进制 Tab 命令模板（对齐 deploy/install-agent.sh 环境变量）：

   ```bash
   PANEL_URL=<origin> SERVER_ID=<id> REGISTER_TOKEN=<token> \
     curl -fsSL <origin>/install-agent.sh | sh
   ```

   注：install 脚本目前随 release tarball 分发，命令下方附一行说明（脚本地址需按实际发布渠道替换）。
3. Docker Tab 命令模板（对齐 deploy/Dockerfile.agent 与 README 约定）：

   ```bash
   docker run -d --name panel-agent --init --restart unless-stopped \
     -v panel-agent-state:/var/lib/panel-agent \
     -e AGENT_PANEL_URL=<origin> \
     -e AGENT_SERVER_ID=<id> \
     -e AGENT_REGISTER_TOKEN=<token> \
     -p 8388:8388 -p 8388:8388/udp \
     vps-node-agent:latest
   ```

   必须含 `--init`（僵尸进程）；附注：镜像需自行构建或在节点可见的 registry；端口按节点实际端口提示修改。
4. 类型安全：不新增 API 类型；命令拼接为纯函数（utils），便于测试。

## Acceptance Criteria

- [ ] Server 详情页可见 Tab 切换的安装命令块，二进制/Docker 内容与上述模板一致且参数（origin/server_id/token）正确注入。
- [ ] 生成 register token 后命令内嵌真实 token；刷新页面（无 fresh token）后显示占位符与提示。
- [ ] 两个 Tab 均可一键复制，剪贴板内容完整。
- [ ] `cd web && npm run typecheck && npm run build && npm run lint` 通过；Go 侧零改动（`go build ./...` 不受影响）。

## Out Of Scope

- 后端新增「安装命令模板」接口；install-agent.sh 的分发渠道搭建；镜像 registry 自动推送。
- 自定义端口/参数表单（MVP 用文本占位提示手动改）。

## Risks

- `window.location.origin` 在非标端口/反代路径下可能不完全等于 Panel 公网地址——代码块可编辑前提示用户按需替换（不做表单化）。
