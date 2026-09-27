# Research: Codebase UI audit for the B data workbench redesign

- Query: Audit the current Vue frontend against the selected B "data workbench" concept, including exact affected files, API feasibility, shared overview polling, compatibility/accessibility/responsive risk, and implementation order.
- Scope: internal
- Date: 2026-09-23

## Findings

### 1. Selected concept and current baseline

The selected prototype is a structural redesign, not a skin. Its defining elements are a 58px horizontal application header, a 34px global status row, a five-column border-separated metric strip, a primary table workspace, and a narrow attention panel (`/mnt/c/Users/Simon/.codex/visualizations/2026/09/23/01a0cd76-1d2c-7633-b344-35ebbcbece9f/vps-node-concept-b.html:31`, `:57`, `:68`, `:82`). It uses teal `#0f766e`, neutral green-gray surfaces, small radii, low elevation, and dense rows (`vps-node-concept-b.html:2-20`, `:83-95`). Its reference reflow is horizontal navigation at <=820px and 3/2-column metrics at <=820/520px (`vps-node-concept-b.html:133-160`).

The repository currently implements a different TDesign-blue system. `tokens.css` identifies itself as TDesign, maps primary to `#0052d9`, uses 3/4/6px radii, and has a 232px sidebar token (`web/src/styles/tokens.css:1-8`, `:29-39`, `:102-132`). The frontend spec still describes the older Xboard/shadcn near-black primary and 256px sidebar (`.trellis/spec/frontend/directory-structure.md:44-59`). Therefore the current code, current spec, and selected concept disagree. The B task must be treated as the new visual source of truth, followed by a spec update after the implementation is verified.

### 2. Files found and ownership boundaries

#### Application and visual foundation

| File | Current responsibility | B-redesign boundary |
|---|---|---|
| `web/index.html` | Pre-bundle theme application and browser `theme-color` metadata | Update light/dark meta colors with the new palette; preserve the FOUC-prevention script (`web/index.html:8-23`). |
| `web/src/main.ts` | Pinia/router installation, initial theme application, global 401 redirect | No structural change required; do not move polling here because it outlives the authenticated layout (`web/src/main.ts:10-23`). |
| `web/src/App.vue` | Public-versus-authenticated shell switch | No change expected (`web/src/App.vue:6-14`). |
| `web/src/styles/tokens.css` | Only owner of primitive palette, semantic colors, radii, spacing, typography, shadows, shell dimensions | Must own every B palette/geometry value and matching dark override; remove sidebar-only tokens only after all consumers are migrated. |
| `web/src/styles/base.css` | Page/card/control/form/toolbar/shared responsive primitives and reduced-motion rule | Must own compact page rhythm, panel/toolbar/table alignment, dense controls, metric/workspace helpers, and page-level overflow prevention. Preserve the 320px floor and reduced-motion block (`web/src/styles/base.css:5-20`, `:27-80`, `:331-337`, `:598-607`). |
| `web/src/router/index.ts` | All route URLs, lazy page loading, auth guard, document titles | Preserve URLs and guard. Routes already cover Login, Dashboard, Users, UserDetail, Servers, Nodes, ServerDetail, Visits, Settings, and 404 (`web/src/router/index.ts:13-90`). |

#### Shell and global state

