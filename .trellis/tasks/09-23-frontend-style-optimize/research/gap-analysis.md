# Research: Frontend style gap analysis vs `frontend-styles` skill principles

- **Query**: Analyze `/root/vps-node/web/src` against the *principles* of `/root/server-check/.cursor/skills/frontend-styles/SKILL.md`, keeping the existing neutral/ink palette.
- **Scope**: internal (code read + grep); external skill read for principles only.
- **Date**: 2026-09-23
- **Target**: `/root/vps-node/web/src` (Vue 3 + TS strict + Vite, no UI framework)
- **Source of design system**: `web/src/styles/tokens.css`, `web/src/styles/base.css`

> Note on method: all line numbers below were verified by reading the files. Grep for `#[0-9a-f]{3,8}`, `rgb(`, `rgba(` across `web/src` returns matches **only in `styles/tokens.css`** — no `.vue` file contains a hardcoded color literal.

---

## 1. Current design-system inventory

### 1.1 Token layers (`styles/tokens.css`)

Two-layer system, exactly as the project spec (`directory-structure.md` L46-47) describes:

- **Primitive palette** (`:root`, L2-26): `--gray-0/25/50/100/200/250/400/500/900`, `--ink-900/950`, `--green-100/200/600`, `--amber-100/200/600`, `--red-100/200/500/600`.
- **Dark primitives** (`[data-theme='dark']`, L100-111): `--dark-gray-950/800/700/400/50`, `--dark-green-400`, `--dark-amber-400`, `--dark-red-400/300/800/700`.
- **Semantic aliases** (`:root`, L28-63): 35 `--color-*` tokens (bg, surface, surface-muted, border, border-strong, text, text-secondary, primary, primary-hover, primary-soft, primary-border, on-primary, success/success-soft/success-border, warning/warning-soft/warning-border, danger/danger-hover/danger-soft/danger-border/danger-strong/danger-strong-hover/on-danger, offline, muted-soft, table-hover, overlay, focus-ring, skeleton, toggle-knob, shell, shell-muted, shell-active) + `--select-chevron`.
- **Geometry/type tokens**: `--radius-sm/md/lg/xl/dialog/full` (L65-70), `--spacing-xs/sm/md/lg/xl` = 4/8/16/24/32 (L72-76), `--card-padding` (L78), `--font-family` (L80-82), `--font-size-xs/sm/md/lg/xl` = 12/12/14/20/24 (L83-87), `--shadow-sm/card/dialog` (L89-91), `--control-height: 36px`, `--shell-width`/`--shell-width-collapsed` (L92-94).
- `color-scheme: light` (L96) / `dark` (L154).

### 1.2 Dark-parity check (token level)

Every one of the 35 `--color-*` tokens declared in `:root` has a `[data-theme='dark']` override (dark block L113-148). The shadow tokens are overridden too (L150-152), as is `--select-chevron` (L148). **Token-level dark parity is satisfied.**

Minor observations:
- `--color-offline` dark value is a raw hex `#5f6c82` (L138) instead of a `--dark-*` primitive — the only semantic token that bypasses the primitive layer.
- Dark mode is applied via `[data-theme='dark']` (JS-driven in `stores/theme.ts` + `index.html` inline script), **not** `@media (prefers-color-scheme)` in CSS. This differs from the skill's mechanism but achieves read coverage. `color-scheme` is set per theme.

### 1.3 Shared primitives in `styles/base.css` (class inventory)

