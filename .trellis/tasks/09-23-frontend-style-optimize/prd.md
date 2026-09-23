# Optimize web frontend with frontend-styles skill principles

## Goal

Improve the `web/` frontend's visual consistency, accessibility, and motion behavior by applying the **principles** of the external `frontend-styles` skill, while preserving the existing neutral/ink design system. User value: a more accessible, consistent, and maintainable UI without a visual re-theme.

## Background

- External skill: `/root/server-check/.cursor/skills/frontend-styles/SKILL.md` — written for a *different* project (blue `--primary`, green `--accent`, `StatusPage.vue`/`Admin.vue` shells). Its source-of-truth files do not exist in vps-node.
- User decision: borrow **principles only**; keep the existing neutral/ink palette. No brand blue/green.
- Target: `/root/vps-node/web/src` — Vue 3 + TS + Vite, no UI framework. Design system in `styles/tokens.css` + `styles/base.css`.
- Evidence base: `research/gap-analysis.md` (verified `file:line` anchors). Design: `design.md`. Plan: `implement.md`.

## Confirmed facts

- No hardcoded hex/rgb/rgba in any `.vue`; all color literals are confined to `tokens.css`.
- Token-level dark parity is satisfied: all 35 `--color-*` `:root` tokens have `[data-theme='dark']` overrides. Dark mode is `data-theme`-driven (JS), not `@media (prefers-color-scheme)`.
- `prefers-reduced-motion` has zero occurrences while the app has ~23 transitions and spin/skeleton keyframes.
- `ErrorBanner.vue` has no `role="alert"`; the `DataTable` scroll region is not focusable/labelled; the command palette input removes its focus ring.
- Shared primitives are cloned in scoped styles: `.card-head` ×7, `.card` padding ×3, `.stat-grid`/skeleton ×2, hand-rolled segmented control, `.head-actions` ×3.
- Breakpoints inconsistent (1200/901/900/700/640/600/560/480); no `320px` floor. No `tabular-nums`. Disabled opacity split `.45/.55/none`.
- No visual/screenshot test harness; only `build`, `typecheck`, `lint`.

## In Scope

R1 — Accessibility/motion (P0): global `prefers-reduced-motion` handling; `role="alert"` on async error banners; restore the command-palette input focus ring; keyboard-focusable + labelled wide-table scroll region; `320px` mobile width floor.

R2 — Consistency/dedup (P1, excluding P1-5): centralize cloned CSS (`.card-head` 560px stacking, `.card.compact`, `.stat-skeleton`, `.head-actions`→`.card-head-actions`, `UserDetailPage` card margins); reuse `SegmentedControl` for the server install tabs; unify disabled opacity (`.52`); consolidate the mono font to `.mono`; unify popover radius; replace literal radii with tokens (`--radius-full`, new `--radius-xs`).

R3 — Low-risk polish (P2): tabular figures for numeric/time data (P2-1); remove confirmed-dead tokens (P2-2); shared `.status-dot` primitive replacing duplicated dots (P2-4).

## Out of Scope

- Adopting the skill's blue/green palette, or any color-literal / `:root`-color change beyond removing dead tokens.
- Converting dark mode to `@media (prefers-color-scheme)`; changing `data-theme` mechanism.
- Data/API/business-logic behavior changes.
- Adding a visual regression/screenshot test harness.
- Deferred to follow-up tasks: P1-5 breakpoint normalization, P2-3 semantic-color review, P1-10 font-size literal tokenization, P2-5 `!important` removal, P2-6 shell-active alias, P2-7 title weight token.

## Acceptance Criteria

- [ ] `npm run typecheck`, `npm run lint`, and `npm run build` all pass in `web/`.
- [ ] No new hardcoded color literals outside `tokens.css`; palette remains neutral/ink; every removed token is confirmed unreferenced.
- [ ] `prefers-reduced-motion: reduce` suppresses non-essential animation/transition/scroll behavior.
- [ ] Async errors use `role="alert"`; wide tables expose a focusable, labelled `role="region"` scroll area with a visible focus ring.
- [ ] The 7 duplicated `card-head` blocks, 3 `.card` padding overrides, 2 stat-skeleton duplicates, and 3 `.head-actions` blocks are removed in favor of shared primitives; `ServerDetailPage` install tabs use `ui/SegmentedControl.vue`.
- [ ] Disabled controls use one shared opacity; popover radius is unified; `999px`/`4px`/`6px` literals replaced with `--radius-full`/`--radius-xs`/`--radius-sm` where applicable.
- [ ] Numeric/time displays use `font-variant-numeric: tabular-nums`.
- [ ] Manual light + dark pass at 320 / 375 / 560 / 700 / desktop on all touched surfaces; no unintended layout regression (especially stat grids and user detail panels).