| File | Current responsibility | B-redesign boundary |
|---|---|---|
| `web/src/components/AppLayout.vue` | 232/56px sidebar, mobile nav, menu search trigger, theme, identity/logout, version, Cmd/Ctrl+K | Full shell rewrite. Remove live sidebar/collapse behavior, but preserve active-route matching, search shortcut, theme modes, auth actions, and `__APP_VERSION__` (`web/src/components/AppLayout.vue:27-82`, `:85-211`). |
| `web/src/components/MenuSearch.vue` | Static route command palette | Keep behavior and overlay; its route list duplicates AppLayout's list (`web/src/components/MenuSearch.vue:23-37` versus `AppLayout.vue:38-52`). Prefer a single `web/src/router/navigation.ts` route-navigation definition consumed by both. |
| `web/src/stores/auth.ts` | Admin hydration/login/logout with in-flight hydration dedupe | Preserve public contract (`web/src/stores/auth.ts:6-52`). |
| `web/src/stores/theme.ts` | `light | dark | system`, persistence, `matchMedia` response | Preserve public contract (`web/src/stores/theme.ts:4-47`). |
| `web/src/stores/overview.ts` (new) | None today | Recommended single owner of `/api/dashboard` state, refresh timestamp, error/staleness, and in-flight request dedupe. |
| `web/src/components/ui/MetricStrip.vue` (new) | None today | Recommended business-free owner of border-separated compact metrics; do not implement it as repeated cards. |

`__APP_VERSION__` is an environment-provided value or Git short revision, not necessarily a semantic product version (`web/vite.config.ts:8-22`). The status row should label it "build" unless deployment guarantees `APP_VERSION` is a release version.

#### Shared components

The following are behavioral owners and should retain their public APIs while their density and appearance change:

- `web/src/components/DataTable.vue`: typed semantic table, stable row keys, loading/empty states, selection/sort, labelled focusable horizontal scroll (`web/src/components/DataTable.vue:15-54`, `:101-246`). Its `min-width: 760px` is intentionally isolated by `.table-box { overflow-x:auto }` (`:248-270`).
- `web/src/components/TablePaginator.vue`: page-size and first/previous/next/last navigation (`web/src/components/TablePaginator.vue:5-31`, `:33-96`).
- `web/src/components/ModalDialog.vue` and `ConfirmDialog.vue`: dialog semantics, focus trap, scroll lock, Escape stack, focus restore, responsive footer (`web/src/components/ModalDialog.vue:29-60`, `:63-115`, `:193-216`).
- `web/src/components/ErrorBanner.vue`, `StatusBadge.vue`, `ProgressBar.vue`, `MetricBar.vue`, `CopyText.vue`, `OneTimeSecret.vue`, `NodeChecklist.vue`, and `TrafficChart.vue`: shared feedback/status/data presentation. Their behavior should not be reimplemented in pages.
- `web/src/components/ui/AppSelect.vue`, `FilterChip.vue`, `OverflowMenu.vue`, `SearchInput.vue`, `SegmentedControl.vue`, `ToggleSwitch.vue`, `EmptyState.vue`, and `LoadingSpinner.vue`: shared controls/states. `SearchInput` already owns its 300ms debounce (`web/src/components/ui/SearchInput.vue:20-53`); pages must not add another debounce.
- `web/src/components/ui/composables.ts`: floating positioning, outside click, focus trapping, scroll lock, and dialog stack. Preserve these behaviors while restyling (`web/src/components/ui/composables.ts:3-72`, `:75-167`).
- `web/src/components/ui/PageHeader.vue`: route page heading/back/action contract. It can be made denser without changing props (`web/src/components/ui/PageHeader.vue:5-47`).
- `web/src/components/ui/StatCard.vue`: currently used by Dashboard and Servers. It becomes unused if both move to `MetricStrip`; remove it only after an `rg` confirms no callers, otherwise leave it as a supported legacy primitive rather than silently changing its contract.

#### Routed pages and business components

All routed pages are in visual scope, but business request flows must remain page-owned:

