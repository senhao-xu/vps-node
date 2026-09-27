# Technical Design - 数据工作台前端重构

## Scope and boundaries

The implementation is confined to `web/` plus the task/spec documentation required by Trellis. Backend APIs, database contracts, routes, authentication, and agent behavior stay unchanged.

The selected B data-workbench direction remains the structural contract, with the user's latest navigation decision taking precedence over the original shell mockup:

- a fixed `216px` desktop sidebar with vertical primary navigation;
- an off-canvas navigation drawer below `900px`;
- a compact content toolbar and global status treatment without horizontal route navigation;
- compact metric strips instead of same-weight stat-card walls;
- tables as the primary work surface;
- attention/exception content as a secondary adjacent surface;
- restrained borders, small radii, low elevation, and high information density.

The current frontend spec still contains older Xboard/TDesign and top-shell decisions. The latest user-approved B revision supersedes those statements for this task. After implementation is verified, the spec must be updated to the implemented contract.

## Visual system

`web/src/styles/tokens.css` remains the only owner of palette and geometry values. Components continue to consume semantic `--color-*`, `--radius-*`, `--spacing-*`, and typography tokens.

Light theme target:

- background `#f6f7f5`;
- surface `#ffffff`;
- secondary surface `#f1f3f0`;
- text `#1d2522`;
- secondary text `#68736e`;
- border `#dde2de`;
- primary/action teal `#0f766e` with matching hover/active/soft/border/foreground tokens;
- existing green/amber/red semantics remain status colors.

Dark theme uses the same semantic roles with near-neutral dark surfaces and a lifted teal action color. Every semantic token added or changed in `:root` receives a `[data-theme='dark']` counterpart. Product UI radii remain 8px or below. Letter spacing is `0`; font sizes are token-based and never viewport-scaled.

Shell geometry is tokenized rather than duplicated in page styles:

- `--shell-sidebar-width: 216px` owns the desktop navigation width;
- a content-toolbar offset token owns sticky positioning inside the main column;
- the obsolete two-row `--shell-header-offset` contract is removed after all call sites migrate.

`base.css` will own the reusable B-layout primitives:

- page width/padding and full-width work area;
- compact panel/header treatment;
- `.metric-strip` layout support through the new component;
- `.data-workspace` and toolbar/pagination alignment;
- dense form/control rhythm;
- responsive stacking and existing reduced-motion behavior.

`web/index.html` updates only its light/dark `theme-color` metadata to match the new surface palette; the existing pre-bundle theme script remains unchanged.

## Application shell

`components/AppLayout.vue` remains the sole authenticated shell and the sole owner of overview polling. At `900px` and above it uses a two-column grid: `216px minmax(0, 1fr)`.

The desktop sidebar is full-height and sticky. It contains:

1. the brand;
2. the six vertical primary routes;
3. a compact footer/status area for Panel state, server availability, last successful overview refresh, and build revision.

The main column owns a compact top toolbar with menu search, the three-state theme control, administrator identity, and logout. If the status treatment does not fit legibly in the sidebar footer, it may remain as a compact rail at the top of the content column, but it must not restore horizontal route navigation.

Below `900px`, the sidebar becomes an off-canvas drawer:

- a visible menu button opens it and receives focus again after close;
- overlay click and Escape close it;
- opening locks background scroll;
- selecting a route closes it after navigation;
- all six routes remain available;
- the drawer follows the existing top-overlay stack and focus-management helpers.

Desktop collapse is deliberately out of scope: there is no collapse button, collapsed width, or persisted sidebar state. Any obsolete localStorage sidebar value is ignored and does not require migration.

The route-navigation definition moves to `router/navigation.ts` and is consumed by both AppLayout and `MenuSearch`, replacing their current duplicated arrays. Existing route contracts remain unchanged. AppLayout continues to own global keyboard handling, theme actions, and logout. MenuSearch adopts the existing shared focus trap and focus restoration behavior while retaining its Cmd/Ctrl+K contract.

## Shared overview data

Add `web/src/stores/overview.ts` as the single frontend owner of `/api/dashboard` overview polling.

State:

- `stats: Dashboard | null`;
- `loading: boolean`;
- `error: string`;
- `lastUpdatedAt: Date | null`;
- a private in-flight promise that deduplicates concurrent refresh requests.

Actions:

- `refresh()` fetches `getDashboard()`, records the last successful time, and preserves the last good data on a transient failure;
- AppLayout starts completion-scheduled 30-second polling on mount, pauses while `document.hidden`, refreshes when the document becomes visible, and cleans up the timeout/listener on unmount;
- Dashboard and ServersPage consume the store instead of issuing independent overview requests. Calling `refresh()` from more than one consumer is safe because the store deduplicates in-flight work.

The status bar must show unavailable/stale state honestly; it must not label the system healthy when no current overview data exists. The UI label is “状态刷新” rather than Agent “同步”, and `__APP_VERSION__` is labelled as a build revision.

## Shared component contracts

### New `components/ui/MetricStrip.vue`

Business-free compact metric strip with typed items:

```ts
type MetricStripItem = {
  key: string
  label: string
  value: string | number
  hint?: string
  icon?: LucideIcon
  tone?: 'default' | 'success' | 'warning' | 'danger'
}
```

It renders one semantic group, tabular values, subtle separators, and responsive 5 -> 3 -> 2 -> 1 column reflow. It is not a row of cards and has no per-item shadow.

