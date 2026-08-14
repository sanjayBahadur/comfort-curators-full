# Comfort Curators Business and Product Requirements

Version: 0.1  
Derived from: Stage 0 feasibility package dated 2 August 2026  
Normative terms: MUST, SHOULD, MAY, and MUST NOT are intentional

## 1. Product definition

The Curators Portal is a multi-tenant operations system for short-term-rental homeowners and the Comfort Curators workforce. It coordinates reservations, turnovers, inspections, maintenance, replenishment, property documents, owner approvals, billing evidence, and operational communications.

Jarvis is the property-scoped automation role inside the Curators Portal. It is not a separate company, unbounded autonomous agent, or independent source of truth.

## 2. Business objectives

### OBJ-01: Reliable property readiness

At least 95% of scheduled turnovers MUST begin within their service window, and at least 92% MUST pass first quality review during the pilot.

### OBJ-02: Owner control without owner micromanagement

An owner MUST be able to define a package, budgets, substitutions, notification rules, and approval thresholds once, then review exceptions rather than every routine action.

### OBJ-03: Lawful, sustainable workforce

Core controlled work MUST use an appropriate employment model, recorded time, transparent pay, safety controls, and a human grievance path. Operational workers MUST be at least 18 during the MVP.

### OBJ-04: Positive property contribution

The system MUST calculate contribution per property from management revenue, task revenue, supply margin, labor, travel, vendor, refund, and direct operational cost. A pilot property SHOULD contribute at least ₹3,000 per month before central overhead.

### OBJ-05: Evidence and accountability

Every property access, task state change, stock movement, purchase approval, document approval, financial export, and Jarvis action MUST be attributable to an actor and timestamp.

## 3. Non-goals for MVP

The MVP MUST NOT include:

- workers under 18;
- passenger transport;
- alcohol, tobacco, controlled medicine, cash, or identity-document errands;
- direct e-bike import;
- worker registration fees or mandatory equipment purchases;
- public labor marketplace listings;
- sponsored product placements;
- autonomous supplier ordering;
- autonomous external document filing;
- autonomous worker rejection or deactivation;
- statutory audit services;
- accounting entries or payments approved by an LLM;
- one server, process, or database per Jarvis; or
- hard deletion of operational or audit records through Jarvis.

## 4. Personas and authority

| Persona | Primary need | Authority boundary |
| --- | --- | --- |
| Homeowner | Control property, packages, budget, and exceptions | Own properties only; cannot view worker HR data |
| Co-host or property manager | Run delegated owner operations | Explicit property and permission assignment |
| Operations lead | Schedule, resolve incidents, verify work | Assigned region and properties; limited HR details |
| Curator employee | Receive work, access checklists, submit evidence, view earnings | Own assignments and limited property data during service window |
| Independent vendor | Quote and complete specialist work | Own vendor jobs; no general property or guest access |
| Inventory coordinator | Receive, count, allocate, and propose replenishment | Stock locations and approved purchasing scope |
| Finance maker | Reconcile and prepare invoices or exports | Cannot approve own payment or vendor changes |
| Finance approver | Approve financial actions within limits | Separate from maker and requestor for high-risk actions |
| Compliance reviewer | Review property, worker, vendor, and document exceptions | Read-only or review authority by record type |
| Platform administrator | Configure platform and respond to security incidents | Privileged actions require MFA and enhanced audit |
| Jarvis | Detect operational needs and use scoped tools | One property context per action; code-enforced permissions |

## 5. Tenant and property requirements

- TEN-001: Every business record MUST belong to a tenant.
- TEN-002: Every property-scoped record MUST include `tenant_id` and `property_id`.
- TEN-003: Authorization MUST enforce tenant and property membership on every read and write, not only in the user interface.
- TEN-004: Support staff access MUST be time-limited, reason-coded, and audited.
- PROP-001: A property MUST have owner authority, service address, geolocation zone, emergency contacts, access method, maximum occupancy, room profile, service checklist, and current status.
- PROP-002: A property MUST NOT become active until mandatory compliance fields and owner contract are complete or a compliance reviewer grants a documented exception.
- PROP-003: The system MUST track permission, registration, insurance, and safety-document expiration.
- PROP-004: An expired critical permission MUST block new service acceptance or raise a mandatory compliance hold according to policy.
- PROP-005: Property access secrets MUST be encrypted and displayed only to an assigned worker during an allowed service window.

