# Screen Inventory

Every route, what is on it, where the data comes from, and its three states.
`PRD.md` says *why* a screen exists; this says *what is on it*.

Every list screen needs **three** states designed: loading (skeleton), empty
(real copy), populated. The demo database starts empty — you will hit the empty
state before anything else.

---

## Route map

```
/                              → redirect by role: owner → /dashboard, staff → /ops/tickets
/login                         → role picker (dev)
/debug                         → Phase 0 proof page (delete before demo)

(owner)
  /dashboard                   Owner home
  /properties/[id]             Property detail
  /properties/[id]/package     ⭐ Inventory shop + cart → SHOP.md
  /onboarding                  Wizard
  /invoices                    Charges + monthly summary
  /documents                   Document list

(ops)
  /ops/properties              Property table
  /ops/tickets                 Ticket queue
  /ops/tickets/[id]            Ticket detail + dispatch
  /ops/tickets/new             Create ticket
  /ops/workers                 Worker roster
  /ops/calendar                Feed health + reservations

(curator)
  /jobs                        Today's jobs (mobile)
  /jobs/[id]                   Job detail + checklist + evidence
```

---

## `/login` — role picker (dev)

Three large cards: **Owner** · **Staff** · **Guest**. Clicking POSTs to the
verified `/api/auth/session/create` endpoint with the role and a fixed contact.

Not a real login. Label it "Demo access" so nobody mistakes it for auth.
Curator and Operations will both use `staff` in their later portals — the
backend has three roles, not six.

---

## `/dashboard` — Owner home ⭐ P1

**Rule: exceptions, not activity.** Routine internal work must not surface here.
If nothing needs the owner, the page is calm and mostly empty — that is correct.

| Block | Source | Empty state |
|---|---|---|
| Property cards — name, state, readiness, next work | `GET /v1/properties` | "No properties yet" + Add property |
| **Needs your attention** | open approvals, compliance holds, incidents | "No exceptions. We'll surface anything that needs your decision." |
| Upcoming work (next 7 days) | `GET /v1/tickets` filtered forward | "Nothing scheduled this week." |
| Your package | `GET /v1/properties/{id}/packages` | "No package yet" + Build one |
| This month | `GET /v1/reports/property-contribution` | "Your first statement appears after the first completed service." |
| Operating standards | computed from tickets | — |

**Operating standards panel — honesty requirement.** Two columns:
*We control* (response timeliness, turnovers inside window, incident resolution)
and *You control* (pricing, calendar availability, cancellations). Never claim
we control the platform designation. Header: *"We operate your property to
Superhost standards."* Not *"We make you a Superhost."*

`--red` appears **only** in "Needs your attention", and only when non-empty. A
dashboard with nothing wrong has no red on it at all — that is the product working.

---

## `/properties/[id]/package` — Inventory Shop ⭐⭐ P0

**Full spec: [`SHOP.md`](SHOP.md).** Do not build from this summary.

One window: filter rail (240px) · scrolling item grid (fluid) · sticky cart
(340px), separated by 1px black rules. Filters are instant and client-side,
held in the URL. Items drag into the cart. Cost is computed by the server on a
debounced POST and never in the browser.


## `/ops/tickets` — Ticket queue ⭐ P0

Dense table, sticky header. Columns: type · property · requested window ·
status · assignee · age. Filters: property, type, status. Row → detail.

`GET /v1/tickets`. Empty: "No tickets. Create one, or generate turnover
proposals from a reservation."

## `/ops/tickets/[id]` — Ticket detail + dispatch ⭐ P0

- **Header** — type, property, status, requested window
- **Detail** — reason, checklist (`/checklist-items`), evidence (`/evidence`)
- **State history** — `GET /v1/tickets/{id}/state-events`, as a vertical
  timeline. Show history, not just a current value; it demos far better.
- **Dispatch** — `POST /{id}/dispatch/candidates` returns *ranked* workers.
  Render rank, name, zone, skills, and **why** each ranks where it does. The
  eligibility logic (skills, zone, availability, conflicts) is real; surfacing
  it is more impressive than a dropdown. Assign → `.../dispatch/assign`.

Candidates return `null` when no eligible worker exists — render "No eligible
workers in this zone", not an empty table.

## `/ops/tickets/new`

Property select · type select (9 types, `INTEGRATION.md §7`) · requested window
(start/end datetime) · reason. → `POST /v1/tickets`.

## `/ops/properties`

Table: name, address, state, readiness, active package, open tickets.
Row → `/properties/[id]`. Lifecycle advance → `POST /{id}/transitions`.

## `/ops/workers`

Roster from `GET /v1/workers`: name, zone, skills, classification, availability.
Detail drawer shows availability windows and assignments.

## `/ops/calendar`

Feed health (`GET /v1/properties/{id}/calendar-health`), reservations list, and
two buttons: **Poll feed** (`POST /v1/calendar-feeds/{id}/polls`) and
**Generate turnover proposals** (`POST /v1/properties/{id}/turnover-proposals/generate`).

These two buttons are what drive the demo's reservation → work chain. Make them
obvious and show the result count (`{"result":{"proposed":N}}`).

---

## `/onboarding` — Owner wizard P1

Seven steps, progress bar from `GET /v1/onboarding/cases/{id}/progress`:
identity & authority → address & basics → documents → inspection evidence →
service preferences + **autonomy level** → package → contract acceptance.

Each step `PUT /v1/onboarding/cases/{case_id}/sections/{section}`. Progress is
server-side — reloading mid-wizard must not lose it.

**Autonomy level:** a three-option control (advisory / assisted / autonomous).
Collect and display it. **Do not wire it to behaviour** — no backend field
enforces it.

## `/invoices`

Charges, credits, monthly summary from `GET /v1/reports/property-contribution`.
Every amount through `<Money>`. Empty: "No charges yet."

## `/documents`

`GET /v1/properties/{id}/documents`. Name, type, status, uploaded date.
Upload is **stubbed** — POST metadata only, no MinIO.

---

## `/jobs` and `/jobs/[id]` — Curator P2

Mobile-first, 390px target.

**`/jobs`** — today's assignments grouped by time. Each card: property, arrival
window, job type, travel zone. Large touch targets, sticky bottom bar.

**`/jobs/[id]`** — property + access instructions, arrival window, checklist
(`/checklist-items`, tappable), required evidence, safety notes.
Sticky bottom: **Complete job** → posts evidence metadata to
`/v1/tickets/{id}/evidence`, then transitions the ticket.

File upload stubbed — capture the metadata, skip MinIO.

---

## Superhost panel P2

A right-hand `<Sheet>` available on owner and ops surfaces.
`GET /v1/agent-runs/{run_id}` and `.../events`.

Render each proposal with its approval state, and label the boundary explicitly:
**Superhost proposes. A human or a deterministic service decides.** The backend
enforces this for real — Superhost writes only to `ai_tool_calls`,
`policy_decisions`, `approval_requests`, never to a business table.

**Do not build a chat input.** Superhost does not execute, and an input box implies
it does.

---

## Shared

**Nav.** Owner: Dashboard · Properties · Package · Invoices · Documents.
Ops: Tickets · Properties · Workers · Calendar. Curator: none (single stack).

**Role switcher.** Dev-only, top-right, re-mints the session. This is how the
demo moves between portals — make it fast and obvious.

**Toasts.** `sonner`. Surface the API's own `message` verbatim; it names the
offending field.

**Seed reset.** A visible dev-only button that re-runs the seed. It will be
needed, probably under time pressure.
