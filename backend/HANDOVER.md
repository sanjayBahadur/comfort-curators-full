# Handover — Comfort Curators production push, orchestrator continuation

**Written:** 2026-08-09, by a Claude Sonnet 5 orchestrator session, for
whoever (human or AI) picks this up next. This assumes zero prior context —
read this in full before touching anything.

> **Update, 2026-08-10 — this document's "Current state" and "What's left"
> sections below are now stale.** Everything Section 3 lists as remaining
> (Wave 3 P4.5–P4.9, all of Wave 4) is done. A full additional day was then
> spent wiring Superhost to a real model provider (it had never been tested
> against anything but the model-stub before), which surfaced and fixed a
> long chain of real bugs the stub had been masking, plus real token
> streaming, portfolio-scoped threads, per-account memory, and a round of
> demo-readiness UI polish found by literally clicking through the app.
> None of that was planned in `ORCHESTRATION.md` — same pattern this
> document's own §5 already describes for `P3.9`/`P3.10`: found live, not
> spec'd ahead of time. Full detail:
> [`logs/phase-2026-08-10-superhost-live-and-demo-polish.md`](logs/phase-2026-08-10-superhost-live-and-demo-polish.md)
> (backend) and the frontend repo's
> `logs/phase-2026-08-10-superhost-ui-and-demo-polish.md` (same date, frontend
> side). Read those before assuming anything below reflects the current
> branch HEAD — Section 3 is left as-written for historical continuity
> rather than rewritten in place.

## 1. What this project is

"Comfort Curators" is a property-management platform being pushed toward an
investor-demo-ready state. There are **two real repos** on this machine (a
much larger number of `comfort-curators-*`-named directories exist; only
these two are live):

- **Backend** — `/home/tatakae/open-code-projects/comfort-curators-backend-alt`
  Go, branch `orch/p0-rename-jarvis-superhost` (not `main` — this is the
  working branch for this whole push; nothing has been merged to `main` yet).
  Remote: `https://github.com/sanjayBahadur/comfort-curators-backend-alt.git`.
- **Frontend** — `/home/tatakae/open-code-projects/ComfortCurators`, project
  root is `app/` inside it. Branch `v1.1`.
  Remote: `git@github.com:sanjayBahadur/ComfortCurators.git`.

The work is driven by two documents on the **frontend repo root**:
`IMPLEMENTATION-SPEC.md` (the technical spec, defect register DEF-01
through DEF-12, decisions D1-D6, section numbers referenced throughout this
doc) and `ORCHESTRATION.md` (the wave/block breakdown, §5 has the
authoritative block table, §6 has the orchestrator's own operating
instructions). Also useful: `SCREENS.md` (per-route UI spec),
`ART-DIRECTION.md` (design system, §14 is the Superhost terminal exception),
`PRD.md` (product narrative).

**The user is Sanjay** (`mallasanjay2099@gmail.com`), pitching investors.
He explicitly wants to be told about important decisions or phase
completions, not narrated every small step — work autonomously through the
block list, surface real findings, keep him informed at milestones.

## 2. The orchestration mechanism — how work actually gets done

This is **not** a normal "write code yourself" session. Both repos have a
`sandcastle`-based dispatch system (`@ai-hero/sandcastle` npm package) that
farms out implementation work to cheaper models running in isolated Docker
sandboxes, while the orchestrator (you) reviews, verifies, fixes bugs, and
merges. This pattern is deliberate and has worked well — **keep using it**,
don't switch to writing everything yourself; that defeats the point and
will burn far more of your budget than necessary.

### 2.1 Dispatch commands

Backend (run from repo root):
```bash
cd /home/tatakae/open-code-projects/comfort-curators-backend-alt
npx tsx .sandcastle/main.mts <BLOCK-ID>   # e.g. P4.5
```

