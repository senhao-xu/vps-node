# Design — Frontend style optimization (skill principles, neutral palette)

## Scope recap

In scope: **P0** (accessibility / reduced-motion), **P1 dedup** (excluding breakpoint numeric normalization P1-5), **low-risk P2** (P2-1 tabular figures, P2-2 dead tokens, P2-4 status dots). Out of scope: palette change, `@media (prefers-color-scheme)`, behavior/data changes, P1-5, P2-3, P1-10 font-size literals (see Deferred).

## Architecture & boundaries

- Design system owner: `web/src/styles/tokens.css` (values) + `web/src/styles/base.css` (shared primitives).
- Page-only layout stays in page `<style scoped>`; component-only layout stays in component `<style scoped>`.
- No new dependencies, no CSS framework, no new color literals.
- Dark mode stays `[data-theme='dark']` (JS-driven in `stores/theme.ts`); do **not** add `prefers-color-scheme`.

## Token changes (`tokens.css`)

1. Add `--radius-xs: 4px` to `:root` geometry block (theme-independent, no dark override required).
2. Remove confirmed-dead tokens (verified: referenced nowhere outside their own declaration):
   - `:root` + dark: `--color-table-hover`, `--color-primary-hover`, `--color-danger-hover`
   - `:root` only: `--radius-lg`, `--radius-xl`, `--gray-25`, `--ink-950`
3. Keep `--font-size-xs` (used by `StatCard.vue`).

## Shared primitives (`base.css`)

Add/change, in this order:

- `html, body { min-width: 320px }` (P0-5).
- `.card.compact { --card-padding: var(--spacing-md) }` (P1-2).
- `.card-head-actions { display: inline-flex; align-items: center; gap: var(--spacing-sm); flex-wrap: wrap; margin-left: auto }` (P1-11).
- `.status-dot` primitive + tone modifiers `success|warning|danger|primary|muted` (P2-4): base 8px circle, default `--color-text-secondary`; `muted` = `--color-offline`.
- `.btn:disabled` opacity `0.55` → `0.52` (P1-6).
- `@media (max-width: 560px) { .card-head { align-items: flex-start; flex-direction: column } }` (P1-1, preserves the exact width already used by the 7 user components).
- `@media (prefers-reduced-motion: reduce) { *, *::before, *::after { animation-duration: .01ms !important; animation-iteration-count: 1 !important; transition-duration: .01ms !important; scroll-behavior: auto !important } }` (P0-1).

## Component changes

### P0
- `components/ErrorBanner.vue` — root `.error-banner` gets `role="alert"`.
- `components/MenuSearch.vue` — delete the `.palette-input:focus { box-shadow: none; ... }` override so the base input focus ring shows.
- `components/DataTable.vue` — `.table-box` gets `role="region"`, `tabindex="0"`, `:aria-label` from a new optional `ariaLabel` prop (default `数据表格`); add `.table-box:focus-visible` ring; add `font-variant-numeric: tabular-nums` to `.data-table` (P0-4 + part of P2-1).
- `styles/base.css` — the `min-width: 320px` floor above.

