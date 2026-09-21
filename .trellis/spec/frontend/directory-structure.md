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

- `styles/tokens.css` owns reusable color, radius, shadow, spacing, and control-size values. Shared components use semantic tokens such as `--color-primary-soft` and `--color-danger-soft` instead of repeating literal translucent colors.
- `styles/base.css` owns page containers, cards, buttons, form controls, common action groups, focus states, and shared mobile behavior. Page-scoped CSS should contain only business-specific layout or presentation.
- `AppLayout.vue`, `ModalDialog.vue`, `DataTable.vue`, and status/feedback components own their responsive behavior. A parent component must not target a child component's internal DOM with an ordinary scoped selector; move the rule to the child owner or use `:deep()` explicitly.
- Management tables remain semantic tables inside a horizontal scroll container on narrow screens. Do not globally convert them into cards or add `overflow: hidden` to a parent card.
- At narrow widths, page headers and multi-field rows collapse vertically while controls remain usable. Shared dialog footers stack their buttons in `ModalDialog.vue`, so individual forms do not duplicate that rule.
- Dialogs keep `role="dialog"`, `aria-modal="true"`, and an `aria-labelledby` relationship with the visible title.

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