- `web/src/pages/DashboardPage.vue`: replace six equal StatCards and card-wrapped traffic table with the B metric/workspace/attention composition. Move overview data to the global store; keep traffic range and expansion behavior (`web/src/pages/DashboardPage.vue:22-117`, `:120-321`).
- `web/src/pages/UsersPage.vue`: keep API filters, local page sort, selection/batch actions, CRUD, and paginator (`web/src/pages/UsersPage.vue:24-157`, `:180-225`, `:228-434`).
- `web/src/pages/ServersPage.vue`: remove the five-card grid. It currently makes a duplicate dashboard request, and `hotCount`/`nodeTotal` only cover the current server page, not the fleet (`web/src/pages/ServersPage.vue:47-57`, `:71-95`, `:133-206`). Consume the overview store only for exact overview fields.
- `web/src/pages/NodesPage.vue`: preserve URL-backed filters and page restoration, selection/batch actions, copy/edit/status/delete flows (`web/src/pages/NodesPage.vue:142-199`, `:201-274`).
- `web/src/pages/VisitsPage.vue`: preserve its multi-source filters and two datasets; switch composition so visit details are primary and top hosts secondary (`web/src/pages/VisitsPage.vue:19-92`, `:117-233`, `:236-395`).
- `web/src/pages/UserDetailPage.vue`: preserve the four-request core load and child ownership (`web/src/pages/UserDetailPage.vue:39-83`, `:86-148`). Visual changes span all `web/src/components/user/*.vue` files because these children own editing, expiry, limits, traffic, devices, node authorization, charts, and visits.
- `web/src/pages/ServerDetailPage.vue`: preserve server mutation/token/install/node/visit flows. It is the highest-risk page because it combines secrets, install commands, resource meters, two tables, pagination, and destructive actions (`web/src/pages/ServerDetailPage.vue:30-131`, `:140-268`).
- `web/src/pages/SettingsPage.vue`: preserve validation/payload and section navigation. Its sticky `top: 88px` assumes the old 64px header and will be wrong under a two-row shell unless replaced with a shared header-offset token or removed (`web/src/pages/SettingsPage.vue:339-352`).
- `web/src/pages/LoginPage.vue` and `NotFoundPage.vue`: public/error surfaces need the B palette/brand/control language while retaining existing auth and routing behavior (`web/src/pages/LoginPage.vue:28-45`, `:48-130`; `web/src/pages/NotFoundPage.vue:7-23`).
- `web/src/components/UserCreateDialog.vue`, `ServerFormDialog.vue`, and `NodeFormDialog.vue`: keep API payload and one-time-secret flows. Most shell appearance should come from `ModalDialog` and base controls; only business-specific layout belongs in these files.

No backend, database, agent, `api/types.ts`, auth contract, or route URL change is required. `api/types.ts` remains the only DTO owner as required by `.trellis/spec/frontend/directory-structure.md:18-31` and `.trellis/spec/frontend/type-safety.md`.

### 3. Existing API capability versus the B mockup

| B surface | Existing source | Feasibility and exact limitation |
|---|---|---|
| Panel reachable/degraded | Success/failure of `GET /api/dashboard` | Supported as frontend reachability. Do not show healthy before a successful request, and retain last-good data as stale after transient errors. |
| Servers online/total | `Dashboard.servers_online/servers_total` | Exact and cheap (`web/src/api/types.ts:152-159`; backend liveness is heartbeat-based at `internal/web/dashboard.go:31-37`). |
| Last synchronization | No global sync timestamp endpoint | Use wording such as "status refreshed" from the overview store's last successful fetch. Calling it agent synchronization would be false. Exact most-recent agent heartbeat requires reading all server pages because server list order is ID, not heartbeat. |
| Build/version | `__APP_VERSION__` | Supported, with the build-versus-release caveat above. |
| User/online/device/today traffic metrics | `GET /api/dashboard` | Supported exactly (`web/src/api/dashboard.ts:4-5`). |
| Active node total | `GET /api/nodes?status=active&page=1&page_size=1`, read `total` | Supported exactly without fetching node rows (`web/src/api/nodes.ts:13-33`). |
| Protocol count hint | No aggregate endpoint | Not directly available. It would require four count requests or a full node scan; omit the hint rather than fabricate it. |
| User traffic primary table | `GET /api/dashboard/user-traffic?range=today|total` | Supported, including status, quota totals, and node expansion (`web/src/api/types.ts:161-187`). The response is unpaginated and contains all users; search/filter/pagination can only be client-side without a backend change. |
| User expiry/online label in prototype table | Not included in `DashboardUserTrafficItem` | Account status is available, but expiry and online presence are not. Keep the real status/range/quota columns instead of copying the prototype literally. |
| Offline/high-load attention list | `GET /api/servers` server DTO | Supported as current snapshots: effective status, CPU, memory, disk, last heartbeat, node count, and online users (`web/src/api/types.ts:124-137`; `internal/web/servers.go:17-52`). |
| Fleet-wide attention list | Paginated server endpoint | A single `page_size=100` request is incomplete when total >100; list endpoints cap page size at 100 (`docs/api-contract.md:44-49`). Either fetch all pages on Dashboard or explicitly define the panel as a first-100 snapshot. The former is accurate and route-local but increases polling cost. |
| "CPU continuously high for 12 min" | Only latest server metrics | Unsupported. Show "current CPU/memory/disk X%"; no duration or trend claim is valid. |
| Traffic delta versus yesterday | Dashboard contains only today's total | Unsupported. Remove the prototype's `+12.6%` hint unless a backend time-comparison endpoint is later added. |
| Visit filters | Existing visits/users/servers/nodes endpoints | Supported, but option preload is capped to the first 100 users/servers/nodes (`web/src/pages/VisitsPage.vue:193-225`). This existing scalability limit should not be worsened or mistaken for complete option coverage. |

