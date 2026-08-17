# Superhost demo hardening — session log

Date: 2026-08-17. Scope: `final-demo/` (self-contained backend + frontend copy).
Goal stated by the user: get the Superhost demo genuinely reliable — dashboard
navigation, budget-aware cart building, and the surrounding UI — ahead of a
live presentation.

Everything below was found live (via the running app, the backend's own
event log, or the database) and fixed in this repo. Items marked **REVERTED**
were tried, broke something, and were rolled back on request — noted here so
the same path isn't retried blind.

## Backend (`backend/`)

- **Superhost kickoff message was a wall of text on every fresh thread.**
  Shortened the auto-fired `system_kickoff` prompt to 1-2 short sentences
  instead of "suggest 2-4 things." — `internal/automation/superhost/thread_store.go`

- **Superhost had no price data at all when building a cart.** The only
  thing it ever received for a catalog item was its name — no price, no
  category — so a stated budget was unenforceable by construction. Fixed on
  the frontend (see below); the prompt was rewritten to match, including
  requiring it to re-read the real running total every 2-3 items instead of
  tracking a budget from memory across many adds. —
  `internal/automation/superhost/prompt/v1.md`

- **Hallucinated navigation.** Superhost would say "I've opened the package
  page for X" when it has no navigate tool at all — it can only `ui_click` a
  real link surface. Added an explicit rule against claiming navigation
  before a new page's own surfaces are actually listed, and tightened the
  surface-id rule to be turn-scoped (never reuse an id from an earlier page).
  — same prompt file.

- **Tool-loop iteration cap (6) was silently forcing budget overshoots.**
  Traced a real failure (Mahanagar Suite, "under 2000 INR"): with only 6
  model↔tool round trips per run, adding 8-9 real items left no room to also
  check the running total between adds, so the model added everything blind
  and checked only once, after the cart was already over budget, one
  iteration before the run got killed by the cap. Raised the cap to 30 so a
  real multi-item build has room to interleave adds with real checks. —
  `internal/automation/tool_loop.go`

## Frontend (`frontend/app/`)

- **Terminal / cart panel / "YOUR PACKAGE" modal were all unscrollable.**
  Root cause: the site's Lenis smooth-scroll instance needs an explicit
  `prevent` callback to respect `data-lenis-prevent` — without it that
  attribute does nothing, and Lenis hijacks wheel events everywhere,
  including inside panels with real `overflow-y: auto`. Fixed once, at the
  source. — `src/components/smooth-scroll.tsx`, plus added the missing
  `data-lenis-prevent` to the shared `Modal` component (the cart-view modal
  never had it at all) — `src/components/ui/Modal.tsx`

- **Payment-boundary popup had no way to dismiss it**, and its fixed
  bottom-right position sat directly over the real Activate button it was
  supposed to hand back to. Added a `×` dismiss control that clears the
  notice without re-granting control. — `src/components/superhost/PaymentBoundary.tsx`,
  `payment-boundary.css`

- **Dashboard didn't scope Superhost to the selected property.** This was
  actually intentional at first (a prior incident is documented in the
  code), but the user asked for it explicitly for the demo. Wired
  `setCurrentProperty` to the dashboard's own Property-cabinet selection,
  debounced 250ms so a fast carousel scroll doesn't thrash scope. —
  `src/routes/dashboard.tsx`, `src/components/superhost/GlobalSuperhostDrawer.tsx`

- **Cart panel "stretched"** — the item list greedily grew with every add,
  pushing the SETUP/MONTHLY/RULES/Activate section off-screen, forcing a
  scroll just to find the button. Capped the item list to a fixed max-height
  (scrolls within itself); checkout section now always stays visible. —
  `src/routes/package-shop.css`

- **A wall of `not_granted` errors was the first thing shown on opening
  Superhost**, on a long-lived thread. Root cause: the safeguard against
  replaying old tool-call history only protected events already in the
  browser's cache at mount time — a genuine cache miss (fresh browser, or a
  thread never opened there before) let the real server-side backlog stream
  in afterward and get *actually re-executed* against a session with no
  control granted yet. Replaced the cache-presence check with a wall-clock
  cutoff: anything that happened before the drawer opened is always history,
  never re-run, regardless of when it happens to arrive over the wire. —
  `src/components/superhost/useSuperhostUIActionDriver.ts`

