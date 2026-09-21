# Research: Clash 覆写机制、XBoard 模板与本项目承载边界

- **Query**: 研究用户所说的“Clash 覆写”在 XBoard 当前实现和常见订阅系统中的具体机制，包括数据模型、管理入口、模板字段、渲染流程、覆盖范围、安全边界、本项目 settings API/DB 的适配性，以及全局、用户级和任意 YAML 方案的事实比较。
- **Scope**: mixed（XBoard 固定 commit 源码、本项目固定 commit 源码、Mihomo/Clash Verge Rev/subconverter 公开文档）
- **Date**: 2026-09-21
- **Source revisions**: XBoard `cedar2025/Xboard@4f48e61a2cbc6db5338872b6bdb45ef954ec1256`; local project `senhao-xu/vps-node@2759f491045f03ef4496727f457a7fd7ed368739`

## Findings

### Files Found

| File Path | Description |
|---|---|
| `Xboard/app/Models/SubscribeTemplate.php:8-45` | XBoard 订阅模板模型；按全局名称读取/写入内容并使用 Redis 缓存。 |
| `Xboard/database/migrations/2025_07_27_000001_create_v2_subscribe_templates_table.php:13-20,28-89` | `v2_subscribe_templates` 表结构、默认模板迁移和旧 settings 键迁移。 |
| `Xboard/app/Http/Controllers/V2/Admin/ConfigController.php:197-207,211-237` | 管理配置读取/保存订阅模板的入口。 |
| `Xboard/app/Http/Requests/Admin/ConfigSave.php:109-114` | 六个模板字段；均仅标为 `nullable`，未见 YAML 结构或大小规则。 |
| `Xboard/app/Http/Routes/V2/AdminRoute.php:29-43` | 管理端 `/config/fetch`、`/config/save` 路由，受 `admin` 与 `log` middleware 保护。 |
| `Xboard/app/Protocols/ClashMeta.php:144-234` | Clash Meta 模板解析、服务节点生成、代理组填充、规则处理和 YAML 输出。 |
| `Xboard/app/Protocols/Clash.php:23-106` | Clash 的同型模板渲染流程。 |
| `Xboard/resources/rules/default.clash.yaml:1-89` | 默认全局模板，含客户端全局参数、DNS、空 `proxies`、代理组和规则。 |
| `Xboard/app/Http/Controllers/V1/Client/ClientController.php:34-87,91-190` | 订阅 URL 仅接受 `types`、`filter`、`flag`；未见模板选择/覆写参数。 |
| `internal/db/migrations/0001_init.sql:152-156` | 本项目通用 `settings(key TEXT PRIMARY KEY, value TEXT, updated_at)` 表。 |
| `internal/repo/settings.go:7-50` | 本项目任意字符串 setting 的 get/set/list 持久层。 |
| `internal/web/settings.go:10-89` | 当前 settings API 的显式字段、范围校验和逐键保存。 |
| `internal/web/web.go:18-24,145-147,202-257` | settings 键、管理员路由、DTO 映射及热路径读取。 |
| `internal/web/dto.go:227-233` | 当前管理 API 公开的 settings DTO，仅含 5 个数值/布尔字段。 |
| `internal/web/respond.go:15,67-76` | API 请求体统一上限为 1 MiB。 |
| `web/src/api/types.ts:144-150` | 前端 `Settings` 类型。 |
| `web/src/pages/SettingsPage.vue:7-68,77-165` | 当前设置管理页面及完整保存字段集合。 |
| `.trellis/spec/backend/node-protocol-settings.md:7-9,31-32` | 本项目节点公开设置与加密秘密分离、通用 DTO 不暴露秘密的契约。 |

### “Clash 覆写”实际指向三种不同机制

公开实现中，“覆写”不是一个统一协议字段，至少存在以下三类机制：

