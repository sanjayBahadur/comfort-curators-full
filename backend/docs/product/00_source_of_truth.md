# Comfort Curators V0 Source of Truth

Status: frozen for Gas Town backend development  
Date: 2 August 2026

## Precedence

When two artifacts appear to differ, use this order:

1. This file and the explicit reconciliations below.
2. `06_product_freeze_v0.md` for V0 product scope, users, and deferred features.
3. `03_business_product_requirements.md` for normative requirement IDs.
4. The protected contracts under `contracts/` for implementation interfaces.
5. `04_technical_architecture.md` and `07_jarvis_hermes_boundaries.md` for architecture and automation boundaries.
6. Stage 0 feasibility, executive decision, and research sources for business, legal, pricing, and launch context.
7. `10_build_sequence.md` and `docs/development/plan.yaml` for development order only.

No development agent may resolve a contradiction by choosing the easier interpretation. It must raise a contract blocker.

## Reconciled V0 decisions

- Launch geography is Uttar Pradesh, India.
- Initial operating cohort is approximately 30 properties. The software V0 target is 30 to 50 active properties without architectural change.
- V0 supports furnished properties throughout lead, qualifying, onboarding, remediation, ready-inactive, active, paused, suspended, offboarding, and archived states.
- Owner and property onboarding is fully in V0.
- The owner is the subscriber. Booking platforms continue paying owners directly.
- Guests receive only the secure operational experience required for arrival guidance, rules, issue intake, and checkout guidance. Guest commerce and direct booking are deferred.
- Core controlled field work is modeled separately from genuinely independent specialist vendors. Operational workers are at least 18 in V0.
- Jarvis is property-scoped automation. Hermes is a narrow communication and service-recovery specialist invoked through Jarvis or an explicit staff workflow.
- Jarvis and Hermes runs are durable logical records with jobs, leases, heartbeats, retries, tools, usage, and audit. They are not permanent server or model processes.
- Automation proposes. Deterministic policy and application services authorize and commit.
- PostgreSQL is the source of truth. PostgreSQL-backed jobs and transactional outbox are used before Redis or an external broker.
- The application and its data services run locally through Docker Compose. Development inference through Codex or OpenCode providers is external unless a separate local model adapter is later approved.
- V0 stops at manual release inspection. Production deployment is outside the autonomous development run.

## Scope that remains documented but deferred

- Public workforce or specialist marketplace
- Workers under 18
- Passenger transport or customer errands
- Direct e-bike import, worker vehicle financing, or mandatory equipment purchases
- Sponsored catalog placement and advertising network
- Guest or Jarvis store checkout
- Room service
- Direct booking and dynamic pricing
- Native mobile applications and property tablets
- External accounting services or statutory audit representation
- Booking payout custody
- Autonomous purchasing, payment, bank-detail change, legal filing, worker adverse action, or high-risk verification
- Microservices, Kubernetes, Kafka, Redis, and service mesh without measured need

## Success boundary

Gas Town completion is not production approval. The backend is development-complete only when all seven phase gates pass from a clean checkout, all 146 normative requirements are traced, all 55 named behaviors and the 16 launch acceptance areas have real evidence, recovery is rehearsed, and the human manual-inspection checklist remains.


