# Phase P2.1 — intro sequence

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built

- Added a session-gated four-beat intro route with a real properties prefetch into the TanStack Query cache.
- Added font readiness, an 1800ms floor, a 5000ms ceiling, and a `Promise.race([work, ceiling])` handoff to `/login`.
- Added a skip affordance available from the first frame and an Escape key handler.
- Added the reduced-motion path, which marks the intro seen and navigates directly to `/login` without rendering beats.
- Added the ruled grid, registration marks, live property count, honest unavailable counters, wordmark landing, and scoped grain overlay.

## Files added or changed

- `app/src/routes/intro.tsx`
- `app/src/routes/intro.css`
- `logs/phase-P2-1-intro-sequence.md`

## Decisions I made

- Used `queryClient.fetchQuery({ queryKey: ["properties"], queryFn: getProperties })` because it returns the payload needed by beat 03 while also filling the cache.
- The properties counter cross-fades the resolved value; it never counts upward.
- Open tickets and curators on shift render `—`. The available API has no single prefetch endpoint for either datum, so inventing counts or speculative requests would make the status readout dishonest.
- Kept the route and CSS self-contained and did not touch `src/index.css`, `main.tsx`, or `login.tsx`.

## What did NOT work

- The first build attempt was blocked because dependencies were not installed (`tsc` and `oxlint` were unavailable). `npm ci` restored the local toolchain; the required checks then passed.

## Deviations from the plan

- The current auth contract only mints a session after a role and contact are selected in `login.tsx`; it has no role-neutral session-mint operation. The intro therefore does not mint an arbitrary role or fabricate credentials. It prefetches with the existing session when present, and login remains responsible for role session minting.

## New API knowledge

- `GET /api/v1/properties` is exposed as `getProperties()` in `src/lib/api/shop.ts` and returns `{ items, next_cursor }`.
- Existing ticket and worker APIs are available, but their current helpers fetch role-specific operational bundles rather than providing a safe standalone intro counter source.
- The route entry in `main.tsx` currently maps `/` to `EntryRoute`, which immediately redirects. P2.4 owns router wiring; it should replace that element with `<Intro />` (or make the equivalent explicit entry composition) rather than leaving both components competing for `/`.

## Open questions

- P2.4 should confirm whether `/` becomes `<Intro />` unconditionally or whether an already-authenticated session should be allowed to bypass it through `EntryRoute` before the intro is shown.
