# Phase P7.4 — Demo runbook + the three honest answers

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-flash
- **Status:** complete

## What I built

Wrote `DEMO-RUNBOOK.md` at the frontend repo root (next to `PRD.md` and
`ORCHESTRATION.md`, not inside `app/`). Documentation only — no application
source file was touched.

The runbook follows the required structure:

1. **Before you start** — the clean-seed procedure: the exact docker commands
   from `phase-P7-2-walkthrough-rehearsal.md`'s Setup (`docker compose down`,
   `docker volume rm comfort-curators-backend-alt_pgdata`, `docker compose up
   -d`), plus the boot/health preamble from `SETUP.md §1`, plus the app-then-seed
   order (`npm run dev` on :3000, then `npm run seed`), with a prominent note
   that `CC_BUILD_TAGS=acceptance` must already be baked into the running images
   or nobody can log in.
2. **The walkthrough** — ten numbered steps, one per beat of `PRD.md`'s
   *Definition of done*, each with the screen, the exact click/drag, and what
   the audience should see. Based strictly on the P7.2 rehearsal's verified
   chain: `/login` OWNER `ENTER →`; onboarding narrated; the drag-and-drop shop
   (six items, ₹3,225.00, no add-to-cart button — flagged as a fumble point);
   `ACTIVATE` → V8; `/ops/calendar` AIRBNB/FRESH feed, 2 reservations, 4
   proposals, idempotent `GENERATE TURNOVER PROPOSALS`; `/ops/tickets/new`
   `CREATE DRAFT →`; `PREPARE FOR DISPATCH →` + `FIND RANKED CANDIDATES` (3
   curators); `ASSIGN ASHA →` (OFFERED); `/jobs`; `/dashboard` "4 COMMITTED"
   and Hazratganj Studio ₹3,225.00/month ACTIVE V8.
3. **The control-handover beat** — framed per `IMPLEMENTATION-SPEC.md §3.9.4`
   as the single strongest moment, not a fallback. Includes the honest note that
   the trigger is a manual simulation (no live model-to-intent pipeline per
   `phase-P4-7-grant-revoke-budgets.md`'s Open Questions), the grant/strip/
   ring/revoke mechanics, the three gates, the exact Gate 3 terminal lines, the
   reliable `/debug` section-15 simulation path, and a real-route alternative —
   neither implying the model acted on its own initiative.
4. **The three honest answers** — verbatim from `PRD.md` *Demo staging*, not
   reworded, as a glanceable quick-reference.
5. **If something breaks on stage** — grounded only in bugs that actually
   happened (the blank `property-detail.tsx` page, the `idx_agent_runs_idempotency`
   race) and the documented silent reservation-chain failure modes (feed poll,
   port 3000 / `host: true` / `allowedHosts`), with a fast recovery ladder
   (refresh → idempotent seed → full clean seed) and the product's own honest
   line to close a gap.

## Sources used

- `logs/phase-P7-2-walkthrough-rehearsal.md` — primary source: the real
  walkthrough chain, the two real bugs, the real numbers.
- `PRD.md` — *Definition of done for the demo* and *Demo staging — the honest
  bits* (verbatim answers).
- `IMPLEMENTATION-SPEC.md` §3.1, §3.9.1–§3.9.4 — control-handover spec,
  budgets, the three gates, the §3.9.4 framing.
- `logs/phase-P4-7-grant-revoke-budgets.md`, `logs/phase-P4-8-payment-boundary.md`,
  `logs/phase-P4-9-agent-surface-annotations.md` — budgets, gates, red frame,
  Gate 3 message, `data-agent` routes.
- `SETUP.md` §1/§6, `INTEGRATION.md` — boot command, acceptance tag
  requirement, iCal/reservation chain, idempotent generate behaviour.
- App source for exact button/label text: `app/src/routes/login.tsx`,
  `dashboard.tsx`, `ops-calendar.tsx`, `ops-ticket-new.tsx`,
  `ops-ticket-detail.tsx`, `ops-shared.tsx`, `curator-jobs.tsx`, `stay.tsx`,
  `debug.tsx`, `package-shop.tsx`, `main.tsx`; `app/src/components/superhost/
  ControlFrame.tsx`, `PaymentBoundary.tsx`, `payment-boundary.css`.
- `app/scripts/seed.ts` and `app/public/demo.ics` — seed expectation (2/3/3/1/2/4)
  and feed properties.

## Decisions I made

1. **Led with the P7.2 rehearsal as ground truth.** Every button, screen name,
   and number in the runbook is traceable to that log (or to a verified earlier
   phase log / app source). No capability is claimed that isn't documented as
   verified.
2. **Onboarding stays a narrated step.** Per the P7.2 log's own note and
   `PRD.md`'s wording, "onboards a property" is one narrated beat, not a
   field-by-field walk. The runbook says so explicitly and points to `/onboarding`
   only as context.
3. **The ₹3,225.00 six-item package.** The rehearsal log records the total but
   not which six SKUs. From `package-shop.tsx` I verified a fresh drop defaults
   to quantity 1 / monthly use 1 and monthly cost = Σ(monthly use × owner price),
   so the total equals the sum of the six owner prices. I derived one
   reproducible six-SKU set that sums to ₹3,225.00 (Bath Towel 500gsm · Hand
   Towel · Microfibre Pillow · Shampoo 50ml · Welcome Kit Premium · First Aid
   Kit = 450+200+480+75+1400+620) and flagged it as a derived pick with the
   caveat that any six summing to the total reproduces the documented outcome.
   The runbook keeps the honest "watch the server compute it" framing rather
   than chasing a figure that depends on the pick.
4. **Control-handover beat leads with the honest trigger.** Per the task, the
   runbook does not write a script implying the model decided on its own. It
   presents the reliable `/debug` section-15 simulation (GRANT CONTROL →
   SIMULATE AGENT ENTERING PAYMENT BOUNDARY, which drives the boundary through
   the real gated driver with a genuinely live session) as the on-stage path,
   and explains why a bare human click can never demonstrate Gate 3.
5. **Recovery section is evidence-only.** Only failures that actually occurred
   (the two P7.2 bugs) and the documented silent feed/reservation failure modes
   are included; nothing speculative.

## Open questions

- **Which six SKUs the rehearsal used is not recorded** in the P7.2 log; the
  runbook offers a derived, reproducible set instead of asserting the rehearsal's
  exact items. Non-blocking.
- **Live Gate 3 on a real route** (grant control, then arrive at the `/stay`
  store quote section or the package shop's activate section) is documented in
  P4.8 to fire via the `PaymentBoundary` mount effect but was not separately
  rehearsed live in P7.2. The runbook's primary on-stage path is therefore the
  `/debug` simulation, which is the mechanism P4.8 verified.
- **Date-sensitive figures.** "4 COMMITTED (up from 3)" and "V8" were recorded
  on 2026-08-09 against the seed's fixed seven-day windows; on another demo day
  the committed count reflects whatever falls inside the live seven-day window.
  The runbook flags this rather than promising the exact count.
