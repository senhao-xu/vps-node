# 重新设计页面样式与布局

## Goal

Redesign the existing web application's styling and layout so the interface is clearer, more efficient, and easier to use across desktop and mobile screens, while preserving existing product behavior.

## Background

- The frontend is a Vue 3 management panel with seven routed page types: login, dashboard, users, user detail, servers, server detail, and settings.
- The application uses plain CSS with shared tokens and primitives; no UI or icon library is installed.
- The current interface has a functional responsive baseline, but weak information hierarchy, duplicated page context in the top bar, visually uniform dashboard metrics, dense wide tables, and long detail/settings pages without strong sectional orientation.
- Shared layout and component ownership are already established in `tokens.css`, `base.css`, `AppLayout.vue`, and reusable table/dialog/status components.

## Requirements

- Review the current frontend implementation before choosing a visual direction.
- Improve the visual hierarchy, spacing, navigation, content layout, and responsive behavior of the existing pages.
- Apply the redesign consistently to the login page, application shell, dashboard, user and server lists, user and server details, settings, dialogs, tables, and shared feedback/status components.
- Use the approved "Enterprise" direction (`concept-enterprise.svg`): cool light blue-gray content background, white navigation sidebar, blue primary accents, clean white cards with soft slate borders, clear status signaling, and information-dense but readable layouts.
- Make the visual hierarchy reflect operational priority: system health and exceptions first, primary actions second, supporting metadata third.
- Preserve zh-CN interface copy unless a wording change is needed to improve clarity.
- Preserve existing application functionality and established domain workflows.
- Build on the current Vue frontend and shared component structure rather than replacing the application architecture.
- Keep management tables semantic and horizontally scrollable on narrow screens, consistent with the established frontend specification.

## Acceptance Criteria

- [x] The redesigned interface has a coherent visual system across shared layout and major pages.
- [x] Primary navigation and page actions are easy to identify and use.
- [x] Major pages remain usable on desktop and mobile viewport sizes.
- [x] Existing workflows and data behavior continue to function after the redesign.
- [x] Frontend lint, type-check/build, and relevant tests pass.
- [x] The implemented result follows the approved Infrastructure Console direction rather than the two rejected concepts.

## Constraints

- Existing uncommitted frontend changes are part of the working baseline and must not be reverted.
- Final visual direction and scope are subject to evidence review and user approval.

## Key Decision

- 2026-09-21: The user clarified the approved concept is `concept-enterprise.svg` (Enterprise blue), not Infrastructure Console. The implemented Infrastructure Console palette was restyled to the Enterprise direction; all other scope stays unchanged.
