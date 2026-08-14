# Phase P1.2 — Select primitive

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built

Built a reusable listbox-pattern `Select` primitive with controlled and uncontrolled value support, options, placeholder, disabled state, form `name`/`required` support, and accessible trigger/listbox/option attributes.

Keyboard behavior includes Enter/Space to open or choose, Arrow Up/Down navigation, Home/End navigation, type-ahead matching, Escape close with focus returned to the trigger, and roving tabindex across enabled options.

Verified by tracing the trigger and option key handlers, the close/choose focus path, the open-transition focus effect, and the type-ahead search path because this repository has no interaction test harness yet.

## Files added or changed

- Added `app/src/components/ui/Select.tsx`.
- Added `app/src/components/ui/Select.css`.
- Added `logs/phase-P1-2-select-primitive.md`.

## Decisions I made

- Used a named `options` API because the later call sites need both controlled filters and uncontrolled form fields.
- Kept the component self-contained and imported co-located plain CSS, matching the repository's existing styling approach.
- Included a hidden named input so later form replacements preserve native form submission behavior.
- Focuses the active option while open rather than putting every option in the tab order.
- Used only existing paper, ink, rule, radius, typography, and status tokens; the panel has a 1px rule, no radius, and no shadow.

## What did NOT work

Nothing identified during implementation.

## Deviations from the plan

Added outside-pointer close and disabled-option handling because they are needed for a reusable replacement primitive, while keeping the public API limited to the current native-select use cases.

## Open questions

P1.6 will decide the exact adapter shape at each route call site, especially for onboarding's uncontrolled fields and filter routes' controlled fields.
