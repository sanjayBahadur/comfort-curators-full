# Integration Map — verified against the live backend

Every call below was **executed against the running stack on 2026-08-08** and
returned the response shown. Nothing here is inferred from documentation.

Phase 2 additions were first read from the halted backend's concrete handlers,
then exercised by two clean, consecutive live seed runs on 2026-08-08.

Base: `http://127.0.0.1:8080` (via the app's `/api` proxy — see ARCHITECTURE §1).
Tenant used throughout: `11111111-1111-4111-8111-111111111111`.

---

## 1. Boot the backend

```bash
cd ~/open-code-projects/comfort-curators-backend-alt
export CC_BUILD_TAGS=acceptance     # REQUIRED — without it there is no login route
docker compose up -d --build api worker postgres minio model-stub
curl -s http://127.0.0.1:8080/health/ready
# {"status":"ok","checks":{"database":"ok","minio":"ok","model":"ok"}}
```

## 2. Auth — `POST /auth/session/create`

```jsonc
// request
{ "tenant_id": "1111...", "contact": "owner@demo.test", "roles": ["owner"] }
// response
{ "roles": ["owner"],
  "session_token": "ac743619dde7...",
  "user_id": "750f5460-09fa-4279-8c94-bb9067369611" }
```

Roles: `owner` · `guest` · `staff` · `superhost`. User is created on first call.
Send the token as `Authorization: Bearer <token>` on every subsequent request.

**Unauthenticated requests return `401`** for everything except
`/health/live`, `/health/ready`, `/auth/otp/request`, `/auth/otp/verify`,
`/auth/session/create`, `/v1/communications/secure-links/redeem`.

## 3. Response conventions

```jsonc
// writes
{ "id": "prop_f13af52428579ba3", "version": 1, "data": { /* the resource */ } }
// errors
{ "code": "VALIDATION_ERROR",
  "message": "invalid substitution policy: \"allow_equivalent\"",
  "request_id": "89d7176e93d1..." }
```

Correlation headers on every response: `X-Request-Id`, `X-Correlation-Id`,
`X-Trace-Id`, `X-Span-Id`. Log `X-Request-Id` on failures — it is greppable in
the API container logs.

## 4. Catalog — the store inventory

### `POST /v1/catalog/items` ✅ verified

```jsonc
{ "sku": "TOWEL-01", "name": "Bath Towel 500gsm", "category": "linen",
  "brand": "Trident", "pack_size": "1",
  "unit_cost_minor_units": 32000,  "unit_cost_currency": "INR",
  "owner_price_minor_units": 45000, "owner_price_currency": "INR",
  "tax_class": "gst_5", "supplier": "Trident Ltd", "country_of_origin": "IN",
  "status": "active", "shelf_life_rule": "none",
  "substitution_group": "towels", "operational_suitability": "high",
  "label": "curators_standard" }
→ { "id": "cit_aba44b275b239cca", "version": 1, "data": {...} }
```

**`label` is required and validated.** Valid: `curators_standard`,
`owner_preferred`, `alternative`. `sponsored` exists as a constant but is
**rejected outright** — sponsored placement is disabled by product policy and
cannot be concealed. Omitting `label` gives
`invalid operational label: ""`.

`status`: `active` | `disabled`.

### `GET /v1/catalog/items` ✅
→ `{ "items": [...], "total": 1 }` — note: **not** the envelope, and empty is
`{"items":[],"total":0}` with `200`.

### Other catalog routes
`GET|POST /v1/catalog/templates` · `GET /v1/catalog/templates/{id}` ·
`GET|POST /v1/catalog/items/{id}/claims`

## 5. Properties

### `POST /v1/properties` ✅ verified

```jsonc
{ "idempotency_key": "demo-prop-1",
  "tenant_id": "1111...",
  "owner_authority_id": "2222-2222-4222-8222-222222222222",
  "service_address": { "line1": "12 Gomti Nagar", "city": "Lucknow",
                       "state": "UP", "postal_code": "226010", "country": "IN" },
  "timezone": "Asia/Kolkata",
  "maximum_occupancy": 4,
  "emergency_contacts": [{ "name": "...", "phone": "...", "role": "..." }] }
→ { "id": "prop_f13af52428579ba3", "version": 1,
    "data": { "state": "lead", "readiness": { "owner_contract_accepted": false,
              "compliance_complete": false, ... }, ... } }
```

**A new property starts in `lead`** with an all-false `readiness` object.
Lifecycle: `lead` → `qualifying` → `onboarding` → `remediation` →
`ready_inactive` → `active` → `paused` → `suspended` → `offboarding` → `archived`.

`idempotency_key` is required — reuse the same key to make the seed re-runnable.

### Also
`GET /v1/properties` · `GET /v1/properties/{id}` ·
`POST /v1/properties/{id}/transitions` (advance lifecycle) ·
`PUT /v1/properties/{id}/readiness` · `GET /v1/properties/{id}/transitions`

Live-verified Phase 2 request details:

- `PUT /v1/properties/{id}/readiness` takes
  `{ "owner_contract_accepted": true, "compliance_complete": true,
     "mandatory_fields_set": true }` and returns the property envelope.
- A property transition requires any non-empty `If-Match` header and a body of
  `{ "to_state": "qualifying", "reason": "...", "evidence_ids": [] }`.
  `idempotency_key` is accepted in JSON but is not used by the handler.
- The create-property handler also accepts `idempotency_key` but does not pass
  it to the service or store. A safe client must list properties and match a
  stable address before POSTing; the key alone does **not** prevent duplicates
  in this halted build.

Verified missing-property response from `GET /v1/properties/not-a-real-property`:
`404 { "code": "NOT_FOUND", "message": "property not found", "request_id": "..." }`.
Phase 1 uses this real response to prove that global toasts surface API messages.

### Owner-authority visibility

Property creation records a grant from the **actor making the create request** to
the supplied `owner_authority_id`. `GET /v1/properties` is tenant-wide for staff,
but an `owner` subject receives only properties whose authority is granted to
that owner actor. Live verification on 2026-08-08 showed the original staff-created
Phase 2 data as `2` properties for staff and `0` for `owner@demo.test`.

Therefore clean demo data must create the managed properties while authenticated
as the stable `owner@demo.test` actor, then switch back to staff for operational
resources. Creating a property as staff and merely putting an owner-like UUID in
`owner_authority_id` does not make it visible to a later owner session. The seed
now detects this legacy mismatch and exits before creating duplicates.

## 6. Packages — the demo centrepiece

### `POST /v1/properties/{property_id}/packages` ✅ verified

```jsonc
{ "effective_date": "2026-08-09T00:00:00Z",
  "substitution_policy": "owner_approval",
  "require_approval_for_price_increase": true,
  "require_approval_for_new_sku": true,
  "items": [{ "catalog_item_id": "cit_aba44b275b239cca",
              "quantity": 6, "expected_monthly_consumption": 12,
              "order_index": 0 }],
  "bundles": [{ "package_template_id": "...", "order_index": 1 }] }
```

**The backend computes the pricing.** Real response:

```jsonc
{ "id": "pkg_8447af804d935ebb", "version": 1,
  "data": { "status": "draft", "version_number": 1,
            "setup_cost_minor_units": 270000,      // ₹2,700  = 6 × ₹450
            "monthly_cost_minor_units": 540000,    // ₹5,400  = 12 × ₹450
            "monthly_consumption_units": 12,
            "currency": "INR",
            "review_summary": { ... } } }
```

**Do not compute cost in the frontend.** POST the draft on every change and
render `setup_cost_minor_units` / `monthly_cost_minor_units` from the response.
The live cost panel is a server round-trip, and it is authoritative.

`substitution_policy` — valid values only: `owner_approval` · `automatic` ·
`restricted`. Anything else → `VALIDATION_ERROR`.

### `POST /v1/properties/{id}/packages/{version_id}/activate` ✅ verified
`draft` → `active`, envelope `version` bumps 1 → 2.
Sibling: `.../reject`. Only a `draft` version can be activated or rejected.

Creating a draft returns HTTP `201` with the normal resource envelope. Every
successful POST creates an immutable package version with a new `pkg_*` ID; the
client must activate the ID from the response matching its current cart, not an
older draft or the envelope's integer `version` field.

The halted package payload has no monthly-budget field. The Phase 3 shop may
capture and display an optional budget limit, but it must not send an invented
field or imply that the backend enforces the limit.

### `GET /v1/properties/{id}/packages` · `GET .../packages/{version_id}`

## 7. Tickets & dispatch

### `POST /v1/tickets` ✅ verified

```jsonc
{ "tenant_id": "1111...", "property_id": "prop_f13af...",
  "type": "turnover",
  "requested_window": { "start": "2026-08-09T10:00:00Z",
                        "end":   "2026-08-09T14:00:00Z" },
  "reason": "Guest checkout turnover",
  "checklist_version_id": "" }
→ { "id": "tkt_c600ad8132664dee", "version": 1,
    "data": { "status": "draft", ... } }
```

Types: `turnover` · `pre_arrival_inspection` · `restock` · `incident` ·
`routine_maintenance` · `specialist_vendor_request` · `property_onboarding` ·
`document_review` · `inventory_count`.

Ticket states begin `draft` → `proposed` → … Advance via
`POST /v1/tickets/{id}/transitions`.

### Dispatch chain ✅ verified
```
POST /v1/tickets/{id}/dispatch/candidates   → ranked eligible workers
POST /v1/tickets/{id}/dispatch/assign       → create assignment
POST /v1/dispatch/assignments/{id}/accept   → worker accepts
POST /v1/tickets/{id}/evidence              → completion evidence
GET  /v1/tickets/{id}/checklist-items
GET  /v1/tickets  ·  GET /v1/tickets/{id}
```

Curator field completion uses the verified `POST /v1/tickets/{id}/checklist-syncs`
route with `{ "items": [{ "template_item_index": 0, "label": "...",
"status": "pending|in_progress|completed|not_applicable", "evidence_required":
true|false, "evidence_ids": [] }] }`. It returns the checklist collection and
updates existing rows by `template_item_index`; the client preserves the server's
`evidence_required` flag. The demo seed attaches two idempotent checklist rows to
each Phase 2 ticket when none exist.

`POST /v1/tickets/{id}/evidence` accepts metadata only:
`{ checklist_item_id?, object_id?, content_hash, file_name?, content_type?,
size_bytes }`. `content_hash` must be a SHA-256 hex digest. No MinIO upload is
performed by the curator frontend. Required evidence is linked to its checklist
item before `evidence_submitted` is requested; the backend enforces the evidence
gate.

`GET /v1/tickets` currently applies `property_id` as a mandatory SQL predicate
even though the handler presents it as an optional filter. Without
`?property_id=prop_*` it returns an empty collection. To build a tenant-wide
ticket list, request each accessible property and combine the collections.

Live-verified Phase 2 dispatch details:

- Tickets must reach `scheduled` before assignment: `draft` → `proposed` →
  `approved` → `scheduled`, using
  `{ "to_state": "...", "reason": "...", "evidence_ids": [] }` at
  `POST /v1/tickets/{id}/transitions`.
- Candidates accepts `{}` (or `{ "work_type": "turnover" }`) and returns
  `{ "data": { "ticket_id": "...", "candidates": [{ "worker_id": "...",
  "eligible": true, "score": 65, "checks": [...] }] } }`.
- The candidate array is returned in database order, not score order. Rank it in
  the client with eligible workers first and score descending; each check is
  `{ "constraint": "skill", "hard": true, "passed": false, "detail": "..." }`.
- Assign takes `{ "worker_id": "wrk_*" }`. Reuse is client-owned: first check
  `GET /v1/tickets/{id}/dispatch/assignments`, because repeated assignment POSTs
  create repeated offers. Its empty response is `{ "items": null,
  "next_cursor": null }`, unlike most collection endpoints that normalize empty
  items to `[]`.
- A successful assign creates an assignment offer but does not change the
  ticket's `status`, populate `assigned_to`, or add a ticket state event. Display
  the assignee from the assignment collection and disable another offer while a
  non-declined assignment exists.
- Dispatch requires at least one availability window. Its skill mapping uses
  `cleaning|turnover` for turnover, `restock|inventory` for restock, and
  `maintenance|general` for routine maintenance. The friendly skill
  `restocking` alone does not satisfy a `restock` ticket.

## 8. Workers — `POST /v1/workers` ✅ verified

```jsonc
{ "legal_name": "Asha Verma", "date_of_birth": "1996-04-12T00:00:00Z",
  "contact_method": "+91...", "classification": "employee",
  "specialist": false, "service_zone": "lucknow-central",
  "skills": ["cleaning","linen"], "verified_identity": true }
```
⚠️ **`date_of_birth` must be full RFC3339.** A bare `"1996-04-12"` returns
`invalid date_of_birth, use RFC3339`. Verified.

`GET /v1/workers` · `GET /v1/workers/{id}/availability-windows` ·
`POST /v1/workers/{id}/availability-windows`

An availability-window POST takes `{ "day_of_week": 0, "start_minute": 0,
"end_minute": 1439, "effective_at": "2026-01-01T00:00:00Z" }`. Days are
Sunday `0` through Saturday `6`; both minutes must be within `0..1439` and
start must be lower than end.

⚠️ **`dispatch/candidates` returns `{"data":{"ticket_id":"...","candidates":null}}`**
when no eligible worker exists — not an error, not an empty array. Seed workers
before tickets, and render `null` as "No eligible workers in this zone".

## 9. Reservations

> ⚠️ **There is no endpoint that creates a reservation.** Reservations exist only
> when the backend **fetches and parses an iCal URL** over HTTP from inside its
> container. Verified: the API container reaches the host at
> `host.docker.internal`, so serving `public/demo.ics` from the Next dev server
> and registering `http://host.docker.internal:3000/demo.ics` works. Full recipe
> in `SETUP.md §6`. Without it the reservation → turnover → ticket chain is dead.

```
POST /v1/properties/{id}/calendar-feeds              # register an iCal feed
POST /v1/calendar-feeds/{feed_id}/polls              # force a poll
GET  /v1/properties/{id}/reservations
GET  /v1/properties/{id}/turnover-proposals
POST /v1/properties/{id}/turnover-proposals/generate # ← drives the demo chain
GET  /v1/properties/{id}/calendar-health
```

`turnover-proposals/generate` ✅ verified — returns
`{"result":{"proposed":N,"updated":N,"cancelled":N,"skipped":false}}`.
`proposed: 0` means no reservations exist yet, not a failure. It is the closest
thing to the protocol engine and is what makes the reservation → work chain
demonstrable.

`POST /v1/properties/{id}/calendar-feeds` body:
`{ "source": "airbnb", "url": "...", "property_timezone": "Asia/Kolkata",
   "stale_after_minutes": 120, "minimum_turnaround_minutes": 180 }`

Live-verified Phase 2 calendar details:

- `GET /v1/properties/{id}/calendar-feeds` returns `{ "items": [envelopes] }`.
  There is no uniqueness constraint on feed URL, so reuse must match the URL
  before creating a feed.
- Poll returns `{ "status": "accepted", "result": { "unchanged": false,
  "events_created": N, "reservations_created": N,
  "proposals_proposed": N, ... } }`.
- Polling new content already synchronizes turnover and inspection proposals.
  Therefore an immediate explicit `turnover-proposals/generate` call is an
  idempotency check and can correctly return `proposed: 0`; verify the proposal
  collection or the poll's `proposals_proposed`, not only the later generate
  count.

## 10. Owner dashboard sources

| Panel | Endpoint |
|---|---|
| Properties + health | `GET /v1/properties`, `GET /v1/properties/{id}` |
| Owner-visible exceptions | `GET /v1/reporting/owner-exceptions` |
| Upcoming work | `GET /v1/tickets?property_id={id}` for every owner-visible property |
| Package + cost | `GET /v1/properties/{id}/packages` |
| Current-period contribution | `GET /v1/reporting/property-contribution?property_id={id}&period_start={RFC3339}&period_end={RFC3339}` |
| All recorded contribution | `GET /v1/reports/property-contribution?property_id={id}` |
| Documents | `GET /v1/properties/{id}/documents` |
| Onboarding progress | `GET /v1/onboarding/cases/{id}/progress` |
| Superhost activity | `GET /v1/agent-runs/{run_id}`, `GET /v1/agent-runs/{run_id}/events` |

The Superhost activity routes are run-ID based and read-only for the frontend. A run
read returns the plain `AgentRun` object (`run_id`, `run_kind`, `state`, provider
and model, attempt counters, optional input/output/error data, usage fields, and
timestamps). The events read returns `{ "run_id": "...", "events": [...] }`,
where each event has `event_id`, `event_name`, optional `event_data`, and
`occurred_at`. There is no list/discovery endpoint for runs. The Phase 8 drawer
therefore renders a linked run only when the URL carries `?run_id=...`; otherwise
it shows an explicit unlinked state and never starts or fabricates a run.

Live-verified Phase 5 owner details:

- The property list is already filtered by the authenticated owner's authority.
  Readiness contains exactly `owner_contract_accepted`,
  `compliance_complete`, and `mandatory_fields_set`; the property is ready only
  when all three are true. There is no property-name field, so owner-facing
  labels use the address.
- `GET /v1/reporting/owner-exceptions` returns an envelope collection of
  `source`, `source_id`, `property_id`, `label`, `summary`, `severity`, `status`,
  `occurred_at`, and `owner_visible`. A quiet portfolio can return
  `{ "items": null }`. The backend suppresses routine internal activity; the
  client additionally restricts results to owner-visible property IDs and
  `owner_visible: true`.
- The period-bounded reporting endpoint returns one envelope with
  `revenue_minor_units`, `supply_margin_minor_units`,
  `vendor_cost_minor_units`, `refund_minor_units`,
  `exception_cost_minor_units`, `discount_minor_units`, `tax_minor_units`,
  `net_contribution_minor_units`, and `currency`. This is the source for the
  dashboard's “This month” panel.
- `GET /v1/reports/property-contribution` is a different, all-time report. It
  accepts no period parameters and returns a collection with one zero-valued
  report even when no activity exists. Do not label it as a monthly statement
  or treat the active package cost as billed contribution.
- Package versions are ordered by version number and at most one can be
  `active`. Select that explicit status only and display its server-provided
  `monthly_cost_minor_units` and `currency`; never substitute the latest draft
  or recompute the price.
- The documents list can return `{ "items": null, "next_cursor": null }`.
  List data provides `title`, `document_type`, `status`, `current_version`,
  timestamps, and optional `expires_at`; use `title` as the display name and
  describe `created_at` as “Added”, not “Uploaded”.
- No verified endpoint currently supplies response percentage,
  turnover-in-window percentage, incident resolution time, or rework rate.
  Operating standards must stay qualitative until those measurements exist.

## 11. Owner onboarding

Phase 6 exercised the owner onboarding path through the browser against the
acceptance backend. One case reached all 15 server checklist items, and a full
reload restored both `15 / 15` progress and the stored autonomy marker without
onboarding state in `localStorage` or `sessionStorage`.

### Start, discover, and resume

- Prefer owner-scoped `POST /v1/owners/onboarding-cases` with
  `{ "property_id": "prop_*", "owner_authority_id": "..." }`. It returns the
  case envelope with `201`, requires the owner role, verifies that the property
  exists and is owner-visible, and must receive the property's exact
  `owner_authority_id`.
- A case requires an existing property. `POST /v1/properties` can first create
  a real `lead` property with `owner_authority_id`, `service_address`,
  `timezone`, `access_method`, and positive `maximum_occupancy`. Property POST
  has no idempotency behavior; check owner-visible properties for a normalized
  address match before retrying.
- `GET /v1/onboarding/cases` returns `{ "items": [...] }`, newest first, but its
  rows contain only case identity, property/authority, status, version, and
  timestamps. The generic list is tenant-scoped. Filter it to property IDs from
  the owner's `GET /v1/properties`, then hydrate the selected case with
  `GET /v1/onboarding/cases/{case_id}`.
- `GET /v1/onboarding/cases/{case_id}/progress` returns
  `{ "progress": [{ "key": "portfolio", "complete": true }, ...] }`.
  This endpoint and the full-case GET are the reload/resume truth; browser
  storage must not hold wizard progress or form payloads.

### Persisted sections

`PUT /v1/onboarding/cases/{case_id}/sections/{section}` takes
`{ "payload": <typed section> }`, returns the full case envelope, and increments
its version. The only valid section path keys are:

```
portfolio · goals · service_preferences · budgets · photographs · amenities
safety · furnishing · remediation · fit_score_inputs
```

`documents`, `legal_evidence`, `safety_evidence`, and `inspections` are progress
keys, not valid PUT section names. Use the dedicated mutation routes:

```
PUT  /v1/onboarding/cases/{id}/contacts
POST /v1/onboarding/cases/{id}/evidence     # kind: document|legal|safety
POST /v1/onboarding/cases/{id}/inspections
```

The progress response always has 15 stable keys: `portfolio`, `goals`,
`service_preferences`, `budgets`, `contacts`, `photographs`, `amenities`,
`safety`, `furnishing`, `remediation`, `fit_score_inputs`, `documents`,
`legal_evidence`, `safety_evidence`, and `inspections`. The seven product steps
are a presentation grouping over those 15 backend checks.

### Autonomy, package, and contract boundaries

- The backend drops unknown section fields. Persist autonomy inside the real
  `service_preferences.automation_limits` array with the documented marker
  `autonomy_level:advisory|assisted|autonomous`; preserve unrelated limits when
  updating the section. No service consumes that marker, so it is a displayed
  preference only and changes no behavior.
- Onboarding, packages, contracts, and property lifecycle are separate
  aggregates joined only by `property_id`. An onboarding save does not create
  or activate a package, accept a contract, transition the property, or enforce
  autonomy.
- `GET /v1/contracts/agreements` returns an envelope collection. Filter it to
  the current property. `POST /v1/contracts/agreements/{agreement_id}/accept`
  has no body and permanently accepts its current immutable version. The UI may
  offer this only when an actual server-held draft and its exact terms exist;
  it never invents agreement terms.
- A case becomes `ready` only when all 15 checks are complete and legal/safety
  evidence holds are clear. `POST /v1/onboarding/cases/{id}/activate` is a
  separate terminal action and is intentionally not implied by finishing the
  intake UI.

## 12. Seed sequence

Run in this order; each step's output feeds the next.

```
1. POST /auth/session/create                    role=staff        → token
2. POST /v1/catalog/items          × ~15                          → cit_*
3. POST /v1/properties             × 2                            → prop_*
4. POST /v1/properties/{p}/transitions          lead → qualifying …
5. POST /v1/properties/{p}/packages             items from step 2 → pkg_* (draft, priced)
6. POST /v1/properties/{p}/packages/{v}/activate                  → active
7. POST /v1/workers                × 3                            → wrk_*
8. POST /v1/tickets                × 3          turnover/restock  → tkt_*
9. POST /v1/tickets/{t}/dispatch/candidates → /dispatch/assign
```

Keep `idempotency_key` stable on properties for forward compatibility, but also
check the property collection by normalized address before creating. In the
halted build the handler currently ignores the key.

## 13. Gotchas that will cost you an hour each

1. **`CC_BUILD_TAGS=acceptance`** — without it `/auth/session/create` does not
   exist and nothing can log in.
2. **No CORS** — use the Vite dev proxy. Direct `:8080` calls from the
   browser fail preflight with 401.
3. **One tenant for every actor** — cross-tenant reads return empty, which looks
   identical to a broken query.
4. **`label` on catalog items** — required, enum-validated, easy to miss.
5. **`substitution_policy`** — `owner_approval` | `automatic` | `restricted`.
6. **Money is minor units.** `45000` = ₹450.00.
7. **Writes are enveloped, reads mostly are not.** `GET /v1/catalog/items`
   returns `{items,total}`; `POST` returns `{id,version,data}`.
8. **Empty is `200`, not `404`.**
9. **Property idempotency is client-owned in this build.** Match by address;
   the accepted `idempotency_key` is not wired to persistence.
10. **Dispatch needs scheduled tickets, matching backend skill aliases, and an
    availability window.** Merely creating the three workers is insufficient.
11. **Calendar poll creates proposals.** A following generate call may report
    zero new proposals even while the reservation chain is fully healthy.
