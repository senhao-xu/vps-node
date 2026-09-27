# Directory Structure

> How frontend code is organized in this project.

---

## Overview

Vue 3 + TypeScript (strict) + Vite + vue-router + pinia. No UI framework, no chart library — plain CSS from `styles/tokens.css` + `base.css` primitives, hand-rolled SVG chart. zh-CN UI text. `qrcode` is the only rendering dependency (subscription QR codes via `toDataURL`).

---

## Directory Layout

```
web/src/
├── api/            # SINGLE OWNER of backend types + decoding
│   ├── types.ts    # every contract object (mirror of docs/api-contract.md)
│   ├── http.ts     # typed fetch wrapper: credentials:'include', error-envelope decode,
│   │               #   ApiError type guards, 401 → login redirect hook
│   └── auth|users|servers|nodes|dashboard|settings.ts  # typed endpoint functions
├── components/     # DataTable (typed generic), paginator, badges, dialogs,
│   │               # OneTimeSecret (secrets shown exactly once), NodeChecklist,
│   │               # TrafficChart (SVG), user/ detail sections
├── pages/          # Login, Dashboard, Users, UserDetail (core page), Servers,
│   │               # ServerDetail, Settings
├── stores/         # auth hydration, theme mode, shared dashboard overview polling
├── router/         # auth guard + redirect param; navigation.ts is the nav source of truth
├── styles/         # tokens.css (design tokens), base.css (button/input/table primitives)
└── utils/          # format (bytes/dates/durations), labels (status/protocol zh-CN),
                    # clipboard, names — shared, never duplicated in pages
```

---

## Conventions

- Pages never hand-cast JSON: they call `api/*` functions and receive typed data.
- Node names in user session rows are resolved client-side from `GET /api/nodes` (rows carry `node_id` only, per contract).
- Vite dev proxy: `/api` → `PANEL_API_TARGET` (default `http://127.0.0.1:8080`).
- Scripts: `dev | build | typecheck (vue-tsc --noEmit) | lint (eslint flat)`. All must pass before reporting done.
- `web/` has a nested `go.mod` barrier so root `go ./...` never traverses `node_modules` — do not remove it.

## Visual System

