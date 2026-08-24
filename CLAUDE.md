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

- **Tests currently write to whatever Postgres is on `localhost:5432`**,
  including `TRUNCATE` and a `schema_migrations` checksum poisoning fixture
  (`tests/database_integration_test.go:154`) that will refuse to let the
  API boot afterwards. **Do not run `go test ./...` against a machine with
  the stack up until `P1-02` lands.** Recovery: reset the version-4
  checksum in `schema_migrations`.
- **Schema changes do not apply.** 147 tables are created by
  `CREATE TABLE IF NOT EXISTS` in per-module `EnsureSchema` functions, so
  altering an existing table is silently skipped. `P1-04` fixes this;
  until then the only reset is `docker compose down -v`.
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