Frontend (run from `app/`, **not** repo root — the dispatcher script's
`existsSync` checks resolve relative to `process.cwd()`, unlike its
`import()` calls which resolve relative to the module's own location; get
this wrong and you'll get a false "missing prompt file" error even though
the file exists):
```bash
cd /home/tatakae/open-code-projects/ComfortCurators/app
npx tsx .sandcastle/main.ts <BLOCK-ID>
```

Both dispatchers:
1. Read `.sandcastle/blocks.json` (backend) / `app/.sandcastle/blocks.json`
   (frontend) for the block's tier → picks a model
   (`opencode-go/deepseek-v4-pro` for tier 1, `opencode-go/gpt-5.6-luna`
   for tier 2, `opencode-go/deepseek-v4-flash` for tier 3).
2. Require the block's prompt file to already exist at
   `.sandcastle/blocks/<ID>.md` — **you must write this prompt yourself
   before dispatching**, pulling the real spec section content, real
   backend/frontend contract details, and exact existing code
   signatures into the prompt text. The dispatched model's sandbox often
   **cannot reach the other repo's filesystem** (e.g. a frontend dispatch
   can't read backend Go source) — paste relevant contract/type
   information directly into the prompt rather than pointing at a
   cross-repo path, or the dispatched agent will just say so in its log
   and guess.
3. `branchStrategy: { type: "merge-to-head" }` — **this was deliberately
   changed from the sandcastle default** (`{ type: "head" }`, which writes
   directly to your host checkout with zero isolation and will corrupt
   concurrent dispatches). Every dispatch gets its own git worktree under
   `.sandcastle/worktrees/`, which self-merges back to the branch on
   success (or is left in place, uncommitted, if the agent runs out of
   turns before committing — see §3 below, this happens often and is
   *not* a failure).
4. Run in the background. `npx tsx ... &` then poll the real child PID
   (see §2.3 — the top-level `npm exec` wrapper PID is not reliable to
   wait on, it can exit while real work continues in a grandchild
   process).

### 2.2 The prompt file convention (write these yourself, every time)

Every `.sandcastle/blocks/<ID>.md` you write should have, in order:
- A `!\`node .sandcastle/blocks.mjs list\`` shell-expansion line (shows the
  live tracker state to the dispatched agent).
- A working-directory note if frontend (`cd app` before touching `src/...`).
- A log-path note (`logs/` at repo root, not `app/logs/`).
- What to read first, with exact file paths — the established files for
  that block's area, so the agent doesn't waste turns rediscovering
  context you already have.
- The rules-book pack for its tier: `.sandcastle/rules/<topic>/<topic>.mini.md`
  (tier 1-2) or `.nano.md` (tier 3) — pick a topic that fits (refactoring,
  clean-architecture, domain-driven-design, a-philosophy-of-software-design
  are the ones used so far).
- The task itself, with real contract/type information pasted in (not
  "go read X" if X is in the other repo).
- An explicit "Do not" section — what NOT to touch, especially shared
  files other blocks own this wave.
- A "Done" checklist ending in: write a phase log at
  `logs/phase-<ID-lowercased>-<slug>.md` with a fixed template (Date,
  Agent/model, Status, What I built, Files changed, Decisions,
  What did NOT work, Deviations, Open questions), then output
  `<promise>COMPLETE</promise>`.

Look at any `.sandcastle/blocks/P*.md` file already in either repo for a
concrete example — there are ~30 of them now, all following this shape.

### 2.3 Waiting on a dispatch correctly

```bash
nohup npx tsx .sandcastle/main.mts P4.5 > /tmp/.../p4.5-dispatch.log 2>&1 &
echo "pid $!"
sleep 6
ps aux | grep "main.mts P4.5" | grep -v grep   # find the REAL grandchild node PID, not $!
```
The dispatch log's first lines print `Dispatching...` and
`[Agent] Started on branch sandcastle/<timestamp>-<hash>`. Then wait on the
**deepest node PID** from the `ps aux` output (there are 2-3 layers: `npm
exec` → `tsx` loader → the actual long-running node process), not the PID
bash gave you from `$!` — that top-level wrapper can exit while the real
work continues, and waiting on the wrong PID makes you think a dispatch
finished when it didn't.

