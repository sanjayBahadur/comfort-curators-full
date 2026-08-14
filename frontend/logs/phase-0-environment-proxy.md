# Phase 0 — Environment and the proxy proof (Next.js) — SUPERSEDED

> **Superseded 2026-08-08 by `phase-0-vite-rebuild.md`.** The stack was changed
> from Next.js to Vite + React Router after this phase completed. Nothing below
> was wrong — the work was verified and the log is accurate. It is kept intact as
> the record of what happened, per `POLICY.md §1`. See `logs/DECISIONS.md` for
> the reasoning behind the change.


- **Date:** 2026-08-08
- **Agent/model:** GPT-5.6 Codex
- **Status:** complete

## What I built

Scaffolded the Next.js application in `app/`, added the required same-origin
rewrite from `/api/*` to the Go backend, installed and mounted TanStack Query,
and added `/debug`. The debug page temporarily mints an owner acceptance session
in browser memory, then makes an authenticated `GET /api/v1/properties` request
and renders the raw JSON response.

The real backend stack was built with `CC_BUILD_TAGS=acceptance` and started.
The backend health check, session mint, proxied authenticated properties request,
debug route, ESLint, TypeScript, and production build all passed.

## Files added or changed

`app/` — official `create-next-app@latest` scaffold using TypeScript, App Router,
Tailwind CSS, ESLint, and npm.

`app/next.config.ts` — rewrites browser `/api/:path*` calls to `API_ORIGIN`.

`app/components/providers.tsx` — owns the single browser `QueryClient` and mounts
`QueryClientProvider` for the App Router tree.

`app/app/debug/page.tsx` — Phase 0 authenticated proxy proof and raw properties
response renderer.

`app/app/page.tsx` — replaces the generated marketing page with a minimal link
to the proxy proof.

`app/app/layout.tsx` — supplies Comfort Curators metadata and mounts providers.

`app/app/globals.css` — removes generated dark-mode styling and uses the specified
paper base colour pending the full Phase 0.5 design tokens.

`app/.env.local` — local API origin and documented demo environment values;
ignored by Git.

`app/.env.example` — committable empty environment template.

`app/.gitignore` — allows `.env.example` while continuing to ignore real env files.

`logs/DECISIONS.md` — records the cross-phase Next.js version decision.

## Decisions I made

The debug proof uses a deliberately temporary browser-memory token flow: it
POSTs to `/api/auth/session/create` and sends the returned token on the proxied
properties request. This proves both authenticated requests cross the same-origin
rewrite without prematurely implementing Phase 1. Phase 1 must replace this with
the specified server-side route and httpOnly `cc_session` cookie.

I kept the current output of the required `create-next-app@latest` command:
Next.js 16.2.3 and React 19.2.4. `ARCHITECTURE.md` names Next.js 15, but pinning an
older version would contradict the phase's exact scaffold instruction. The
generated `next.config.ts` is functionally equivalent to the documented
`next.config.mjs`; its rewrite was tested through the real stack.

I initialized Git at the build-pack root before scaffolding, so the specifications,
logs, and `app/` are one repository rather than leaving the documentation outside
a nested application repository.

## What did NOT work

The first sandboxed npm commands could not complete network downloads. They were
rerun with approved network access. A second install overlapped the tail of the
scaffold install and emitted many transient `TAR_ENTRY_ERROR ENOENT` warnings;
the completed package tree subsequently passed `npm ls`, ESLint, TypeScript, and
the full production build.

The first production build failed only inside the restricted sandbox because
Turbopack could not bind a local helper port (`Operation not permitted`). The
same build passed when rerun with the required local-process permission.

The first local curl check ran in the isolated network namespace and could not
see the host services. The approved host-side check passed immediately.

A bounded read-only review was sent to OpenCode Go's
`deepseek-v4-flash`. OpenCode would not run the configured `explore` subagent as
a primary CLI agent, fell back to `build`, and produced no review after several
minutes, so I terminated it. No result from that attempt was used to claim
completion.

## Deviations from the plan

The scaffold is Next.js 16.2.3 rather than the Next.js 15 named in
`ARCHITECTURE.md`, for the reason recorded above. The current create-next-app
template generates `next.config.ts` rather than `next.config.mjs`; the documented
rewrite is unchanged in substance. No product functionality from a later phase
was built.

## New API knowledge

None. The live responses matched `INTEGRATION.md`: readiness returned all checks
`ok`, the acceptance session route minted an owner token, and the empty properties
read returned HTTP 200 with `{"items":[],"next_cursor":null}`.

## How to verify (human runs these)

The backend and Next development server are already running. Verify:

```bash
curl -s http://127.0.0.1:8080/health/ready
```

Expected: JSON with `"status":"ok"` and database, MinIO, and model all `ok`.

Open `http://localhost:3000/debug` in the browser.

Expected: the page settles on an authenticated properties JSON response,
currently `{"items":[],"next_cursor":null}`. It must not show a CORS error or
401. In DevTools Network, both requests should target `localhost:3000/api/...`,
not browser-direct `:8080` URLs.

Optional repeatable code checks:

```bash
cd ~/comfort-curators-frontend/app
npm run lint
npm run build
```

Expected: both exit zero.

## Open questions for the human

None.

## What's next

After the human confirms Phase 0, begin Phase 0.5 only. Install the four local
Fontsource packages plus Lenis, then implement the exact tokens, sharp-corner
rules, grain, motion curves, reduced-motion handling, custom difference cursor,
and `/debug` style sheet from `ART-DIRECTION.md` and `INTERACTION.md`.
