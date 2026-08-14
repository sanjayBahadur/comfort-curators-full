# Comfort Curators App — Product Requirements

Four surfaces in one Vite + React app. Scoped for a demo, structured so the demo code
is not thrown away afterwards.

**The business in one line:** homeowners hand us their short-term rental; we run
it to Superhost standards using our own inventory, our own field staff, and an
AI property supervisor (Superhost) that coordinates the work.

**Revenue:** subscription · property packages (we sell owners our inventory) ·
operations capacity sold to HR partners.

---

## Priority

| Surface | Demo weight | Ship |
|---|---|---|
| **P0 — Owner: Inventory Shop** | The differentiator | Must |
| **P0 — Ops: Tickets & Dispatch** | Proves we operate, not just sell | Must |
| **P1 — Owner: Dashboard** | Best polish-to-effort ratio | Should |
| **P1 — Owner: Onboarding** | Opens the narrative | Should |
| **P2 — Curator: Job view** | Closes the loop visually | Nice |
| **P2 — Superhost panel** | Cosmetic without the agent loop | Nice |
| **HR portal** | **Cut.** Three-role backend cannot secure it | No |
| **Guest store** | **Cut.** Deferred in the V0 product freeze | No |

---

## P0 — Owner: Inventory Shop

> **Full spec: [`SHOP.md`](SHOP.md)** (layout, drag mechanics, states) and
> [`ART-DIRECTION.md`](ART-DIRECTION.md) (visual system). This section is the
> *why*; build from those.

**Job to be done:** *"Show me what you'll put in my property and what it costs
me every month, and let me change it."*

This is the screen that makes Comfort Curators a product rather than a cleaning
contract. It is also the one with the strongest backend support — cost is
computed server-side and returned on every draft.

**Layout.** Three panes: item palette (left, grouped by category, searchable),
package canvas (centre, drag targets, per-item quantity + expected monthly
consumption), cost panel (right, sticky).

**Behaviour**
- Drag an item from palette → canvas. Set `quantity` and
  `expected_monthly_consumption` inline.
- On every change, POST the draft to
  `POST /v1/properties/{id}/packages` and render the returned
  `setup_cost_minor_units` and `monthly_cost_minor_units`.
  **Never compute cost client-side** — the server is authoritative and already
  returns a `review_summary`.
  Debounce ~400ms so a drag doesn't fire ten round-trips.
- Policy controls: `substitution_policy` (`owner_approval` | `automatic` |
  `restricted`), `require_approval_for_price_increase`,
  `require_approval_for_new_sku`, optional `monthly_budget_limit_minor_units`.
- **Activate** → `POST .../packages/{version_id}/activate`. Draft becomes
  active, envelope `version` bumps. Show the state change.

**Acceptance**
- Dragging three items shows a setup cost and a monthly cost that visibly change
- Cost figures come from the API response, not from local arithmetic
- Activating flips a visible `draft` → `active` badge
- Rejecting an invalid substitution policy shows the API's own message
- Money renders as `₹2,700.00` from `270000` + `"INR"`

**Deliberately not built:** the package does not automatically generate tickets.
See *Demo staging* below.

---

## P0 — Ops: Tickets & Dispatch

**Job to be done:** *"Work arrived. Who is doing it, and is it done?"*

**Screens**
1. **Ticket list** — filter by property, type, status. Columns: type, property,
   requested window, status, assignee.
2. **Ticket detail** — window, reason, checklist items, evidence, state history
   (`GET /v1/tickets/{id}/state-events`).
3. **Dispatch** — `POST /v1/tickets/{id}/dispatch/candidates` returns ranked
   eligible workers. Assign → `POST /v1/tickets/{id}/dispatch/assign`.
   Show *why* a candidate ranks where it does; the eligibility logic is real
   (skills, zone, availability, conflicts) and it is worth showing.
4. **Create ticket** — type, property, requested window, reason.

**Acceptance**
- A ticket can be created and appears in the list
- Candidates return a ranked worker list from real availability data
- Assigning shows the assignment on both the ticket and the worker
- State transitions are visible as history, not just a current value

---

## P1 — Owner: Dashboard

**Job to be done:** *"Is my property fine, and what is it costing me?"*