```bash
while kill -0 <real-pid> 2>/dev/null; do sleep 15; done; echo DONE
```
Run that as a **background** shell command (this environment has a
mechanism for backgrounding long waits and getting notified on
completion — use it; don't block synchronously on multi-minute Tier-1
dispatches).

### 2.4 After a dispatch finishes — the mandatory verification pipeline

**Never trust a dispatched agent's own "Status: complete" + "all tests
pass" claim at face value.** Every single dispatch sandbox this whole
session had **no reachable Postgres**, so any claim of passing
database-backed tests was actually a `t.Skip()` false positive. This
produced multiple real, shipped bugs that only surfaced under genuine
verification. The pipeline that caught them, every time, without
exception:

1. Check `git worktree list` and `git log --oneline` — did it self-commit
   and self-merge (fast-forward, shows up in the branch's log), or is it
   sitting uncommitted in its own worktree (`git status --short` in that
   worktree dir)? Both outcomes are common and both are fine — if
   uncommitted, you finish the commit yourself after verifying.
2. **Read the diff and the phase log fully** before running anything —
   you will often catch real bugs by inspection alone (wrong types, a
   guessed field name, an obviously-wrong condition) before ever touching
   a terminal.
3. **Backend: spin up a real throwaway Postgres**, never touch the shared
   dev stack for destructive testing:
   ```bash
   docker run -d --name cc-verify-pgN -e POSTGRES_PASSWORD=postgres \
     -e POSTGRES_DB=comfort_curators -p 1544N:5432 postgres:16-alpine
   # wait for pg_isready, then:
   export CC_DB_HOST=127.0.0.1 CC_DB_PORT=1544N CC_DB_USER=postgres \
          CC_DB_PASS=postgres CC_DB_NAME=comfort_curators
   go build ./... && go vet ./... && go test -p 1 ./internal/automation/... \
     ./internal/platform/app/... ./internal/procurement/...
   docker rm -f cc-verify-pgN   # always clean up
   ```
   **Critical gotcha**: `internal/automation`'s test helpers read
   `CC_DB_HOST`/`CC_DB_PORT`/`CC_DB_USER`/`CC_DB_PASS`/`CC_DB_NAME`
   (defaults: `localhost:5432`, `ccuser`/`ccpass`), **not**
   `CC_DATABASE_URL`. Setting the wrong env var silently makes tests fall
   through to whatever's on `localhost:5432` — which is the shared,
   16+-hour-old dev Postgres — producing misleading results without any
   error. This was discovered mid-session after several earlier
   "verifications" had actually been running against the shared DB by
   accident.
   **Also**: always use `-p 1` on `go test` invocations that touch
   multiple packages under the same throwaway Postgres — without it, Go
   runs packages in parallel and they race against the shared DB,
   producing false failures that look exactly like real bugs (this
   specific false-failure signature — `reservations_feed_id_fkey`
   violations, migration/lease tests failing when they pass in
   isolation — recurred *many* times before being fully understood; if
   you see it again, re-run with `-p 1` before concluding anything).
4. **Frontend**: `cd app && npm run build && npm run lint` (oxlint) — this
   has been sufficient; there's no live-backend integration test suite on
   the frontend side, so cross-checking against real backend contracts
   during prompt-writing (§2.2) is the main defense against wrong field
   names.
5. If you found and fixed a bug: write a `logs/phase-<ID>-<slug>-review.md`
   addendum documenting exactly what was wrong, why, and how you verified
   the fix — this repo has a strong convention of honest, traceable
   review logs; keep it up. Then commit (crediting the dispatched model
   in the commit body, noting your fix), merge to the working branch,
   `git worktree remove ... --force`, and
   `node .sandcastle/blocks.mjs close <ID>` (backend) /
   `node app/.sandcastle/blocks.mjs close <ID>` (frontend) — this just
   verifies the phase log exists with `Status: complete`, it doesn't
   mutate anything, safe to run any time.

**Track record this session**: this pipeline caught real, shipped bugs in
roughly 70% of Tier-1/Tier-2 blocks — API-key leak paths, loop-control bugs
that silently corrupted state, wrong-order test setup that hid a fully
working implementation behind a false failure, a UX bug that permanently
locked users out after a transient network error, an accessibility fix that
introduced a blank-text regression, wrong guessed field names corrected
against real backend structs. Do not skip steps to save time — every
skipped step so far has hidden a real bug.

## 3. Current state — what's actually done

### Backend (`orch/p0-rename-jarvis-superhost`, HEAD `4c6e356`)

**Wave 0** (Jarvis→Superhost rename) and **Wave 2 backend track** (P3.0
through P3.10) are **fully complete and verified**. This includes two
blocks (**P3.9**, **P3.10**) that were **not in the original
`ORCHESTRATION.md` plan** — the orchestrator found real gaps mid-session
and added them:

- **P3.9** — `internal/procurement/store/` (built in P3.8) was never wired
  to HTTP. Added `GET /v1/store/catalog`, `POST /v1/store/quotes`,
  `POST /v1/store/orders` (`internal/procurement/store/handler.go`).
- **P3.10** — there was no way to decide a pending Superhost approval or
  resume the paused conversation (P3.3/P3.4 had both flagged this and left
  it unassigned). Added `POST /v1/superhost/approvals/{request_id}/decide`,
  a `messages_json` checkpoint column on `agent_runs`, and a real resume
  path in `runner.go`. **This is the fix that makes the demo's central
  moment — agent proposes → pauses → human confirms → agent continues —
  actually work end to end.** Verified with a full round-trip test.

Full block list and what each did: `git log --oneline` on the branch, or
read `logs/phase-P3-*.md` (each has a paired `-review.md` if the
orchestrator found and fixed something). `.sandcastle/blocks.json` has the
tracker; `node .sandcastle/blocks.mjs list` shows what's still open (as of
this writing: only Wave 0's original P0.1-P0.4, which is a **cross-repo
tracker cosmetic quirk** — the backend's own separate historical tracker
already shows those closed; this doesn't reflect real unfinished work,
ignore it).