| Class | Line | Notes |
|---|---|---|
| `.page` | 26 | page container; mobile overrides L502-514 |
| `.card`, `.card + .card`, `.stat-grid > .card + .card`, `.two-col > .card + .card` | 30-49 | card surface + stacked margin + grid reset |
| `.card-title`, `.card-head` | 51-67 | |
| `.text-secondary`, `.text-danger` | 69-75 | |
| `.btn` + modifiers `.outline/.ghost/.icon/.secondary/.danger/.small/.link/.is-loading` | 97-212, 337-340 | full button system |
| form controls (`input[type=text/password/number/datetime-local]`, `select`, `textarea`) | 214-262 | |
| `.field`, `.field-error`, `.form-row`, `.form-help` | 269-308 | |
| `.actions`, `.table-toolbar`, `.checkbox-label` | 291-324 | |
| `.mono`, `.empty-tip` | 326-335 | |
| `.chip` | 342-353 | |
| `.theme-switch`, `.theme-option` | 355-387 | |
| `.kbd` | 389-401 | |
| `.skeleton` + `@keyframes skeleton-pulse` | 403-418 | |
| `.info-grid`, `.info-item`, `.info-label` | 420-436 | |
| `.menu-list`, `.menu-list-item` | 438-467 | |
| `.toggle-row`, `.toggle-row-text/-label/-desc` | 469-494 | |
| global focus: `button/a/input/select/textarea:focus-visible` | 89-95 | 3px `--color-focus-ring` |
| focus: `input/select/textarea:focus` | 231-236 | 3px ring |
| breakpoints | 496, 502, 516 | 700 / 700 / 480 |

### 1.4 Components that already act as primitives

- **`components/ui/` (business-free primitives)**: `StatCard`, `SegmentedControl`, `ToggleSwitch`, `AppSelect`, `OverflowMenu`, `FilterChip`, `SearchInput`, `LoadingSpinner`, `EmptyState`, `PageHeader` (+ `composables.ts`).
- **App-level shared**: `DataTable`, `TablePaginator`, `ModalDialog`, `ConfirmDialog`, `StatusBadge`, `ProgressBar`, `MetricBar`, `ErrorBanner`, `CopyText`, `AppLayout`, `MenuSearch`, `OneTimeSecret`, `NodeChecklist`, form dialogs.
- `StatusBadge` owns the badge primitive (tones `success/warning/danger/primary/muted`, L32-60); `StatCard` owns the stat card; `SegmentedControl` owns the segmented group.

---

## 2. Principle-by-principle conformance