The backend list contract defaults to 20 and caps at 100 (`docs/api-contract.md:44-49`). Dashboard user traffic intentionally returns every user and sorts by traffic (`internal/web/dashboard.go:76-147`), so polling cost grows with all users and their node details.

### 4. Recommended global overview/polling architecture

Add one Pinia store, `stores/overview.ts`, and make it the sole caller of `getDashboard()` in authenticated UI code.

```text
AppLayout mount
  -> overview.start/refresh (30s self-scheduling poll)
  -> System status row reads stats/error/lastSuccessAt/build
  -> RouterView
       DashboardPage reads the same overview stats
         + page-local user-traffic poll
         + page-local active-node count
         + page-local all-server attention snapshot
       ServersPage reads the same overview stats
```

Required store semantics:

1. `refresh()` holds a private `Promise<Dashboard> | null`; concurrent callers receive the same promise. This prevents AppLayout, Dashboard, Servers, and post-mutation invalidation from duplicating `/api/dashboard`.
2. Keep `stats` after a transient failure, record `error`, and derive `stale/unavailable` separately. `lastSuccessAt` changes only on success.
3. Use a completion-scheduled `setTimeout`, not a blind `setInterval`, so a slow request cannot overlap its next tick. The current Dashboard uses `setInterval` and can overlap (`web/src/pages/DashboardPage.vue:108-117`).
4. Pause while `document.hidden`, refresh immediately when visible again, and clean up timer/listener when AppLayout unmounts/logout redirects.
5. Do not put user traffic or the full server fleet in the always-mounted global store. They are heavier and only needed on Dashboard. Route-local loaders should use request sequence IDs (or AbortSignal support added centrally) so a stale today/total or route-param response cannot overwrite a newer choice.
6. After successful create/delete/status mutations that change overview counts, call the deduplicated `overview.refresh()`; page list reloads remain page-local.

The concurrently written design proposes `stores/overview.ts`, which matches this ownership model (`.trellis/tasks/09-23-frontend-data-workbench-redesign/design.md:55-73`). Its timer should incorporate points 3-4 above. The design's single `listServers({page:1,pageSize:100})` attention request (`design.md:106-113`) needs the >100-server accuracy decision noted above.

### 5. High-risk compatibility, accessibility, and responsive areas

