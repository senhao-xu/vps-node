# Implementation Plan - 数据工作台前端重构

## Current revision baseline

The B workbench palette, overview store, metric strip, dashboard/list/detail/settings/login/dialog migration, custom selects, and prior quality fixes are already implemented and have passed typecheck, lint, build, and focused visual checks. Baseline Batches 1, 3, and 4 are retained below for traceability and marked complete; the remaining work is Batch 2R, Batch 5, and Batch 6. Existing completed behavior must be preserved while executing those revision batches.

## Validation commands

Run from `web/` after every batch:

```bash
npm run typecheck
npm run lint
npm run build
```

For visual validation, run the Vite dev server and use Playwright at 320, 375, 560, 700, 1024, and 1440px in light and dark themes. Check `scrollWidth === clientWidth` at the page level; horizontal overflow is allowed only inside labelled table regions.

## Baseline Batch 1 - Foundation and overview state (complete)

- [x] Capture current desktop/mobile screenshots for comparison before editing.
- [x] Update `web/index.html` light/dark theme metadata while preserving the pre-bundle theme script.
- [x] Retune `styles/tokens.css` to the approved B light/dark palette and remove the prior shell geometry after call sites are migrated.
- [x] Update `styles/base.css` page, panel, toolbar, control, and responsive primitives; preserve reduced-motion and focus behavior.
- [x] Add typed `stores/overview.ts` with in-flight refresh deduplication, last-good-data retention, error state, refresh timestamp, completion-scheduled polling, and visibility pause/resume.
- [x] Add business-free `components/ui/MetricStrip.vue` and verify 5/3/2/1-column layouts.
- [x] Validate typecheck, lint, and build.

## Batch 2R - Left application shell and shared primitives

- [x] Add `--shell-sidebar-width: 216px` and a main-toolbar sticky offset; remove the obsolete two-row header offset after all call sites migrate.
- [x] Rewrite `AppLayout.vue` as `216px minmax(0, 1fr)` at desktop widths with a sticky/full-height brand, vertical routes, and compact footer/status area.
- [x] Keep search, theme modes, admin identity, logout, and any required compact status rail in the content toolbar; remove horizontal route navigation.
- [x] Continue consuming the single `router/navigation.ts` source from AppLayout and MenuSearch.
- [x] Keep `AppLayout` as the only overview polling owner and preserve the existing deduplicated 30-second visibility-aware refresh lifecycle.
- [x] Preserve MenuSearch, Cmd/Ctrl+K, theme modes, admin identity, logout, version display, and active-route semantics.
- [x] Below `900px`, render navigation as an off-canvas drawer with a menu trigger, overlay click, Escape, scroll lock, route-change close, focus containment, and trigger focus restoration.
- [x] Do not add desktop collapse behavior or a persisted collapse preference; ignore obsolete sidebar localStorage values.
- [x] Preserve the already-restyled `DataTable`, paginator, status, form, select, toggle, and overflow primitives; limit shared visual changes to verified polish defects and the revised `PageHeader` contract.
- [x] Preserve `ModalDialog` and shared confirmation focus/stacking contracts while integrating the mobile drawer into the overlay model.
- [x] Validate typecheck, lint, build, keyboard navigation, drawer lifecycle, and shell screenshots at 320/375/560/700/1024/1440px.

## Baseline Batch 3 - Dashboard as the reference page (complete)

- [x] Recompose `DashboardPage.vue` around compact header, `MetricStrip`, primary user-traffic table, and attention side panel.
- [x] Use overview store stats and keep today/total switching plus row-level node expansion.
- [x] Load the complete paginated server fleet (100 per page) and active-node total through existing APIs; handle partial request failure without discarding available dashboard data.
- [x] Omit unsupported traffic delta, sustained-load duration, protocol-count, expiry, and online-presence claims from the illustrative mockup.
- [x] Add the existing create-user dialog as the dashboard primary shortcut.
- [x] Implement responsive stacking at tablet/mobile widths with no page-level overflow.
- [x] Compare the result against the selected B mockup before migrating the remaining pages.
- [x] Validate typecheck, lint, build, and light/dark screenshots.

## Baseline Batch 4 - List workspaces (complete)