| # | Principle (from skill) | Status | Evidence |
|---|---|---|---|
| 1 | Reuse an existing primitive before writing new CSS | **partial** | Hand-rolled segmented control in `ServerDetailPage.vue:677-700` (`.install-tabs`) duplicates `ui/SegmentedControl.vue`. Dropdown menu-item styling duplicated in `AppSelect.vue:218-239`, `OverflowMenu.vue:142-158`, `MenuSearch.vue:192-209`, `base.css:445-467`. `.head-actions` duplicated `UserDetailTraffic.vue:182`, `UserNodeAuth.vue:100`, `UserVisits.vue:120`. |
| 2 | No hardcoded hex/rgb outside tokens | **satisfied** | Grep for hex/rgb/rgba in `web/src/**/*.vue` → **zero** matches. All literals live in `tokens.css`. Named colors in `.vue` are only `transparent`/`currentColor` (e.g. `StatusBadge.vue:26`, `LoadingSpinner.vue:27-28`). Inline `style=` only for dynamic geometry (`DataTable.vue:121,168,210`, `ModalDialog.vue:77`, `ProgressBar.vue:33`, floating panels). Caveat: `LoadingSpinner.vue:27` uses `color-mix(in srgb, currentColor 25%, transparent)`. |
| 3 | Semantic color meaning not mixed | **partial** | `TrafficChart.vue:129-131,143-145` uses `--color-success` for the **download** series (green = direction, not health). `OneTimeSecret.vue:44-46,81` uses `--color-warning` for the informational "secret revealed once" box. `labels.ts:16` maps user `disabled` → `tone: 'danger'`; `labels.ts:39` maps server `disabled` → `tone: 'warning'` (same product state, two different semantics). `SettingsPage.vue:377` + `AppLayout.vue:297,501` reuse `--color-shell-active` for content nav. |
| 4 | Dark-mode parity for every `:root` token | **satisfied** | 35/35 `--color-*` overridden (tokens.css L113-148); shadows L150-152; `--select-chevron` L148. `--gray-25` (L3), `--ink-950` (L13) are unused primitives (dead, not a parity gap). `--color-offline` dark raw hex L138. |
| 5 | Focus-visible rings on interactive elements | **partial** | Global ring in `base.css:89-95` covers native controls. **Violation**: `MenuSearch.vue:181-184` sets `.palette-input:focus { box-shadow: none }`, removing the only focus indicator on the palette search input. `ModalDialog.vue:138` sets `outline: none` on the `tabindex="-1"` dialog container (acceptable). No custom focus style on `.page-btn`, `.filter-chip`, `.search-trigger`, etc., but they inherit the global `button:focus-visible` ring. |
| 6 | Disabled states | **partial/inconsistent** | `.btn:disabled { opacity:.55 }` `base.css:149-152`; `.toggle:disabled` `.55` `ToggleSwitch.vue:57-60`; `.app-select:disabled` uses muted bg **without opacity** `AppSelect.vue:178-182`; `.page-btn:disabled { opacity:.45 }` `TablePaginator.vue:161-164`; `.btn.link:disabled` recolors only `base.css:210-212`. Four different disabled treatments. No `filter: grayscale` anywhere. |
| 7 | `prefers-reduced-motion` handling | **violated** | Grep `reduced-motion` across `web/src` → **zero matches**. Animations/transitions that ignore the preference: `@keyframes spin` `LoadingSpinner.vue:49-53`; `@keyframes skeleton-pulse` `base.css:409-418`; `.chevron/.collapse-icon/.thumb` transforms `DashboardPage.vue:464-470`, `AppLayout.vue:349-351`, `ToggleSwitch.vue:69-74`; `ProgressBar.vue:67`; all 0.15-0.2s transitions listed in base + components. |
| 8 | Responsive breakpoints (shared set) | **violated/inconsistent** | Nine distinct widths, no shared scale: `1200` (`DashboardPage.vue:331`), `900` (`AppLayout.vue:452`, `PageHeader.vue:93`), `901` (`AppLayout.vue:353`), `700` (`base.css:496,502`; `VisitsPage.vue:416`; `UserDetailPage.vue:178`; `ServerDetailPage.vue:761`; `SettingsPage.vue:405`; `PageHeader.vue:99`; `SearchInput.vue:92`), `640` (`DashboardPage.vue:337`, `TablePaginator.vue:166`), `600` (`AppLayout.vue:506`), `560` (8 user/edit components), `480` (`base.css:516`). No `min-width: 320px` floor on `html/body` (`base.css:5-9`). |
| 9 | No scoped clones of shared primitives | **violated** | `.card` re-declared in scoped styles: `UserDetailExpiry.vue:132`, `UserDetailLimits.vue:155`, `UserDetailTraffic.vue:178` (all set `--card-padding`). `.card + .card` margin rules duplicated from `base.css:39-49` into `UserDetailPage.vue:158-176`. `.card-head` mobile stacking duplicated in 7 components (see §3). `.stat-grid`/`.stat-skeleton` duplicated in `DashboardPage.vue:325-360` and `ServersPage.vue:286-309`. |
| 10 | Numeric/tabular figures for times/numbers | **violated** | Grep `font-variant-numeric` / `tabular-nums` → **zero matches**. Time/data columns (`DataTable.vue:260-279`), stats (`StatCard.vue:80-85`), progress percent (`ProgressBar.vue:82-87`), `MetricBar.vue:32-37`, paginator page info (`TablePaginator.vue:130-133`) use proportional digits. |
| 11 | Accessibility: `role="status"`/`role="alert"` on async banners | **partial** | `role="status"` exists only in `LoadingSpinner.vue:18`. `ErrorBanner.vue:12-25` (used by every async view, incl. `topError`) has **no** `role="alert"`/`aria-live`. Table scroll region `DataTable.vue:99-102` has no `tabindex="0"` and no label (grep `tabindex` → only `ModalDialog:76`, `AppSelect:125`, `OverflowMenu:90`, `composables.ts:99`). |

---

## 3. Inconsistency findings (concrete)

### 3.1 Duplicated / cloned styling