## 6. Reservation and calendar requirements

- CAL-001: The system MUST ingest iCalendar feeds for each supported listing source.
- CAL-002: Calendar ingestion MUST be idempotent and preserve the external event identifier and source.
- CAL-003: A changed or cancelled reservation MUST update affected task proposals without silently deleting assigned work.
- CAL-004: The internal reservation record MUST remain the operational source of truth after ingestion.
- CAL-005: Jarvis MUST NOT edit or delete external calendar events through the MVP iCalendar integration.
- CAL-006: Feed failures, stale feeds, overlapping bookings, impossible turnaround windows, and timezone ambiguity MUST create exceptions.
- CAL-007: The system SHOULD poll active feeds at least every 15 minutes, subject to source limitations.
- CAL-008: An owner or operator MUST be able to confirm, merge, or reject a suspected duplicate reservation.

## 7. Ticket and workflow requirements

### Ticket types

The MVP MUST support:

- turnover;
- pre-arrival inspection;
- restock;
- incident;
- routine maintenance;
- specialist vendor request;
- property onboarding;
- document review; and
- inventory count.

### State model

Normative states:

`Draft -> Proposed -> Approved -> Scheduled -> Assigned -> In Progress -> Evidence Submitted -> Verified -> Closed`

Alternate states: `Blocked`, `Cancelled`, and `Rejected`.

- TKT-001: Every state transition MUST be validated by code.
- TKT-002: Every transition MUST record actor, time, previous state, new state, reason, and relevant evidence.
- TKT-003: Operational tickets MUST use cancellation or closure, not hard deletion.
- TKT-004: An approved or assigned ticket change affecting time, scope, access, safety, or price MUST notify affected actors and preserve the prior value.
- TKT-005: Blocked tickets MUST include a blocker type, responsible party, next review time, and escalation policy.
- TKT-006: A high-severity incident MUST notify the on-call operations role and owner according to a defined response matrix.
- TKT-007: No actor may verify their own high-risk repair, access incident, or financial exception.
- TKT-008: Reopening a closed ticket MUST create a linked follow-up or a reasoned reopen event.

## 8. Dispatch and workforce requirements

- WFM-001: Worker profiles MUST record legal name, verified identity status, age eligibility, contact method, employment or vendor classification, skills, training, service zone, availability, emergency contact, and status.
- WFM-002: The system MUST support separate employee and vendor agreements and MUST NOT represent one as the other.
- WFM-003: Employees MUST receive appointment terms outside the scheduling flow before assignment.
- WFM-004: Assignment MUST consider skill, service zone, availability, maximum working hours, rest, travel time, safety restrictions, and access conflicts.
- WFM-005: The system MUST record paid work time, required travel, waiting caused by company operations, overtime, incentives, and authorized expense.
- WFM-006: The worker MUST see expected pay or wage treatment before accepting an optional task.
- WFM-007: No deduction may be applied without an authorized reason, evidence, worker notice, and payroll review.
- WFM-008: The platform MUST provide a wage and grievance statement in plain Hindi and English.
- WFM-009: Worker location MAY be collected only while on an accepted active route or task, with visible status and automatic shutoff.
- WFM-010: A worker MUST have an SOS path that shares current task and location with operations.
- WFM-011: A rating or AI score MUST NOT automatically reject, suspend, or terminate a worker.
- WFM-012: Adverse action MUST show the evidence considered and provide human review.
- WFM-013: The system MUST prevent solo assignment to tasks marked as two-person or high-risk.
- WFM-014: Chemical, ladder, electrical, gas, pest, heavy-load, and other restricted work MUST require explicit certification or route to a specialist vendor.

## 9. Vehicle and route requirements

- VEH-001: Each fleet asset MUST record model, serial number, rated motor power, maximum design speed, battery serial, charger, compliance documents, purchase date, warranty, service history, assigned custodian, and status.
- VEH-002: An e-bike MUST NOT be dispatched if a safety inspection, service, battery, brake, tire, light, or compliance item is overdue.
- VEH-003: The worker MUST complete a short pre-use check and report damage.
- VEH-004: Custody events MUST record handover, return, condition, accessories, and acknowledgements.
- VEH-005: A vehicle incident MUST create a linked safety ticket and freeze the asset until reviewed.
- VEH-006: Route planning MUST minimize travel while respecting time windows and worker limits.
- VEH-007: Route optimization MUST be advisory during the pilot; operations can override with a reason.
- VEH-008: The platform MUST NOT enable passenger requests.
- VEH-009: Off-duty tracking MUST be disabled.

