# Comfort Curators Direct Development Instructions

## Repository objective

Build the frozen Comfort Curators V0 backend as a Go modular monolith with an
API, worker, PostgreSQL, private S3-compatible storage, and a localhost-only
Docker Compose runtime. The product source of truth is
`docs/product/00_source_of_truth.md`. The executable scope is
`docs/product/06_product_freeze_v0.md`, the normative requirements in
`docs/product/03_business_product_requirements.md`, the architecture document,
and the protected contracts under `contracts/`.

## Task rules

- Work only on the assigned task from `docs/development/plan.yaml`, or on a
  controller-generated phase-gate repair bead after a real gate failure.
- Inspect the current repository before editing and preserve completed work.
- Implement executable behavior and real tests. Do not replace implementation
  with a plan, report, placeholder, generated evidence, or mocked success.
- Commit the completed task on the current branch with the exact `CC-Task`
  trailer requested in the assignment.
- Do not create branches, worktrees, pull requests, or extra orchestration.
- Do not modify product documents, contracts, the development plan,
  `.harness/`, or this file.
- If Docker is unavailable inside the provider session, run the non-Docker
  checks available there. The launcher owns the Docker phase gates.
- Use synthetic development data only. Do not use production credentials,
  production data, cloud services, or public network listeners.

## Build interface

The launcher verifies phases by running `make -C <repo> phase-N` from outside
any provider session. The repository must therefore provide, at its root:

- A `Makefile` with one target per phase: `phase-1` through `phase-7`. Each
  target must bring up the Compose topology from clean volumes, run migrations,
  run `tests/acceptance/run --phase N`, and exit non-zero on any failure.
  A phase target whose behavior is not implemented yet must fail loudly. It
  must never exit zero by skipping.
- A `compose.yaml` at the repository root. The launcher starts the finished
  application with `docker compose -f compose.yaml up --build`.
- `tests/acceptance/run`, the executable black-box acceptance runner.
- A liveness endpoint at `http://127.0.0.1:8080/health/live`.
- For side-by-side local builds, Compose MUST publish that container endpoint as
  `127.0.0.1:${CC_HTTP_PORT:-8080}:8080`, and HTTP acceptance probes MUST use
  `CC_BASE_URL` when set. The default remains port 8080 outside the isolated launcher.
- Compose MUST NOT set `container_name`, use external networks or volumes, or create
  globally named runtime resources. The launcher owns `COMPOSE_PROJECT_NAME`.

These four are a hard contract with the launcher, not a style preference. The
launcher cannot verify a phase, tag it, or write its report without them, and
it has no way to ask you for them mid-run. Task `p0-build-interface` creates
this skeleton; every later task keeps it working and extends the phase target
it belongs to.

## Architecture invariants

- One Go API and one Go worker in a modular monolith.
- PostgreSQL is transactional truth.
- Domain state and required outbox intent commit atomically.
- Durable jobs use PostgreSQL leases, heartbeat, bounded retries, and visible
  terminal failure.
- Every business record is tenant-scoped; property resources also enforce
  property scope.
- Authorization fails closed before resource existence is disclosed.
- Money uses integer minor units and ISO 4217 currency.
- Operational, financial, access, evidence, approval, communication, audit,
  and agent records are not hard-deleted.
- Model output is untrusted and can only propose typed actions. Application
  policy authorizes and commits actions.
- Every model-assisted workflow has a deterministic or human-operated fallback.
- No Redis, Kafka, Kubernetes, service mesh, microservices, or second
  application database without an explicit frozen requirement.
- Every published port binds to localhost. Certified runtime dependencies are
  local services or deterministic stubs.

## Completion standard

A task is complete only when its behavior is implemented, relevant tests pass,
and the task-specific commit exists. A phase is complete only when the launcher's
phase command passes twice. Development ends after Phase 7 at manual inspection.

<!-- BEGIN BEADS INTEGRATION v:1 profile:minimal hash:7510c1e2 -->
## Beads Issue Tracker

This project's beads database backs `gc`'s bead routing (`gc sling` / `gc hook`).
Task dispatch, claiming, and closing are handled by the Gas City harness
(`gc hook --claim`, commits with the `CC-Task:` trailer) as described above —
do not run `bd` commands directly inside a task session, and do not run
`git push` (this repo has no configured remote; the completion contract is
commit + close the claimed work item + stop, not push).
<!-- END BEADS INTEGRATION -->