1. **订阅服务端模板装配（XBoard）**：服务端先读取一份全局完整 YAML 模板，再把当前用户可用节点装入其中。这是订阅生成器行为，不是 Mihomo 自身的通用“覆写文件”协议。
2. **客户端配置后处理（Clash Verge Rev 扩展脚本）**：客户端收到订阅后，把 YAML 转成对象交给 JavaScript `main(config, profileName)`；脚本可替换 `dns`、`proxy-groups`、`rules` 等任意对象字段。文档明确脚本无网络 IO/文件 IO，但其输入输出是完整配置对象。该机制位于终端客户端，不由订阅 URL 或 XBoard 服务端执行。
3. **Mihomo `proxy-providers.<name>.override`**：这是加载 provider 节点时对**单个代理节点字段**的覆写机制，支持名称前后缀、正则改名、`udp`、`tfo`、`skip-cert-verify`、Hysteria 上下行等，以及受限的 yq 风格 `override-expr`。它不等同于覆盖顶层 `rules`、`dns` 或 `proxy-groups`。

另有 **subconverter 外部配置**：其 `/sub?target=...&url=...&config=...` 中 `config` 是外部转换配置文件，负责转换和规则/组装配；这是一种订阅转换服务参数。它与 XBoard 的 `flag`、`types`、`filter` 参数不是同一种机制。

因此，如果需求中的“Clash 覆写”来自 XBoard 语境，源码事实最接近“服务端全局 Clash Meta 完整模板 + 受保护的节点注入”，而不是用户上传 merge patch，也不是 Mihomo provider 的 `override` 字段。

### XBoard 数据模型和管理入口

XBoard 在所检查 revision 中使用独立表而非用户字段：

```php
Schema::create('v2_subscribe_templates', function (Blueprint $table) {
    $table->id();
    $table->string('name')->unique();
    $table->mediumText('content')->nullable();
    $table->timestamps();
});
```

见 `database/migrations/2025_07_27_000001_create_v2_subscribe_templates_table.php:13-18`。固定模板名包括 `singbox`、`clash`、`clashmeta`、`stash`、`surge`、`surfboard`（`:31-58`）。迁移优先搬运旧 `v2_settings.subscribe_template_<name>`，否则从规则文件读取默认值，然后删除旧 settings 记录（`:60-89`）。这表明当前模型是**按输出格式全局一份**，不是按用户、套餐或订阅 URL 一份。

`SubscribeTemplate::getContent($name)` 以 `subscribe_template:<name>` 为 Redis key 缓存 3600 秒；`setContent` 按 `name` update-or-create 后主动失效缓存（`app/Models/SubscribeTemplate.php:17-35`）。没有 `user_id`、模板版本、租户或选择器字段。

管理员入口位于受 `admin` middleware 保护的配置 API：`GET /<secure_path>/config/fetch` 与 `POST /<secure_path>/config/save`（`app/Http/Routes/V2/AdminRoute.php:29-43`）。fetch 返回：

- `subscribe_template_clash`
- `subscribe_template_clashmeta`
- `subscribe_template_singbox`
- `subscribe_template_stash`
- `subscribe_template_surge`
- `subscribe_template_surfboard`

见 `ConfigController.php:197-207`。save 将这些 API 字段映射成表内短名称并调用 `SubscribeTemplate::setContent`（`:211-227`）。请求规则只标记 `nullable`，在检查路径中未发现 YAML schema、允许键、文档数、别名展开或内容长度验证（`ConfigSave.php:109-114`）。管理 UI 的具体源码不在该 Laravel 仓库已检查的内置 `resources` 中定位到；后端管理入口可以确认，前端编辑器形态无法从本 revision 确认。

### XBoard Clash Meta 模板字段及渲染顺序

默认模板是完整 Clash 配置，不是一个局部 patch。`resources/rules/default.clash.yaml:1-89` 明确含：

- 顶层运行参数：`mixed-port`、`allow-lan`、`bind-address`、`mode`、`log-level`、`external-controller`、`unified-delay`、`tcp-concurrent`；
- 完整 `dns` mapping；
- 空 `proxies`；
- 三个 `proxy-groups`；
- 完整 `rules` 列表。

