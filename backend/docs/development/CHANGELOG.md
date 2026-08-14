# Comfort Curators Build Changelog

Narrative build log, generated per verified phase. See `reports/build/` for the mechanical per-phase record (task/commit/gate hashes).

## Phase 1 -- Executable foundation

Phase 1 stood up the launcher-facing skeleton and the platform spine everything else builds on: the Makefile phase contract and Compose root, the Go modular-monolith layout (api, worker, model-stub binaries), and a pinned local Compose topology with PostgreSQL and MinIO. On top of that it added the horizontal platform services -- typed config, structured logging, health endpoints, correlation middleware, forward-only SQL migrations, idempotency/outbox durability, durable leased jobs, private S3 object metadata, and security/audit/encryption primitives -- and capped it with the oracle-backed black-box acceptance runner.

### Issues found and fixed

- **Acceptance runner raced API startup** (`fix(scripts)`): `compose up -d` does not block for the api container's healthcheck, and the acceptance runner's HTTP probes had a single 2-second dial with no retry, so the suite failed with connection refused even when the stack came up correctly moments later. Fixed by waiting for api health before running the acceptance suite.

- **Concurrent schema creation race and lease-recovery SQL bug** (`fix(phase-1)`): when the api and worker containers started at the same moment, both called `EnsureSchema` and the `CREATE TABLE IF NOT EXISTS` race threw a `pg_type_typname_nsp_index` unique violation; separately, `RecoverExpiredLeases` had three SQL placeholders but only two bound parameters, producing "could not determine data type of parameter $1". The schema creation was patched with a retry loop and the SQL parameter numbering was corrected.

- **Schema initialization still raced despite the retry** (`fix(app)`): the retry loop was insufficient because `files.Migrate`, `security.EnsureSchema` and `audit.EnsureSchema` each ran raw `IF NOT EXISTS` DDL with no locking of their own -- both processes could pass the existence check before either committed, and the loser failed on a duplicate-key error on a different object every run (encryption_keys, file_grants, idx_file_objects_object_key). The whole schema-setup sequence was wrapped in a session-held `pg_advisory_lock` so only one process runs it at a time.

Full task/commit/gate-hash record: `reports/build/phase-1.md`

## Phase 2 -- Owner, property and onboarding

Phase 2 added the identity and tenancy layer -- distinct owner/guest/staff identities with hashed OTP, sessions and MFA policy, plus tenants, memberships and time-limited support access with deny-before-disclose authorization. It also built the property aggregate (explicit lifecycle state machine, readiness and compliance holds), the resumable owner-onboarding flow with immutable inspection evidence, the contracts module (deterministic quotes, versioned immutable service agreements, fee base), compliance expiry scanning with renewal warnings and holds, and the protected owner-property API slice validated against the OpenAPI contract.

### Issues found and fixed

- **Phase 2 gate failures** (repair commit): five problems surfaced on the gate runs -- property creation required an `access_method` the service layer rejected; agreement versioning only accepted PUT and needed a POST route; `EnsureUser` collapsed owner and guest onto a single user when the same contact held different roles; support-access-grant columns used UUID where TEXT was expected; and the expired-support-access probe was missing. Each was fixed: the field requirement removed, the POST versions route added, distinct users per role created, columns aligned to TEXT, and the probe implemented with a real TTL test.

- **Lifecycle probe skipped the readiness gate** (`fix`): `probeCCONB001LifecycleTransitions` tried to activate a property without first setting the mandatory readiness flags (owner_contract_accepted, compliance_complete, mandatory_fields_set), so the activation gate correctly answered NOT_READY. The probe was fixed to call the readiness endpoint after reaching ready_inactive and before transitioning to active.

Full task/commit/gate-hash record: `reports/build/phase-2.md`

## Phase 3 -- Reservations, calendar sync and operations

Phase 3 built the reservation-to-turnover operational core: read-only iCalendar ingestion with normalized reservations, audited human conflict resolution and deterministic turnover/inspection proposals, plus the full operations stack -- a ticket state machine with versioned checklists, blockers and reopen, immutable evidence with incident escalation and service recovery, workforce records with age/rating/restricted-work controls, hard-constraint dispatch with attributed overrides, and communications with versioned templates and expiring secure stay links. It also added idempotent offline checklist/evidence sync for field use.