- **Cross-page continuation (dashboard → property's package page, mid
  conversation) would stall out silently.** Two compounding bugs:
  1. The nudge that tells Superhost "you're on a new page now, use what's
     here" only fired on a *thread* change — but once dashboard selection
     syncs scope up front, the thread often doesn't change across that
     navigation at all, so the nudge never fired.
  2. Even after fixing that, a second leftover guard (`pending.threadId ===
     threadId`) — written when a thread change was assumed to always
     accompany a page change — silently blocked the nudge again for exactly
     the same reason.
  Fixed both: trigger on route pathname change (not just thread change), and
  removed the now-incorrect thread-identity guard. —
  `src/components/superhost/SuperhostMount.tsx`

- **Deterministic "dashboard → auto-navigate → build a package" demo path.**
  Live model reasoning across that exact page jump proved too flaky to trust
  live on stage even after the above fixes. Added a scripted path,
  triggered only by a budget/package request sent from a property-scoped
  dashboard thread that hasn't reached a package page yet: it performs the
  same *real* actions (a real `ui.click` navigation, real `ui.click` adds on
  the real catalog, through the same gated driver and control-session
  accounting as everything else) against a fixed 8-item plan, landing at a
  proven ₹2,565 of a ₹3,000 ask every time. The fine-grained, genuinely live
  budget-aware building is completely untouched — it's what runs for this
  same wording everywhere else, including immediately after landing on the
  page this scripted path opens. Two real bugs found and fixed building
  this: it grabbed *any* dashboard link instead of the selected property's
  own, and a stale `useCallback` closure meant it read `propertyId` from
  before the dashboard sync had ever set it. — `src/components/superhost/SuperhostMount.tsx`

- **`too_fast` control-session errors and a rare stray cursor-ring
  position.** Made the `too_fast` retry a bounded loop instead of one
  attempt (absorbs a transient timing gap instead of surfacing it as a
  failure). Added a one-frame settle wait before the cursor ring ever trusts
  an element's position, to close a suspected race right after a page
  navigation/render. — `src/components/superhost/driver-gated.ts`

- **Decor/furniture defaulted to a recurring monthly cost**, same as
  consumables — nobody re-buys a side table every month. New cart lines for
  `decor`/`furniture` category items now default to 0 monthly use (still
  human-editable); everything else unchanged. Confirmed this cannot affect
  the budget ceiling Superhost checks against: setup cost = quantity × price
  and monthly cost = consumption × price are fully independent server-side
  calculations. — `src/routes/package-shop.tsx`

- **Running-total surface wording clarified.** Investigating a real ₹2,200
  vs. ₹2,000 overshoot, the user asked directly whether Superhost might be
  checking monthly cost instead of setup cost — no hard proof either way,
  but the two numbers were presented with equal visual weight, so the
  surface now leads with SETUP explicitly labeled *"the number a stated
  budget applies to"* and calls out monthly as *"not the budget figure."* —
  `src/routes/package-shop.tsx`

- **"CLEAR ALL" added to the cart-view modal**, with the same inline
  confirm pattern already used elsewhere in the app (preset-swap) rather
  than a silent one-click wipe. — `src/routes/package-shop.tsx`, `package-shop.css`

- **REVERTED**: attempted to fix a solid-black empty trailing cell in the
  62-item catalog grid (the grid's own background, meant to only ever peek
  through as a 1px line between cards, shows solid when a trailing row is
  short and a cell has no card at all) by swapping it for a diagonal
  cross-hatch pattern. The user reported this broke the page and asked for
  an immediate revert. Reverted in full — `.shop-grid`'s background is back
  to the original `var(--ink)`, confirmed byte-identical to before. The
  black cell is back too; not re-attempted this session.

## Guest tab rework + Lucknow → Noida (second pass, same day)