Clash Meta 渲染流程是：

1. `subscribe_template('clashmeta')` 从全局 DB 模板取字符串（`ClashMeta.php:150`）。
2. `Yaml::parse($template)` 将整份 YAML 解析为 PHP 结构（`:152`）。
3. 根据已经过订阅控制器筛选的 `$servers` 构造每个用户的协议代理和节点名（`:156-197`）。节点凭据来自当前用户/服务数据，而不是模板占位符。
4. `config['proxies'] = array_merge(template proxies, generated proxies)`（`:199`）。所以模板自带代理会保留，生成代理追加在后；它不是“模板的 proxies 被生成值替换”。
5. 遍历模板的每个 `proxy-groups`（`:200-220`）：
   - 若组内条目被识别为正则，则删除该正则字符串，并加入名称匹配的生成节点；
   - 只要该组出现正则，代码完成该组正则处理后不执行默认的“加入所有节点”；
   - 没有正则的组则在原有组成员后追加全部生成节点。
6. 移除最终 `proxies` 为空的组并重建数组索引（`:221-224`）。
7. `buildRules` 在规则最前面插入当前订阅 Host 的 `DOMAIN,<host>,DIRECT`（`:225,239-254`）。模板其余规则保持模板顺序。
8. `Yaml::dump` 输出两层缩进的 YAML，再对输出文本执行 `$app_name` 字符串替换（`:227-228`）。

传统 Clash renderer 的同型逻辑位于 `app/Protocols/Clash.php:23-106`。区别主要是支持的协议集合；模板、代理追加、组填充和规则前插机制一致。

### 作用域结论：全局模板，不是用户模板或 URL merge

对 XBoard 当前实现逐项判断：

| 候选机制 | 源码结论 |
|---|---|
| 全局模板 | **是**。`v2_subscribe_templates.name` 唯一，Clash Meta 固定读取 `clashmeta`。 |
| 用户级模板 | **否**。模板表没有 `user_id`，订阅渲染未按用户选择模板。 |
| 订阅 URL 参数选择模板 | **否**。控制器仅验证 `types`、`filter`、`flag`（`ClientController.php:34-40`）；`flag` 选 renderer，另两项筛节点，不传模板/merge 内容。 |
| 规则层 merge | **部分且固定**。订阅 Host 的 DIRECT 规则固定前插；没有通用用户规则 merge API。 |
| 节点字段 override | **不是 XBoard 模板主机制**。模板可自带静态 proxies，但用户节点由 renderer 构造后追加；Mihomo provider `override` 是另一机制。 |

XBoard 的模板选择与用户身份只在“同一全局模板被不同用户节点数据填充”处相交。没有证据表明用户能通过其 bearer 订阅 URL 提交模板内容。

### 哪些配置部分能被模板影响

按实际 XBoard 合并顺序，可分为三组：

#### 1. 模板完全决定的部分

除后述三个特例外，模板中的任意顶层键原样进入结果。因此默认模板已经证明可配置全局参数、`dns`、`proxy-groups`、`rules`；任意 YAML 还可表达 Mihomo 支持的 `rule-providers`、`proxy-providers`、`tun`、`listeners`、`external-controller`、`secret`、`authentication`、`geox-url` 等顶层能力。Mihomo 官方配置目录列出的功能面包含 Inbounds/listeners、Proxies、Proxy Providers、Proxy Groups、Rules、Rule Providers、DNS、TUN 和 experimental。

#### 2. 模板与生成数据合并的部分

- `proxies`: 模板代理在前，当前用户生成代理在后；两边都能存在（`ClashMeta.php:199`）。
- `proxy-groups[*].proxies`: 模板可预置内置策略名、其他组名、静态代理名或正则；renderer 再按上述正则/全量规则填入生成节点（`:200-220`）。组本身的名称、类型、URL、interval 等由模板决定。
- `rules`: 模板决定主体，renderer 强制在头部加入订阅域名 DIRECT 规则（`:239-245`）。

