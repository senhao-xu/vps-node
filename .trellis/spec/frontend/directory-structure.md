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