| Pattern | Locations |
|---|---|
| `.card-head` stacks vertically at `max-width:560px` (same 3 declarations) | `UserDetailExpiry.vue:187-192`, `UserDetailLimits.vue:171-180`, `UserDetailTraffic.vue:232-245`, `UserDevices.vue:107-112`, `UserNodeAuth.vue:117-126`, `UserTrafficStats.vue:155-160`, `UserVisits.vue:130-135` |
| `.card` scoped padding override (`--card-padding: var(--spacing-md)`) | `UserDetailExpiry.vue:132-136`, `UserDetailLimits.vue:155-157`, `UserDetailTraffic.vue:178-180` |
| `.card + .card` margin override | `base.css:39-49` vs `UserDetailPage.vue:158-176` |
| Stat grid + skeleton block (identical CSS) | `DashboardPage.vue:325-360` vs `ServersPage.vue:286-309` |
| `.head-actions` flex row | `UserDetailTraffic.vue:182-186`, `UserNodeAuth.vue:100-105`, `UserVisits.vue:120-124` |
| Segmented control markup/CSS | `ui/SegmentedControl.vue:37-67` vs `ServerDetailPage.vue:677-700` (`.install-tabs`) |
| Dropdown option/menu item (padding `7px 10px`, radius sm, hover `--color-primary-soft`) | `base.css:445-467`, `AppSelect.vue:218-239`, `OverflowMenu.vue:142-158`, `MenuSearch.vue:192-209` |
| Small dot / indicator element | status dot: `NodesPage.vue:512-522`, `AppSelect.vue:254-280`, `DashboardPage.vue:375-390`, `AppLayout.vue:310-316`; chart legend swatch: `TrafficChart.vue:118-123` |
| Mono font stack literal | `base.css:326-329` (`.mono`) and `base.css:398` (`.kbd`), `ServerDetailPage.vue:726`, `NodeFormDialog.vue:1045`, `OneTimeSecret.vue:62` — 4 copies outside the `.mono` primitive |
| `--card-padding` defined once (`tokens.css:78`) but re-declared in 3 scoped blocks + overridden in `base.css:508` |

### 3.2 Radii not using the token scale

Tokens available: `--radius-sm:6px`, `--radius-md:12px`, `--radius-lg:12px`, `--radius-xl:16px`, `--radius-dialog:8px`, `--radius-full:999px`.

- Literal `4px`: `base.css:372` (`.theme-option`), `SegmentedControl.vue:50`
- Literal `6px`: `base.css:395` (`.kbd`), `DashboardPage.vue:453` (`.expand-btn`)
- Literal `2px`: `TrafficChart.vue:121` (`.dot`)
- Literal `50%`: circles — `ModalDialog.vue:173`, `ErrorBanner.vue:53`, `AppSelect.vue:258`, `AppLayout.vue:314,335`, `NodesPage.vue:516`, `LoadingSpinner.vue:29`, `ToggleSwitch.vue:66` (no circle token)
- `999px` literals instead of `--radius-full`: `StatusBadge.vue:27`, `ProgressBar.vue:54,66`, `ToggleSwitch.vue:47`, `DashboardPage.vue:368`, `NodeFormDialog.vue:953`
- `--radius-lg` (L67) and `--radius-xl` (L68) are **never referenced**.

### 3.3 Spacing / font-size literals bypassing tokens

- Fixed px padding/gap outside scale: `NodeFormDialog.vue:888` (`gap: 28px`), `:970` (`padding: 14px`), `:1003` (`padding: 20px`); `ModalDialog.vue:146,183,190` (`24px`, `12px 24px`); `LoginPage.vue:193` (`padding: 24px`); `DashboardPage.vue:366` (`7px 11px`); `SettingsPage.vue:358` (`8px 12px`); `SegmentedControl.vue:48` (`4px 12px`); `AppLayout.vue:234` (`14px`), `:428` (`0 12px`). All `--spacing-*` values exist but aren't always used.
- Font-size literals bypassing `--font-size-*`: `AppLayout.vue:256` (`17px`, brand mark), `LoginPage.vue:171` (`19px`, brand mark), `ErrorBanner.vue:46` (`16px`, close glyph), `base.css:399` (`11px`, `.kbd`), `base.css:328` (`0.92em`, `.mono`).
- `--font-size-xs` and `--font-size-sm` are both `12px` (`tokens.css:83-84`) — duplicate scale value.
- Font-weight literals cluster at 400/500/600/700 with no tokens (40 occurrences); page titles use `700` (`PageHeader.vue:75`) while the skill principle suggests `720` for titles — cosmetic difference, not a bug.

### 3.4 Breakpoint / layout inconsistencies

