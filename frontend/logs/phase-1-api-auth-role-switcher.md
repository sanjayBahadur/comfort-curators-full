# Phase 1 — API client, auth, role switcher

- **Date:** 2026-08-08
- **Agent/model:** GPT-5.6 Codex (orchestrator); DeepSeek V4 Flash, DeepSeek V4 Pro, Qwen 3.7 Plus, GPT-5.6 Luna (read-only specialists)
- **Status:** complete

## What I built
The app now has the typed client foundation every later phase uses: bearer tokens are restored from `sessionStorage` and attached to API requests, API failures become structured `ApiError` instances and global Sonner toasts, and backend request IDs are retained for diagnostics. `/login` is a responsive, explicitly non-production demo role switcher for owner, staff, and guest. It mints a real backend session, proves an authenticated properties request, exposes a real error-message acceptance control, and renders `45000` minor INR units as `₹450.00` through the single money formatter.

## Files added or changed
`app/package.json` — adds the architecture-specified standalone `sonner` toast dependency.

`app/package-lock.json` — locks Sonner and the resulting dependency graph.

`app/src/lib/api/client.ts` — builds robust headers, attaches the current bearer token, parses structured API errors, keeps the request ID, and displays the API message globally.

`app/src/lib/auth/session.ts` — mints owner/staff/guest sessions, restores token and role from this tab, surfaces session API errors, and clears both values on sign-out.

`app/src/lib/money.ts` — provides the sole integer-minor-unit money formatter without fractional float arithmetic.

`app/src/components/money.tsx` — provides the shared tabular money rendering boundary.

`app/src/routes/login.tsx` — implements the honest demo-access role cards, live authenticated check, session status, token-switch proof, real-error control, and money proof.

`app/src/main.tsx` — adds the `/login` route, redirects unknown/root routes to it, and mounts the global unstyled Sonner viewport.

`app/src/index.css` — styles the maximal login, role cards, session console, responsive states, focus states, and square hard-edged toasts.

`SCREENS.md` — replaces the stale Next.js `/api/auth/session/route` reference with the verified Vite-proxied `/api/auth/session/create` contract and aligns the Phase 1 cards to owner/staff/guest.

`INTEGRATION.md` — records the live missing-property error used for toast acceptance.

`logs/DECISIONS.md` — records the cross-phase global API error/toast rule.

## Decisions I made
Sonner is mounted globally but completely unstyled so it provides reliable announcements without importing rounded/shadowed SaaS visuals. `api()` owns API-error toasts; callers catch only to recover local control flow and never replace the backend message. Session role is stored beside the required token so the role switcher can truthfully restore its state after reload, while the token remains scoped to `sessionStorage` rather than persistent local storage.

The formatter derives the currency's fraction digits from `Intl.NumberFormat`, separates whole/fractional minor units with `BigInt`, and passes only the whole integer through locale grouping. This produces Indian grouping and currency symbols without dividing money into a JavaScript floating-point value.

OpenCode routing was task-specific: DeepSeek Flash handled the contract audit, DeepSeek Pro was attempted for architecture risk, Qwen handled the final code blocker review, and Luna handled visual acceptance. The two initial deep repository synthesis runs were bounded and terminated when they kept exploring without returning findings; their incomplete output was not used.

## What did NOT work
The first Sonner install was attempted inside the network-restricted sandbox and stalled; rerunning it with the approved package-manager network permission succeeded with zero vulnerabilities. The first money build hit TypeScript's optional typing for `maximumFractionDigits`; a safe `?? 2` currency fallback resolved it. The first desktop capture exposed a misregistration layer that ignored the headline line break and visually repeated “keys”; duplicating each line independently fixed it. The first direct `tsx` formatter check could not create its IPC socket inside the sandbox; the approved host run passed. Puppeteer Core was installed without saving only for the live acceptance run, then pruned; it is not a project dependency.

## Deviations from the plan
The login remains on `/login` after role selection and shows authenticated acceptance state instead of redirecting to owner/ops product routes. Those destinations belong to Phases 4 and 5 and do not exist yet; redirecting now would either land on a catch-all or require out-of-phase placeholder screens.

## New API knowledge
Live verification established that `GET /v1/properties/not-a-real-property` returns HTTP 404 with `{ "code": "NOT_FOUND", "message": "property not found", "request_id": "..." }`. This was added to `INTEGRATION.md` and is the request behind the visible “Force API error” control.

## How to verify (human runs these)
1. Run `cd ~/comfort-curators-frontend/app && npm run lint && npm run build` — expected: both commands exit successfully.
2. Run `cd ~/comfort-curators-frontend/app && npm run dev`, then open `http://localhost:3000/login` — expected: the paper/black demo access screen renders with three large Owner, Staff, and Guest cards and an explicit “NOT A REAL LOGIN” mark.
3. Open the browser Network tab, click Owner, and inspect `GET /api/v1/properties` — expected: request header `Authorization: Bearer …`, the session console reads `OWNER ACTIVE`, and the request returns successfully.
4. Reload `/login` — expected: `OWNER ACTIVE` and “Session restored from this tab” remain visible.
5. Click Staff — expected: the console changes to `STAFF ACTIVE` and reads “Different session token minted”.
6. Click “Force API error” — expected: a square global toast says exactly `property not found`, not `Error` or a frontend paraphrase.
7. Inspect the black operating-sample panel — expected: `₹450.00` is rendered from `45000` minor units.
8. Click Guest, then “Clear session” — expected: Guest can mint a real session, and clearing removes the active role from the page and this tab's session storage.
9. Repeat at approximately 390 px wide — expected: no horizontal overflow or clipped controls, with the same hard-edged visual language.

## Open questions for the human
1. GitHub publication still requires a valid CLI credential. Options: run `gh auth login -h github.com`, or export a fresh project-scoped `GH_TOKEN` in the shell; recommendation: use `gh auth login` and then let the orchestrator create the private repository and publish the logged phases in one intentional initial commit.

## What's next
Wait for manual Phase 1 acceptance. After approval, begin Phase 2's idempotent seed script from the verified sequence in `INTEGRATION.md §11`, without modifying the halted backend. The Vite server must be running when Phase 2 seeds the iCal feed.