## 10. Package and catalog requirements

- CAT-001: Each catalog item MUST have SKU, name, category, brand, pack size, unit cost, owner price, tax class, supplier, country of origin where required, status, shelf-life rule, substitution group, and operational suitability.
- CAT-002: Items MUST be labeled as `Curators standard`, `Owner preferred`, `Alternative`, or `Sponsored` when that later feature is enabled.
- CAT-003: A sponsored status MUST NOT be concealed inside operational ranking.
- CAT-004: The owner MUST be able to add individual items or a bundle to a property package.
- CAT-005: The owner MUST see one-time setup cost, estimated monthly consumption, estimated monthly cost, and substitution behavior before approval.
- CAT-006: The owner MUST be able to set SKU, category, order, and monthly budget limits.
- CAT-007: The owner MUST be able to require approval for any substitution, price increase, or new SKU.
- CAT-008: Package changes MUST be versioned with effective dates.
- CAT-009: An automatic package MUST still present a review summary before first activation.
- CAT-010: Claims about quality, sustainability, performance, or origin MUST have source evidence.

## 11. Inventory and procurement requirements

- INV-001: Inventory MUST use an append-only movement ledger. Current balance is derived from movements and verified counts.
- INV-002: Movement types MUST include receipt, allocation, transfer, consumption, return, damage, expiry, shrinkage, count adjustment, and supplier return.
- INV-003: Every consumption MUST link to a property and ticket or include a documented exception.
- INV-004: The system MUST support central, in-transit, worker-kit, and property stock locations.
- INV-005: Reorder point MUST be calculated from lead-time consumption and safety stock.
- INV-006: A stock recommendation MUST show demand basis, on-hand, committed, in-transit, lead time, safety stock, supplier, unit cost, and budget effect.
- INV-007: Jarvis or the inventory AI MAY draft a requisition but MUST NOT approve its own requisition.
- INV-008: Purchases over a configurable threshold, from a new supplier, or with an out-of-band price MUST require human approval.
- INV-009: Receiving MUST require quantity and condition confirmation and SHOULD capture invoice or delivery evidence.
- INV-010: Inventory counts MUST be scheduled by risk and variance.
- INV-011: Food SKUs MUST remain disabled until the required FSSAI activity and premise approvals are recorded.
- INV-012: The system MUST track expiry or best-before dates where applicable and prevent issue of expired stock.
- INV-013: Supplier rebates, credits, and sponsored income MUST be recorded separately from item cost.

## 12. Documenthand requirements

- DOC-001: A document MUST have owner, record type, property or worker scope, issuer, issue date, expiry date, version, sensitivity, review status, and retention class.
- DOC-002: Original files MUST be immutable; corrections create a new version.
- DOC-003: Extracted fields MUST carry source location and confidence.
- DOC-004: Low-confidence or legally material fields MUST require human verification.
- DOC-005: Checklists MUST identify missing, expired, inconsistent, and upcoming-renewal items.
- DOC-006: External submission MUST require an authorized human confirmation in the MVP.
- DOC-007: The system MUST preserve the exact submitted version and receipt.
- DOC-008: Electronic signatures MUST use an approved signing path and MUST NOT be simulated by AI.
- DOC-009: Retention and deletion MUST follow the record class and applicable law.
- DOC-010: Jarvis MAY assemble a review packet but MUST NOT certify its legal sufficiency.

## 13. Billing, accounting, and approval requirements