- **Guest thread showed staff-facing operational data** (stock balances,
  ticket queues, compliance holds) — technically correct data, completely
  wrong audience. Root cause: `ContextAssembler` never knew the requesting
  account's role at all; the exact same context got assembled and handed to
  Superhost regardless of who was asking. Added `ActorRole` to
  `PropertyContext`/`PortfolioContext`, threaded the real, server-resolved
  role (never client-supplied) through every call site
  (`handler.go`/`thread_store.go`), and gave the prompt a dedicated
  "Talking with a guest" section: don't lead with operational detail a guest
  has no reason to see, and a new, separate guest kickoff message (warm,
  short, stay-focused) instead of the owner/staff operational one. —
  `internal/automation/superhost/{models,context,handler,thread_store}.go`,
  `prompt/v1.md`

- **"Do its best guess and make the help ticket, but only when actually
  asked, and just instruct for misc questions."** This was less a build
  than a policy: added an explicit rule that a real, described problem gets
  a real proposal/ticket the same way staff gets one, but a passing
  question ("what time is checkout") gets answered directly, never ticketed
  -- and to ask rather than guess when it's unclear which one something is.
  — `prompt/v1.md`

- **Real bug found verifying the above, live**: asking Superhost (as a
  guest) to report a genuine problem drove the Stay page's real help form
  correctly in narration but silently submitted with an *empty* ticket
  type — confirmed via a real `422 VALIDATION_ERROR "type is required"`
  from the backend that nothing ever surfaced back to the model (a
  `ui_click`'s own `PolicyAllowed.v1` only confirms the click was sent, not
  that the resulting async form submission was accepted). Root cause: the
  `<select>`'s registered surface label never said what its valid literal
  values actually were, so the model set a human-readable description
  instead of one of the three real option values, which a controlled
  `<select>` accepts without any error while quietly never actually
  changing state. Fixed by spelling out the exact literal values in the
  surface's own label (same fix pattern as the earlier price/category
  labels), plus a general prompt rule against ever substituting a
  description for a dropdown's real value. Verified live end-to-end after
  the fix: a real `restock` ticket, correctly typed, actually created in
  the database from a plain-English guest complaint. — `src/routes/stay.tsx`,
  `prompt/v1.md`

- **"Lucknow" removed from everywhere, replaced with "Noida"** — city
  field and postal codes (real Noida-range PINs, not the Lucknow 226xxx
  range) across all 5 seeded properties, catalog vendor/brand flavor names
  ("Lucknow Essentials/Textiles/Decor/Woodworks" → "Noida..."), the
  contact desk name, worker service-zone slugs, every UI string
  ("GUEST PORTAL / LUCKNOW", "PORTFOLIO / LUCKNOW", the "LUCKNOW · INR"
  fallback), and test fixtures/spec docs. Deliberately did **not** rename
  the specific locality names themselves (Hazratganj, Gomti Nagar, Indira
  Nagar, Aliganj, Mahanagar) or anything derived from them (property
  labels, idempotency keys, ICS filenames, env var names) — those are
  real Lucknow neighborhoods and arguably ought to change too for full
  geographic coherence, but doing so cascades into file names and env var
  names wired through several config files for no benefit the user asked
  for; flagging this scope choice explicitly rather than silently deciding
  it. Full reset (`down -v` + rebuild + reseed) run afterward since
  `seed.ts` changed. — `scripts/seed.ts` and about a dozen frontend/backend
  files, full list in the commit diff.

## Catalog cards redesigned back to "stretch to fit" (no forced square)

- After the square-aspect-ratio fix directly below shipped, the user said
  the forced square wasn't how the catalog used to look at all -- it used
  to stretch each card vertically to fit its own photo, no cropping, and
  that looked fine. Checked: `aspect-ratio: 1` has been in this file since
  this repo's very first commit (`46dabe9`), so it isn't a regression from
  anywhere in this session -- whatever version the user remembers predates
  this frozen copy. Rather than keep debugging a design this repo never
  actually had, rebuilt the card visual to match what was described:
  `.shop-product-visual` no longer forces `aspect-ratio: 1` -- each photo
  now renders at `height: auto` off its own real proportions, and CSS
  Grid still aligns every card in a row to the tallest one (a min-height
  keeps the no-photo two-letter-mark fallback from collapsing, since it
  has no photo to size itself from). The earlier fix's column-width lock
  (`grid-template-columns: minmax(0, 1fr)` on `.shop-product-open`) stays
  -- still required regardless of square vs. stretch, or the same
  horizontal blowout comes back. Verified with real `getBoundingClientRect()`
  measurements and screenshots in both Chromium and Firefox: rows stay
  even, columns stay locked to 262px, no overflow, portrait photos
  (the floor lamps) and the no-photo fallback cards all render cleanly. —
  `src/routes/package-shop.css`

