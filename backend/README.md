# Comfort Curators — Backend

> A 120,455-line production-shaped Go backend for a property-operations business,
> written almost entirely by AI agents over four days for **$15.39** in inference.
>
> It boots. Its tests pass. Its auth holds. Read on for exactly what that does
> and does not mean.

**Status: HALTED 2026-08-08.** Feature-complete enough to demo, deliberately
paused before refactoring. See [Status](#status) for what that means and
[Known gaps](#known-gaps) for what an engineer picking this up must know.

---

## What it is

The operational core of Comfort Curators: a managed service that runs short-term
rental properties on behalf of homeowners — onboarding, inventory packages,
turnover operations, field dispatch, documents, compliance, and billing — with
an AI property supervisor (**Jarvis**) coordinating the work.

Go modular monolith. Two binaries (`cmd/api`, `cmd/worker`). PostgreSQL as
transactional truth. **One direct dependency** — `pgx`. No web framework; routing
is stdlib `net/http` with Go 1.22 method patterns.

## How it was built

This repository is an artifact of autonomous AI development. A deterministic
orchestrator decomposed a frozen product specification into a task graph, then
dispatched each task to a branch-isolated coding agent and an independent
reviewer agent on a different model, gating every merge behind deterministic
verification.

| | |
|---|---|
| **Total inference cost** | **$15.39 USD** |
| Tokens | 777,784,543 |
| API requests | 8,476 |
| Builder model | `deepseek-v4-pro` — 5,189 requests / 397.9M tokens |
| Reviewer model | `deepseek-v4-flash` — 3,287 requests / 379.9M tokens |
| Wall-clock | Aug 2–5, 2026 |
| Commits in this repo | 210 (Aug 4 → Aug 8) |

That budget covers the full multi-attempt arc, including two earlier rigs that
were abandoned. This repository is the one that survived.

**≈ 7,800 lines of retained, tested, reviewed Go per dollar.**

The inverse number is the honest one: **~6,500 tokens burned per line of code
that survived.** Autonomous development is not cheap per attempt — it is cheap
per *outcome*, because the attempts are.

### What the pipeline actually enforced

- **Branch-per-task isolation** — `orch/p<N>-<slug>.b1` build branch → `.integrate` merge.
- **Independent review** — the reviewer ran a different model family and had no
  write access. A builder could not approve its own work.
- **Deterministic gates** — `gofmt`, `go vet`, `go test -race`, migration lint,
  OpenAPI conformance, and a diff-scope check, all run by the controller, never
  self-reported by a model.
- **Evidence binding** — every merge carries commit trailers recording task,
  phase, attempt, agent, reviewer, and verdict.
- **Seven phase gates**, each requiring a clean boot from empty volumes.

## Verified state

Re-verified from a cold start on **2026-08-08**, not inferred from documentation:

```
docker compose up -d --build     → all services healthy
GET /health/ready                → {"database":"ok","minio":"ok","model":"ok"}
GET /v1/catalog/items  (no auth) → 401
POST /auth/session/create        → session token issued
GET /v1/properties     (authed)  → 200
go build ./...                   → clean
go test ./...                    → 55 packages ok, 0 failures
```

| Metric | Value |
|---|---|
| Go source | 120,455 lines across 336 files |
| Domain modules | ~40 under `internal/` |
| HTTP routes | 340 |
| Test functions | 1,050 across 111 files |
| Direct dependencies | 1 |

## Architecture

```
CorrelationID → Recovery → RequestLogging
  → AuthMiddleware            (subject from session)
    → RequireAuthByDefault    (deny unless on a 6-path allowlist)
      → Metrics → Tracing → RateLimit
        → mux → handler → service (authorizes) → store
```

**Business flow.** iCal feed → VEVENT normalized to UTC, deduped, change-detected
→ reservation raises turnover/inspection *proposals* → deterministic policy checks
readiness, scope, time windows, access, stock, staff eligibility, approval limits
→ dispatch to a curator or vendor → field execution captures checklist, evidence,
expenses, stock custody → incidents route to service recovery → completed work
creates supported charges.

### The agent authority model

The most interesting property of this codebase, and the one it gets right.

**Jarvis proposes. It never mutates.** Verified: `internal/automation/jarvis`
writes to exactly three tables — `ai_tool_calls`, `policy_decisions`,
`approval_requests` — and never touches a business table. Tools are a typed
registry carrying `kind` (read / propose / request / restricted),
`RequiresApproval`, and `Idempotent`, with an explicit sentinel error:

```go
ErrToolDirectMutation = errors.New("jarvis: direct mutation tools do not exist")
```

Application services authenticate the caller, authorize tenant and property
scope, validate state, enforce approval limits, execute the transaction, and
emit audit plus outbox events. The agent is a participant, never an authority.

**Every flow works with the model offline.** Reservations, tickets, dispatch,
access, inventory, approvals, billing, and incidents are deterministic. During a
model outage, rules create the required work and humans write communication from
approved templates.

### Security engineering worth noting

- **The acceptance auth-bypass is compile-time gated, not runtime-flagged.**
  `fixtures_production.go` is a `//go:build !acceptance` no-op; the bypass route
  is physically absent from a production binary. There is no flag to misconfigure.
- **Default-deny authorization.** `RequireAuthByDefault` rejects everything
  outside an explicit six-path allowlist — the inverse of per-handler opt-in,
  which had already let several endpoints ship unguarded.
- **Tenant identity comes from the authenticated subject**, never the request
  body — 159 call sites derive it from `subject.TenantID`.
- **Audit writes are atomic with the business mutations they describe.**

## Module map

| Domain | Modules |
|---|---|
| Identity & tenancy | `iam`, `access` |
| Owner & property lifecycle | `onboarding`, `property`, `contracts`, `compliance` |
| Reservations | `reservations` |
| Operations | `operations` (tickets, dispatch), `maintenance`, `quality` |
| Supply chain | `catalog` (packages), `inventory`, `procurement`, `fleet` |
| Workforce | `workforce` |
| Money | `billing`, `documents` |
| AI agents | `automation/jarvis`, `automation/hermes`, `automation/evaluation` |
| Guest & consumer | `consumer`, `communications` |
| Platform | `platform/{app,database,http,security,audit,jobs,files,durability,health,logging}` |
| Cross-cutting | `privacy`, `reporting`, `observability`, `recovery`, `release`, `security` |

## Known gaps

Documented deliberately. A repository that only advertises its strengths is not
trustworthy, and the next engineer needs these more than they need the highlights.

1. **Three roles, not six.** The product defines Owner, Guest, Curator,
   Operations supervisor, Vendor, and HR provider. The code has `owner`, `guest`,
   `staff` — Curator, Supervisor, Vendor, and HR provider all collapse to `staff`,
   so segregation of duties is not enforceable. `RequireRole` guards 5 of 340
   routes. **Blocks the HR-partner portal.**
2. **No migrations.** Schema comes from 143 `CREATE TABLE IF NOT EXISTS`
   statements across 24 `EnsureSchema()` functions run at boot. No versioning, no
   rollback, and `ALTER` is impossible. **Fix before any production data exists.**
3. **No protocol engine.** Packages exist. Turnover proposals exist. Tickets
   exist. Nothing connects *"this property bought Package X"* to *"generate these
   tasks after each checkout."* This is the missing spine of the business model.
4. **65 of 340 routes are in the OpenAPI contract.** The conformance checker
   validates declared operations but never asserts the reverse, so the drift is
   invisible to CI. Use `docs/development/ROUTE_INVENTORY.md` instead.
5. **The authorization layer is untested.** 1,050 tests, all green — but
   `internal/api`, `internal/platform/app`, `internal/platform/http`,
   `internal/platform/security`, and `internal/platform/audit` have zero test
   files. The domain services are well covered; the routing and authz layer is not.
6. **Silent degradation.** Every route registers behind `if svc != nil`. With
   `CC_SKIP_DB=true` the API boots with no business routes and no auth middleware
   while `/health/live` still returns 200.

## Quickstart

```bash
export CC_BUILD_TAGS=acceptance     # REQUIRED — see below
docker compose up -d --build api worker postgres minio model-stub
curl -s http://127.0.0.1:8080/health/ready
```

> **Without `CC_BUILD_TAGS=acceptance` you cannot log in.** That tag compiles in
> `POST /auth/session/create`, the no-OTP session route. A plain
> `docker compose up` is a correct production build with no way to authenticate
> against it locally. This is intentional (see Security above) and is the single
> most common way a first run fails.

Full boot, login, and teardown: **[`docs/development/DEMO_RUNBOOK.md`](docs/development/DEMO_RUNBOOK.md)**
All 340 routes with handler files: **[`docs/development/ROUTE_INVENTORY.md`](docs/development/ROUTE_INVENTORY.md)**
Frozen product specification: **[`docs/product/`](docs/product/)**

## Status

**Halted 2026-08-08**, in a deliberately chosen state: verified running,
demo-ready, and *not* mid-refactor. The known gaps above were identified in a
full architecture review and consciously deferred rather than discovered later.

Resume order, when work restarts:

1. **Protocol engine** — changes what the product *is*
2. **Six-role model** — unblocks the HR-partner portal
3. **Real migrations** — must land before production data
4. **Test the routing/authz layer** — the gap that hides everything else

The successor tooling distilled from building this — a harness that interviews a
client to specification before writing any code — lives at
[`afk-orchestrator`](https://github.com/sanjayBahadur/afk-orchestrator).
It is also halted, at design-complete.