**No remaining backend work is currently planned** — Wave 3/4 are frontend-
only per `ORCHESTRATION.md`. If frontend work (below) surfaces a new
backend gap the same way P3.9/P3.10 did, the pattern is: write a prompt,
add a `blocks.json` entry (backend's file doesn't auto-generate from
`ORCHESTRATION.md`, you edit it by hand — follow the existing entry shape),
dispatch, verify hard, merge.

**Dev stack**: `docker compose` stack is running
(`comfort-curators-backend-alt-{api,worker,postgres,minio,model-stub}-1`
containers) but **the `api`/`worker` containers are stale** — built from
source *before* today's P3.1-P3.10 work landed. If you need to manually
test against a live server (as opposed to the throwaway-Postgres unit-test
pattern above), **rebuild first**:
```bash
cd /home/tatakae/open-code-projects/comfort-curators-backend-alt
docker compose build api worker && docker compose up -d api worker
```
(A rebuild earlier this session hit a real migration-checksum-drift issue
from test pollution in `tests/database_integration_test.go` — if you hit
`schema_migrations` checksum errors on startup after a rebuild, that's a
known, separate, already-diagnosed issue; check `logs/` or ask the user
before doing anything destructive to the shared dev Postgres.)

### Frontend (`v1.1`, HEAD `1deab52`)

**Wave 0, Wave 1, all of Wave 2 (P2.1-P2.5 routing + P5.1-P5.6 missing
surfaces), and P4.1-P4.4 of Wave 3** are done and verified. Specifically
landed this session: `/ops/calendar`, `/ops/properties`, `/ops/workers`,
`/properties/:propertyId`, `/invoices`, `/documents`, `/stay` (full guest
portal + real store), the Superhost terminal shell
(`components/superhost/Terminal.tsx`), its SSE client + typewriter reveal
(`behavior.ts`, `lib/api/superhost-stream.ts`), `ConfirmBlock` (bound to
the real P3.10 decide endpoint), and a verified denial-rendering +
accessibility pass.