- `styles/tokens.css` owns a two-layer token system: primitive palette values (e.g. `--gray-*`, brand/semantic scales) and semantic tokens (`--color-bg`, `--color-surface`, `--color-text`, `--color-border`, `--color-primary`, soft/border variants, `--shadow-*`). Components MUST reference semantic tokens only — never primitives, never literal hex/rgba in `.vue` files.
- Dark mode: `:root` defines the light theme; `[data-theme='dark']` overrides every semantic color/shadow token (the two sets must stay in sync — adding a token means adding its dark override). `color-scheme` is set per theme so native controls and scrollbars follow.
- Theme switching lives in `stores/theme.ts` (`light | dark | system`, persisted to localStorage key `vps-node-theme`, `matchMedia` watcher for system mode). `main.ts` applies the resolved theme before mount, and `index.html` has an inline script that sets `data-theme` pre-bundle to prevent FOUC. The toggle button lives in `AppLayout.vue` topbar.
- `styles/base.css` owns page containers, cards, buttons, form controls, common action groups, focus states, and shared mobile behavior. Page-scoped CSS should contain only business-specific layout or presentation.
- `AppLayout.vue`, `ModalDialog.vue`, `DataTable.vue`, and status/feedback components own their responsive behavior. A parent component must not target a child component's internal DOM with an ordinary scoped selector; move the rule to the child owner or use `:deep()` explicitly.
- Management tables remain semantic tables inside a horizontal scroll container on narrow screens. Do not globally convert them into cards or add `overflow: hidden` to a parent card.
- `DataTable.vue` owns its labelled, focusable horizontal-scroll region. It uses a bordered container by default and a borderless `bordered=false` mode when embedded in an existing work panel; pages must not add negative margins or clip the table wrapper. Dense cell spacing and the table minimum width remain component-owned.
- At narrow widths, page headers and multi-field rows collapse vertically while controls remain usable. Shared dialog footers stack their buttons in `ModalDialog.vue`, so individual forms do not duplicate that rule.
- Dialogs keep `role="dialog"`, `aria-modal="true"`, and an `aria-labelledby` relationship with the visible title.
- Palette direction is the neutral HHUB-style theme: light surfaces are neutral white/gray and `--color-primary` is near-black ink (`#18181b`, paired with `--color-on-primary` white); dark surfaces are neutral dark gray with a near-white (`#fafafa`) action color. Accent color lives only in status/badge tokens (green/amber/red status, `--color-badge-purple*` for protocol badges). Keep these values in `tokens.css`; Vue components consume only semantic tokens.
- The app is not color-theme-agnostic: a token used as a background must ship a matching foreground token. `--color-primary` pairs with `--color-on-primary`; solid danger buttons use `--color-danger-strong` + `--color-on-danger`, while `--color-danger` remains the readable-on-surface text/tint variant (light `#EF4444`, dark `#ef5b5b`).
- `AppLayout.vue` owns the authenticated shell. At `>=900px` it is a `--shell-sidebar-width: 216px` fixed-width navigation column plus `minmax(0, 1fr)` content; the sidebar stays full-height and the content toolbar stays sticky. Do not add desktop collapse behavior or persist a collapse preference. Sticky page content uses `--shell-toolbar-offset`.
- Below `900px`, the same sidebar becomes an off-canvas dialog drawer. It must close on overlay click, Escape, and route change; lock document scrolling; trap focus while open; restore trigger focus on close; and participate in the shared dialog stack so Escape only closes the top overlay. The closed drawer is inert and hidden from assistive technology.
- Navigation, panel/server state, last successful overview refresh, and the build revision remain in the sidebar; search, theme modes, administrator identity, and logout remain in the content toolbar. Preserve these responsibilities when polishing either region.
- `router/navigation.ts` is the single source for authenticated route labels, icons, and search keywords. `AppLayout` and `MenuSearch` must consume it rather than maintain parallel arrays.
- Radius scale is `--radius-xs: 4px` / `sm: 6px` / `md: 10px` / `lg: 14px` / `dialog: 18px` / `full: 999px`; use tokens instead of literal component radii. Cards, summary panels, and `MetricStrip` use `--radius-lg`; badges/chips use `--radius-full`.
- Motion and viewport floors live in `base.css`: a global `@media (prefers-reduced-motion: reduce)` block neutralizes animation/transition/scroll behavior, and `html, body { min-width: 320px }` is the minimum supported width. Add new motion or sub-320 layouts with those constraints in mind.
- Disabled controls share one opacity (`0.52`) across `.btn`, `.toggle`, `.page-btn`, and `.app-select`; floating overlay panels (AppSelect/OverflowMenu/FilterChip) all use `--radius-md` + `--shadow-dialog`.

### Shared overview and build revision

`stores/overview.ts` is the only authenticated-UI owner of `getDashboard()`. Its `refresh()` deduplicates concurrent calls, preserves the last good `stats` on transient failures, and updates `lastUpdatedAt` only after success. `AppLayout` owns the completion-scheduled 30-second poll, pauses it while `document.hidden`, refreshes when visible, and stops it on unmount. Pages may consume this store and request a refresh after mutations; they must not add independent dashboard polling or a second mount-time refresh.

The status row reports frontend-observable semantics only: "状态刷新" is the last successful overview request, not an Agent synchronization time; unavailable and stale states remain distinct. A page may dismiss its own overview error banner locally, but must not clear the shared store error merely to hide UI because doing so falsely marks stale or unavailable data as healthy.

### Build-time version constant

The sidebar status area shows the deployed git revision as "构建修订". It is injected at build/dev time, never hardcoded or fetched at runtime:

```ts
// web/vite.config.ts
function appVersion(): string {
  try { return execSync('git rev-parse --short HEAD').toString().trim() || 'dev' }
  catch { return 'dev' }
}
define: { __APP_VERSION__: JSON.stringify(appVersion()) }
```

- The default export type is declared once in `web/src/vite-env.d.ts` as `declare const __APP_VERSION__: string`; reuse it instead of re-declaring.
- `execSync` is wrapped in try/catch so a build without git (or a shallow checkout) falls back to `'dev'` instead of failing.
- Adding any new global constant requires both the Vite `define` entry and an eslint `globals` entry in `web/eslint.config.js` (flat config), or lint will report `no-undef`.

```css
/* Shared state color: use the semantic token. */
.notice {
  border-color: var(--color-primary-border);
  background: var(--color-primary-soft);
}

/* Wrong: a parent scoped rule cannot reliably style child component internals. */
.dialog-footer .btn { width: 100%; }

/* Correct when the child cannot own the rule. */
:deep(.dialog-footer .btn) { width: 100%; }
```
