# Phase P9.17 — reconnect SSE stream after send when previously closed

- **Date:** 2026-08-10
- **Agent/model:** gpt-5.6-sol (codex, medium effort)
- **Status:** complete

## What I built

- Added an explicit `generation` input to `useSuperhostStream`. Changing it is the only new same-thread reconnect trigger; normal renders do not reconnect.
- Added a reconnect generation counter to `SuperhostMount`.
- After `sendSuperhostMessage` succeeds, `handleSend` reads the latest connection state and increments the generation only when the stream is neither `open` nor `connecting` (that is, only for `done` or `error`).
- Added focused hook coverage for explicit reconnects, replay de-duplication, and fresh-thread history reset.

## How history is preserved across a reconnect

The backend's `internal/automation/stream.go` resolves a thread stream to the thread's latest run and starts that run's event cursor at zero. A new connection therefore replays the latest run, not the full history of every run in the thread.

For a reconnect to the same `threadId`, the hook now keeps the existing event array and merges delivered events by `event_id`. Replayed events are ignored and events from the newly selected run are appended. When `threadId` genuinely changes, the hook still clears the old event history before consuming the new stream.

## Files added or changed

- `app/src/lib/api/superhost-stream.ts`
- `app/src/components/superhost/SuperhostMount.tsx`
- `app/src/__tests__/SuperhostStream.test.tsx`
- `logs/phase-P9-17-stream-reconnect.md`

## What I verified live vs. build/lint-only

Live verification used Playwright with real Chrome (`channel: "chrome"`, headless) against this worktree on `http://localhost:3001`; port 3000 was already serving the main checkout and was left untouched. The healthy Docker backend and model stub were reachable through the normal Vite proxy.

On one property-detail page, I opened the Superhost drawer and sent three messages sequentially without navigating away, waiting after each. All three remained visible as separate checklist tasks and each reached `DONE`; none remained `NOT STARTED`. The browser observed four stream requests for the same thread (the initial stream plus one reconnect for each post-terminal send) and no console errors.

Automated verification:

- Focused reconnect tests: 2 passed.
- Full unit suite: 27 passed across 5 files.
- `npm run build`: passed. Vite emitted only its existing large-chunk advisory.
- `npm run lint`: passed with exit code 0. Oxlint emitted only existing warnings in out-of-scope files.

## Decisions I made

- Used a declarative generation input instead of an imperative hook return because the neighboring hooks are argument-driven and this keeps connection lifecycle ownership inside the effect.
- Used a ref for the connection-state check after the asynchronous send so the decision uses the latest rendered stream state rather than the state captured when submission began.
- Preserved same-thread history locally and de-duplicated by the wire contract's stable `event_id`, because backend inspection confirmed that reconnect replay covers only the latest run.

## What did NOT work

- The first focused-test attempt resolved dependencies from the read-only parent checkout; Vite could not create its config cache there and the shared install lacked `@vitejs/plugin-react`. Running the repository-prescribed `npm install` in this worktree created the local dependency tree, after which focused tests, the full suite, build, and lint passed.
- `AGENTS.md` was requested but is absent from this worktree, the surrounding repository, and Git's tracked files. The complete available clean-architecture pack was read and followed.

## Open questions

None.
