# Production Readiness Contract

“Production ready” means the automated contract below passes. It does not mean the harness deployed production or replaced human acceptance.

For this V0 harness, the certified deployment topology is localhost-only. All published ports bind to loopback, all runtime networks are internal, infrastructure and model behavior have local services or stubs, and certification fails on a required external runtime endpoint. This constrains deployment location without weakening production engineering checks.

## L0: fast correctness

- formatting, vetting, linting, generated artifacts, migration syntax, and Compose configuration;
- no forbidden files, secrets, host-only assumptions, or dependency drift;
- finishes quickly enough to run after each bounded section.

## L1: unit and module behavior

- unit tests, module contract tests, state transitions, policy tables, and property tests;
- deterministic calculations, denial paths, duplicate requests, concurrency conflicts, and model-free fallback.

## L2: integrated behavior

- real PostgreSQL and object-storage integration;
- migrations from empty state and supported recovery path;
- outbox, jobs, webhook replay, reservations to work, Curator completion, owner approval, inventory, access, billing, and report flows;
- phase-wide race checks where supported.

## L3: security and privacy

- static, dependency, container, and secret scans;
- cross-tenant, role, access-secret, object ownership, input, webhook, rate, and audit tests;
- prompt injection and agent tool-policy tests;
- no unresolved high or critical findings.

## L4: release candidate

- clean rebuild, complete test suite, OpenAPI compatibility, migration rehearsal, backup/restore rehearsal, smoke flow, performance budget, model outage, and dependency outage checks;
- health, metrics, structured logs, request correlation, alert conditions, dashboards or queries, and runbooks;
- requirement-to-test traceability complete;
- rollback or forward-recovery decision documented;
- image bill of materials and pinned base images recorded.

## Minimum performance scenario

Seed at least 50 properties, 1,000 guest or reservation records, concurrent owner and Curator reads, reservation ingestion, and background work. The gate must declare measured targets before asserting pass. Optimize only measured failures.

## Release blockers

Any failed required command, invalid migration path, cross-tenant disclosure, unbounded access-secret disclosure, unauthorized money action, lost outbox event, unrecoverable backup, untracked deferred scope, high or critical security finding, or model dependency in the manual core blocks the release candidate.

After L4 and independent review pass, the harness stops at the manual owner, Curator device, and recovery inspections in `.harness/goal.yaml`.

