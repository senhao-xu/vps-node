# Component Guidelines

> How components are built in this project.

---

## Overview

Vue 3 SFC (`<script setup>`), TypeScript strict, no UI library. Two component tiers:

- `components/ui/` — business-free primitives built during the xboard-style overhaul (2026-09): `ToggleSwitch`, `AppSelect`, `OverflowMenu`, `FilterChip`, `LoadingSpinner`, `EmptyState`, `PageHeader`, `StatCard`, plus `composables.ts`.
- `components/` — app-level shared components: `DataTable`, `TablePaginator`, `ModalDialog`, `ConfirmDialog`, `StatusBadge`, `ProgressBar`, `MetricBar`, `ErrorBanner`, `CopyText`, `AppLayout`, `MenuSearch`, form dialogs, `user/*` panels.

---

## Component Structure

- SFC with `<script setup lang="ts">`; scoped styles; zh-CN UI text.
- Icons come from `lucide-vue-next` only (tree-shaken) — never inline hand-written SVG paths.
- Shared cross-component CSS primitives live in `styles/base.css` (`.chip`, `.kbd`, `.skeleton`, `.menu-list`, `.toggle-row`, `.info-grid`, `.card-head`, `.actions`). Never re-define these in scoped styles — that copy-paste pattern was explicitly eliminated.

## Props Conventions

- Typed props via `withDefaults(defineProps<{...}>(), ...)`; `v-model` via `defineModel` or `modelValue` + `update:modelValue`.
- Generic components keep the `T extends Record<string, unknown>` + typed slots pattern (`DataTable`, `AppSelect`).
- Every page header uses `PageHeader` (`title` + `subtitle` + `#actions`); do not hand-roll `.page-header` markup.

## DataTable v2 contract

- `rowKey: (row: T) => string | number` is **required** — index keys are forbidden (they glitch under polling refresh).
- Optional: `selectable` + `v-model:selected`, `sortKey`/`sortDir` + `@sort` (sorting logic stays in the page), `#toolbar` slot, `row-extra` slot + `row-click` (dashboard drill-down).
- Loading renders skeleton rows; empty renders `EmptyState`; both are built in — pages must not render their own "加载中…" text.

## Overlays (selects, menus, dialogs)

- Floating panels (AppSelect, OverflowMenu, FilterChip) use the shared composables in `components/ui/composables.ts`: `useFloatingPanel` (fixed positioning + viewport flip/clamp), `useClickOutside`, `useFocusTrap`, `useScrollLock`, and the dialog stack (`pushDialog`/`isTopDialog`) so Esc closes only the top-most dialog.
- All overlays must support: outside-click close, Esc close, keyboard navigation (↑↓/Enter/Space).
- `ModalDialog`: title id via `useId()` (never a hard-coded id — pages mount multiple dialogs), focus trap + scroll-lock on mount, focus restore on close, right-aligned `#footer`.

## Styling Patterns

- Semantic tokens only (see directory-structure.md Visual System); every new token ships a `[data-theme='dark']` override.
- No inline `style=""` except dynamic bindings (widths, positions).
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
