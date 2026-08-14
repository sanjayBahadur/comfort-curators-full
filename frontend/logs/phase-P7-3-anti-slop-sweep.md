# Phase P7.3 — Anti-slop sweep (ART-DIRECTION.md §12)

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## Method (browser available, or source-level sweep)

Source-level sweep. No Chromium, Chromium Browser, or Google Chrome binary was available in the sandbox, so no live browser render was possible. Reviewed every route registered in `app/src/main.tsx`, all route CSS, shared UI CSS, `Money`, and `index.css` token/grain definitions. Ran targeted searches for raw colors, shadows, gradients, radii, thick rules, rotations, Inter, emoji, generic seed copy, and empty-state copy. Ran `npm run build` and `npm run lint` from `app/` after installing the existing lockfile dependencies with `npm ci`.

## Violations found and fixed, by screen

- `/dashboard`: changed the active-attention frame from 2px red to a 1px rule; changed displayed money values to JetBrains Mono; replaced the check-mark glyph with plain `OK` text.
- `/login`: removed the 2px hover rule thickening; preserved the permitted small transform.
- `/stay`: changed the 2px action underline to 1px; changed the quote total to JetBrains Mono.
- `/properties/:propertyId`: changed the compliance hold banner and active-holds frame from 2px red rules to 1px rules.
- `/properties/:propertyId/package`: changed the drag target and active cart-row rules from 2px to 1px; changed package cost values to JetBrains Mono.
- `/invoices` and `/documents`: shared owner-record money values now use JetBrains Mono through `Money` and the owner-records money style.
- `/ops/calendar`: replaced raw `#fff2ed` and inset shadow with `var(--paper-2)` and a 1px red left rule.
- `/ops/tickets` and `/ops/workers`: removed drawer shadows.
- `/debug`: removed the check-mark glyph from the copy confirmation text; money proof now inherits the shared mono money treatment.
- Shared `Money`: added a mono, tabular-numeral class so all rendered money is consistently styled as required.
- Shared Superhost drawer: removed its box shadow.

## Checked and clean, by screen

- `/` — token colors, grain, typography, empty/error copy, and decorative rules checked.
- `/login` — checked after the rule fix; no Inter, emoji, generic seed copy, gradients, or shadows.
- `/expansion` — checked; numeric SVG labels and axes use JetBrains Mono.
- `/stay` — checked after money/rule fixes; catalog and reservation empty states have written copy.
- `/dashboard` — checked after attention/money/glyph fixes; dashboard collection empty states have written copy.
- `/onboarding` — checked; money, IDs, dates, and step counts use mono metadata styles.
- `/invoices` — checked; scoped and no-charge states have written copy.
- `/documents` — checked; scoped and no-document states have written copy.
- `/jobs` — checked; unavailable and no-assignment states have written copy.
- `/jobs/:ticketId` — checked; unavailable and no-checklist states have written copy.
- `/properties/:propertyId` — checked after hold-rule fix; lifecycle, contact, and readiness empty states have written copy.
- `/properties/:propertyId/package` — checked after cart-rule fix; empty catalog/cart states have written copy.
- `/ops/tickets` — checked; operator table has no grain, rotation, tearing, or shadow treatment.
- `/ops/tickets/new` — checked; form fields and validation copy use the ops visual system.
- `/ops/tickets/:ticketId` — checked; checklist, evidence, history, and candidate empty states have written copy.
- `/ops/calendar` — checked after stale-card fix; ops tables have no grain, rotation, tearing, or shadow treatment.
- `/ops/properties` — checked; table has no grain, rotation, tearing, or shadow treatment and has a written empty state.
- `/ops/workers` — checked after drawer fix; table has no grain, rotation, tearing, or shadow treatment and has a written empty state.
- `/debug` — checked as the style-sheet reference page; its documented demonstration rotations remain isolated there.

## Routes not reachable / not checked, and why

- No route was reachable in a live browser because the sandbox has no browser binary. All 18 priority routes and `/debug` were checked at source level.

## Decisions I made

- Kept the documented halftone `repeating-radial-gradient` in the shop and cookie slip. ART-DIRECTION.md §7 explicitly defines this as an allowed collage treatment despite the broad no-gradient checklist wording.
- Kept the documented decorative rotations for the cookie slip, sticker, debug demonstration, modal, and shop marker. Text-bearing layout remains unrotated or within ±3 degrees.
- Kept the 2px red payment-boundary frame and 2px control-handover frame because ART-DIRECTION.md §14 explicitly documents those exceptions.
- Kept `box-shadow: none` and `border-radius: 0` resets. They create no visual shadow or rounding and enforce the sharp-corner system.
- Treated red attention/hold borders as ordinary status frames rather than the documented safety-frame exceptions, so reduced them to 1px.

## Open questions

- Live visual verification remains unavailable in this sandbox; browser-level responsive inspection should be repeated when a browser runner is available.
- `npm run lint` passes with six existing Fast Refresh warnings in Superhost and agent-surface context files; none were introduced by this block.
