# Implement — Frontend style optimization

Validation commands (run from `web/` after every batch):

```bash
npm run typecheck && npm run lint && npm run build
```

Manual visual pass after batches 2, 4, 5: light + dark at 320 / 375 / 560 / 700 / desktop; check `document.documentElement.scrollWidth === clientWidth` unless a labelled table region scrolls.

## Batch 1 — Tokens + shared primitives (foundation)

Touch: `web/src/styles/tokens.css`, `web/src/styles/base.css`.

- [ ] Add `--radius-xs: 4px` to `:root` geometry block in `tokens.css`.
- [ ] Remove dead tokens: `--gray-25`, `--ink-950`, `--radius-lg`, `--radius-xl` (`:root`); `--color-table-hover`, `--color-primary-hover`, `--color-danger-hover` (`:root` **and** `[data-theme='dark']`).
- [ ] `base.css`: `html, body { min-width: 320px }`.
- [ ] `base.css`: add `.card.compact { --card-padding: var(--spacing-md) }`.
- [ ] `base.css`: add `.card-head-actions` primitive.
- [ ] `base.css`: add `.status-dot` + `success|warning|danger|primary|muted` tones.
- [ ] `base.css`: `.btn:disabled` opacity `0.55` → `0.52`.
- [ ] `base.css`: add `@media (max-width: 560px) { .card-head { align-items: flex-start; flex-direction: column } }`.
- [ ] `base.css`: add the `prefers-reduced-motion: reduce` global block at the end.
- [ ] Replace literal radii in `base.css`: `.theme-option` `4px` → `--radius-xs`, `.kbd` `6px` → `--radius-sm`.
- [ ] Validate: `npm run lint && npm run build`.

## Batch 2 — P0 accessibility + high-value components

Touch: `ErrorBanner.vue`, `MenuSearch.vue`, `DataTable.vue`, `SegmentedControl.vue`, `ToggleSwitch.vue`, `TablePaginator.vue`, `AppSelect.vue`, `StatusBadge.vue`, `ProgressBar.vue`, `MetricBar.vue`, `StatCard.vue`, `FilterChip.vue`, `OverflowMenu.vue`.

- [ ] `ErrorBanner.vue`: add `role="alert"` to the root banner.
- [ ] `MenuSearch.vue`: delete the `.palette-input:focus { box-shadow: none; border-color: var(--color-border) }` override.
- [ ] `DataTable.vue`: add `ariaLabel?: string` prop (default `'数据表格'`); root `.table-box` gets `role="region"`, `tabindex="0"`, `:aria-label="ariaLabel"`; add `.table-box:focus-visible { box-shadow: 0 0 0 3px var(--color-focus-ring); outline: none }`; add `font-variant-numeric: tabular-nums` to `.data-table`.
- [ ] `SegmentedControl.vue`: segment `border-radius: 4px` → `var(--radius-xs)`.
- [ ] `ToggleSwitch.vue`: `999px` → `--radius-full`; disabled opacity `.55` → `.52`.
- [ ] `TablePaginator.vue`: disabled opacity `.45` → `.52`; `.page-info` gains `font-variant-numeric: tabular-nums`.
- [ ] `AppSelect.vue`: `.app-select:disabled` gains `opacity: .52`; remove per-child `opacity:.5` overrides; panel radius stays `--radius-md`.
- [ ] `StatusBadge.vue`: `border-radius` `999px` → `--radius-full`.
- [ ] `ProgressBar.vue`: two `999px` → `--radius-full`; `.percent` gains tabular-nums.
- [ ] `MetricBar.vue`: `.metric-value` gains tabular-nums.
- [ ] `StatCard.vue`: `.stat-value` gains tabular-nums.
- [ ] `FilterChip.vue`: panel `border-radius` `--radius-sm` → `--radius-md`.
- [ ] Validate: `npm run typecheck && npm run lint && npm run build`; manual pass (reduced-motion via devtools emulation).

## Batch 3 — SegmentedControl reuse + mono + stat-grid

Touch: `ServerDetailPage.vue`, `NodeFormDialog.vue`, `OneTimeSecret.vue`, `DashboardPage.vue`, `ServersPage.vue`.