### Existing primitives

- `PageHeader`: preserve current props and actions slot, promote `backTo`/`backTitle` into an outlined icon-plus-text command, and add a typed `title-extra` slot for status metadata adjacent to the resource name.
- User detail passes “返回用户列表”; server detail passes “返回服务器列表”. The return command must wrap without collision and keep a visible keyboard focus state at 320px.
- `DataTable`: keep all current props/events/slots; restyle header, row height, selection, focus ring, and borders for dense workbench use. Preserve semantic table and labelled horizontal scroll region.
- `TablePaginator`: keep its API; align it visually with the table footer and compact controls.
- `StatusBadge`, `ProgressBar`, `SegmentedControl`, `SearchInput`, `FilterChip`, `OverflowMenu`, `AppSelect`, `ToggleSwitch`: retain behavioral contracts; retune tokens and density only.
- `ModalDialog` and confirmation/form dialogs: keep focus trap, scroll lock, stacking, and title relationships; migrate appearance to the B system.

No generic `Panel` or dashboard-specific alert component is added. Existing `.card`/panel primitives are sufficient, and the attention list belongs to `DashboardPage.vue`.

## Page composition

### Dashboard

- compact `PageHeader` labelled “运营数据” with a create-user shortcut using the existing `UserCreateDialog`;
- `MetricStrip` for users, traffic, devices, servers, and active nodes;
- main workspace: user traffic `DataTable` with existing today/total range and row expansion;
- adjacent attention panel derived by paging `listServers` until all `total` rows are loaded (100 per request), prioritizing offline servers, then CPU/memory/disk >= 80%;
- active-node count from `listNodes({ status: 'active', page: 1, pageSize: 1 }).total`;
- no unsupported “versus yesterday”, sustained-load duration, protocol-count, user-expiry, or online-presence claims are copied from the illustrative mockup;
- on mobile, the table workspace and attention panel stack without converting table rows into cards.

### List pages

- Users/Nodes: compact header, existing search/filter controls, primary table, selection summary, batch actions, paginator.
- Servers: replace stat-card grid with `MetricStrip`; keep server table and all create/edit/enable/delete behaviors.
- Visits: use the visit table as the main surface and top-host results as the adjacent secondary surface; preserve filters and pagination.

### Detail/settings/public pages

- User detail: group the prominent return command, username, status, ID/authorization metadata, and metric strip into one coherent identity area. Status sits next to the username through `title-extra`, not isolated at the far edge of a wide screen. Keep existing editing, limits, expiry, traffic, node authorization, devices, trends, and visit sections in un-nested work panels.
- Server detail: use the same resource-header contract and prominent “返回服务器列表” command; keep agent/token/install actions, resource meters, nodes, and visits, with tables remaining semantic.
- Settings: preserve its existing section navigation and form behavior while adopting B spacing, surfaces, and controls.
- Login and 404: adopt the same brand, palette, type, controls, and small-radius surfaces without marketing content.
- All create/edit/confirm dialogs are visually migrated without changing request payloads or success flows.

The controlled full-site polish pass covers page-title/action alignment, panel and workspace heading hierarchy, toolbar density, shared loading/empty/error presentation, long-text behavior, dark-theme contrast, and mobile spacing. It must reuse shared tokens/components where possible and must not refactor request flows or backend-derived fields.

## Data flow and compatibility

All API response types remain owned by `api/types.ts`. No raw `fetch`, payload casts, or duplicated DTOs are introduced.

```text
/api/dashboard -> overview store -> AppLayout status row
                              -> Dashboard metric strip
                              -> ServersPage metric strip

/api/dashboard/user-traffic -> Dashboard primary table
/api/servers                -> Dashboard attention list / existing server views
/api/nodes?status=active    -> Dashboard active-node metric
existing page APIs          -> unchanged list/detail/forms
```

Existing route paths, query-string restoration on NodesPage, search debounce, table selection/sort, modal focus behavior, one-time secret handling, and theme persistence remain compatible.

`SettingsPage` must not retain an offset derived from the old top-navigation shell. Its sticky section navigation uses the shared main-toolbar offset on desktop and becomes non-sticky where drawer/mobile toolbar geometry is not stable. Pages with multiple tables pass distinct `ariaLabel` values.

## Responsive and accessibility contract

- Verify 320, 375, 560, 700, 1024, and 1440px widths.
- The app shell and page container must have no horizontal overflow.
- Only labelled table regions may scroll horizontally; primary navigation uses the desktop sidebar or mobile drawer, never a horizontal route scroller.
- Verify drawer overlay close, Escape close, route-change close, scroll lock, focus containment, and focus restoration.
- All icon-only actions retain `aria-label`; status meaning is paired with text, not color alone.
- Focus rings, `role="alert"`, dialog semantics, top-dialog Escape handling, and `prefers-reduced-motion` remain intact.
- Long usernames, hostnames, addresses, and version strings must truncate or wrap without overlapping adjacent controls.
- Verify user/server return commands and resource header grouping in light/dark themes, including at 320px.

## Rollout and rollback

The already implemented B workbench is the revision baseline. The shell, shared resource header, detail pages, and remaining route polish are changed in small batches that remain type-checkable. There is no database or deployment migration. Rollback is a normal Git revert of frontend changes or rebuilding the prior `web/dist`; obsolete sidebar localStorage values remain ignored.

