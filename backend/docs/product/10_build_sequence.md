# Dependency-Aware V0 Build Sequence

This is the planning baseline, not permission to add scope. The planner may merge adjacent work when acceptance remains observable, but it may not omit a frozen requirement.

## Phase 1: executable foundation

Create the Go module, API and worker entry points, pinned Docker Compose stack, configuration, health, structured logging, PostgreSQL migrations, transactions, jobs, outbox, private S3 adapter, fixtures, and the service contracts required by the protected `tests/acceptance/run` driver. Do not recreate or edit that driver outside the independent acceptance-oracle task. Exit with a clean containerized build and integration test.

## Phase 2: identity, tenancy, property, onboarding, and contracts

Implement authentication foundations, roles, tenant and property scope, audited support access, property lifecycle, readiness holds, owner and property onboarding, inspection evidence, fit score, deterministic quote inputs, service agreements, approval policy, and activation.

## Phase 3: reservations, operations, dispatch, workforce, and communication foundation

Implement iCalendar ingestion, reservations, conflicts, tickets, checklists, evidence, incidents, recovery, availability, qualifications, assignment, Curator synchronization, HR adapter boundary, templates, secure stay links, and delivery state.

## Phase 4: access, inventory, and maintenance

Implement custody and disclosure, stock ledgers and counts, consumption, reorder proposals, maintenance triage, estimates, approval, vendor work, evidence, and warranty history.

## Phase 5: documents, billing, and reporting

Implement document versions and review, owner charges and invoices, credits, reconciliation, quote-versus-actual contribution, owner health, performance metrics, and monthly reports. Booking payouts and guest purchases remain excluded.

## Phase 6: Jarvis and Hermes

Add context assembly, typed proposal tools, policy envelopes, approvals, audit, prompt and schema versioning, Hermes drafts, exception suppression, retries, cost and usage records, injection defenses, and complete manual fallback. Add no autonomous spending or direct data mutation.

## Phase 7: cross-cutting hardening and release candidate

Complete OpenAPI, observability, rate and abuse controls, security tests, dependency scanning, performance scenario, backup and restore, migration rehearsal, failure injection, runbooks, traceability, and final L4 evidence. Stop at manual inspection.

