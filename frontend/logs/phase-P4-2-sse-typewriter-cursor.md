# Phase P4.2 — SSE client, typewriter reveal, block cursor

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built

- Added a bearer-authenticated `fetch`/`ReadableStream` SSE client with manual `data:` parsing, `[DONE]` handling, connection states, and abort-safe cleanup.
- Added mapping for all terminal-relevant event names. Approval requests remain distinct data for P4.3 rather than becoming ordinary lines; unknown events are ignored safely.
- Added a 12ms/character typewriter capped at 400ms per line. It keys animation to genuinely new line IDs and immediately renders when `prefers-reduced-motion` is enabled.
- Added a timed `/debug` canned sequence proving the typewriter, cursor state, approval exposure, verbatim denial, and visible `OFFLINE FALLBACK` marker.

## Files added or changed

- `app/src/lib/api/superhost-stream.ts`
- `app/src/components/superhost/behavior.ts`
- `app/src/routes/debug.tsx`
- `logs/phase-P4-2-sse-typewriter-cursor.md`

`Terminal.tsx`, `Terminal.css`, `src/index.css`, and `src/main.tsx` were not modified.

## Decisions I made

- Kept transport in `superhost-stream.ts` instead of folding it into the non-streaming API module, because stream lifecycle and cancellation are materially different concerns.
- Kept event mapping and presentation timing in `components/superhost/behavior.ts`, leaving `Terminal` presentational and preserving its existing children slot for `ConfirmBlock`.
- Used the existing `/api` same-origin convention and `getToken()` bearer token source.

## What did NOT work

- A live backend thread was not available in this sandbox, so an end-to-end authenticated stream, server-side `[DONE]`, and live reconnect behavior could not be exercised.
- Dependencies were initially absent; `npm ci` from the existing lockfile resolved that setup issue.

## Deviations from the plan

- The debug proof uses a local timer-driven event array as requested rather than attempting a live network call.
- No prop gap was found in `Terminal`; no changes were needed to the frozen component or CSS.

## Open questions

- P4.3 should consume the exposed `approval_required` entries and render its `ConfirmBlock` through `Terminal`'s children slot.
