# Comfort Curators V0 Product Freeze

Status: frozen for backend development  
Capacity: 30 to 50 active properties and at least 1,000 accumulated guest or reservation records

## Product definition

Comfort Curators is a premium property-operations system and managed service for furnished stays. It helps owners reach and maintain Superhost-level operating standards through accountable staff, controlled automation, evidence, and exception-focused owner communication. It does not guarantee an Airbnb designation.

The subscriber is the owner. The guest experience affects reviews and retention, but booking-platform payouts continue to go directly to owners.

## Users

| Role | V0 responsibility |
| --- | --- |
| Owner | Onboards the portfolio, accepts scope, approves exceptions, pays Comfort Curators, and reviews property health |
| Guest | Receives a secure stay link, arrival guidance, rules, issue intake, and checkout guidance |
| Curator | Completes assigned field jobs, checklists, evidence, expenses, custody, and escalations |
| Operations supervisor | Qualifies properties, controls activation, dispatch, incidents, approvals, quality, and service recovery |
| Vendor | Performs approved specialist work under a bounded estimate and evidence requirement |
| HR provider | Exchanges replaceable workforce and payroll records without owning operational dispatch |

Owner and guest identities are distinct even when one person can appear in both contexts.

## Required owner and property lifecycle

Owner onboarding records identity, authority, portfolio, goals, service preferences, budgets, automation limits, contacts, documents, quote, contract acceptance, and billing setup.

Property onboarding records furnishing and listing state, inspection evidence, amenities, access, safety, compliance, service standards, remediation, economics, and activation holds.

Lifecycle states are `lead`, `qualifying`, `onboarding`, `remediation`, `ready_inactive`, `active`, `paused`, `suspended`, `offboarding`, and `archived`. Transitions are explicit and audited.

## Required operational flows

1. A read-only calendar event is normalized and checked for duplicates, changes, cancellation, overlap, timezone, and feed freshness.
2. The reservation creates or updates turnover and inspection proposals without destroying accepted work.
3. Deterministic policy checks property readiness, service scope, time windows, access, stock, staff eligibility, owner approval limits, and conflicts.
4. Dispatch recommends or assigns a qualified Curator or vendor.
5. Field execution records checklist state, evidence, expenses, stock use, access custody, timestamps, and escalation.
6. Incidents and failed commitments enter service recovery with severity, responsibility, owner visibility, and resolution evidence.
7. Completed work creates supported charges and property-level quote-versus-actual reporting.

## Owner product contract

The backend must support a concise owner experience with:

- property readiness and health;
- approvals and exceptions;
- schedule and upcoming work;
- active incidents and resolution state;
- expenses, invoices, credits, and earnings summary;
- documents and compliance holds;
- Jarvis activity and monthly report;
- evidence drill-down when requested.

Routine internal noise must not be promoted to the owner exception feed.

## Curator product contract

The backend must support jobs, route and arrival window, access instructions, checklist, safety notes, required evidence, stock custody, issue escalation, expense submission, completion, local draft state, idempotent synchronization, and visible conflict handling.

## Money boundary

V0 bills owners for onboarding, recurring service, completed work, approved pass-through expenses, credits, and adjustments. Guest purchases and booking payout custody are excluded. Every charge links to the accepted contract component or operational evidence.

Accommodation revenue is never inferred from iCalendar. Until a platform provides an authorized,
reconcilable revenue source, the percentage-fee base comes from an owner-approved statement or
structured manual import with source period, currency, exclusions, evidence, reviewer, and audit
history. Every percentage, minimum, reserve, tax treatment, and markup is a versioned contract rule;
the application ships with no commercial rate selected by default.

## Superhost operations metrics

Track controllable performance: response timeliness, host-caused cancellation risk, rating trajectory, turnovers inside the promised window, guest-impacting incidents, resolution time, avoidable rework, and recurring complaint themes. Do not claim that Comfort Curators controls platform designation.

## Deferred

Property tablets, guest commerce, Jarvis store checkout, room service, direct booking, dynamic pricing, native applications, public marketplaces, imported e-bike business, sponsored products, external accounting services, booking payout custody, and autonomous purchasing are not V0.

