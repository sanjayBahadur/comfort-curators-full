# Comfort Curators — working agreement

Short-term-rental property operations in Noida, UP. Owners hand over their
STR; we run it with our own inventory, our own field staff, and
**Superhost**, an AI property supervisor.

**This is the canonical repository.** Consolidated 24 Aug 2026 from
`comfort-curators-backend-alt` and `ComfortCurators`, both of which are
archived at `~/archive/comfort-curators/` with a README explaining the
verification that preceded it. Do not resume work in those checkouts.

```
backend/    Go modular monolith — API + worker binaries, PostgreSQL, MinIO
frontend/   React + Vite SPA — owner, ops, curator and guest surfaces
```

Planning documents live outside the repo at `~/comfort-curators-scratch-pad/`
(compliance status, feature disposition, agent architecture, infrastructure,
task board).

---

## Settled decisions — do not re-litigate without new information

| ID | Decision |
|---|---|
| **D-01** | `comfort-curators-full` is canonical. Mainline is `main` |
| **D-06** | The agent gets **no** authority over money or access secrets, permanently, regardless of measured performance. Enforcement stays in *which tables the agent may write to* — never move it into relaxable policy config |
| **D-07** | **Manual first.** Operate ~a month with a supervisor dispatching by hand, then automate what proved boring. Mode A (control session) stays live throughout |
| **D-08** | The overall supervisor is a **deterministic reconciler** — cron plus SQL invariant checks — not a governing agent. Escalate to a model only when a check fails |

Still open: worker classification (D-02), whether rental income passes
through the company account (D-03), LLM hosting provider and region
(D-04), dropping continuous person-tracking (D-05).

---

## Architectural invariants

These are enforced in code and must not be regressed. Several have tests
that fail if they are broken; treat a failure as a stop, not a flake.

1. **The agent proposes; a human or deterministic code decides.** Superhost
   writes only to `ai_tool_calls`, `policy_decisions` and
   `approval_requests` — never to a business table.
2. **Tool arguments may never widen the run context.** Tenant, property and
   actor derive from the run, not from what the model asked for. See
   `internal/automation/hermes/policy.go`.
3. **Everything the agent can do, a human can do directly.** The agent is a
   faster path, never the only path.
4. **Money is integer minor units.** No float arithmetic, anywhere.
5. **Every query is tenant-scoped.** Until row-level security lands
   (`P2-06`), this is by hand and by review.
6. **Location collection is custody-gated.** Enabled only while a worker
   holds an active asset custody; off-duty collection is refused and
   audited. Requirements `VEH-009`, `WFM-009`. Do not extend this to
   person-tracking.
7. **Never build Aadhaar or biometric attendance.** Private employers
   cannot compel Aadhaar authentication.
8. **Four tables are append-only ledgers**, enforced by database triggers
   that raise on `UPDATE` and `DELETE`: `audit_events`,
   `inventory_movements`, `onboarding_inspections`,
   `service_contract_versions`. Corrections are new compensating rows, never
   edits. Do not disable these triggers to make a cleanup or a migration
   convenient — an audit trail you can quietly delete from is not one, and
   the same applies to a stock ledger. If a schema change genuinely needs
   the data rewritten, do it as an explicit, reviewed migration that records
   what it did.

## Workforce controls that must not regress

The workforce test suite encodes labour rules as enforced behaviour. These
are a genuine differentiator and the thing most likely to be broken by
accident during the auth, roles and dispatch rework:

- age eligibility checked before assignment
- restricted work requires certification or a specialist vendor
- adverse action is human-reviewed and evidenced
- a rating score cannot deactivate a worker (no algorithmic firing)
- pay treatment is shown before the worker accepts
- high-risk jobs require two people
- dispatch overrides require a reason and attribution

---

## Known traps