`npm run dev` is running on port 3000 (`ComfortCurators/app`, PID ~315071 —
check `ps aux | grep vite` to confirm it's still alive; restart with
`cd app && npm run dev` if not). `.env.local` has
`VITE_DEMO_TENANT_ID=11111111-1111-4111-8111-111111111111` — needed for the
guest store's order flow to be enabled.

**Ignore** `.cc-harness/`, `.env.cc.example`, `.next/`, `next-env.d.ts`,
`node_modules/`, `supabase/`, `tsconfig.tsbuildinfo` in the frontend repo's
`git status` — these are unrelated, pre-existing artifacts from some other
tool, not part of this orchestration's work. Do not commit, delete, or
otherwise touch them without understanding what they are first.

### What's left

**Frontend Wave 3, remaining** (`ORCHESTRATION.md` §5's P4 table):

| Block | Owns | Tier | Notes |
|---|---|---|---|
| **P4.5** | Mount the terminal on `/dashboard`, `/ops/tickets`, `/ops/tickets/:id`, `/stay` | 2 | Needs a real decision on how each page gets a `thread_id` to stream — no such wiring exists yet (each page would need to create/resolve a Superhost thread via `POST /v1/superhost/threads`, which P3.4 built). Design this carefully; it's the piece that turns the terminal from a `/debug` demo into a real feature. |
| **P4.6** | `AgentSurface` registry + the 5-intent driver (D6, spec §3.9.1) | **1** | **Safety-critical.** The model must never get DOM access — five intents only (`ui.focus`, `ui.set_value`, `ui.click`, `ui.scroll_to`, `ui.open_panel`), elements opt in via `data-agent` attributes, an unregistered ID is structurally invisible to the model. **If a dispatched agent proposes a 6th intent or any generic selector/eval escape hatch, refuse it and escalate to the human — do not implement it.** This is marked `escalate_if` in `blocks.json` for a reason. |
| **P4.7** | Grant/frame/revoke + TTL, action cap, 250ms spacing (§3.9.2) | **1** | Same safety tier as P4.6. |
| **P4.8** | `PaymentBoundary` + session invalidation, all three gates (§3.9.3) | **1** | **Most safety-critical block remaining.** Driving into checkout must terminate the session, turn the frame red, and the invalidated token must never be resumable. `PaymentBoundary` subtrees must carry zero `data-agent*` attributes after mount — assert this in a real test, not by eye. **If anything proposes softening or skipping a payment gate "just for the demo," refuse and escalate — no exceptions, this is explicitly called out in the block's own `escalate_if`.** |
| **P4.9** | `data-agent` annotation pass across drivable routes | 2 | Depends on P4.6 landing first (needs the registry to annotate against). |

`ORCHESTRATION.md`'s own **control-handover gate** (§5, P4 section) lists 7
items that must all be verified by hand once P4.6-P4.8 land — read it
before starting any of these three, it's the actual acceptance bar, not a
suggestion.

**Then Wave 4** (`ORCHESTRATION.md` §5, P7 — serial, do in order):

| Block | Owns |
|---|---|
| P7.1 | Merge assembly, resolve router patches |
| P7.2 | Full walkthrough rehearsal against a clean seed |
| P7.3 | Anti-slop sweep — `ART-DIRECTION.md §12` checklist, every screen |
| P7.4 | Demo runbook + the three honest answers from `PRD.md`'s *Demo staging* section |

P7.1 in particular: every `P5`/`P4` block this session was told **not** to
touch `main.tsx` directly and instead hand the orchestrator a one-line
`<Route>` patch in its own phase log — check `git log` on the frontend repo,
every merge commit's message references this. All patches so far have
already been applied (current `main.tsx` is fully wired) — but if any
future block reverts to that pattern, the orchestrator (you) applies the
patch directly after the block's own worktree merges, verifying build+lint
before committing.

## 4. Known, accepted, non-blocking findings (don't re-litigate these)

Documented and consciously not fixed, for good reason — re-reading these
before "fixing" them again will save time:

- **`TestRestartRecoversExpiredLease`** occasionally showed as failing
  early in the session; root-caused to a pre-existing bug that predates
  this whole push (reproduced identically checking out a commit from
  before any of this session's work). It has since passed cleanly in every
  later verification run on a fresh throwaway Postgres — likely
  environment/timing-sensitive against the previously-stale shared dev DB,
  not a real code bug. Not something to chase further unless it recurs.
- **`ThreadStore.CreateThread`'s narrow concurrency race** (P3.4's review
  log): two genuinely concurrent requests with the same idempotency key
  could hit a generic 500 instead of gracefully returning the existing
  thread. Sequential retries (the realistic case) work correctly. Flagged,
  not fixed — low priority.
- **`Claim()`'s `attempt` counter conflation with approval-resume** (P3.10's
  review log): resuming a run after approval increments the same counter
  used for retry-limit tracking. Harmless at the default `max_attempts=3`
  for one or two approval round-trips; a run needing many sequential
  approvals could hit the cap prematurely on an unrelated later failure.
  The failure direction is safe (fails slightly early, never retries past
  a real limit). Not fixed.
