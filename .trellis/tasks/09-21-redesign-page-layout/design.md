# Technical Design

## Scope And Boundaries

The redesign is limited to the Vue frontend under `web/src/`. It changes presentation, layout, and interaction affordances while preserving API calls, route names, backend contracts, authentication behavior, and existing business actions.

The shared visual system owns reusable decisions. Pages own only domain-specific composition. Existing uncommitted changes in the worktree are treated as a baseline and must not be reverted.

## Visual System

- Apply the approved Enterprise palette (`concept-enterprise.svg`): cool light blue-gray content background (#f6f8fb), white shell/sidebar and topbar with slate borders, blue primary actions (#2563eb), blue soft states (#eff6ff), green healthy states, amber attention states, and red destructive states.
- Extend `styles/tokens.css` for palette, typography scale, radii, shadows, shell dimensions, and responsive breakpoints. Avoid repeating literal colors in page styles.
- Keep `styles/base.css` responsible for page containers, headings, cards, controls, action groups, feedback, and common responsive behavior.
- Use a restrained type hierarchy and operational labels rather than decorative gradients or dense ornamentation.

## Application Shell

`AppLayout.vue` will provide the primary visual anchor:

- Desktop: persistent white sidebar with brand, grouped navigation, active route indicator, and administrator area; content uses a readable max-width and a compact top context row.
- Mobile: preserve accessible horizontal/stacked navigation behavior without hiding the current route or essential account action.
- Keep route matching behavior unchanged. If icons are introduced, use small inline SVGs or CSS marks owned by the shell; do not add a UI dependency for decorative icons.

## Page Composition

- Dashboard: prioritize a health/traffic summary block, then differentiated metric cards, traffic visualization, and server status. Use only data already available from the dashboard API; no fabricated activity feed or new endpoint is required.
- List pages: strengthen page header/action hierarchy, move filters into an obvious toolbar, preserve semantic tables and horizontal scrolling on narrow screens, and keep row actions usable.
- Detail pages: introduce clearer section headers, summary/meta treatment, and responsive two-column layouts while preserving all existing forms, secrets, node tables, and actions.
- Settings: group related controls with stronger section hierarchy and keep the save action visible and understandable without changing submission behavior.
- Login: align the public page with the approved visual language while retaining the same authentication flow and error handling.

## Data And Compatibility

No API or DTO changes are planned. Existing typed API functions remain the sole data boundary. Existing table, dialog, status, feedback, and chart components remain reusable and receive styling/ownership improvements where needed.

Management tables remain actual tables in a horizontal scroll container. Dialog accessibility attributes and existing one-time-secret behavior must remain intact.

## Responsive Strategy

- Desktop layouts target wide operational screens without allowing content to become excessively sparse.
- At tablet widths, collapse shell/navigation deliberately and allow page headers, toolbars, and multi-column sections to stack.
- At phone widths, prioritize action reachability, readable touch targets, clipped metadata only where safe, and table scrolling rather than cardifying rows.

## Rollback And Risk

The change is CSS/template-local and can be rolled back by reverting only the files listed in the implementation plan. Avoid broad dependency or router changes. Validate at desktop and narrow viewport sizes because the highest risk is visual regression in dense tables and dialogs.