- FIN-001: Owner charges MUST distinguish management fee, task service, purchased goods, reimbursement, vendor fee, discount, rebate, tax, refund, and credit.
- FIN-002: The percentage-fee base MUST exclude taxes, refundable deposits, and pass-through cleaning unless the owner contract explicitly states otherwise.
- FIN-003: Every invoice line MUST link to a contract rule, ticket, order, or approved manual adjustment.
- FIN-004: The system MUST maintain an operational subledger and export to the licensed accounting system of record.
- FIN-005: AI MAY propose coding and reconciliation but MUST NOT post a final journal without human approval.
- FIN-006: Vendor creation, bank-detail change, purchase approval, and payment approval MUST be separated according to risk.
- FIN-007: Bank-detail changes MUST require out-of-band verification.
- FIN-008: No user may both create and approve the same high-risk payment.
- FIN-009: Statutory audit representation MUST be disabled unless a legally eligible CA-led service is separately established.
- FIN-010: Financial corrections MUST use reversals or credit notes, not deletion.
- FIN-011: The system MUST generate property contribution reports showing revenue, direct labor, travel, supply margin, vendor cost, refund, and exception cost.

## 14. Jarvis requirements

### Property scope

- HM-001: Every Jarvis invocation MUST begin with an authenticated tenant, property, actor, trigger, and correlation identifier.
- HM-002: Jarvis MUST receive only the minimum property context required for the task.
- HM-003: Jarvis tools MUST enforce property scope in code. Prompt instructions are insufficient authorization.
- HM-004: A cross-property operation MUST use a separate operations workflow, not a property Jarvis session.

### Allowed MVP actions

Jarvis MAY:

- read an authorized property calendar and operational record;
- create a draft or proposed ticket;
- propose schedule and worker requirements;
- request owner or operations approval;
- send an approved template notification;
- query property stock;
- propose a restock requisition;
- assemble a document review packet;
- summarize incidents and exceptions; and
- escalate to an authorized human.

### Prohibited MVP actions

Jarvis MUST NOT:

- hard-delete a ticket, reservation, document, stock movement, invoice, or audit record;
- place an unapproved external order;
- approve its own purchase or reimbursement;
- release payment or alter bank details;
- sign or file a regulated document;
- expose property access secrets outside an authorized service window;
- reject, suspend, or terminate a worker;
- change wages or contractual terms;
- mark high-risk work verified without an independent human; or
- message an owner or guest outside permitted templates and communication policy.

### Reliability

- HM-005: Every tool call MUST be recorded with input class, result, policy decision, and actor context. Sensitive values MAY be redacted.
- HM-006: A failed or uncertain tool result MUST fail closed and create an exception rather than assume success.
- HM-007: Repeated tool calls MUST use idempotency keys.
- HM-008: Jarvis MUST identify uncertainty and request review when required information conflicts.
- HM-009: A model response alone MUST NOT change operational state.
- HM-010: All model and prompt versions used for an action MUST be traceable.

## 15. Notifications and communications

- COM-001: The system MUST separate transactional, urgent, marketing, and sponsored communication consent.
- COM-002: Owners MUST be able to configure severity, channel, quiet hours, and escalation contacts.
- COM-003: Workers MUST receive assignment changes, access revocation, and safety alerts immediately through an approved channel.
- COM-004: Message templates MUST be versioned and localized in Hindi and English where relevant.
- COM-005: Free-form AI messages to guests or owners MUST require review until a later safety gate.
- COM-006: Delivery status and failures MUST be recorded.
- COM-007: Sensitive access details MUST NOT appear in insecure notification previews.

## 16. Privacy and security requirements

- SEC-001: Collect only the personal data necessary for a documented purpose.
- SEC-002: Provide purpose-specific notice and consent or other lawful-basis record where required.
- SEC-003: Provide data access, correction, withdrawal, grievance, and erasure workflows subject to retention law.
- SEC-004: Aadhaar MUST be voluntary with an alternate identity method.
- SEC-005: Store masked Aadhaar or a verification result where sufficient; do not retain biometric data.
- SEC-006: Encrypt data in transit and sensitive data at rest.
- SEC-007: Administrators, finance approvers, and support access MUST use MFA.
- SEC-008: Property access secrets MUST use a dedicated encrypted field and short-lived disclosure.
- SEC-009: All privileged access MUST be logged and reviewed.
- SEC-010: Security logs MUST be retained in India for the period required by applicable CERT-In directions, with an incident-reporting process [S13].
- SEC-011: A vendor handling personal data MUST have a processor contract and security review.
- SEC-012: Production personal data MUST NOT be copied into model-evaluation or development datasets without approved de-identification.
- SEC-013: Secrets MUST NOT be embedded in prompts, source code, or client applications.
- SEC-014: The system MUST support remote session revocation and rapid access-code rotation.