#### 3. renderer 最终控制的部分

- 当前用户节点的协议字段和凭据由 `buildShadowsocks`、`buildVless`、`buildHysteria` 等函数构造，不由模板占位符渲染（`ClashMeta.php:156-197,257+`）。
- HTTP 响应头中的流量、总额、过期时间来自当前 `$user`，不来自 YAML（`:229-233`）。
- `$app_name` 只有一个无上下文区分的最终字符串替换点（`:227-228`）。未见 token、UUID、密码等通用模板变量。

因此，XBoard 的完整模板不能直接改写 renderer 已生成代理对象的字段；但它可以额外提供同名或其他静态代理、改变组/规则引用，并通过完整 Mihomo 配置打开其他客户端能力。源码中未见代理名碰撞校验或对模板静态代理凭据的过滤。

### 常见订阅系统中的作用层差异

#### Mihomo proxy-provider override

官方文档将 `override` 明确定义为“加载集合时覆写节点内容”，支持：

- 名称：`additional-prefix`、`additional-suffix`、正则 `proxy-name`；
- 通用/协议节点字段：`tfo`、`mptcp`、`udp`、`udp-over-tcp`、Hysteria `up/down`、`skip-cert-verify`、`name-cert-verify`、`dialer-proxy`、`interface-name`、`routing-mark`、`ip-version`；
- `override-expr`: 受限 yq v4 风格表达式，对每个节点 mapping 顺序执行。

文档同时明确表达式不支持文件/环境访问、多文档、任意未列语法；每项最终必须产生一个 mapping。这一边界比“任意顶层 YAML”窄，且只影响 provider 加载的代理节点，不负责顶层 DNS、规则或组。

#### Clash Verge Rev 客户端扩展脚本

官方文档描述 `main(config, profileName)` 取得整份 YAML 对应对象并返回修改后的对象。示例直接覆盖 `config['dns']`、`config['proxy-groups']`、`config['rule-providers']`、`config['rules']`，也能读取/过滤 `config.proxies`。脚本执行器支持对象、数组、正则等 JavaScript API，但文档明确不支持网络 IO 和文件 IO。这是用户设备本地的可编程 post-processing，作用面大于服务端字段 allowlist。

#### subconverter 外部配置

subconverter README 的公共接口为 `/sub?target=%TARGET%&url=%URL%&config=%CONFIG%`；`url` 可用 `|` 合并多个订阅，`config` 指向外部转换配置。这意味着模板/规则选择可以是 URL 参数，但该 URL 是转换服务接口，而非 XBoard 当前订阅路由。若 `config` 允许远程 URL，转换服务还承担远程资源获取和 SSRF/可用性边界；这不是本地项目当前存在的能力。

### 凭据、节点完整性和危险配置的边界事实

“模板注入”需要区分两类资产：**服务端持有但不应输出的秘密**，以及**订阅客户端运行时配置的能力**。

#### XBoard 当前实现建立的边界

- 模板是管理员保存的全局静态文本；订阅请求没有模板正文参数，所以普通订阅持有人不能通过 URL 注入模板。
- 生成代理凭据不经过模板占位符：renderer 从用户和 server 数据构建代理后追加。因此模板本身没有读取 Reality 私钥、任意用户字段或订阅 token 的模板表达式。
- Symfony YAML parse/dump 使保存文本先成为数据结构再输出，而不是把节点 YAML 片段直接拼进管理员文本。
- `$app_name` 是 dump 后的文本替换；它不是通用求值语言。

#### XBoard 当前完整模板未建立的边界