### Issues found and fixed

- **Operations routes never registered** (`fix`): `ticketSvc` was declared with `:=` inside the wiring block, shadowing the outer-scope nil `ticketSvc`; the `if ticketSvc != nil` guard was therefore always false and `TicketHandler.RegisterRoutes()` never ran, disconnecting all 22 operations endpoints (tickets, evidence, incidents, alerts, recovery) from the HTTP mux. Changed `:=` to `=` so the outer variable receives the initialized service.

- **Eight acceptance probe failures** (repair): tzdata was missing from the Alpine images (timezone resolution failed), the jarvis role was absent from `ValidRole()` so sessions could not be created for it, and the dispatch-constraint probe compared worker names instead of IDs. The repairs added tzdata to all Alpine images, added the jarvis role, fixed the dispatch check, and implemented the evidence-gating, offline-replay and incident-escalation probes.

- **Dispatch progression, evidence gate and feed reachability** (repair 2): the dispatch probe never walked proposed/approved/scheduled so dispatch state checks failed; a `ChecklistVersionID` guard made the evidence gate skip gating after a sync; and the api/worker containers had no `host.docker.internal` mapping, so iCal feed polling could not reach host-side test servers on Linux. Fixed by driving the probe through the full transition chain, removing the guard, and adding the mapping.

- **iCal test servers bound to loopback** (`fix(tests)`): host-side test servers listened on `127.0.0.1:0` while the generated feed URLs used `host.docker.internal`, which only routes to non-loopback-bound services -- every calendar-feed-fetch probe failed from inside the containers and surfaced as empty `items:[]`. Binding the test servers to `0.0.0.0` dropped acceptance failures from 8 to 3 and let the reservation probes reach real business logic.

- **Cancellation rolled back on a SQL parameter bug** (`fix(reservations)`): `CancelProposalsForReservation`'s UPDATE referenced `$3` for updated_at but only bound two arguments, leaving `$2` unreferenced and `$3` unbound; Postgres rejected it with "could not determine data type of parameter $2", rolling back the whole cancellation transaction so a reservation's status never actually flipped to cancelled even though every earlier step in the transaction had succeeded. Correcting the placeholder index fixed the in-place cancellation.

- **Audit events lost after the first write, and evidence never unblocked items** (`fix(audit)`): every `appendAudit` helper except workforce's and communications' inserted `audit_events` rows with an empty TEXT primary key, so only the very first audit event a process ever wrote succeeded and every later one hit a silently-dropped duplicate-key violation -- most audited actions across the app were permanently un-audited after process start. Generating `newID("aud")` ids fixed it. Separately, `RegisterEvidence` stored a `checklist_item_id` but never updated that item's own `evidence_ids` (the field `RequiredEvidenceBlocking` actually checks), so evidence for an item could never unblock it; `linkEvidenceToChecklistItem` now attaches the evidence id idempotently, and the acceptance probe was fixed to pass `checklist_item_id` on registration.

Full task/commit/gate-hash record: `reports/build/phase-3.md`

## Phase 4 -- Access, fleet, catalog, inventory, procurement and maintenance

Phase 4 delivered the property-ops modules beyond core turnover: encrypted property-access secrets with grants, timed disclosures, custody ledgers and holds; the fleet module (e-bike/battery assets, inspections, custody, incident freezes and disabled off-duty tracking); catalog with bundles and versioned owner property packages; the append-only inventory ledger with counts and reconciliation; procurement covering suppliers, requisitions, purchase orders, goods receipt and rebates; and maintenance with triage, immutable estimates, approvals, vendor work orders and warranty history.

### Issues found and fixed

- **Duplicate disclosure audit and mutable movement ledger** (repair): the disclosure probe discarded the body of its first disclose call and made a second redundant call, creating a duplicate audit record for out-of-window grants -- fixed by capturing the body from the first call. The inventory movements table also had no database-level append-only enforcement, so a `BEFORE UPDATE OR DELETE` trigger was added on `inventory_movements` to match the existing pattern used by `audit_events` and contract versions.