1. **Header width at tablet sizes.** Six route links plus brand, search, three theme buttons, identity, and logout cannot be assumed to fit in one line. The route list needs `min-width:0`, a labelled focusable horizontal-scroll region, and no overlap with fixed utility actions. At 320px, brand and essential actions stay visible while route navigation moves to its own scroll row.
2. **Two-row sticky offset.** `SettingsPage` hardcodes `top:88px` (`web/src/pages/SettingsPage.vue:350-352`). A new header/status height must not be duplicated as numeric offsets in pages.
3. **Page overflow.** Tables deliberately remain at least 760px wide (`web/src/components/DataTable.vue:266-271`). Never add `overflow:hidden` to a containing panel. Validate `document.documentElement.scrollWidth === clientWidth`; only labelled nav/table regions may scroll.
4. **Multiple table labels.** `DataTable` defaults every region to "data table" (`web/src/components/DataTable.vue:28-40`). Dashboard, Visits, ServerDetail, and UserDetail child tables must pass distinct `ariaLabel` values.
5. **Command palette focus.** `MenuSearch` is marked as a modal dialog and locks scrolling but does not use the shared focus trap or restore trigger focus (`web/src/components/MenuSearch.vue:44-65`, `:94-147`). Shell work should close this existing gap rather than regress keyboard behavior.
6. **Overlay stacking.** AppSelect/FilterChip/OverflowMenu use Teleport and fixed positioning. New sticky header/status z-indexes must remain below overlay/dialog layers, and mobile horizontal scroll containers must not clip panels (`web/src/components/ui/composables.ts:23-72`).
7. **Dark token parity and contrast.** Every semantic background needs a paired foreground in both themes. The teal primary, focus ring, status soft backgrounds, selected rows, browser theme-color, and disabled states require light/dark contrast review. Components must not reference the concept's raw hex values directly.
8. **Long operational strings.** Usernames, hosts, node addresses, install commands, and build revisions need explicit `min-width:0`, wrap, or ellipsis. This is especially risky in the top header, attention panel, ServerDetail install block, and 320px dialogs.
9. **Large forms.** `NodeFormDialog.vue` is 1112 lines and contains protocol-specific conditional sections. Restyle its shell and spacing without refactoring validation/payload logic. `ModalDialog` already stacks full-width footer actions below 560px (`web/src/components/ModalDialog.vue:193-216`).
10. **No frontend automated test harness.** `web/package.json` exposes only dev/build/preview/typecheck/lint (`web/package.json:6-11`). Visual, focus, and polling-network verification must be explicit; prior task notes mention Playwright screenshots but no harness is committed.

### 6. Recommended phased implementation and validation order

1. **Baseline and contract lock:** capture current light/dark desktop/mobile screenshots; record the exact B reference. Resolve the known frontend-spec visual drift in the task design, but update specs only after verified implementation.
2. **Foundation:** change `index.html`, `tokens.css`, and `base.css`; add `MetricStrip.vue` and `overview.ts`. Validate all existing pages before structural changes with `npm run typecheck`, `npm run lint`, and `npm run build`.
3. **Shell:** rewrite AppLayout, centralize navigation data, connect honest overview status, and preserve search/theme/auth/version. Gate at 320/375/560/700/820/900/1024/1440px before page migration.
4. **Shared primitives:** retune PageHeader, DataTable, paginator, badges, progress, controls, overlays, and dialogs without API changes. Keyboard-check focus ring, Tab order, arrow/Enter/Escape behavior, modal focus restoration, and labelled scroll regions.
5. **Dashboard reference page:** implement metric strip, client-side traffic workspace, active-node total, and accurate server attention loading. Validate partial failures independently and inspect network traffic to confirm one deduplicated `/api/dashboard` request per refresh cycle with no hidden-tab polling.
6. **List workspaces:** migrate Users, Servers, Nodes, then Visits. Preserve each page's current URL/search/filter/sort/selection/batch/pagination behavior. Remove Servers' page-local `getDashboard()` and page-slice-only summary claims.
7. **Details and forms:** migrate UserDetail plus all `components/user/*`, then ServerDetail, Settings, Login/404, and the three form dialogs. Keep the large NodeForm and ServerDetail business logic untouched except where layout wiring requires it.
8. **Final validation:** run `cd web && npm run typecheck && npm run lint && npm run build`; exercise every route, major dialog, theme, filter, sort, selection, pagination, search palette, logout, and 401 redirect. Capture light/dark screenshots at required widths, check page-level scroll width, test reduced motion, and verify no raw fetch/type duplication/hardcoded component colors. A frontend-only change does not require backend contract edits; `go test ./...` is a useful final integration smoke check if the embedded build path is exercised.