- 保存校验仅为 `nullable`，检查路径中未见顶层键 allowlist、递归 schema 或 YAML 资源限制。
- 模板允许自带 `proxies`，其中可直接写 `password`、`uuid`、provider `header.Authorization` 等任意静态内容；这些内容会分发给每个 Clash/Meta 订阅用户。
- Mihomo 全局配置能够定义监听器、`allow-lan`/`bind-address`、API `external-controller`/`secret`、认证、远程 proxy/rule providers 和外部资源 URL。官方文档说明 `external-controller` 提供 REST API，`listeners` 可监听指定地址/端口，provider 可设置 URL 和 Authorization header。故任意顶层模板的能力远大于“分流规则覆写”。
- 模板可自带静态代理并可改变组/规则所引用的名字。即使 renderer 始终追加授权节点，最终生效流量也可以只引用模板自带代理、`DIRECT` 或 `REJECT`。
- 未见代理名唯一性、组引用完整性、规则目标存在性或 Mihomo 核心级 dry-run 验证；YAML 可解析不等于 Mihomo 配置可运行。

#### 可把“生成节点不可覆盖”变成结构事实的方式比较

以下是机制事实，不是对产品规划的修改：

| 装配方式 | 对生成 `proxies`/凭据的结构结果 | 可配置面 |
|---|---|---|
| 先解析受限模板，最后强制赋值 `result.proxies = generated` | 模板不能提供或替换任何代理对象；凭据只来自 renderer | DNS、组、规则等允许键 |
| XBoard 式 `template proxies + generated proxies` | 生成节点仍被追加，但模板也能分发静态代理/凭据；同名语义交给客户端 | 几乎完整 YAML |
| 先生成完整配置，再做任意递归 merge | 若 merge 允许 `proxies`，模板可替换/删除/改写节点及凭据 | 完整对象 |
| 对模板做顶层 allowlist，并拒绝递归出现 `proxies`、`proxy-providers`、监听/API/认证键 | 模板数据结构无法到达凭据和额外代理能力 | 明确受限 |
| 保存后重新覆盖 protected keys，并对组/规则做引用校验 | 最终 protected keys 由 renderer 决定；模板仍可控制获准部分 | 取决于 protected/allowed 集合 |

对本项目现有契约而言，`.trellis/spec/backend/node-protocol-settings.md:7-9,31-32` 已把节点数据分成公开 `nodes.settings` 和加密 `nodes.secret_enc`，并要求秘密不出现在通用 DTO。此前研究还确认订阅 renderer 需要自行导出最小客户端凭据：VLESS/Hysteria2 用户 UUID、派生 Shadowsocks 密码和 Reality 公钥，而不能把节点 secret map 整体交给模板。模板 merge 位于这些最小代理对象之外时，才不会扩大可读取秘密的范围。

### 本项目 settings API/DB 对全局 Clash Meta 模板的适配事实

#### 数据库存储能力

本项目已有通用 `settings` 表：`key TEXT PRIMARY KEY, value TEXT NOT NULL, updated_at INTEGER NOT NULL`（`internal/db/migrations/0001_init.sql:152-156`）。`Repo.SetSetting` 对任意 key/value 直接 upsert，`GetSetting` 和 `GetSettingOr` 可读取字符串（`internal/repo/settings.go:7-22,42-50`）。SQLite `TEXT` 可以承载多行 YAML，当前 schema 没有长度约束或 JSON 类型约束。因此，一份**全局字符串模板**在持久化形态上与该表匹配，不需要仅为大文本而新增关系表。

这个模型不具备：

- 用户外键或 per-user override；
- 多模板/模板选择/版本历史；
- 乐观并发版本；
- 独立模板缓存失效机制（当前读取直接查 DB）；
- secret 分类或加密字段。

所以它能表达“一个已知 key 对应一份全局模板”，不能原生表达用户级、多版本或带秘密的模板模型。

#### 当前 API 能力

