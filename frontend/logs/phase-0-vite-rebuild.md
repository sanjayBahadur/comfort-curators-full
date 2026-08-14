# Phase 0 — Environment and the proxy proof (Vite)

- **Date:** 2026-08-08
- **Agent/model:** Claude Opus 5
- **Post-handoff audit:** GPT-5.6 Codex orchestrating DeepSeek V4 Flash,
  DeepSeek V4 Pro, Qwen 3.7 Plus, and GPT-5.6 Luna
- **Status:** complete

Supersedes `phase-0-environment-proxy.md` (Next.js), which is kept intact as the
record of what happened, per `POLICY.md §1`.

## What I built

Re-scaffolded the app on Vite 8 + React 19 + React Router 7 + TanStack Query
after the stack decision, and re-proved Phase 0 end to end against the live
backend: the app serves on :3000, the same-origin `/api` proxy reaches the Go
backend, an unauthenticated call 401s (proving it proxies rather than 404s), and
an authenticated call returns 200. Also proved the backend **container** can
fetch `/demo.ics` from the dev server, which Phase 2 depends on.

## Files added or changed

`app/` — `npm create vite@latest --template react-ts`. Previous Next.js app moved
to `app-next-superseded/` (not deleted — see Open questions).

`app/vite.config.ts` — proxy, plus three load-bearing server settings (below).

`app/src/lib/api/client.ts` — `api()`, `ApiError`, `Envelope<T>`, `unwrap()`.

`app/src/lib/auth/session.ts` — `signIn` / `getToken` / `signOut`; token in
memory with a `sessionStorage` fallback.

`app/src/routes/debug.tsx` — the Phase 0 proof. Signs in, then reads
`/api/v1/properties` through the proxy. **Delete before the demo.**

`app/src/main.tsx` — Query + Router providers, `/` and `/debug`.

`app/src/routes/home.tsx` — keeps the minimal home route out of the entry module
so the React fast-refresh lint rule passes cleanly.

`app/src/index.css` — Tailwind 4 import + paper base only. Full tokens are Phase 0.5.

`app/public/demo.ics` — two reservations, for Phase 2.

`app/.env.local`, `app/.env.example` — `VITE_DEMO_TENANT_ID`.

`app/README.md`, `app/index.html` — project-specific local instructions and
application title, replacing Vite template copy.

`PHASES.md`, `SETUP.md`, `ARCHITECTURE.md` — removed stale Next.js/auth-route
language left by the migration and documented the preview proxy.

## Decisions I made

The API origin lives in the Vite proxy, **not** in a `VITE_` env var. Anything
prefixed `VITE_` is inlined into the client bundle; keeping the origin in the
proxy means the browser only ever sees same-origin `/api/*`.

`strictPort: true` — a silent fallback to 3001 would break the iCal feed URL in a
way that is hard to diagnose. Better to fail loudly.

## What did NOT work

**`server.host: true` is required and its absence fails silently.** Vite binds to
localhost by default, so the backend container's fetch of
`http://host.docker.internal:3000/demo.ics` was refused at the TCP level. My
earlier reachability test used `python3 -m http.server --bind 0.0.0.0`, which
masked this. Phase 2's calendar poll would have returned zero reservations with
no obvious cause.

**`allowedHosts` is also required.** With `host: true` the connection succeeded
but Vite returned **403 Forbidden** — its DNS-rebinding protection rejects
unrecognised `Host` headers, and the backend arrives as `host.docker.internal`.

**`erasableSyntaxOnly` broke the documented `ApiError`.** The Vite react-ts
template enables it, which rejects TypeScript parameter properties
(`constructor(readonly status: number)`). The snippet in `ARCHITECTURE.md §4` had
exactly that form. Rewritten to explicit field assignment; the doc is corrected.

`pkill -f vite` killed the parent shell as well. Use `fuser -k 3000/tcp`.

A post-handoff audit found one Oxlint fast-refresh warning because `Home` lived
inside `src/main.tsx`; moving it to `src/routes/home.tsx` made lint clean. It
also found that the preview server lacked the same proxy/host settings as dev,
despite `ARCHITECTURE.md` requiring parity; the configuration now shares the
same API proxy and reachability settings.

OpenCode Go's Kimi K2.7 Code passed a minimal API smoke test but stalled after
repo-reading tool calls, so it was not used to approve code. Qwen 3.7 Plus was
used as the code-review fallback and returned `NO_BLOCKERS` on the repaired
files. OpenCode Go's Flash endpoint was transiently unavailable during one
audit, so the configured direct DeepSeek provider handled the cheap spec audit.
The main orchestrator checked all model claims against the files and live tests;
conditional or incorrect model findings were not accepted as facts.

## Deviations from the plan

The stack itself: `PHASES.md` said Next.js when this phase first ran. The docs
now say Vite and the code matches. Reasoning is in `logs/DECISIONS.md`.

Vite resolved to **8.2.1** and React to **19.2.4**. The post-handoff audit
updated `ARCHITECTURE.md` from the stale Vite 6 label to Vite 8. Dependencies
remain semver-ranged because the instruction is to use the current template.

## New API knowledge

None. Live responses matched `INTEGRATION.md`: health returned all three checks
`ok`, the acceptance route minted an owner token, and the empty properties read
returned `200` with `{"items":[],"next_cursor":null}`.

## How to verify (human runs these)

```bash
cd ~/comfort-curators-frontend/app && npm run dev     # must say :3000
curl -s http://127.0.0.1:8080/health/ready            # "status":"ok"
```

Open `http://localhost:3000/debug` — it must render
`{"items":[],"next_cursor":null}`, **not** a CORS error and **not** a 401. In
DevTools Network, every request must target `localhost:3000/api/...`, never
`:8080` directly.

```bash
# the Phase 2 dependency — must print BEGIN:VCALENDAR
cd ~/open-code-projects/comfort-curators-backend-alt
docker compose exec -T api sh -c 'wget -q -O - http://host.docker.internal:3000/demo.ics'

cd ~/comfort-curators-frontend/app && npm run build    # exits 0
cd ~/comfort-curators-frontend/app && npm run lint     # exits 0, no warnings
```

The dev proxy and the repaired production-preview proxy both returned 200 for
an authenticated properties request. A headless browser screenshot showed the
expected empty JSON, and GPT-5.6 Luna independently confirmed that no CORS, 401,
or runtime error was visible. All checks passed here on 2026-08-08.

## Open questions for the human

1. What should happen to `app-next-superseded/` (~700MB, including ~611MB of
   `node_modules`)?
   - (a) delete it after Phase 0 acceptance to reclaim space
   - (b) keep it as a local reference
   - **Recommendation:** (a); the Vite handoff preserves the relevant history,
     and the old dependency tree can be recreated if needed.
2. When should the zero-commit repository get its first rollback point?
   - (a) create the initial commit immediately after Phase 0 acceptance
   - (b) wait until Phase 0.5 is accepted
   - **Recommendation:** (a); the stack migration is a natural baseline before
     the visual system changes many files.

## What's next

**Phase 0.5 — Design foundation.** Install the four Fontsource packages, Lenis,
GSAP and SplitType, then implement tokens, sharp corners, grain, the
blend-difference cursor, the real easing curves, and the `/debug` style sheet
from `ART-DIRECTION.md` and `INTERACTION.md §8`. Get it signed off before Phase 1.