Full task/commit/gate-hash record: `reports/build/phase-4.md`

## Phase 5 -- Documents, billing, finance, privacy and reporting

Phase 5 built the owner-facing financial and trust layer: an immutable documents module with extraction provenance, human review gates and submission packets; owner-only charges, idempotent invoices, credits and an append-only operational subledger; maker-checker approvals with out-of-band bank-change verification and reconciliation exceptions; consumer controls for price disclosure, claims evidence, tenant-scoped history export and anti-dark-pattern rules; privacy rights covering notices, consent, retention-blocked erasure and a de-identified evaluation boundary; rebuildable reporting read models and a monthly owner report; and a protected owner-finance API slice exposed over all of it.

### Issues found and fixed

- **Owner authority resolution was a stub** (`fix(property,api)`): `newAuthorityResolver` ignored its identity service and returned the actor's own ID, so an owner's session ID never matched the caller-supplied `owner_authority_id` and every owner-scoped check (including `GET /v1/properties/{id}`) denied owners their own property. Fixed by adding an `owner_authority_grants` table populated at property creation and rewiring resolution through `PropertyService.ResolveActorAuthorities`. Separately, `handleCreateCharge`/`handleIssueInvoice`/`handleIssueCredit` had no role check at all, so any authenticated subject including a guest could create billing records; the owner-or-staff gate was added to all three.

- **Acceptance probes matched assumed, not real, response shapes** (`fix(tests)`): the CCBIL001/CCDOC001 probes asserted flat evidence fields instead of the `evidence_links` array, a `{items, total}` envelope instead of the standard `{items:[{id,version,data}], next_cursor}`, and the `ALREADY_SUBMITTED` code instead of the handler's `CONFLICT`. One guest probe also created its session in a brand-new tenant that had no owner, which IAM correctly refuses. The probes were corrected to the real shapes and to reuse the owner's own tenant.

Full task/commit/gate-hash record: `reports/build/phase-5.md`

## Phase 6 -- Jarvis and Hermes

Phase 6 built the model-assisted automation layer on the durable platform spine: a protected agent-run runtime whose worker leases, heartbeats, retries and cancels runs through a pluggable model provider (deterministic local stub by default), the property-scoped Jarvis that assembles only approved context and enforces a fail-closed typed-tools policy with maker-checker approval envelopes, and Hermes, which turns approved facts into reviewed, idempotently-delivered communications. It closed out with an adversarial and policy evaluation runner that scores how both agents handle prompt injection, ambiguous inputs and unsupported claims, and an outage suite proving the core API, manual retry and human workflows keep working with every model provider down.

### Issues found and fixed

- **First gate repair** (`0718163`): the jarvis's compliance-holds query referenced an `updated_at` column the table does not have, Hermes' draft/review/delivery parameter structs lacked JSON tags so underscore-named request keys never deserialized, and the Hermes service returned zero-value IDs that caused follow-up 404s. Fixed by querying `created_at` instead, adding JSON struct tags, and generating draft/review/delivery IDs in the service layer with crypto/rand.

- **NULL time scan in reservation counting** (`65aa13d`, repair-2): `countActiveReservations` scanned `MAX(updated_at)` into a `time.Time`, and for a property with no active reservations the aggregate is NULL, which crashed the scan. Wrapped it in `COALESCE(MAX(updated_at), NOW())`, matching the pattern already used by `countOpenCriticalHolds` and `summarizeStock`.

- **Ambiguous column in the stock summary** (`99a0d87`): `summarizeStock` joined `inventory_movements` and `stock_locations`, both of which have a `created_at`, and referenced `MAX(created_at)` unqualified, so Postgres rejected the query with an ambiguity error and every jarvis context assembly that needed a stock summary failed outright. Qualified it to `im.created_at`, since movements is the table that records stock-change time.