当前 `GET/PUT /api/settings` 均受管理员会话保护（`internal/web/web.go:145-147,180-194`），适合管理级可见配置。API 不是任意 settings map passthrough，而是显式 DTO：五个 retention/collection/freshness/offline 字段（`internal/web/dto.go:227-233`）。更新请求同样显式声明字段并做范围校验，随后只保存出现的字段（`internal/web/settings.go:10-16,44-81`）。因此 DB 虽能存模板，**当前 API 与前端尚不能读写它**。

API 的统一 JSON body 上限是 1 MiB（`internal/web/respond.go:15,67-75`），这会成为通过该 API 保存模板的硬上限。当前 decode 未调用 `DisallowUnknownFields`，未知 JSON 字段会被忽略；模板字段若加入显式 DTO，仍需自己的字符串/YAML验证才能获得保存语义。

`settingsDTO()` 调用 `ListSettings` 读取所有 key/value，再白名单映射为 DTO（`internal/web/web.go:202-217`）。`sessionFreshness()` 与 `offlineAfter()` 也调用 `ListSettings`（`:244-257`）。这意味着把大 YAML 放入同一 settings 表在语义上可行，但现有这些读取会把它一并从 SQLite 载入 map，即使调用方只需要一个整数。外部响应仍不会自动泄漏该 key，因为 DTO 是白名单，不直接返回 map。

#### 前端能力

前端 `Settings` 类型仅含相同五字段（`web/src/api/types.ts:144-150`）；`SettingsPage.vue` 的 reactive form 和 PUT payload 也只含这些字段（`:7-13,33-68`）。当前 UI 没有 YAML 文本编辑、错误行列展示或模板预览入口。

#### YAML 工具依赖

项目已经直接依赖 `gopkg.in/yaml.v3 v3.0.1`（`go.mod:5-9`），目前用于应用配置文件读取。因此无需新增 YAML parser 才能完成“解析成结构”的能力；但仓库当前没有 Clash Meta schema validator 或 Mihomo 配置检查器。解析成功只能证明 YAML 语法成立。

### 可实施方案的事实比较

下表按需求点比较三类方案的必需数据面和行为面。它不决定产品选择。

| 维度 | 最小全局受限模板 | 用户级受限模板 | 任意完整 YAML（XBoard 类） |
|---|---|---|---|
| 典型作用域 | 全部 Clash Meta 订阅共享一份 | 每用户一份或全局基线 + 用户差异 | 全部订阅共享一份（若仿 XBoard） |
| 可承载字段 | 例如仅 `dns`、`proxy-groups`、`rules`、可选 `rule-providers` 这类显式集合；`proxies` 由 renderer 强制生成 | 与左同，但每用户存储/选择 | Mihomo 支持的任意顶层键，包括模板自带 `proxies`、监听/API/provider 等 |
| merge 语义必须定义的事实 | 数组是 replace/prepend/append；组如何引用动态节点；renderer 最后覆盖 protected keys | 同左，并多出 global/user 优先级与空值/删除语义 | XBoard 已有明确行为：template proxies 后追加生成 proxies；组正则或全量填充；Host 规则前插 |
| 当前 DB 匹配度 | 高：单一 `settings` TEXT key 可表达 | 低：当前 users/settings 均无 per-user template 关联字段 | 高（若全局）：单一 TEXT 可保存；但配置能力面最大 |
| 当前 API 改动面事实 | settings DTO/request/前端增加一个全局字段及验证 | 用户详情专用 API/DTO、用户持久化和授权路径均需新数据面 | 与全局模板类似，但 validator 不能仅用小型字段 allowlist |
| 凭据边界 | 可从结构上禁止模板触及 `proxies`，生成凭据只进入 renderer 输出 | 同左；用户只能影响自己的配置仍需服务端约束 | 若允许模板 `proxies`，管理员可分发模板静态凭据；若任意 post-merge，甚至可改写生成凭据 |
| 节点授权边界 | 最终 `proxies` 强制使用已授权且 eligible 的生成集合 | 同左，且模板记录必须绑定同一用户 | XBoard 的生成节点仍来自筛选集合，但模板可以额外提供不在授权集合中的静态代理 |
| 危险客户端能力面 | 取决于 allowlist；可排除 listeners、controller、authentication、远程 providers | 同左 | 包含 Mihomo 完整配置能力；保存者能定义监听、API、远程下载、认证头等 |
| 校验对象 | YAML 单文档、大小/深度/别名限制、允许键类型、组/规则引用、禁止键 | 同左 + 用户记录归属和配额 | YAML 语法 + Mihomo 全配置兼容性；静态字段 allowlist 与“任意”目标相冲突 |
| 发布/失败半径 | 一次修改影响所有用户，但字段面有限 | 单用户失败，记录数量和测试组合增加 | 一次修改影响所有 Clash 用户且可能让整份配置无法加载 |
| 与 XBoard 一致度 | 中：同为全局模板，但限制字段且通常替换 `proxies` | 低：XBoard 无此数据模型 | 高：完整全局 YAML + renderer 注入节点 |