- [x] Migrate `UsersPage.vue` to the compact B workbench composition; preserve search, filters, sort, selection, batch status, CRUD, and pagination.
- [x] Migrate `ServersPage.vue`; replace the stat-card wall with `MetricStrip`, consume overview store, and preserve create/edit/status/delete behavior.
- [x] Migrate `NodesPage.vue`; preserve URL-backed filters, sort, selection, copy/edit/status/delete, and pagination.
- [x] Migrate `VisitsPage.vue`; make visits the primary table and top-host data the secondary panel while preserving all filters/date ranges.
- [x] Normalize loading, empty, error, selected, disabled, and busy states across all four pages.
- [x] Validate typecheck, lint, build, and representative narrow/wide screenshots.

## Batch 5 - Details, settings, public pages, and dialogs

- [x] Upgrade `PageHeader` so detail returns are outlined icon-plus-text commands and add a `title-extra` slot for status adjacent to the resource name.
- [x] Recompose `UserDetailPage.vue` so “返回用户列表”, username, status, ID/authorization metadata, and `MetricStrip` read as one coherent identity area.
- [x] Apply the same resource-header contract and “返回服务器列表” command to `ServerDetailPage.vue`.
- [x] Preserve every user/server detail business panel, action, payload, loading flow, and API request while changing composition.
- [x] Replace SettingsPage's old top-shell sticky offset with the shared content-toolbar offset or responsive non-sticky behavior.
- [x] Polish shared title/action alignment, panel/workspace heading hierarchy, toolbar density, loading/empty/error states, long text, dark contrast, and mobile spacing across all authenticated routes.
- [x] Keep `LoginPage.vue`, `NotFoundPage.vue`, dialogs, and one-time-secret surfaces visually aligned without expanding product scope.
- [x] Remove orphaned old-shell/stat-card CSS and unused imports only after all pages have moved.
- [x] Validate typecheck, lint, build, dialog keyboard behavior, detail return focus, and representative screenshots.

## Batch 6 - Full verification and documentation

- [x] Run `npm run typecheck`, `npm run lint`, and `npm run build` from a clean final frontend state.
- [x] Start the local Vite server and exercise every route, major form dialog, filters, sorting, selection, pagination, theme modes, menu search, and mobile drawer.
- [x] Verify drawer overlay click, Escape, route-change close, scroll lock, focus containment, and trigger focus restoration.
- [x] Capture Playwright screenshots in light/dark at 320/375/560/700/1024/1440px and check for text overlap, blank surfaces, clipped overlays, or unexpected horizontal overflow.
- [x] Confirm the user/server return commands are visually prominent and keyboard-visible, and the user identity group remains coherent at wide and 320px widths.
- [x] Inspect network activity: one deduplicated `/api/dashboard` request per refresh cycle, no overlapping requests, and no polling while the tab is hidden.
- [x] Confirm no hardcoded colors were added to Vue components, no raw `fetch` exists outside `api/http.ts`, and no API types were duplicated.
- [x] Run `git diff --check` and inspect remaining old top-navigation selectors/tokens.
- [x] Update `.trellis/spec/frontend/` to record the implemented B workbench, fixed desktop sidebar, mobile drawer, and resource-header contracts.
- [x] Run the Trellis quality check and resolve all verified findings before completion.

## Risky files and review gates

- `AppLayout.vue` and shell tokens: every authenticated route depends on them. Gate Batch 2R at desktop and mobile widths before detail polishing.
- Mobile drawer and `MenuSearch`: share overlay/focus concerns; verify stacked Escape handling, scroll lock, route close, and focus restoration behavior rather than screenshots alone.
- `PageHeader.vue` / `UserDetailPage.vue`: shared API and resource composition changes affect both detail routes; gate at 320px and wide desktop in both themes.
- `SettingsPage.vue`: sticky positioning can regress under the new toolbar geometry; verify desktop scrolling and mobile non-sticky behavior.
- `ServerDetailPage.vue` and dialog surfaces remain large; avoid business-logic or request-flow refactors while polishing them.

## Rollback points

- Batch 2R: shell geometry, navigation placement, and mobile drawer only.
- Batch 5: shared resource header, detail composition, settings offset, and route polish.
- Batch 6: verification and spec documentation only.

Each batch must remain buildable so a failed visual direction can be reverted without touching backend state.

