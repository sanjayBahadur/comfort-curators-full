# Phase UI 1.3.1 — Application navbar spacing

- **Date:** 2026-08-13
- **Agent/model:** GPT-5
- **Status:** complete

## What I built
Separated each application page's section label from its primary navigation so labels such as `01 / TICKET QUEUE` read as their own header region instead of running into the first navigation entry. Applied the treatment across staff and owner surfaces, excluded the expansion pitch, removed the left-side partition requested by the user, and reverted the unrelated expansion navbar edit from the previous pass.

## Files added or changed
`app/src/routes/ops.css` — gives the operations section label balanced spacing while preserving the compact mobile arrangement.

`app/src/routes/dashboard.css` — applies matching spacing to the owner dashboard header.

`app/src/routes/onboarding.css` — applies matching spacing to the onboarding header.

`app/src/routes/property-detail.css` — applies matching spacing to property-detail headers.

`app/src/routes/owner-records.css` — applies matching spacing to invoice and document headers.

`app/src/routes/curator.css` — reserves the global back-button column before the curator wordmark and adds scroll-driven ticket entrances on the jobs page.

`app/src/routes/curator-header.tsx` — provides one operations-style navbar for all curator routes, with explicit page context and clearance for both global controls.

`app/src/routes/curator-jobs.tsx`, `app/src/routes/curator-job-detail.tsx`, `app/src/routes/curator-zone-map.tsx`, `app/src/routes/curator-property-detail.tsx` — use the shared curator navbar with route-specific context labels.

`app/src/routes/expansion.css` — restored to its state before the incorrectly targeted navbar pass.

`logs/phase-ui-1.3.1-ops-navbar-spacing.md` — records this focused correction.

## Decisions I made
Kept all navigation labels, dimensions, and the existing black active state. Separation is created with balanced horizontal padding; the first navigation item's existing edge remains the only partition. Curator headers have no separate page-section label, but now use the same reserved back-button and wordmark columns as the other application headers. The package-shop header is contextual rather than navigational, so it did not require the numbered-label rule.

The jobs animation uses a CSS view timeline so movement is tied directly to scroll position without React observers or runtime listeners. It combines upward travel with a restrained perspective scale and opacity transition, and disables fully when reduced motion is requested.

## What did NOT work
The first pass modified the expansion story navbar, which was not the navbar shown by the user. That change was fully reverted.

## Deviations from the plan
none

## New API knowledge
none

## How to verify (human runs these)
Run `cd ~/open-code-projects/ComfortCurators/app && npm run build`. Expected: the production build completes successfully.

Open `/ops/tickets` around 1920×1080. Expected: `01 / TICKET QUEUE` occupies a comfortably padded region before `TICKETS`, with no added partition on its left.

Open owner dashboard, onboarding, a property detail, invoices, and documents. Expected: each numbered page label has the same balanced spacing before its navigation.

Open `/jobs`. Expected: the back button appears first, followed by an unobstructed `COMFORT CURATORS / CURATOR` wordmark. As job cards enter the viewport they rise and gently move toward the viewer; reduced-motion mode shows them statically.


Resize below 820px. Expected: the section label returns to the compact right-aligned mobile treatment without excess height.

## Open questions for the human

## What's next
Visually confirm the operations header at the target browser aspect ratio, then continue the UI 1.3.1 polish pass.
