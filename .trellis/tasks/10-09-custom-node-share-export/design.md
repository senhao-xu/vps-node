# Design — 自定义节点分享导出与编辑回显

## 1. Architecture & Boundaries

沿用既有分层：`internal/web` 负责 HTTP 与 DTO，`internal/subscription` 负责纯渲染，`internal/repo` 负责读取，`internal/secrets` 负责加解密。前端 `web/src/api/*` 是唯一的类型/解码层，页面/组件只消费类型化函数。

新增两个只读管理接口（均走 `requireAdmin`）：

| Method | Path | 用途 |
|---|---|---|
| `GET` | `/api/custom-nodes/{id}/share` | 生成 Clash proxies 片段 + 明文链接列表 |
| `GET` | `/api/custom-nodes/{id}/content` | 回显单个节点的解密原文 |

两者都不写库（`share` 对 `subscription` 来源可能触发懒拉取并写缓存，与现有渲染路径一致）。

## 2. Contracts

### GET /api/custom-nodes/{id}/share

Response `200`:

```json
{
  "source_type": "links",
  "has_cache": false,
  "fetched_at": null,
  "clash": "proxies:\n    - name: HK-1\n      type: ss\n      ...\n",
  "links": ["ss://...", "vless://..."],
  "skipped": ["garbage line"]
}
```

- `clash`：`yaml.Marshal({"proxies": [...]})` 的片段；无可用条目时为 `"proxies: []\n"`（或等价空序列）。
- `links`：明文逐行链接数组（**不** base64）。`subscription` 上游若为 Clash-only（只有 proxies、无链接），`links` 可为空。
- `skipped`：不可转换为 Clash 的条目（链接行原文，或上游 proxy 的 name / `proxy #i`）。
- `has_cache` / `fetched_at` 语义与 `GET /api/custom-nodes/{id}/nodes` 一致。

错误：未登录 `401`；未知 id `404`。

### GET /api/custom-nodes/{id}/content

Response `200`:

```json
{ "content": "ss://...\nvless://..." }
```

`links` 为原始链接文本，`subscription` 为原始订阅 URL。解密失败 → `500 internal`；未知 id `404`；未登录 `401`。**列表接口不返回 content。**

## 3. Data Flow

```
CustomNodesPage → CustomNodeShareDialog
  → GET /api/custom-nodes/{id}/share
      handler:
        repo.GetCustomNode(id)
        customNodeSourceContent(cn)        // links: decrypt content_enc
                                           // subscription: lazy fetch (TTL + stale) 或 cache
        subscription.RenderCustomProxies(source, sourceID, onSkip) → proxies[]
        clash = yaml.Marshal({"proxies": proxies})
        links = source.Links（明文）
  ← { clash, links, skipped, ... }

CustomNodeFormDialog(edit) → GET /api/custom-nodes/{id}/content → 预填 textarea
```

`customNodeSourceContent`（`internal/web/custom_nodes.go:102`）已能返回 `(links, proxies, err)`：
- `links` 来源：解密 `content_enc` → `SplitLinkLines`。
- `subscription` 来源：读 `cached_content` → `NormalizeFetchedContent`。

分享接口对 `subscription` 来源需要"取到内容"，因此使用 `fetchCustomNodeContent`（`subscriptions.go:345`，TTL 内用缓存、过期懒拉取、失败回退旧缓存）拿到内容后 `NormalizeFetchedContent`。这与用户订阅渲染路径一致。

## 4. Renderer Change (internal/subscription)

现 `RenderClashFilteredMerged`（`render.go:279`）把 custom 条目的转换逻辑内联。抽出可复用函数，避免重复：

```go
// RenderCustomProxies converts one source's links + upstream proxies into
// Clash proxy maps with unique names. Unparseable items are reported via onSkip.
func RenderCustomProxies(source CustomSource, onSkip func(sourceID int64, item string, err error)) []map[string]any
```

`RenderClashFilteredMerged` 改为调用该函数，保持输出**字节级不变**（现有 `custom_test.go` 的 `TestRenderClashFilteredWithoutCustomMatchesLegacy` 与合并用例必须继续通过）。

分享接口再包一层：

```go
func RenderProxiesFragment(proxies []map[string]any) ([]byte, error) {
    return yaml.Marshal(map[string]any{"proxies": proxies})
}
```

`CustomSource` 复用现有结构（`ID`, `Name`, `Links`, `Proxies`）。分享为单 source，不需要用户/模板。

## 5. Frontend

### types.ts
```ts
export type CustomNodeShare = {
  source_type: CustomNodeSourceType
  has_cache: boolean
  fetched_at: string | null
  clash: string
  links: string[]
  skipped?: string[]
}
export type CustomNodeContent = { content: string }
```

### api/customNodes.ts
```ts
export function getCustomNodeShare(id: number): Promise<CustomNodeShare>
export function getCustomNodeContent(id: number): Promise<CustomNodeContent>
```

### CustomNodeShareDialog.vue（新组件）
- props `open`, `node`；打开时请求 `getCustomNodeShare`。
- 两段：Clash（`proxies` 片段）、V2 原生（`links.join('\n')`）；各带复制按钮，复用 `utils/clipboard.copyText` / `CopyText`。
- `skipped` 非空时以提示列表展示。
- 空状态：对应格式为空时用 `EmptyState` / 说明文案。
- 用 `ModalDialog`，遵循组件规范（role/aria、footer）。

### CustomNodesPage.vue
- `rowActions` 增加 `{ label: '分享', onSelect: () => (shareTarget.value = row) }`。
- 挂载 `<CustomNodeShareDialog :open="shareTarget !== null" :node="shareTarget" @close="shareTarget = null" />`。

### CustomNodeFormDialog.vue
- 编辑打开时：请求 `getCustomNodeContent(id)` 并预填 `content`；请求期间显示 loading，失败显示 `ErrorBanner` 但不阻塞其他字段。
- 去掉"内容不回显"文案，改为"内容已回显，可修改"。
- 提交逻辑保持：`content` 为空则不带 `content` 字段（沿用旧值）。

## 6. Compatibility & Security

- 兼容：`RenderClashFilteredMerged` 重构后输出必须不变；订阅路径复用同一函数，降低漂移风险。
- 安全：明文只经 `GET .../content`（单节点、管理员）返回；列表/用户 custom-nodes 接口不变，不泄露明文。分享同样仅管理员。遵循 `error-handling.md` 的 secret 处理与错误信封。
- 无迁移、无 schema 变更。

## 7. Trade-offs

- **subscription 分享触发懒拉取**：GET 带副作用（写缓存）。为与用户订阅渲染保持一致、并保证管理员拿到可用内容，选择懒拉取 + stale 回退；失败时不报 500，而是回退缓存/空内容。备选（纯 cache-only）会让新节点分享为空，体验差。
- **Clash 仅 proxies 片段**：便于管理员并入自有配置；不生成完整 config，避免把面板默认 DNS/规则强加给导出结果。
- **V2 明文而非 base64**：按用户确认，直接可读可复制；不追加编码。

## 8. Rollback

纯增量接口 + 一个组件 + 一处可逆的渲染函数抽取。回滚即还原 `render.go` 抽取点并移除两个路由/组件；无数据变更。