## Catalog grid layout broken in Chrome (product cards overflowing into each other)

- **Reported live**: on the package/catalog page, some product cards'
  photos overflowed way past their own card -- spilling up into the row
  above and down into the row below, with the card's real name/price
  shoved out of view and its ADD button floating mid-image. Confirmed via
  direct DOM measurement (not guessed): a portrait product photo
  (392x800px natural size) was rendering its `.shop-product-visual` box
  at 459x805px instead of the expected 262x262 square -- nearly 3x too
  tall, and not even square despite `aspect-ratio: 1` being set and
  correctly computed on the element. Root cause, confirmed in two parts:
  1. `.shop-product-open`'s implicit single-column grid had no
     `grid-template-columns`, so that column sized itself off its
     widest child's max-content instead of stretching to the card's real
     width -- and since the photo is a replaced element, its own natural
     aspect ratio fed into that max-content calculation.
  2. Even after fixing (1), height still leaked from the photo's natural
     ratio: grid items get an automatic content-based `min-height: auto`,
     which for a box holding a replaced element comes from the photo's
     intrinsic size, not from the box's own `aspect-ratio: 1`.
  Fixed both: `grid-template-columns: minmax(0, 1fr)` on
  `.shop-product-open`, `min-height: 0` on `.shop-product-visual`.
  Verified with direct `getBoundingClientRect()` measurements across the
  whole 62-item catalog (every card now a uniform 262x262 square,
  confirmed with a forced reflow too, so it isn't a load-timing fluke) and
  a full Playwright screenshot scroll-through, including the exact scene
  from the original report (towel sets next to a wall-art tapestry) now
  rendering aligned. — `src/routes/package-shop.css`

## Opening screen had no real content

- **The "LIVE READOUT" beat on the first-visit intro screen (`/`, before
  login) was structurally dead for its actual audience.** Two of its three
  stats (`Open Tickets`, `Curators on Shift`) were hardcoded `null` forever;
  the third fetched `/v1/properties` client-side, which isn't in the
  backend's public-path allowlist (`internal/iam/authgate.go`) and requires
  an authenticated session — confirmed live via network logging that it
  401s for every genuine first-time visitor, not occasionally. So the entire
  "live numbers" section was always empty for the only audience it's ever
  shown to. Beat 2 was also decorative-only (a ruled grid, no heading).
  Rewrote both: removed the dead fetch entirely, gave beat 2 a real heading
  ("Turnover. Dispatch. Billing. Documents. Compliance. / One system, not
  five tools stitched together."), and replaced the fake stats in beat 3
  with three static, always-true capability statements about how Superhost
  actually works (PROPOSES / A HUMAN DECIDES / WHERE YOU ALREADY ARE).
  Verified with `tsc -b --noEmit` (clean) and a fresh no-session Playwright
  pass through all four beats (screenshotted each once its crossfade
  settled — an early screenshot mid-transition looked like an overlap bug,
  confirmed to be just transition timing, not a real defect). —
  `src/routes/intro.tsx`, `src/routes/intro.css`

## Operational notes

- Full environment reset performed once mid-session (`docker compose down
  -v` + rebuild + `npm run seed`) to clear testing debris out of the demo
  data after heavy live testing. Property IDs regenerate on every reseed.
- The backend image must be rebuilt with `CC_BUILD_TAGS=acceptance` for the
  demo-only session-fixture login route (`scripts/seed.ts` depends on it);
  a plain `docker compose up` does not compile that route in.
- Recommended safe demo paths going into the presentation:
  1. **Fine-grained build**: open a property's package page directly, hand
     over control, ask for a budget — verified reliable and reasonably
     fast (interleaves adds with real total-checks).
  2. **Dashboard cross-page demo**: dashboard → select property → "budget
     3000, prepare a package" — now goes through the scripted path above;
     verified reliable and fast (~20s end to end) across repeated runs on
     different properties.
