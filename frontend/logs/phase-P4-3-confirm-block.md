# Phase P4.3 — ConfirmBlock bound to approval requests

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built
Added a self-contained `ConfirmBlock` that renders the mapped approval summary inside the terminal, posts approved or rejected decisions to the frozen Superhost approvals endpoint, shows a pending state while posting, renders the server response on success, and renders the real error message on failure. Successful decisions cannot be submitted again. Wired the block into the `/debug` mock-event terminal demo.

## Files added or changed
- `app/src/components/superhost/ConfirmBlock.tsx`
- `app/src/components/superhost/ConfirmBlock.css`
- `app/src/routes/debug.tsx`

## Decisions I made
- Used the shared `api()` helper so authentication, response parsing, and error toasts remain consistent with the rest of the frontend.
- Encoded the request ID in the endpoint path and sent the exact `{ decision }` request body.
- Keyed the debug block by `approval.requestId` so its local lifecycle belongs to one approval request.
- Kept phosphor styling scoped under `.superhost-terminal` and reused the existing terminal tokens.

## What did NOT work
- A live POST could not be exercised because the debug approval uses mock data and there is no live backend thread in this sandbox. Code review verifies the request path, body, pending state, response rendering, error handling, and duplicate-submit guard.

## Deviations from the plan
- None.

## Open questions
- A live connection should verify the backend's success and error payloads against the debug interaction, including the requeued run's later SSE events.