- **Nullable columns crashed run reads** (`1376034`, `7964085`, `a134730`): `error_message`, `idempotency_key` and `lease_owner` are nullable TEXT columns but were scanned directly into plain-string fields, so any run without those values set -- the common case -- died with "cannot scan NULL into *string". All four read sites now scan into a local `*string` and assign only when non-nil, and empty idempotency keys are bound as SQL NULL so runs without an explicit key stop colliding on the partial unique index; the lease_owner half surfaced as an apparently unrelated `run_id mismatch` in the jarvis probes.

- **Idempotency short-circuit inverted** (`1376034`): `GetByIdempotencyKey` excludes cancelled/failed runs (those may be retried), but Submit only returned an existing match when it was non-terminal, so a completed run under the same key fell through to a fresh INSERT and hit the unique index instead of returning the original -- the opposite of idempotent replay. Submit now returns any non-cancelled/non-failed match it finds.

Full task/commit/gate-hash record: `reports/build/phase-6.md`

## Phase 7 -- Security, reliability and release hardening

Phase 7 was the hardening and release-readiness pass over the already-built system: it proved the live API conforms to the OpenAPI contract, shipped a security layer (rate limiting, webhook replay protection, secret scanning, prompt-injection defense, findings engine, authorization matrix) wired into the API, and added business-aware observability -- correlation, metrics, traces, alerts -- spanning the API, worker and outbox. It closed with a recovery module proving backup/restore, migration recovery and idempotent outbox replay, the NFR-001 capacity scenario with p95 latency measurement and WCAG/localization dispositioning, and a release package tracing all 146 requirements to evidence that stops short of auto-approving production.

### Issues found and fixed

- **Logging self-deadlock, live since phase 1** (`b265dca`): `L()` locked the package mutex and then called `Init()`, which locks the same non-reentrant mutex again, so any call to `L()` before an explicit `Init()` hung that goroutine forever. In normal Docker operation this never surfaced, but unit tests that log without calling `Init()` first deadlocked and looked indistinguishable from container/environment contention at `go test`'s 10-minute timeout. Fixed by extracting logger construction into an `initLocked()` helper that never locks itself, so both `Init()` and `L()` acquire the mutex exactly once.

- **Agent-run Claim() rejected at parse time** (`b6061db`): the claim query bound one placeholder to both a TEXT equality (`state = $1`) and a timestamp comparison (`lease_expires_at < $1`) against a single `time.Time`, so Postgres rejected the SQL outright and no worker could ever claim a queued agent run. Split into `$1` (state, TEXT) and `$2` (now, timestamp), with the optional `run_kind` filter shifted to `$3`. The bug stayed hidden until the logging deadlock above stopped hanging test binaries first.

- **minio/model-stub unreachable and minio healthcheck broken** (`2298b72`): `CC_S3_HOST`/`CC_MODEL_HOST` were never wired into the api/worker service blocks, so recovery checks resolved localhost -- the api container itself -- instead of the compose service names, and `/health/ready` reported both dependencies down even though both were healthy. Separately, the minio image has no curl, so its healthcheck always failed with "executable file not found in $PATH" and permanently marked the container unhealthy. Fixed by exporting the host env vars with the fixed internal container ports and switching the healthcheck to minio's own `mc` client.

- **Jarvis fixture truncated a table that doesn't exist** (`b095ec4`): `truncateAll()` truncated `evidence_records`, which no `EnsureSchema` ever creates (the table is `ticket_evidence`), so every jarvis test failed at setup with 'relation "evidence_records" does not exist' once the logging deadlock no longer masked it. Corrected the cleanup to truncate the real `ticket_evidence` table.

- **Rate limiter tripped on the suite's own legitimate traffic** (`d6eaa9a`): burst=20/rate=100-per-minute was tuned only against the security package's unit tests, but the acceptance suite creates a session per probe, so ~19 unrelated probes failed with 'too many requests' and the whole phase gate failed closed under its own load. Raised to burst=300, rate=1200/min, which still throttles a single IP sustaining abusive request rates against one path.