#### 最小全局模板方案的具体结构事实

若“最小”被定义为不允许模板拥有代理对象，则数据边界可以是模板仅含获准的顶层键，renderer 按如下顺序形成结果：解析并验证模板 → 生成用户节点 → 把动态节点名填入受控代理组 → 最终强制设置 `proxies` → 验证组/规则引用 → dump。此时 `proxies`、节点服务器地址、端口、UUID/password/Reality public key 均不是模板输入。它与本项目现有全局 settings 表一对一匹配。

这一方案仍需明确 `rules` 的顺序语义，因为 Clash/Mihomo 规则按顺序匹配；replace、prepend、append 不是等价行为。XBoard 的事实语义是“Host DIRECT 强制 prepend，模板 rules 保持其后”。

#### 用户级方案的具体结构事实

本项目当前 `settings` 的主键只有全局字符串 key，没有用户外键。把用户 ID 编进 setting key 虽能技术上形成键值，但仓库当前用户 CRUD、级联删除和详情 DTO 均不会获得关系约束；真正的用户级数据模型需出现在用户关联存储和用户详情专用 API 中。每次订阅渲染还需决定“无用户模板时用全局模板”“用户字段是完整替换还是 patch”“用户模板失效时是否回退”等确定语义。

安全能力不因“只作用于自己的订阅”而自动缩小：若用户可编辑任意完整 YAML，仍可在自己的设备配置中加入额外代理、监听器和远程 provider；区别只是服务端数据隔离与影响半径，而非 YAML 本身能力。

#### 任意 YAML 方案的具体结构事实

任意 YAML 与 XBoard 最接近，也最能覆盖 DNS、规则、组和全部 Mihomo 新字段，而无需服务端逐字段升级。但“任意”意味着不能同时声称模板只控制规则/DNS：除非 renderer 删除或覆盖 protected keys，否则模板也控制监听、controller、认证和外部资源。

XBoard 采用“模板为基底、生成 proxies 追加”而非通用 deep merge。若采用不同的 generic merge library，数组处理尤其会改变行为：`proxies`、`proxy-groups`、`rules` 都是序列，replace/append/index-wise merge 会产生不同结果。任意 YAML 方案必须以实际装配算法而非“merge”一词描述兼容性。

### External References

