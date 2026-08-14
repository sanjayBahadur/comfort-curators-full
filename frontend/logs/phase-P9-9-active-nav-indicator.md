# Phase P9.9 — active-route highlighting across every header nav

- **Date:** 2026-08-10
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** partial

## What I built

Added router-derived `aria-current="page"` state to the owner dashboard, operations, property detail, owner records, and curator header navigation. Extended each matching header stylesheet's hover selector with the same active-state selector used by onboarding.

## Every header touched, and how "active" is computed for each (exact vs. prefix match)

- **Owner dashboard:** `/dashboard`, `/onboarding`, `/invoices`, `/documents`, `/debug`, and `/login` use exact-or-descendant prefix matching. The property package link uses the selected property's `/properties/:propertyId/package` target.
- **Operations shared header:** `/ops/tickets`, `/ops/properties`, `/ops/workers`, `/ops/calendar`, `/jobs`, `/debug`, and `/login` use exact-or-descendant prefix matching, so ticket detail and ticket creation remain under TICKETS.
- **Property detail:** DASHBOARD, ONBOARD, and ACCESS DESK use exact-or-descendant prefix matching. PACKAGE uses the `/properties/:propertyId` section prefix so both property detail and its package sub-route identify the property section.
- **Owner records shared header:** DASHBOARD, ONBOARD, INVOICES, DOCUMENTS, and ACCESS DESK use exact-or-descendant prefix matching.
- **Curator jobs:** OPS and ACCESS use exact-or-descendant prefix matching.
- **Curator job detail:** JOBS and OPS use exact-or-descendant prefix matching, so a job detail remains under JOBS.
- **Onboarding:** intentionally unchanged, as requested; its existing static active marker is the reference pattern.

## Files added or changed

- Added `logs/phase-P9-9-active-nav-indicator.md`.
- Changed `app/src/routes/dashboard.tsx` and `app/src/routes/dashboard.css`.
- Changed `app/src/routes/ops-shared.tsx` and `app/src/routes/ops.css`.
- Changed `app/src/routes/property-detail.tsx` and `app/src/routes/property-detail.css`.
- Changed `app/src/routes/owner-records.tsx` and `app/src/routes/owner-records.css`.
- Changed `app/src/routes/curator-jobs.tsx`, `app/src/routes/curator-job-detail.tsx`, and `app/src/routes/curator.css`.

## What I verified live vs. build/lint-only

- **Build:** `npm run build` passed.
- **Lint:** `npm run lint` completed with existing warnings in unrelated files; no errors.
- **Live:** not completed. The app started successfully at `http://localhost:3000`, but Playwright `channel: "chrome"` could not launch because `/opt/google/chrome/chrome` is absent. `npx playwright install chrome` could not install it because the sandbox requested an unavailable root password.

## Decisions I made

- Kept the existing header markup and visual language; only added pathname-derived `aria-current` values and the corresponding CSS selectors.
- Used section prefixes for nested operational routes and property routes, and exact-or-prefix matching consistently through small local `isActive` helpers.
- Did not touch onboarding, global styles, main entrypoint, or superhost components.

## What did NOT work

- The first build/lint attempt could not find `tsc` or `oxlint` because dependencies were not installed. `npm ci` resolved that; the subsequent checks passed.
- Real Chrome verification was blocked by the missing Chrome binary and failed privileged installation.

## Open questions

- The curator jobs landing header has no JOBS link inside its nav, so there is no existing nav item to mark active on `/jobs`; adding one would be a navigation-layout change outside this active-state-only task.