- **Non-atomic `SaveMessages` + `TransitionState`** (P3.10): a narrow
  window if the DB write between them is interrupted. Same risk class as
  several other multi-statement (non-transactional) writes already
  established elsewhere in `store.go` — consistent with the codebase's
  existing tolerance for this pattern.
- **The backend's `internal/automation/superhost` package's long-standing
  `reservations_feed_id_fkey` test failures** were a real bug (a shared
  test helper, `seedReservation` in `context_test.go`, referenced a
  hardcoded `feed_id` that was never actually seeded) — **this was found
  and fixed** during P3.4's review pass. If you see this exact error
  again, something regressed; it should not currently be happening.

## 5. If you find a new backend gap while doing frontend work

This happened twice this session (P3.9, P3.10) and is very likely to
happen again in P4.5-P4.9 (control-handover almost certainly needs *some*
backend support for session invalidation in P4.8, if nothing else already
covers it — check first, don't assume). The pattern that worked:

1. Verify it's real by reading actual backend source (`grep -rn`,
   `mux.HandleFunc`), not by assuming from the spec.
2. Write a normal block prompt on the backend repo, add a `blocks.json`
   entry (there's no `PX.Y` numbering rule beyond "doesn't collide" — the
   `P3.9`/`P3.10` precedent used the next free number in that wave).
3. If it's genuinely architecturally significant (changes how a core flow
   works, adds real scope beyond "wire an existing thing to HTTP") —
   **ask the user first**, the way P3.10 was preceded by an
   `AskUserQuestion` call laying out the tradeoff (build it properly vs. a
   minimal stub vs. defer). Don't silently take on a large scope increase.
4. Dispatch, verify hard (§2.4), merge, then continue the frontend block
   that was blocked on it.

## 6. Everything else you need is in the repos themselves

- `IMPLEMENTATION-SPEC.md`, `ORCHESTRATION.md`, `SCREENS.md`,
  `ART-DIRECTION.md`, `PRD.md`, `SHOP.md`, `INTEGRATION.md` — frontend repo
  root.
- `contracts/api/superhost-stream.yaml` — backend repo, the frozen SSE
  event contract (P3.0). Read this in full before touching P4.5.
- `AGENTS.md` — backend repo, the standing rules every dispatched agent is
  told to read first (notably: never touch `contracts/` without recorded
  human sign-off).
- `.sandcastle/rules/*/` — the tier-appropriate "rules book" packs handed
  to dispatched agents.
- `logs/phase-*.md` (both repos) — the full paper trail of every block,
  in order. This is the actual history; trust it over any summary
  (including this one) if they ever disagree.

Good luck. The pipeline works — the main risk from here is skipping
verification steps to move faster, which has cost real time every time it's
been tried.