### 7. Prior Trellis frontend work

- `.trellis/tasks/archive/2026-09/09-21-frontend-redesign/`: introduced the semantic token/dark-theme architecture and `stores/theme.ts`; it established that visual changes should not alter API/data flow (`design.md:3-20`, `:41-57`).
- `.trellis/tasks/09-21-redesign-page-layout/`: adopted an Enterprise-blue operational layout and identified global CSS/AppLayout/DataTable/ServerDetail as the highest-risk owners (`design.md:9-43`; `implement.md:27-38`).
- `.trellis/tasks/09-21-frontend-ui-refresh/`: introduced current UI primitives, DataTable v2, MenuSearch, Lucide, overlay composables, and the list/detail page conventions (`implement.md:15-72`; `research/frontend-survey.md:16-45`).
- `.trellis/tasks/09-21-xboard-style-revamp/` and `.trellis/tasks/09-22-xboard-visual-parity-v2/`: established the Xboard/shadcn neutral system and current sidebar/page patterns. The latter documents that previous palette/local adjustments did not satisfy the user (`prd.md:12-20`) and implemented the current shared component architecture (`implement.md:9-29`).
- `.trellis/tasks/09-23-frontend-style-optimize/`: added reduced motion, labelled/focusable table regions, alert roles, shared CSS primitives, and density/accessibility cleanup (`design.md:22-57`). Those behavioral improvements remain valid under B.

All six non-archived historical frontend task records still have `status: in_progress` and `commit: null`; the archived frontend redesign also records `commit: null`. Because this research role forbids Git operations, exact commit attribution was not resolved. Treat task documents as design history and current source as authoritative.

## External References

- Runtime/package versions are repository-pinned: Vue `^3.5.13`, Pinia `^3.0.3`, Vue Router `^4.5.0`, Vite `^7.1.7`, TypeScript `^5.9.2`, and `lucide-vue-next` `^1.0.0` (`web/package.json:13-28`).
- Prior Xboard research references cedar2025/Xboard, cedar2025/xboard-admin-dist, and satnaing/shadcn-admin in `.trellis/tasks/09-21-xboard-style-revamp/research/xboard-admin-style.md:130-140`. These are historical references only; the user-selected local B prototype is the governing design for this task.
- No external network research was required for this audit.

## Related Specs

- `.trellis/spec/frontend/index.md`
- `.trellis/spec/frontend/directory-structure.md`
- `.trellis/spec/frontend/component-guidelines.md`
- `.trellis/spec/frontend/type-safety.md`
- `.trellis/spec/frontend/state-management.md` (placeholder; no project-specific polling/cache guidance yet)
- `.trellis/spec/frontend/hook-guidelines.md` (placeholder)
- `.trellis/spec/frontend/quality-guidelines.md` (placeholder)
- `.trellis/spec/guides/code-reuse-thinking-guide.md`
- `.trellis/spec/guides/cross-layer-thinking-guide.md`

## Caveats / Not Found

- The prototype's traffic comparison and sustained-load duration are not backed by current APIs. They must be removed or reworded, not simulated.
- The prototype says "recent sync" but there is no global synchronization timestamp. Last successful overview refresh is the only zero-extra-request truthful value.
- A one-page server attention fetch is not fleet-complete above 100 servers. Exactness requires all pages or a future backend aggregate/filter endpoint, which is out of scope.
- Dashboard user traffic is all-user and unpaginated; dense client-side search/pagination improves usability but not payload cost.
- Visits and several dialogs preload only the first 100 related records, an existing limitation outside the visual redesign unless explicitly brought into scope.
- Exact historical Git commits are not recorded in Trellis task metadata and were not inspected because Git operations are forbidden for this research role.