### P1 dedup
- `card-head` 560px stacking: remove the duplicate scoped blocks in `UserDetailExpiry.vue`, `UserDetailLimits.vue`, `UserDetailTraffic.vue`, `UserDevices.vue`, `UserNodeAuth.vue`, `UserTrafficStats.vue`, `UserVisits.vue`; rely on the global rule. (Now also applies to `ServerDetailPage.vue` / `VisitsPage.vue` card-heads — accepted consistency change; verify at ≤560.)
- `card.compact`: `UserDetailLimits.vue`, `UserDetailTraffic.vue`, `UserDetailExpiry.vue` add `compact` to their root card class; delete the `--card-padding` declaration. `UserDetailExpiry.vue` keeps a scoped `.card { display:flex; flex-direction:column }` (unique) but drops the padding line.
- `head-actions`: `UserDetailTraffic.vue`, `UserNodeAuth.vue`, `UserVisits.vue` switch `class="head-actions"` → `class="card-head-actions"` and delete local `.head-actions` rules; keep only their unique mobile overrides scoped to `.card-head-actions`.
- Stat grid: add shared `.stat-skeleton`/`.skeleton-line`/`.skeleton-value` to `base.css`; remove duplicated blocks in `DashboardPage.vue` and `ServersPage.vue`. Each page keeps only its own `.stat-grid` column strategy (Dashboard 4→2→1; Servers `auto-fill minmax(180px,1fr)`).
- `SegmentedControl` reuse: in `ServerDetailPage.vue` replace `.install-tabs`/`.install-tab` markup with `<SegmentedControl :items v-model="installTab" />` (bind `installTab`), delete `.install-tabs`/`.install-tab(.active)` CSS; keep the 700px `.install-head` stacking and give the control `class="install-segmented"` with `align-self:flex-start` in that media query.
- `UserDetailPage.vue`: remove the redundant `.two-col .card + .card` rule (already covered by `base.css` `.two-col > .card + .card`); keep `.page .card + .card` and `.two-col + .card`.
- Disabled unification: `TablePaginator.vue` `.45` → `.52`; `ToggleSwitch.vue` `.55` → `.52`; `AppSelect.vue` add `opacity:.52` to `.app-select:disabled` and drop the per-child `opacity:.5` overrides.
- Popover radius: `FilterChip.vue` panel `--radius-sm` → `--radius-md` (all three overlays then use md + `--shadow-dialog`).
- Radii tokens: `999px` → `--radius-full` in `StatusBadge.vue`, `ProgressBar.vue` (2×), `ToggleSwitch.vue`, `DashboardPage.vue` (`.health-pill`), `NodeFormDialog.vue`; literal `4px` → `--radius-xs` in `SegmentedControl.vue` + `base.css` `.theme-option`; `6px` → `--radius-sm` in `base.css` `.kbd` and `DashboardPage.vue` `.expand-btn`.
- Mono font: add `class="mono"` to the relevant nodes in `ServerDetailPage.vue` (`.install-code`), `NodeFormDialog.vue`, `OneTimeSecret.vue`; delete the duplicated `font-family` declarations (keep scoped size/line-height).

### P2 (low-risk)
- P2-1 tabular figures: `font-variant-numeric: tabular-nums` on `StatCard.vue` `.stat-value`, `ProgressBar.vue` `.percent`, `MetricBar.vue` `.metric-value`, `TablePaginator.vue` `.page-info` (plus `.data-table` above).
- P2-2: token removals listed under Token changes.
- P2-4: add `.status-dot` primitive; migrate `NodesPage.vue` (drop duplicate base `.status-dot` block, keep/rename `.on`→use `success`), `AppLayout.vue` `.version-dot` → `<span class="status-dot success">`, `AppSelect.vue` `.dot*` → `.status-dot` + tones. Leave `DashboardPage.vue` `.health-pill i` and `TrafficChart.vue` legend swatch as documented exceptions (inline pill indicator / chart series swatch, not standalone dots).

## Contracts & compatibility

- `DataTable` gains one optional prop `ariaLabel?: string` with a default — non-breaking for all existing call sites.
- `ServerDetailPage` install tabs keep the same `installTab` ref type `'binary' | 'docker'`, so `activeInstallCommand` / `installNotes` lookups are unchanged.
- No template API changes to `SegmentedControl`, `ToggleSwitch`, `ErrorBanner`.
- Removing dead tokens is safe per verified zero usages; if typecheck/lint reveal a miss, restore the token instead of expanding scope.

## Trade-offs

- Deduping `card-head` globally at 560px intentionally extends stacking to 2 pages that previously didn't stack; chosen over leaving 7 clones.
- Deferring P1-5 (breakpoint normalization) and P2-3 (semantic colors) keeps the change set hand-verifiable given there is no visual test harness.
- Deferring P1-10 (font-size literal tokenization) avoids inventing single-use tokens / changing brand-mark sizes.

## Rollout / rollback

- Single branch, no migration. Rollback = `git revert` of the implementation commit(s); CSS/token changes are additive/removable.
- Verify after each batch with `npm run typecheck`, `npm run lint`, `npm run build` (in `web/`), then a manual light/dark pass at 320 / 375 / 560 / 700 / desktop.

## Deferred (follow-up tasks, not this task)

- P1-5 breakpoint normalization (560/600/640/900/1200 → shared scale).
- P2-3 semantic-color review (`TrafficChart` success=download, `OneTimeSecret` warning box, `labels.ts` disabled tones).
- P1-10 font-size literal tokenization.
- P2-5 `!important` removal, P2-6 shell-active alias, P2-7 title weight token.
