# Comfort Curators — Frontend Build Pack

Everything needed to build the Comfort Curators frontend, for handoff to any
coding agent (ChatGPT / Codex, Claude, Cursor, …).

**Built one phase at a time, with a manual human check between each.**

## Read in this order

| # | File | What it is |
|---|---|---|
| 1 | **`POLICY.md`** | Binding rules. Dev logs, stop-at-phase-boundary, honesty. **Read first.** |
| 2 | **`SETUP.md`** | Exact commands: backend, scaffold, deps, env, seed data, the iCal feed |
| 3 | **`PHASES.md`** | 9 phases, each with a 2-minute human acceptance test |
| 4 | **`ARCHITECTURE.md`** | Stack, structure, API client, auth — and the CORS blocker + its fix |
| 5 | **`INTEGRATION.md`** | Every endpoint, **verified live** against the running backend |
| 6 | **`PRD.md`** | What each screen is for, and why |
| 7 | **`ART-DIRECTION.md`** | ⭐ Visual system. Punk-collage × high-fashion. **Design is the product.** |
| 8 | **`INTERACTION.md`** | ⭐ Forensic study of the reference sites: real easing curves, libraries, signature moves |
| 9 | **`SHOP.md`** | ⭐ The inventory shop + drag-to-cart — the centrepiece screen |
| 10 | **`SCREENS.md`** | Every other route, its data source and three states |
| 11 | **`SKILLS.md`** | Working practices, adapted from mattpocock/skills to need no tooling |
| — | `logs/` | One dev log per phase. **Mandatory.** |

## The four things that will cost you an hour each

1. **`CC_BUILD_TAGS=acceptance`** when starting the backend, or `/auth/session/create`
   is not compiled in and nothing can log in.
2. **No CORS on the backend**, and its `OPTIONS` preflight returns 401. Use the
   Vite dev proxy (`ARCHITECTURE.md §1`). Do not call `:8080` directly
   from the browser.
3. **No database seed exists.** Every screen is empty until `scripts/seed.ts`
   (Phase 2) runs.
4. **Reservations cannot be created directly.** They only exist when the backend
   *fetches* an iCal URL. Serve `public/demo.ics` and register the feed at
   `http://host.docker.internal:3000/demo.ics` — verified reachable from the API
   container. Without this the reservation → turnover → ticket chain is dead
   (`SETUP.md §6`).

## Backend

`~/open-code-projects/comfort-curators-backend-alt` — Go, `:8080`, 340 routes.

> **Update, 2026-08-10:** the "halted, do not modify" line below describes an
> earlier checkpoint on `main`. The branch this frontend actually integrates
> against, `orch/p0-rename-jarvis-superhost`, is under active development —
> most recently, wiring Superhost to a real model provider and real token
> streaming. See that repo's
> `logs/phase-2026-08-10-superhost-live-and-demo-polish.md` and this repo's
> own `logs/phase-2026-08-10-superhost-ui-and-demo-polish.md` for what
> changed. `POLICY.md §3`'s original constraint (below) still applies to
> `main`; it does not describe the working branch.

**Halted and verified working. Do not modify it** (`POLICY.md §3`).

```bash
cd ~/open-code-projects/comfort-curators-backend-alt
export CC_BUILD_TAGS=acceptance
docker compose up -d --build api worker postgres minio model-stub
curl -s localhost:8080/health/ready     # {"status":"ok",...}
```

## Start here

Read `POLICY.md`, then find the first phase in `PHASES.md` with no matching log
in `logs/`. That is your phase. Build it, log it, stop.