## 17. Consumer and marketplace requirements

- CON-001: Owner prices, taxes, recurring charges, substitutions, cancellation, refund, and grievance contacts MUST be visible before purchase.
- CON-002: Country of origin and seller information MUST be displayed where legally required.
- CON-003: Sponsored placement MUST be conspicuously labeled.
- CON-004: The system MUST NOT use false urgency, hidden recurring charges, preselected paid add-ons, or obstruction of cancellation.
- CON-005: Every advertised claim MUST be supported by retained evidence.
- CON-006: The owner MUST be able to export order, invoice, package, and service history.

## 18. Reporting and controls

The MVP MUST report:

- service-level performance by property and cluster;
- first-pass quality and rework;
- ticket aging and severity;
- worker time, travel, utilization, and overtime;
- property contribution;
- inventory balance, variance, stockout, expiry, and days on hand;
- owner approvals and approval time;
- AI proposal acceptance, rejection, edit, error, and escalation rates;
- property compliance expiration;
- access events and access anomalies;
- complaints and resolution time; and
- vehicle availability, maintenance, and incidents.

Metrics MUST NOT rank workers without context or become an automatic disciplinary decision.

## 19. Non-functional requirements

- NFR-001: The MVP SHOULD support 30 properties, 100 workers or vendors, 50,000 tickets, and 100,000 inventory movements without architectural change.
- NFR-002: The target architecture MUST have a credible path to 10,000 properties through horizontal application scaling and database evolution.
- NFR-003: Owner and worker core pages SHOULD achieve a p95 server response below 500 ms, excluding third-party integrations and file upload.
- NFR-004: Calendar changes SHOULD become visible internally within 15 minutes.
- NFR-005: Critical alerts SHOULD be queued within one minute of the triggering event.
- NFR-006: Operational state changes MUST be durable before the user receives success.
- NFR-007: Background jobs MUST be retryable and idempotent.
- NFR-008: Worker checklists SHOULD function with intermittent connectivity and synchronize safely.
- NFR-009: The system MUST use Indian Standard Time for operating display while storing timestamps in UTC.
- NFR-010: Accessibility SHOULD meet WCAG 2.2 AA for owner and worker core flows.
- NFR-011: Owner and worker critical flows MUST be usable in English and Hindi.
- NFR-012: Backups, restoration tests, recovery time, and recovery point targets MUST be documented before the 15-property launch.

## 20. Launch acceptance suite

The MVP is not ready for the five-property pilot until it demonstrates:

1. Tenant and property isolation tests.
2. Reservation create, change, cancel, overlap, stale-feed, and timezone tests.
3. Ticket-state and permission tests for every role.
4. No hard-delete path for operational records.
5. Duplicate calendar and duplicate order idempotency tests.
6. Worker assignment limits, age gate, and restricted-task routing.
7. Access-secret time window and revocation.
8. Inventory movement reconciliation and count adjustment.
9. Owner budget, substitution, and purchase approval.
10. Maker-checker financial approval.
11. Jarvis prohibited-action tests.
12. Human review for worker adverse action.
13. Offline worker checklist synchronization.
14. Audit-log completeness.
15. Backup restoration and incident simulation.
16. Plain-language consent, grievance, and privacy flows.

## 21. Later-stage gates

### Public specialist marketplace

May begin only after vendor insurance, seller disclosures, tax treatment, consumer grievance, platform due diligence, and service-quality controls are complete.

### Sponsored catalog

May begin only after operational ranking is technically separated, sponsorship labels pass review, and rebate accounting is live.

### External accounting support

May begin only as separately contracted bookkeeping and management-reporting services with qualified supervision, confidentiality controls, and a licensed accounting system. Statutory audit remains outside scope unless delivered by an eligible CA-led practice.

### Youth program

May be evaluated separately, never silently enabled by lowering the age field. It requires a legal memo, parental and data-consent design, daylight and supervision rules, and a non-hazardous task catalog.

### Direct e-bike import

May be evaluated only at the 20-bike demand gate and after compliance, landed-cost, warranty, service, parts, recall, and insurance evidence.