- [XBoard repository at inspected commit](https://github.com/cedar2025/Xboard/tree/4f48e61a2cbc6db5338872b6bdb45ef954ec1256) — 本研究的 XBoard 固定源码 revision。
- [XBoard SubscribeTemplate model](https://github.com/cedar2025/Xboard/blob/4f48e61a2cbc6db5338872b6bdb45ef954ec1256/app/Models/SubscribeTemplate.php) — 全局模板存取和 Redis 缓存。
- [XBoard subscribe template migration](https://github.com/cedar2025/Xboard/blob/4f48e61a2cbc6db5338872b6bdb45ef954ec1256/database/migrations/2025_07_27_000001_create_v2_subscribe_templates_table.php) — 独立表、默认模板和旧 settings 迁移。
- [XBoard ClashMeta renderer](https://github.com/cedar2025/Xboard/blob/4f48e61a2cbc6db5338872b6bdb45ef954ec1256/app/Protocols/ClashMeta.php#L144-L234) — 模板与用户节点装配的权威流程。
- [XBoard default Clash template](https://github.com/cedar2025/Xboard/blob/4f48e61a2cbc6db5338872b6bdb45ef954ec1256/resources/rules/default.clash.yaml) — 完整模板字段实例。
- [Mihomo configuration index](https://wiki.metacubex.one/en/config/) — Mihomo 顶层能力目录，包括 DNS、inbounds、proxies、groups、rules 和 providers。
- [Mihomo proxy-provider override](https://wiki.metacubex.one/en/config/proxy-providers/) — 节点级 `override`、固定字段与受限 `override-expr` 的官方说明。
- [Mihomo general configuration](https://wiki.metacubex.one/config/general/) — `allow-lan`、监听地址、external controller/API secret、authentication 和外部资源等配置能力。
- [Mihomo listeners](https://wiki.metacubex.one/config/inbound/listeners/) — 完整 YAML 可定义客户端入站监听器的依据。
- [Mihomo rule providers](https://wiki.metacubex.one/config/rule-providers/) — 远程 URL、path、Authorization header 和 inline payload 能力。
- [Clash Verge Rev custom script docs](https://www.clashverge.dev/guide/script.html) — 客户端对完整 config 对象后处理、可覆盖 DNS/groups/rules，且脚本无网络/文件 IO。
- [subconverter README](https://github.com/tindy2013/subconverter/blob/master/README.md) — `target`、`url`、`config` URL 参数和多订阅合并机制。
- [Local project at inspected commit](https://github.com/senhao-xu/vps-node/tree/2759f491045f03ef4496727f457a7fd7ed368739) — 本项目结论对应的源码 revision。

### Related Specs

- `.trellis/tasks/09-21-user-subscription-links/prd.md:28-29,56-58` — 首版包含 Clash Meta，R12 要求确定覆写作用域/字段/merge 语义，当前 open question 即 Clash 覆写边界。
- `.trellis/tasks/09-21-user-subscription-links/research/xboard-subscription-research.md` — 既有 XBoard URL、格式协商、凭据和节点 eligibility 研究；本文件补足模板机制。
- `.trellis/spec/backend/node-protocol-settings.md:7-9,31-32` — 公开节点设置与加密秘密的现有隔离契约。
- `.trellis/spec/backend/database-guidelines.md:7-16` — DB/schema/query 变更及 migration/repo 约定。

## Caveats / Not Found

- active-task CLI 在本次研究开始时返回 `(none)`；用户明确给出了任务目录，研究文件仅写入该目录。
- XBoard 结论限定于 commit `4f48e61a2cbc6db5338872b6bdb45ef954ec1256`。仓库 shallow clone 无 release tag，因此不能把行为归因于特定发布版。
- 未在检查的 XBoard Laravel `resources` 或内置 theme 中定位到订阅模板编辑器的前端源码；可以确认管理 API 和字段，不能确认当前官方管理前端的控件、提示或客户端校验。
- XBoard 保存路径未见显式 YAML/schema 校验，但 Laravel 全局 middleware、Symfony YAML 默认选项或未检查插件可能施加额外运行时行为；这里不把“未找到”表述为绝对不存在。
- 未执行 Mihomo 二进制 dry-run，也未验证不同客户端对重复代理名、未知顶层字段和组引用错误的具体容错；这些行为可能随 Mihomo/Clash 客户端版本变化。
- “常见订阅系统”没有统一标准。本研究用 XBoard、Mihomo provider override、Clash Verge Rev 客户端脚本和 subconverter 四种有公开源码/文档的机制说明该术语的不同作用层，不能代表所有机场面板。
