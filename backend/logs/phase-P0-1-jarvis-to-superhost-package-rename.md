# Phase P0.1 — jarvis package rename

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-pro
- **Status:** complete

## What I built
Renamed the `jarvis` Go package to `superhost`: directory, package declarations, all Go identifiers (types, functions, variables, constants), import paths across the codebase, and test command paths in `evidence.go`. Preserved all string literals that look like persisted or wire-format values (`run_kind` column, route paths, role discriminators in other packages).

## Files added or changed

| File | Change |
|------|--------|
| `internal/automation/jarvis/` → `internal/automation/superhost/` | Directory rename (`git mv`) |
| `internal/automation/superhost/context.go` | `package jarvis` → `package superhost`; error message prefixes `jarvis:` → `superhost:` |
| `internal/automation/superhost/context_test.go` | `package jarvis_test` → `package superhost_test`; import path updated; all `jarvis.` → `superhost.` |
| `internal/automation/superhost/handler.go` | `package jarvis` → `package superhost`; descriptive error strings updated; `run_kind = "jarvis"` and route path `/v1/jarvis/runs` preserved (P0.2/P0.3 scope) |
| `internal/automation/superhost/models.go` | `package jarvis` → `package superhost`; sentinel error prefixes updated |
| `internal/automation/superhost/policy.go` | `package jarvis` → `package superhost`; `PolicyVersion` string updated to `"superhost-policy-v1.0"`; error prefixes updated |
| `internal/automation/superhost/schema.go` | `package jarvis` → `package superhost`; error prefixes updated |
| `internal/automation/superhost/tools.go` | `package jarvis` → `package superhost`; `AgentKindJarvis` → `AgentKindSuperhost` (string value `"jarvis"` preserved); `jarvisToolRegistry` → `superhostToolRegistry`; error prefixes updated |
| `internal/automation/superhost/tools_test.go` | `package jarvis_test` → `package superhost_test`; import path updated; all `jarvis.` → `superhost.`; `TestJarvis*` → `TestSuperhost*` |
| `internal/automation/evaluation/scenarios.go` | Import path updated; all `jarvis.` → `superhost.`; `jarvisScenarios` → `superhostScenarios`; `JarvisScenarios` → `SuperhostScenarios`; `EngineJarvis` preserved (out of scope) |
| `internal/automation/evaluation/runner.go` | `NewJarvisRunner` → `NewSuperhostRunner`; `JarvisScenarios()` → `SuperhostScenarios()` |
| `internal/platform/app/app.go` | Import path updated; all `jarvis.` → `superhost.`; `jarvisPool` → `superhostPool`; `jarvisAssembler` → `superhostAssembler`; error message updated |
| `internal/release/evidence.go` | Test command paths `./internal/automation/jarvis/...` → `./internal/automation/superhost/...`; business requirement descriptions preserved |
| `tests/automation/model_outage_test.go` | Import path updated; all `jarvis.` → `superhost.`; `superhost.AgentKindJarvis` → `superhost.AgentKindSuperhost` |
| `tests/automation/evaluation/evaluation_test.go` | Import path updated; all `jarvis.` → `superhost.`; `TestJarvis*` → `TestSuperhost*`; `NewJarvisRunner()` → `NewSuperhostRunner()` |
| `tests/api/contract_integration_test.go` | Import path updated; all `jarvis.` → `superhost.`; comment updated |

## Decisions I made
- **`PolicyVersion` string changed** from `"jarvis-policy-v1.0"` to `"superhost-policy-v1.0"` — this is a metadata tag stored in DB columns (`policy_version`), not a business discriminator linking to other records. It names the policy ruleset, which was authored in the `jarvis`/`superhost` package. Updating it keeps the versioning traceable to the renamed package.
- **`context_source` string changed** from `"jarvis-context-assembler"` to `"superhost-context-assembler"` — similar reasoning; this is an origin label embedded in the API response, not a database enum.
- **`EngineJarvis` in `evaluation/models.go` preserved** — the task explicitly lists this as out of scope (`"do not touch these at all, not even the Go identifier"`). It is in a package this block does not own and is used as a type discriminator.
- **String `"jarvis"` in role names (`tools_test.go` actor roles) preserved** — these are test fixture data representing the old role name. The task only requires renaming Go identifiers, not test data strings that may correspond to persisted role values.

## What did NOT work
Nothing failed. All builds, vet checks, and tests pass.

## Deviations from the plan
- **`evidence.go` has no Go import for `jarvis`**: The task listed it as one of "the three non-test files that import this package," but `evidence.go` does not actually import `jarvis`. It only contained test command path strings referencing `./internal/automation/jarvis/...`. These string paths were updated to `./internal/automation/superhost/...`.

## Open questions

These string literals containing `"jarvis"` were found outside the scope of this block and left untouched. They may need coordinated updates in future blocks (P0.2, P0.3) or a separate block:

### Persisted value discriminators (out of scope — not touched)

| File | Line | Identifier | String Value | Apparent Purpose |
|------|------|------------|-------------|-----------------|
| `internal/iam/models.go` | 41 | `RoleJarvis = "jarvis"` | `"jarvis"` | IAM role discriminator; persisted in `subjects.roles` and auth check tables |
| `internal/compliance/models.go` | 22 | `RoleJarvis = "jarvis"` | `"jarvis"` | Duplicate of the IAM constant; may be independently persisted in compliance audit records |
| `internal/reservations/models.go` | 41 | `RoleJarvis = "jarvis"` | `"jarvis"` | Duplicate of the IAM constant; may be independently persisted in reservation records |
| `internal/automation/evaluation/models.go` | 24 | `EngineJarvis Engine = "jarvis"` | `"jarvis"` | Engine type discriminator for evaluation scenarios; `scenarios.go` uses it in the same package |

The duplication of `RoleJarvis` across `iam`, `compliance`, and `reservations` suggests these may be independently persisted role discriminators. Changing them without a migration could silently break role-based authorization checks. This needs a separate assessment.

### Acceptance runner (out of scope — not touched)

| File | Lines | Content | Apparent Purpose |
|------|-------|---------|-----------------|
| `tests/acceptance/probes.go` | ~1912, ~4615, ~4789 | `"jarvis"` as role name and `Kind` check | Black-box acceptance probes; needs coordinated update after P0.2/P0.3 land |

### Preserved in renamed files (intentionally kept for P0.2/P0.3)

| File | Lines | Content | Reason |
|------|-------|---------|--------|
| `internal/automation/superhost/handler.go` | 23, 46, 89 | `"/v1/jarvis/runs"`, `"jarvis"` (run_kind) | Route P0.3, migration P0.2 |
| `internal/automation/superhost/tools.go` | 17 | `AgentKindSuperhost = "jarvis"` | String value preserved; identifier renamed |