- Breakpoint widths listed in §2 #8 (1200/901/900/700/640/600/560/480).
- `DashboardPage.vue` fixes `.stat-grid` to 4 columns then 2 (`1200`) then 1 (`640`), while `ServersPage.vue:286-290` uses `auto-fill minmax(180px,1fr)` — two different stat-grid strategies for the same `StatCard` primitive.
- `TablePaginator.vue:166` hides the page-size selector at `640px`; `PageHeader.vue:93` changes spacing at `900px`; user panels at `560px`.
- No `min-width: 320px` floor (`base.css:5-9`).
- `DashboardPage.vue:401-408` `.traffic-header` hand-rolls `display:flex/space-between/flex-wrap` that overlaps `.card-head` (`base.css:58-63`).

### 3.5 Inline `style` usage

Only dynamic bindings, permitted by spec (`component-guidelines.md` L45): `ModalDialog.vue:77` (width), `DataTable.vue:121,168,210` (column align/width), `ProgressBar.vue:33` (fill width), `FilterChip.vue:65` / `OverflowMenu.vue:91` / `AppSelect.vue:126` (floating panel position via `panelStyle`). No static `style="` in templates.

### 3.6 `!important` and specificity hacks

- `ModalDialog.vue:199` (`width:100% !important` at ≤560px, overriding inline width)
- `NodeFormDialog.vue:923` (`color`), `:924` (`font-size`), `:955` (`color`), `:1010` (`flex`), `:1073` (`flex-basis`)

### 3.7 Unused / dead tokens

| Token | Declared | Referenced outside tokens.css |
|---|---|---|
| `--color-table-hover` | `tokens.css:55` / `:140` | none (rows use `--color-muted-soft` instead: `DataTable.vue:282,286`) |
| `--color-primary-hover` | `tokens.css:36` / `:121` | none (`.btn:hover` uses `opacity:.9`, `base.css:145-147`) |
| `--color-danger-hover` | `tokens.css:47` / `:132` | none (`.btn.danger` uses `--color-danger-strong-hover`) |
| `--radius-lg` / `--radius-xl` | `tokens.css:67-68` | none |
| `--gray-25` / `--ink-950` | `tokens.css:3,13` | none |

---

## 4. Prioritized optimization backlog

> All proposed fixes reference **existing** tokens/primitives. No new hex, no brand color introduced (palette stays neutral/ink).

### P0 — correctness / accessibility / motion

| # | Title | Evidence | Proposed fix | Effort |
|---|---|---|---|---|
| P0-1 | Add global `prefers-reduced-motion: reduce` handling | Zero `reduced-motion` matches; animations in `LoadingSpinner.vue:49-53`, `base.css:409-418`, `ProgressBar.vue:67`, `ToggleSwitch.vue:69`, `DashboardPage.vue:465`, `AppLayout.vue:230,340,350`, all `.15s`/`.2s` transitions | Add one `@media (prefers-reduced-motion: reduce)` block in `base.css` setting `*, *::before, *::after { animation-duration:.01ms !important; animation-iteration-count:1 !important; transition-duration:.01ms !important; scroll-behavior:auto !important }`; optionally kill `.chevron/.thumb` transforms | low |
| P0-2 | Announce async errors | `ErrorBanner.vue:12-25` has no `role`; used by every page | Add `role="alert"` (or `aria-live="assertive"`) to `.error-banner`; keep `v-if="message"` so it mounts on change | low |
| P0-3 | Restore focus ring on command palette input | `MenuSearch.vue:181-184` `box-shadow:none` on `:focus` | Remove the `:focus { box-shadow:none }` override (or scope it to non-focus states) so the base input ring (`base.css:231-236`) shows | low |
| P0-4 | Make wide tables keyboard-scrollable & labelled | `DataTable.vue:99-102,240-246` `.table-box` `overflow-x:auto`, no `tabindex`, no label | Add `tabindex="0"` + `role="region"` + `aria-label` prop (or derive from PageHeader title) on `.table-box`; expose optional `ariaLabel` prop | low |
| P0-5 | Add mobile width floor | `base.css:5-9` no `min-width` | Add `min-width: 320px` on `html, body` (matches skill floor; prevents sub-320 squish) | low |

### P1 — consistency / dedup

