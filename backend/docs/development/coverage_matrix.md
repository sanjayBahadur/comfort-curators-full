# V0 Coverage Matrix

The validator expands selectors in `plan.yaml` against all normative IDs and fails on an omission or duplicate owner.

| Product area | Requirement IDs | Primary phase | Primary task |
| --- | --- | ---: | --- |
| Business objectives | OBJ-01 through OBJ-05 | 7 | `p7-traceability-release` |
| Tenancy | TEN-001 through TEN-004 | 2 | `p2-tenancy` |
| Properties | PROP-001 through PROP-005 | 2 | `p2-properties` |
| Calendars and reservations | CAL-001 through CAL-008 | 3 | `p3-calendar-ingestion` |
| Tickets and workflow | TKT-001 through TKT-008 | 3 | `p3-ticket-state-machine` |
| Workforce and dispatch | WFM-001 through WFM-014 | 3 | `p3-workforce` |
| Fleet and routes | VEH-001 through VEH-009 | 4 | `p4-fleet` |
| Catalog and packages | CAT-001 through CAT-010 | 4 | `p4-catalog-packages` |
| Inventory | INV-001 through INV-013 | 4 | `p4-inventory-ledger` |
| Documents | DOC-001 through DOC-010 | 5 | `p5-documents` |
| Billing and finance | FIN-001 through FIN-011 | 5 | `p5-charges-invoices` |
| Jarvis | HM-001 through HM-010 | 6 | `p6-jarvis-context`, `p6-typed-tools-policy` |
| Communications | COM-001 through COM-007 | 3 | `p3-communications` |
| Consumer controls | CON-001 through CON-006 | 5 | `p5-consumer-controls` |
| Security and privacy | SEC-001 through SEC-014 | 1 and 5 | `p1-security-audit`, `p5-privacy-rights` |
| Non-functional | NFR-001 through NFR-012 | 1, 3 and 7 | durability, offline sync, recovery, capacity and accessibility tasks |

The plan also covers V0 areas that do not have a dedicated Stage 0 ID prefix: complete owner onboarding, contracts and deterministic quotes, access custody, maintenance and vendor work, service recovery, operational reporting, durable agent runs, Hermes, provider outage, observability, backup, migration rehearsal, and final manual inspection.

## Independent acceptance ownership

| Gate | Named group | Behaviors |
| --- | --- | ---: |
| Phase 1 | CC-FND-001 | 5 |
| Phase 2 | CC-IAM-001, CC-ONB-001 | 9 |
| Phase 3 | CC-RES-001, CC-OPS-001 | 8 |
| Phase 4 | CC-ACC-001, CC-INV-001 | 8 |
| Phase 5 | CC-BIL-001, CC-DOC-001 | 8 |
| Phase 6 | CC-HOU-001, CC-HER-001 | 8 |
| Phase 7 | CC-SEC-001, CC-REL-001 | 9 |

Total: 55 named behaviors, each with a protected real-world observation in `contracts/acceptance/oracle.yaml`.