- **Two SEC-001 probe assertions encoded the wrong requirements** (`c4e7632`): the redaction probe embedded the secret under test inside the X-Correlation-ID value and then asserted it never appears, breaking the correlation echo the oracle spec requires; the cross-tenant write-spoof check demanded an explicit FORBIDDEN response even though the handler derives tenancy solely from the authenticated subject and never consults the client-supplied tenant_id. Fixed by using a plain, non-secret correlation id and asserting the actual invariant -- the write never lands under the spoofed tenant, whether the API answers with an explicit denial or silently redirects it to the caller's own tenant.

- **Dead outbox side-effect check removed from the cross-tenant probe** (`22636dc`): the probe executed an outbox query, discarded the result, and never asserted anything, while assuming `outbox_events.tenant_id` is UUID-typed when every other table uses TEXT tenant ids, so it failed outright with 'invalid input syntax for type uuid'. Removed; the probe's retained check that tenant A keeps exactly its own property count after the cross-tenant write attempt already covers the no-side-effect requirement.

Full task/commit/gate-hash record: `reports/build/phase-7.md`

## Phase P0 -- Jarvis to Superhost rename

Phase P0 renamed the model-assisted agent from Jarvis to Superhost across the whole codebase and, in doing so, proved the name was load-bearing rather than cosmetic. The `jarvis` Go package became `superhost` -- directory, package declarations, every Go identifier and import path across the codebase -- while deliberately preserving the string literals that look like persisted or wire-format values. On top of that came a data migration: new agent runs now write `run_kind = 'superhost'` and an idempotent backfill in `EnsureSchema()` rewrites existing `'jarvis'` rows. `POST /v1/superhost/runs` became the primary API route, with `POST /v1/jarvis/runs` kept as a 308 redirect shim so old clients keep working for a release, and the `contracts/` files were aligned with the new name (the run_kind enum drops `jarvis`; the old path stays in the spec as a documented deprecated redirect entry). The part a future developer most needs to know is the IAM role dual-accept work: chasing the leftover `"jarvis"` literals showed that `RoleJarvis = "jarvis"` is not a name -- it is a real, security-relevant session role persisted in `subjects.roles`, validated by `ValidRole()`, and enforced by compliance-hold and reservation-mutation denial gates across `iam`, `compliance` and `reservations`. A blind rename could have silently broken authorization. Instead `"superhost"` was added as a second recognized value everywhere, so both role strings now trigger the same checks, and the old `"jarvis"` value was deliberately never removed for backward compatibility with sessions and tokens that may still carry it.

### Issues found and fixed

- **`context_source` wire-value regression** (`P0.1b`): P0.1 renamed the response origin label `"jarvis-context-assembler"` to `"superhost-context-assembler"`, but `tests/acceptance/probes.go` still hard-requires the old value and its update is deferred until after the P0.2/P0.3 work lands. The independent review caught the drift, and the wire value was restored to `"jarvis-context-assembler"` so the live API keeps matching the black-box probes.

- **`AgentKindSuperhost` value mismatch** (`P0.1b`): `const AgentKindSuperhost = "jarvis"` in `tools.go` disagreed with the two `req.RunKind = "superhost"` literals in `handler.go` -- two sources of truth for the same run-kind value, free to drift apart. The constant was corrected to `"superhost"` and both handler assignments now reference `AgentKindSuperhost` instead of duplicating the literal.

- **Loose test assertions in the route-redirect test** (`P0.3b`): the redirect test asserted exclusions (`not 404` / `not 308`) rather than the deterministic outcome. Since `handleCreateRun` calls `subjectFromRequest` first and returns `401` before the body or store are touched when no auth header is present, both the direct and followed-redirect paths were tightened to assert an exact `401` status -- still catching a missing route and a non-redirecting shim, plus any other unexpected status.

- **Compliance handler's missing HTTP-level authorization coverage** (`P0.7b`): the existing compliance denial tests exercised only the service layer, never the handler-level `hasRole` gate that the role checks actually live behind. A new `internal/compliance/handler_test.go` proves both `jarvis` and `superhost` receive `403 FORBIDDEN` across all three role-gated endpoints, with `owner`-role allowed-path cases for create and renew.

Full per-block task/commit/gate record: `logs/phase-P0-*.md`, with cross-block decisions in `logs/DECISIONS.md`