The product doc is explicit that **routine internal noise must not reach the
owner**. Show exceptions, not activity. If nothing is wrong, the page should
feel calm and mostly empty — that is the product working.

**Panels**
| Panel | Source |
|---|---|
| Property cards — state, readiness | `GET /v1/properties` |
| Needs your attention | open approvals, compliance holds, incidents |
| Upcoming work | `GET /v1/tickets` filtered to future windows |
| Your package + monthly cost | `GET /v1/properties/{id}/packages` |
| This month | `GET /v1/reports/property-contribution` |
| Documents | `GET /v1/properties/{id}/documents` |

**Superhost panel — honesty requirement.** The product doc forbids claiming we
control platform designation. Show controllable metrics (response timeliness,
turnovers inside window, incident resolution time, avoidable rework) and clearly
separate what the owner still controls (pricing, calendar, cancellations).
*"We operate to Superhost standards; here is the gap on what we don't control."*
That gap view is a reason to log in, and it is defensible.

---

## P1 — Owner: Onboarding

**Job to be done:** *"Get me set up without a twenty-field form."*

Wizard over `POST /v1/onboarding/cases` and
`PUT /v1/onboarding/cases/{id}/sections/{section}`, with progress from
`GET /v1/onboarding/cases/{id}/progress`.

Steps: identity & authority → property address & basics → documents →
inspection evidence → service preferences and **autonomy level** → package
selection → contract acceptance.

**Autonomy level has no backend field.** Collect it, store it in the onboarding
section payload, and display it. Do not wire it to behaviour — nothing enforces
it yet, and pretending otherwise is the kind of thing that gets discovered live.

---

## P2 — Curator: Job view

Mobile-first. Today's jobs → job detail (property, access instructions, arrival
window, checklist, required evidence) → complete.

Read-mostly for the demo. Evidence upload can stub the file and POST metadata to
`POST /v1/tickets/{id}/evidence`; MinIO upload is out of scope.

---

## P2 — Superhost panel

A right-hand drawer on the owner and ops surfaces showing agent activity:
`GET /v1/agent-runs/{run_id}` and `.../events`.

**Show the authority model — it is a selling point, not an implementation
detail.** Superhost *proposes*; a human or a deterministic service *decides*.
Render proposals with their approval state and make the boundary legible.
The backend enforces this for real: Superhost writes only to `ai_tool_calls`,
`policy_decisions`, and `approval_requests`, never to a business table.

Do not build a chat box that implies Superhost is executing. It isn't.

---

## Demo staging — the honest bits

Three gaps will be visible if probed. Have answers rather than discovering them
on stage.

1. **Packages do not auto-generate tickets.** The protocol engine does not
   exist. Demo the chain by clicking: activate the package, then generate
   turnover proposals via
   `POST /v1/properties/{id}/turnover-proposals/generate`, then create the
   ticket. Showing a capability by walking it is fine; saying "this fires
   automatically" is not. The honest line: *"the trigger is what we're building
   next."*
2. **Portals are not access-isolated.** Curator, Supervisor, and Vendor are all
   `staff` in the backend. If asked about scoped contractor access, the answer
   is that the six-role model is the next backend milestone.
3. **Autonomy level is captured, not enforced.**

---

## Cross-cutting requirements

- **Money** — one `formatMoney(minorUnits, currency)` helper, used everywhere.
  Never float maths.
- **Empty states** — every list is empty on a fresh database. Design them; the
  demo will hit them.
- **Errors** — surface the API's `message` verbatim; it names the offending
  field. Show `request_id` in a dev-only corner.
- **Loading** — skeletons, not spinners, on the dashboard and lists.
- **Role switcher** — a dev-only control to re-mint the session as
  `owner` / `staff` / `guest`. This is how the demo moves between portals.
- **Seed reset** — a visible way to re-run the seed. It will be needed.

## Definition of done for the demo

A single uninterrupted walkthrough:

> Owner signs in → onboards a property → drags six items into a package and
> watches the monthly cost compute → activates it → a reservation produces a
> turnover proposal → ops turns it into a ticket → dispatch ranks three curators
> → one is assigned → the curator view shows the job → the owner dashboard shows
> the work and the month's cost.

Every endpoint in that path is verified working in `INTEGRATION.md`.
