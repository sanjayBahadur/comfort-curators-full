# P1-02 — Point tests at a disposable database, with a guard

## Problem

Go test helpers across this repo default to host `localhost`, port `5432`,
database **`comfort_curators`**, user `ccuser`, password `ccpass` — which is
the exact database the running application uses. Several tests `TRUNCATE`
tables, and `tests/database_integration_test.go:154` deliberately poisons
`schema_migrations.checksum` with a `deadbeef…` sentinel to exercise drift
detection.

Consequence, already observed on this machine: running `go test ./...`
while the docker stack is up wiped live tables and left the migration
checksum poisoned, after which **the API refused to boot** with
`migration checksum drift at version 4`.

There is no separate test database and no guard preventing a run against a
production or demo target.

## Scope

37 test files each carry their own copy of `postgresAvailable()` and
`dbConnString()` (or an inline equivalent). Find them with:

```
grep -rln 'func dbConnString\|func postgresAvailable\|ccpass' --include='*_test.go' .
```

## Required design — follow this, do not redesign

1. **Create one shared package: `internal/platform/testdb`.**
   Exported helpers only; every test file uses it. Do not leave duplicated
   connection logic behind.

2. **Default database name becomes `comfort_curators_test`.**
   Never `comfort_curators`.

3. **The guard.** Before returning any connection, resolve the target
   database name and refuse if it does not end in `_test`:
   - Fail **loudly** — `t.Fatalf` with a message naming the offending
     database and how to fix it. Do **not** `t.Skip`; a silent skip is how
     this goes unnoticed.
   - The guard fires regardless of how the name was supplied (default or
     `CC_DB_NAME` override).

4. **Preserve the existing skip-when-unavailable behaviour.** If no
   Postgres is reachable at the host/port, still `t.Skip` as today. Order
   matters: check reachability first, then apply the name guard, then
   connect.

5. **Auto-create the test database if it is missing.** Connect to the
   `postgres` maintenance database and `CREATE DATABASE
   comfort_curators_test` when absent, so a fresh checkout needs no manual
   setup. Handle the concurrent-creation race (duplicate-database error is
   not a failure).

6. **Env overrides still work** — `CC_DB_HOST`, `CC_DB_PORT`, `CC_DB_USER`,
   `CC_DB_PASS`, `CC_DB_NAME` — but the guard in (3) still applies to the
   resolved name.

## Acceptance criteria

- [ ] `internal/platform/testdb` exists and is the only place a test
      database connection string is built.
- [ ] All 37 files use it. `grep -rn 'ccpass' --include='*_test.go' .`
      returns only matches inside `internal/platform/testdb`.
- [ ] `go build ./...` and `go vet ./...` are clean.
- [ ] **Falsification test** (this is the point of the task): a test that
      sets `CC_DB_NAME=comfort_curators` and asserts the guard rejects it.
      Prove the guard fires — a guard nobody has seen fire is not a guard.
- [ ] Running the suite with the application stack up does not modify the
      application database.

## Out of scope

Do **not** try to fix failing tests in this task. Roughly 30 tests
currently fail in `internal/automation`, `internal/automation/superhost`
and `internal/platform/jobs` — largely because they share one database and
deadlock against each other. That is task **P1-03** and it depends on this
one. Changing test *logic* here makes both tasks unreviewable.

Confine changes to connection setup, the new package, and the falsification
test.

## Notes

- Module is `comfort-curators-backend`; import path will be
  `comfort-curators-backend/internal/platform/testdb`.
- Test files live in `package X_test` packages, so the helper must be
  exported.
- You are in an isolated git worktree on branch
  `task/p1-02-test-db-guard`. Commit your work there. Do not touch `main`.