- ~~Tests write to whatever Postgres is on `localhost:5432`~~ **Closed by
  `P1-02`.** Tests now resolve their target through
  `internal/platform/testdb`, default to `comfort_curators_test`, and
  **panic** on any database whose name does not end in `_test`. Running
  `go test ./...` with the stack up is safe and verified: the checksum
  poisoning fixture in `tests/database_integration_test.go` runs and
  writes `deadbeef…` to the *test* database, leaving the application
  database byte-identical. Never weaken `testdb.ValidateName`, and never
  turn its failure into a `t.Skip`.
- **Test-safety audits must cover `.go`, not just `_test.go`.** The first
  pass of `P1-02` rewrote all 36 `*_test.go` files and still left the worst
  offender in place: `tests/acceptance/probes.go` is an ordinary `.go` file
  in a `package main` that carries test files, so `go test ./...` ran it and
  its capacity probe wrote 50,000 tickets and 100,000 inventory movements
  into `comfort_curators` on every run. Live-stack probes are now gated
  behind `CC_ACCEPTANCE_LIVE=1` plus an explicit guarded `CC_DB_NAME`.
- **1.2M synthetic `tenant-capacity-*` rows remain in `inventory_movements`**
  and ~144 in `audit_events`. The other 614,547 leaked rows were deleted;
  these two cannot be, because both are append-only ledgers (invariant 8) and
  the triggers were correctly left in place. They are tenant-scoped, so real
  queries filter them out. `docker compose down -v` plus a reseed is the only
  clean removal — which would also fix the thin catalog (1 item against the
  62 the seed describes).
- **The suite is red: 59 failing tests across 7 packages** (`P1-03`).
  Down from 104 in 18 packages. Not a regression from `P1-02` — the same
  failures reproduce against a database cloned from the seeded application
  database, so they are genuine. The earlier "~30 failing" figure was an
  undercount from a run that aborted partway. Until `P1-03` lands, a red
  suite says nothing new; do not treat it as a signal.
- **`workers.user_id` now exists** (migration 006) but **nothing populates
  or reads it yet**, and there is still no `/me` endpoint. Treat a null
  `user_id` as "this worker cannot sign in yet" — never as "any session
  matches". Wiring it up is the remaining work in `P2-04`/`P2-05`.
- ~~Schema changes do not apply~~ **Closed by `P1-04`.** The module schema
  is migration `005_module_schema_baseline.sql`, generated from the real
  startup path by `TestGenerateSchemaBaseline` and verified byte-identical
  to a live database. Add a column with an ordinary forward migration — see
  `006_workers_user_id.sql`. A migration may declare
  `-- baseline-if-exists: <table>` to be recorded rather than executed on a
  database that already has the schema; **only** use that marker for a
  genuine baseline. Regenerate the baseline, never hand-edit it.
- **`EnsureSchema` still runs at startup**, so the schema has two sources of
  truth. It is idempotent and harmless, but new tables must go in a
  migration, not in an `EnsureSchema` body, or they will not exist on a
  database built from migrations alone. Retiring the 25 calls in
  `initializeSchema` is outstanding.
- **There is no production login.** The frontend's only sign-in calls
  `/auth/session/create`, which exists only under the `acceptance` build
  tag. A default build has no working auth (`P2-01`, `P2-02`).
- **`getCuratorJobs()` fetches the whole tenant ticket queue** and filters
  client-side, so every curator sees every property's access method
  (`P2-04`).
- **The backend sends no CORS headers** and its OPTIONS preflight 401s.
  Production must be same-origin routed.

## Running it

```bash
cd backend && docker compose up -d --build   # api, worker, postgres, minio, model-stub
curl http://localhost:8080/health/ready

cd frontend/app && npm install && npm run dev # http://localhost:3000
npm run seed                                  # demo data, idempotent
```

The demo login route requires `CC_BUILD_TAGS=acceptance` at build time.
A plain build does not compile it in — that is deliberate.

## Commit conventions

Conventional commits. Reference task IDs from
`~/comfort-curators-scratch-pad/06-tasks.md` where one applies, e.g.
`fix(iam): scope curator jobs to the assigned worker (P2-04)`.