| # | Title | Evidence | Proposed fix | Effort |
|---|---|---|---|---|
| P1-1 | Centralize card-head mobile stacking | 7 identical `@media (max-width:560px) .card-head{}` blocks (see §3.1) | Move to `base.css` under the shared `700px` breakpoint: `@media (max-width:700px){ .card-head{ align-items:flex-start; flex-direction:column } }`; delete the 7 scoped copies | medium (touches 7 files) |
| P1-2 | Replace scoped `.card` clones with a compact modifier | `UserDetailExpiry.vue:132`, `UserDetailLimits.vue:155`, `UserDetailTraffic.vue:178` | Add `base.css` class `.card.compact { --card-padding: var(--spacing-md) }`; components add `compact` and drop the scoped `.card` rule | low |
| P1-3 | Dedup stat grid + skeleton | `DashboardPage.vue:325-360` vs `ServersPage.vue:286-309` | Add shared `.stat-grid`/`.stat-skeleton`/`.skeleton-line`/`.skeleton-value` to `base.css`; pages keep only their column-count media queries | low |
| P1-4 | Reuse `SegmentedControl` for install tabs | `ServerDetailPage.vue:677-700` | Replace `.install-tabs` markup with `ui/SegmentedControl.vue` (`items` + `v-model`); delete scoped CSS | low |
| P1-5 | Normalize breakpoints to a shared set | widths 1200/901/900/700/640/600/560/480 | Define shared set `900` (layout) + `700` (forms/panels) + `480` (compact); migrate `560`→`700` (or document `560` as the panel tier), `640`/`600`→`700`, keep `1200` only if grid needs it. Document in `base.css` comment | medium/high (regression risk, needs visual pass) |
| P1-6 | Unify disabled treatment | `.45` (`TablePaginator.vue:161`), `.55` (`base.css:149`, `ToggleSwitch.vue:57`), none (`AppSelect.vue:178`) | Standardize on one value (skill = `0.52`); apply a shared `:disabled { opacity:.52; cursor:not-allowed }` and make `AppSelect` use it too | low |
| P1-7 | Consolidate mono font into `.mono` | `ServerDetailPage.vue:726`, `NodeFormDialog.vue:1045`, `OneTimeSecret.vue:62`, `base.css:398` | Apply existing `.mono` / add `.mono-sm` modifier instead of re-declaring the font stack | low |
| P1-8 | Unify popover shadow/radius | `AppSelect.vue:214-215` (md + `--shadow-dialog`), `OverflowMenu.vue:138-139` (md + dialog shadow), `FilterChip.vue:118-119` (sm + dialog shadow) | Pick one panel radius (e.g. `--radius-md`) and one shadow (add/reuse `--shadow-sm` or a lighter shadow) for all three; `--shadow-dialog` reserved for `ModalDialog` | low |
| P1-9 | Replace literal radii with tokens | §3.2 | `4px`/`6px` → `--radius-sm` or a new `--radius-xs`; `999px` → `--radius-full`; `50%` stays (no circle token) | low |
| P1-10 | Replace literal font sizes | `AppLayout.vue:256`, `LoginPage.vue:171`, `ErrorBanner.vue:46`, `base.css:399` | Map to nearest `--font-size-*` or add `--font-size-brand`/`--font-size-sm` token; avoids magic numbers | low |
| P1-11 | Dedup `.head-actions` | `UserDetailTraffic.vue:182`, `UserNodeAuth.vue:100`, `UserVisits.vue:120` | Add `.card-head-actions` to `base.css`; components drop local rule | low |
| P1-12 | Dedup `UserDetailPage` card-margin rules | `UserDetailPage.vue:158-176` vs `base.css:39-49` | Keep the shared grid reset in `base.css`; remove page-level `.two-col .card + .card` duplicate, keep only `.two-col + .card` if needed | low |

### P2 — polish

