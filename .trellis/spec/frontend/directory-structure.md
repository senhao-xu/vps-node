# Directory Structure

> How frontend code is organized in this project.

---

## Overview

Vue 3 + TypeScript (strict) + Vite + vue-router + pinia. No UI framework, no chart library — plain CSS from `styles/tokens.css` + `base.css` primitives, hand-rolled SVG chart. zh-CN UI text.

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
├── stores/auth.ts  # hydration via /api/admin/me, login/logout
├── router/         # auth guard + redirect param
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
- `DataTable.vue` renders full-bleed inside a card: `.table-wrap` negates `--card-padding` on the inline axis so row borders reach the card edges, while `th/td:first-child` and `:last-child` re-apply `--card-padding` so cell text lines up with the card title. Cell padding is `14px var(--spacing-md)`. `--card-padding` lives in `tokens.css` and is overridden to `--spacing-md` on `.card` below 700px — new table-like lists should reuse this instead of inventing their own negative-margin hack.
- At narrow widths, page headers and multi-field rows collapse vertically while controls remain usable. Shared dialog footers stack their buttons in `ModalDialog.vue`, so individual forms do not duplicate that rule.
- Dialogs keep `role="dialog"`, `aria-modal="true"`, and an `aria-labelledby` relationship with the visible title.
- Palette direction is the Xboard/shadcn neutral theme: `--color-primary` is near-black (`#0F172A`, and near-white `#F8FAFC` in dark), surfaces are white/`#020817`, and the secondary fill is `#F1F5F9` / `#1E293B`. Semantic green/amber/red stay as accents — do not introduce a brand-colored primary.
- The app is not color-theme-agnostic: a token used as a background must ship a matching foreground token. `--color-primary` pairs with `--color-on-primary`; solid danger buttons use `--color-danger-strong` + `--color-on-danger`, while `--color-danger` remains the readable-on-surface text/tint variant (light `#EF4444`, dark `#ef5b5b`).
- Sidebar width is token-driven (`--shell-width: 256px`, `--shell-width-collapsed: 56px`); the collapsed state is persisted in `localStorage['sidebar-collapsed']` and only applies on desktop (`min-width: 901px`).

### Build-time version constant

The sidebar footer shows the deployed git revision. It is injected at build/dev time, never hardcoded or fetched at runtime:

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
