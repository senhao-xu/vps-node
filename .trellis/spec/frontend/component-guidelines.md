# Component Guidelines

> How components are built in this project.

---

## Overview

Vue 3 SFC (`<script setup>`), TypeScript strict, no UI library. Two component tiers:

- `components/ui/` — business-free primitives built during the xboard-style overhaul (2026-09): `ToggleSwitch`, `AppSelect`, `OverflowMenu`, `FilterChip`, `SearchInput`, `LoadingSpinner`, `EmptyState`, `PageHeader`, `StatCard`, `SegmentedControl`, plus `composables.ts`.
- `components/` — app-level shared components: `DataTable`, `TablePaginator`, `ModalDialog`, `ConfirmDialog`, `StatusBadge`, `ProgressBar`, `MetricBar`, `ErrorBanner`, `CopyText`, `AppLayout`, `MenuSearch`, form dialogs, `user/*` panels.

---

## Component Structure

- SFC with `<script setup lang="ts">`; scoped styles; zh-CN UI text.
- Icons come from `lucide-vue-next` only (tree-shaken) — never inline hand-written SVG paths.
- Shared cross-component CSS primitives live in `styles/base.css` (`.chip`, `.kbd`, `.skeleton`, `.stat-skeleton`/`.skeleton-line`/`.skeleton-value`, `.menu-list`, `.toggle-row`, `.info-grid`, `.card-head`, `.card-head-actions`, `.card.compact`, `.status-dot`, `.actions`, `.table-toolbar`). Never re-define these in scoped styles — that copy-paste pattern was explicitly eliminated.
- `.card.compact` is the only way to shrink a card's padding (`--card-padding`) for dense two-column panels; do not re-declare `--card-padding` in a scoped `.card`.
- `.card-head` is globally responsive at `max-width: 560px` (stacks to a column) — do not add a per-component `.card-head` media block.
- `.card-head-actions` is the shared right-aligned action group inside `.card-head` (has `margin-left: auto`); page/panel-specific mobile overrides may still target it in scoped CSS.
- `.status-dot` (8px circle) is the shared dot primitive; use the tone modifiers `.success/.warning/.danger/.primary/.muted` instead of a local dot class.
- Async error banners must carry `role="alert"`; scrollable `DataTable` regions must expose `role="region"` + `tabindex="0"` + `aria-label` (pass `ariaLabel` for multiple tables on one page).

## Props Conventions

- Typed props via `withDefaults(defineProps<{...}>(), ...)`; `v-model` via `defineModel` or `modelValue` + `update:modelValue`.
- Generic components keep the `T extends Record<string, unknown>` + typed slots pattern (`DataTable`, `AppSelect`).
- Every page header uses `PageHeader` (`title` + `subtitle` + `#actions`); do not hand-roll `.page-header` markup. Detail pages pass `backTo`/`backTitle` for the back link (the top bar no longer renders titles or back links).
- `StatCard` renders label + top-right icon + big value + optional `hint` line; `SegmentedControl` (`items` + `v-model`, generic over `T extends string`) is the only segmented/toggle-group control — do not hand-roll range switches in pages.

## DataTable v2 contract

- `rowKey: (row: T) => string | number` is **required** — index keys are forbidden (they glitch under polling refresh).
- Optional: `selectable` + `v-model:selected`, `sortKey`/`sortDir` + `@sort` (sorting logic stays in the page), `row-extra` slot + `row-click` (dashboard drill-down), `bordered` (default `true` = table sits in its own `.table-box` rounded border; pass `false` when the table is already inside a `.card` to avoid a double border).
- There is **no `#toolbar` slot**: list pages render a `.table-toolbar` row (with `SearchInput` + `FilterChip`) above the table, outside any card (shadcn-admin layout). `SearchInput` owns the 300ms debounce and emits `search`; do not add a second debounce/watch in the page.
- Loading renders skeleton rows; empty renders `EmptyState`; both are built in — pages must not render their own "加载中…" text.

## Overlays (selects, menus, dialogs)

- Floating panels (AppSelect, OverflowMenu, FilterChip) use the shared composables in `components/ui/composables.ts`: `useFloatingPanel` (fixed positioning + viewport flip/clamp), `useClickOutside`, `useFocusTrap`, `useScrollLock`, and the dialog stack (`pushDialog`/`isTopDialog`) so Esc closes only the top-most dialog.
- All overlays must support: outside-click close, Esc close, keyboard navigation (↑↓/Enter/Space).
- `ModalDialog`: title id via `useId()` (never a hard-coded id — pages mount multiple dialogs), focus trap + scroll-lock on mount, focus restore on close, right-aligned `#footer`.

## Styling Patterns

- Semantic tokens only (see directory-structure.md Visual System); every new token ships a `[data-theme='dark']` override.
- No inline `style=""` except dynamic bindings (widths, positions).
- `.card + .card` adds a stacked-card `margin-top` (base.css). Cards laid out **side by side** in a grid must opt out: `.stat-grid > .card + .card` and `.two-col > .card + .card` are reset in base.css. A new card grid needs the same reset, otherwise the later cards are pushed down inside their cell and the row looks misaligned (and the first card stretches taller).
- State feedback trio is mandatory on every data view: loading → `LoadingSpinner`/skeleton, empty → `EmptyState`, error → `ErrorBanner`. Ad-hoc `text-danger` blocks are forbidden.

## Accessibility

- Dialogs: `role="dialog"`, `aria-modal="true"`, `aria-labelledby` pointing at the visible title.
- Toggles: `role="switch"` with keyboard support (native `<button>`).
- Selects/menus: `aria-haspopup="listbox"`, `role="option"`, visible focus state via `--color-focus-ring`.

## Common Mistakes

- Hand-rolling a table instead of `DataTable` (DashboardPage pre-overhaul; ~130 lines of duplicated CSS deleted).
- Copy-pasting `.eyebrow`/`.toolbar-label`/`.card-head` into pages — these are shared now; duplication is a review blocker.
- `window.confirm` for destructive actions — always `ConfirmDialog`.
- Button loading as text swap ("保存中…") — use the spinner + `is-loading` class.