| # | Title | Evidence | Proposed fix | Effort |
|---|---|---|---|---|
| P2-1 | Tabular figures for numeric/time data | zero `tabular-nums` | Add `font-variant-numeric: tabular-nums` to `DataTable` numeric cells, `StatCard .stat-value`, `ProgressBar .percent`, `MetricBar .metric-value`, paginator page info; optional `.num` utility | low |
| P2-2 | Remove dead tokens or wire them up | §3.7 | Either delete `--color-table-hover`, `--color-primary-hover`, `--color-danger-hover`, `--radius-lg/xl`, `--gray-25`, `--ink-950`, or use them (e.g. `.btn:hover:not(:disabled){ background:var(--color-primary-hover) }` instead of `opacity:.9`) | low |
| P2-3 | Review semantic color reuse | `TrafficChart.vue:129-131,143-145` success=download; `OneTimeSecret.vue:44-46` warning=secret | Keep palette; consider a dedicated chart-series token pair (neutral primary + muted) instead of success for direction; use `--color-primary-soft`/`--color-border` for secret box | medium |
| P2-4 | Consolidate status `.dot` styles | 5 implementations (§3.1) | Add a shared `.status-dot` primitive with tone modifiers and reuse | low |
| P2-5 | Remove `!important` hacks | `NodeFormDialog.vue:923,924,955,1010,1073`; `ModalDialog.vue:199` | Replace with more specific scoped selectors / a variant class; document dialog width override via CSS var | medium |
| P2-6 | Unify sidebar/theme-switch active states | `SettingsPage.vue:377`, `AppLayout.vue:297,501` use `--color-shell-active` in content | Introduce a content `--color-active` alias or reuse `--color-muted-soft`; keeps shell tokens shell-scoped | low |
| P2-7 | Normalize page title typography | page titles `700` (`PageHeader.vue:75`, `LoginPage.vue:180`) | Optional: adopt a `--font-weight-title` token per skill's weight discipline | low |

---

## 5. Unknowns / risks

1. **PRD is empty** (`.trellis/tasks/09-23-frontend-style-optimize/prd.md` all TBD). Scope/acceptance criteria need a product decision before implementation. This backlog is *raw material*, not a committed scope.
2. **`task.py current` reports "none"** for this session; the task dir exists but is not marked active. Output was written to the requested path `.trellis/tasks/09-23-frontend-style-optimize/research/`.
3. **Breakpoint normalization (P1-5) carries real regression risk** — `560px` is used deliberately by 8 panel components; collapsing to `700px` may change when two-column detail panels stack. Needs a visual pass at 320/375/560/700/desktop, light + dark.
4. **No visual test harness / screenshot suite** was found (only `build`, `typecheck`, `lint` scripts in `web/package.json`). Every CSS change must be manually verified in both themes.
5. **`.card` scoped padding overrides may be intentional** (compact two-column user panels). P1-2 must preserve the compact look; verify the two-col layout in `UserDetailPage.vue`.
6. **`color-mix()` in `LoadingSpinner.vue:27`** requires Chrome 111+/Safari 16.2+. If older browsers are supported, a fallback (`border-color: var(--color-border)`) is needed. Not a palette issue.
7. **Dark mode is `data-theme`-driven (JS), not a CSS media query** — any new "parity" check must inspect the `[data-theme='dark']` block, not add `@media (prefers-color-scheme)` (which would conflict with the manual light/dark/system toggle).
8. **Semantic-color changes (P2-3) touch product meaning** — e.g. re-coloring the TrafficChart download bar, or disabled→danger mapping in `labels.ts`, is a product decision, not pure styling.
9. **`--shadow-dialog` on popovers** (P1-8) is a visual choice; changing it affects all three overlay primitives at once — verify light/dark contrast.
10. **Keeping the neutral/ink palette is a hard constraint**: none of the proposed fixes introduce a brand blue/green. `--color-primary` stays near-black (`--ink-900`) / near-white in dark.

---

## Appendix — spec & guide cross-references

- `.trellis/spec/frontend/directory-structure.md` L44-58 (Visual System, dark-mode, palette direction, `--card-padding`), L49-50 (page-scoped CSS scope).
- `.trellis/spec/frontend/component-guidelines.md` L20 (shared primitives must not be redefined), L42-47 (styling patterns, state-feedback trio, no inline `style` except dynamic).
- `.trellis/spec/frontend/component-guidelines.md` L49-53 (accessibility expectations), L55-60 (common mistakes: copy-pasted shared CSS is a review blocker).
- `.trellis/spec/frontend/type-safety.md` L15-21 (single-owner types; not directly style-related).
- `.trellis/spec/guides/index.md` L70-79 (pre-modification "search before changing a value" rule) — relevant to P1-5/P2-2 token changes.
