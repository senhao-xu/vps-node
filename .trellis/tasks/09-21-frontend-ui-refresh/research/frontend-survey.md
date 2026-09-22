# Frontend Survey (explore agent, 2026-09-21)

## Stack
Vue 3.5 + TS strict + Vite 7 + Pinia + vue-router. No UI lib, plain CSS (`styles/tokens.css` + `styles/base.css` + SFC scoped). Main chunk 112K. zh-CN hard-coded strings. Hand-rolled fetch wrapper `api/http.ts` (cookie auth, 401 handler).

## Routes (`web/src/router/index.ts`)
/login, / (DashboardPage 628ln), /users (349), /users/:id (192 + 7 user/* panels), /servers (255), /servers/:id (746), /nodes (473 + NodeFormDialog 765), /settings (293), catch-all → / (no 404).

## Layout
`components/AppLayout.vue` (522ln): collapsible sidebar 256→56px (localStorage), sticky topbar (title from nav match — wrong on detail pages, no breadcrumb), theme cycle icon, logout. Mobile <900px: sidebar → horizontal scroll nav PLUS topbar = double header.

## Styling
- `styles/tokens.css` (146ln): two-layer tokens, full dark set under `[data-theme='dark']`. Palette already Xboard/shadcn neutral: `--color-primary` near-black #0F172A (spec directory-structure.md:55).
- `styles/base.css` (359ln): .page/.card/.btn/input/.field/.filters/.actions/.empty-tip etc.

## Shared components
DataTable (typed generic, 144ln), TablePaginator, ModalDialog, ConfirmDialog, StatusBadge, ProgressBar, MetricBar, TrafficChart (SVG, no axis), ErrorBanner, CopyText, OneTimeSecret, NodeChecklist; form dialogs: NodeFormDialog (765ln), ServerFormDialog, UserCreateDialog; user/* detail panels ×7.

## Duplication
- DashboardPage hand-rolls its own table (196-332 + ~130ln CSS) instead of DataTable — only table not using shared component.
- `.eyebrow` copy-pasted into 7 files with inconsistent letter-spacing; `.toolbar-label` ×4; `.info-grid` ×2; `.card-head` re-defined ×3 despite base.css; `.actions` ×3.
- Copy-with-timer logic ×3 (CopyText, OneTimeSecret, ServerDetailPage:58-65).

## Rough spots (file:line)
1. window.confirm for destructive rotation — components/user/UserDetailBasic.vue:58
2. Bare-text "加载中…" everywhere, no spinner/skeleton — DashboardPage.vue:154-164, DataTable.vue:46-55 etc.
3. Inline styles — UserDetailTraffic.vue:139, ProgressBar.vue:33, DataTable.vue:39,76, ModalDialog.vue:46 (fixed 480px)
4. Stat-card priority keyed on Chinese label strings — DashboardPage.vue:143
5. Topbar title wrong on detail pages, no breadcrumb — AppLayout.vue:148-150
6. Mobile triple-stacked titles — AppLayout.vue:449-521
7. DataTable min-width:760px; pages diverge with :deep() overrides (NodesPage.vue:423-426, ServersPage.vue:218-221). Spec: tables stay semantic in scroll container (directory-structure.md:51)
8. DataTable row key = array index — DataTable.vue:71 (dashboard polls 30s → row state glitches)
9. TrafficChart: no axis/grid/ticks, native title tooltips, preserveAspectRatio="none" — TrafficChart.vue:10-12,54-60
10. Inconsistent error display: ErrorBanner vs ad-hoc text-danger — UserTrafficStats.vue:88-93, UserDetailBasic.vue:147-152, UserSessions.vue:66-71; ServerDetailPage.vue:262-269 stacks two banners
11. Native checkboxes (NodesPage.vue:341-346), native datetime-local (UserCreateDialog)
12. ModalDialog: no focus trap/scroll-lock, duplicate id="modal-dialog-title" (:45,50); pages mount two NodeFormDialogs
13. Button loading = text swap only; some action links silently disable (UsersPage.vue:244-257)
14. Theme toggle cryptic 3-state single icon — AppLayout.vue:152-195; brand logo bare "V"
15. No 404 page — router/index.ts:48-51

## Bundle
dist main chunk 112K JS + 16K CSS; pages 4-24K each. Adding lucide-vue-next ≈ +20K (tree-shaken). UI kit (Naive/Element) would add ~1M — rejected.

## Target style (from user-provided xboard screenshots)
Page header: bold title + gray subtitle. Topbar: ⌘K menu search, theme/lang/avatar. Stat cards row: label + big number + right icon. Table toolbar: black primary btn + search input + filter chips; sortable headers (↕). Cells: ID outline chip, enable toggle, dot+name, tag chips, traffic value+%+thin bar, "..." menu. Footer: "已选择 N 项，共 M 项" + page-size + pager. Soft gray/green/red pill badges. Settings: left sub-nav + right sections (label+input+helper). Modals: large radius, title+type chip+subtitle, × close, footer right-aligned (高级设置/取消/黑提交), dark overlay. Custom dropdown with colored protocol dots + ✓. Toggle card rows (label+desc+switch).
