# Continuous integration

The repository's CI lives in `.github/workflows/ci.yml`. It runs on every push
to and pull request against the `main` and `production` branches, and is
split into five jobs so a failure points straight at the category that broke.
The CI is the automated subset of the release-verification pipeline the
launcher runs by hand at a phase gate; the launcher's own `make phase-N`
invocation remains the authoritative gate, but CI is meant to catch the same
classes of regression before a human ever looks at the PR.

All Go steps pin the toolchain to the version in `go.mod` (currently the
`go 1.25.0` directive) via `actions/setup-go@v5`, which also caches the module
download and build caches. No separate cache step is required.

## Jobs

### `lint-and-vet`

The cheapest correctness gate. It compiles the module twice — once with a
plain `go build ./...` (the production variant) and once with `go build -tags
acceptance ./...`, which compiles in the acceptance-only session-fixture route
under `internal/iam/testfixtures.go`. A regression in either variant would
silently break either real deployments or the acceptance suite, so both must
build. It then runs `go vet ./...` and asserts that `gofmt -l .` produces no
output, failing the job if any file is named rather than silently
auto-formatting it (an auto-rewrite would mask a real style regression).

### `unit-tests`

Runs the entire Go test suite with the data-race detector enabled:
`go test -race -count=1 ./...`. The race detector is what catches the
concurrency-sensitive code the hardening brief calls out specifically —
`internal/platform/jobs`, `internal/platform/durability`, and
`internal/automation` (goroutines, locking, leases) — and running the whole
suite under `-race` rather than only those three packages keeps the detector
on any new concurrency the modules grow later. Tests that need a live
PostgreSQL detect its absence and self-skip, so this job is safe to run
without the Compose stack and stays fast.

### `security-scans`

Runs three scans in a single job so they share one checkout:

- **govulncheck** (`go install golang.org/x/vuln/cmd/govulncheck@latest`)
  reports only vulnerabilities whose reachable code paths this module's
  call graph actually exercises, which is high-signal output. It is set to
  `continue-on-error` and its report is uploaded as a workflow artifact, so
  it surfaces findings for review without hard-blocking on dependency
  vulns that are out of scope for the CI-pipeline task itself to fix.
- **gitleaks** scans the full repository for committed secrets via
  `gitleaks/gitleaks-action@v2`, which uploads results to GitHub code
  scanning as SARIF and comments on the PR. It is set to `continue-on-error`
  for the same reason — pre-existing fixtures or dummy env values can trip
  it — but the SARIF upload keeps any finding visible as an annotation
  rather than hidden. This is the report/annotate path, not silent ignore.
- **Trivy filesystem scan** of the whole repo (Go module deps + source)
  uploads SARIF and **blocks** on `HIGH` or `CRITICAL` severities, which is
  Trivy's documented release-gating convention. Lower severities annotate
  only.

### `docker-build`

Does a clean, no-cache `docker compose build`, proving the `Dockerfile`
builds every service target (api, worker, model-stub, seed) end to end. Once
the images exist, it runs Trivy image scans over the built `api` and `worker`
images, gating on `HIGH`/`CRITICAL` so a vulnerable base layer is caught here
rather than mid-acceptance-gate. The image names follow Compose's default
`<project>-<service>` convention using a fixed `COMPOSE_PROJECT_NAME=cci`
for this job only (the acceptance gates use their own per-runner project
names and are unaffected).

### `acceptance-gates`

A `fail-fast: false` matrix over phases 1 through 7. Each matrix leg runs on
its own runner with its own isolated Docker daemon, so the seven phases run
in parallel. Each leg invokes `make phase-N`, which calls `scripts/run-phase
N`; that script tears the stack down, rebuilds it with the `acceptance` build
tag, brings it up with Compose on `127.0.0.1`, waits for the `/health/live`
probe to return healthy, and runs `tests/acceptance/run --phase N` against the
live stack. This is the same thing the launcher does at a phase gate, so a
green CI run is a strong signal that a manual gate will pass.

This job `needs: docker-build`, so a broken image build fails fast and the
expensive seven-way acceptance matrix is not run against a build that can
never pass. If a phase fails, the leg captures `docker compose ps` and
`docker compose logs` as job output before `scripts/run-phase`'s exit trap
tears the stack down, to make the failure debuggable from the GitHub UI.

## Blocking policy at a glance

| Check                | Blocks on failure?        | Why                                                       |
| -------------------- | ------------------------ | --------------------------------------------------------- |
| `go build` (both)    | yes                      | correctness                                                |
| `go vet`             | yes                      | correctness                                                |
| `gofmt -l`           | yes                      | correctness                                                |
| `go test -race`      | yes                      | correctness + race safety                                  |
| `docker compose build` | yes                    | image must build                                           |
| `make phase-N`       | yes (per matrix leg)     | the contract                                                |
| govulncheck          | no (annotates + artifact)| may surface out-of-scope pre-existing dep vulns          |
| gitleaks             | no (annotates via SARIF) | may surface out-of-scope pre-existing fixture secrets     |
| Trivy (fs + image)   | yes on HIGH/CRITICAL     | tool's own default release-gating convention               |