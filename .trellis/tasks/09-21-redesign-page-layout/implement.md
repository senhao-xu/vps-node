# Implementation Plan

## Ordered Checklist

- [x] Re-read frontend specs and inspect the current dirty diff before editing any touched file.
- [x] Update design tokens and base primitives for the Infrastructure Console palette, typography, surfaces, controls, tables, feedback, and responsive spacing.
- [x] Redesign `AppLayout.vue` shell/navigation and verify route matching, logout, and mobile navigation behavior remain unchanged.
- [x] Redesign `LoginPage.vue` using the shared visual system without changing submit/auth logic.
- [x] Recompose `DashboardPage.vue` and its page-scoped styles using only existing dashboard data.
- [x] Apply consistent hierarchy and responsive toolbar/table treatment to `UsersPage.vue` and `ServersPage.vue`.
- [x] Apply consistent section hierarchy and responsive detail treatment to `UserDetailPage.vue`, `ServerDetailPage.vue`, and their existing child sections.
- [x] Improve `SettingsPage.vue` section structure and save-state presentation without changing validation or payload behavior.
- [x] Refine shared `DataTable.vue`, `ModalDialog.vue`, status, feedback, paginator, metric, and secret presentation where required by the new system.
- [x] Run frontend lint, typecheck, and production build.
- [x] Review desktop and mobile render behavior, including tables, dialogs, focus states, loading/error/empty states, and long settings content through code-level responsive review.
- [x] Run the full relevant test suite and inspect the final diff for unintended behavior changes.

## Validation Commands

```bash
npm run lint --prefix web
npm run typecheck --prefix web
npm run build --prefix web
go test ./...
```

## Risky Areas

- `web/src/components/AppLayout.vue`: shell breakpoints and active navigation.
- `web/src/styles/base.css` and `web/src/styles/tokens.css`: global visual regressions.
- `web/src/components/DataTable.vue` and dialog components: narrow-screen overflow and accessibility.
- `web/src/pages/ServerDetailPage.vue`: very long page with secrets, installation commands, tables, and destructive actions.

## Rollback Points

- After tokens/base CSS: verify all existing pages still render before page-specific changes.
- After shell: verify authenticated routing and mobile navigation.
- After each page group: run lint/typecheck to catch template and style regressions.
- Before completion: compare behavior-focused diff and leave unrelated dirty files untouched.