- [ ] `ServerDetailPage.vue`: replace `.install-tabs` markup with `<SegmentedControl class="install-segmented" :items v-model="installTab" />`; import the component; delete `.install-tabs`/`.install-tab` CSS; keep `.install-segmented { align-self: flex-start }` inside the existing `@media (max-width:700px)`.
- [ ] `ServerDetailPage.vue`: `.install-code` add `class="mono"`, delete duplicated `font-family`; `999px` literals → `--radius-full` if any.
- [ ] `NodeFormDialog.vue`: add `class="mono"` where the mono stack is declared, delete duplicate `font-family`; `999px` → `--radius-full`.
- [ ] `OneTimeSecret.vue`: add `class="mono"`, delete duplicate `font-family`.
- [ ] `base.css`: ensure shared `.stat-skeleton`/`.skeleton-line`/`.skeleton-value` exist (added above) and are the single source.
- [ ] `DashboardPage.vue`: delete duplicated `.stat-skeleton`/`.skeleton-line`/`.skeleton-value`; keep the page's `.stat-grid` column queries; `999px` → `--radius-full`; `6px` → `--radius-sm` on `.expand-btn`.
- [ ] `ServersPage.vue`: delete duplicated `.stat-skeleton`/`.skeleton-line`/`.skeleton-value`; keep `auto-fill` grid.
- [ ] Validate: `npm run typecheck && npm run lint && npm run build`; manual pass on ServerDetail install block + dashboards.

## Batch 4 — User detail/panel dedup

Touch: `UserDetailPage.vue`, `UserDetailExpiry.vue`, `UserDetailLimits.vue`, `UserDetailTraffic.vue`, `UserDevices.vue`, `UserNodeAuth.vue`, `UserTrafficStats.vue`, `UserVisits.vue`.

- [ ] Remove the 7 duplicate `@media (max-width:560px) .card-head { ... }` blocks.
- [ ] `UserDetailLimits.vue` / `UserDetailTraffic.vue`: root card gets `compact`, delete scoped `.card` padding block.
- [ ] `UserDetailExpiry.vue`: root card gets `compact`; keep scoped `.card { display:flex; flex-direction:column }`, drop `--card-padding`.
- [ ] `UserDetailTraffic.vue` / `UserNodeAuth.vue` / `UserVisits.vue`: `class="head-actions"` → `class="card-head-actions"`; delete local `.head-actions` rules; keep unique mobile overrides scoped to `.card-head-actions`.
- [ ] `UserDetailPage.vue`: delete `.two-col .card + .card` (covered by `base.css`); keep `.page .card + .card` and `.two-col + .card`.
- [ ] Validate: `npm run typecheck && npm run lint && npm run build`; manual pass on `/users/:id` at 375 / 560 / 700 / desktop, light+dark.

## Batch 5 — P2 status dots + final cleanup

Touch: `NodesPage.vue`, `AppLayout.vue`, `AppSelect.vue`.

- [ ] `NodesPage.vue`: delete duplicate base `.status-dot` block, keep only the `.on` modifier (or switch to shared `success` tone).
- [ ] `AppLayout.vue`: `.version-dot` → use `<span class="status-dot success">`, delete `.version-dot` CSS.
- [ ] `AppSelect.vue`: replace `.dot*` classes with `.status-dot` + tones; delete the `.dot*` CSS.
- [ ] Final full validation: `npm run typecheck && npm run lint && npm run build`.
- [ ] Confirm no hardcoded hex/rgb outside `tokens.css`; palette unchanged.
- [ ] Regenerate `web/dist` (build output) if the project tracks it.

## Rollback points

| After batch | Rollback scope |
|---|---|
| 1 | tokens/base only — revert tokens.css + base.css |
| 2 | per-component; revert batch-2 components + base focus rules |
| 3 | ServerDetail/dialog/dashboard components |
| 4 | user/*.vue components |
| 5 | nodes/layout/select components |

## Risky files (verify manually)

- `base.css` `.card-head` 560px global rule — newly affects `ServerDetailPage.vue` / `VisitsPage.vue`.
- `ServerDetailPage.vue` install tabs — template + ref binding swap.
- `AppSelect.vue` dots — class rename must match template.
- `DashboardPage.vue` / `ServersPage.vue` stat grid — removed CSS must not change column layout.
