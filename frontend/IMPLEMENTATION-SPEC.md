# Implementation Spec — Production Push

**Status:** frozen 2026-08-09. Supersedes nothing; extends `PRD.md`, `SCREENS.md`,
`ART-DIRECTION.md`, `PHASES.md`.

This document is the *what and why*. `ORCHESTRATION.md` is the *who, in what
order, with which model*. Implementing agents read both, plus the existing spec
set, before writing code.

---

## 0. Frozen decisions

These four were decided by the human on 2026-08-09 and are **not open for
re-litigation by an implementing agent.** If you believe one is wrong, write it
in your phase log's *Open questions* section and continue building as specified.

| # | Decision | Consequence |
|---|---|---|
| **D1** | Terminal is a **governed inverted exception** — the one machine surface on a paper page | `ART-DIRECTION.md` gets a new §14. Green is scoped to `.superhost-terminal` only |
| **D2** | Superhost runs a **real backend tool-loop** — policy engine genuinely in the path | Phase 3 is real Go work, not a UI illusion. Requires canned fallbacks |
| **D3** | Store providers are **named but unbranded** — no logos, no scraped content | `StoreProvider` adapter, provider names as plain mono text |
| **D4** | **Rename Jarvis → Superhost everywhere**, code included | Serial Phase 0. 925 occurrences, 62 files, 2 repos, 1 data migration |
| **D5** | Superhost runtime model is **GPT-5.6 Luna** | OpenAI-compatible endpoint the existing provider already speaks. 1.05M context holds the full assembled context plus every tool schema without summarizing |
| **D6** | Superhost gets **real DOM control**, explicitly granted, **invalidated at any payment boundary** | New §3.9. Capability-scoped registry, never `document` access. Three independent gates |

**D5 note — build-time models and the runtime model are different things.**
`opencode.json` sets DeepSeek V4 Pro/Flash for the *agents that build this
repository*. D5 governs the model that *runs inside the product*. Changing one
does not change the other, and an agent must not "helpfully" align them.

### Standing note on D4

The human was advised that *Superhost* is Airbnb's registered program name, that
`PRD.md` already carries an honesty rule distinguishing "we operate to Superhost
standards" from "we make you a Superhost", and that the rename carries trademark
exposure into the investor deck. The human reaffirmed the decision. It is
recorded here once so it is not rediscovered and re-argued in every phase log.

The honesty rule in `PRD.md` and `SCREENS.md` **still stands** and is unaffected
by the rename. The dashboard panel header remains *"We operate your property to
Superhost standards"* — never *"We make you a Superhost."* Renaming the agent
does not license claiming the designation.

---

## 1. What exists today (verified, not recalled)

### Backend — `comfort-curators-backend-alt`

~66k lines of Go, 340 routes, 29 domain packages. Verified present and working:
transactional outbox with relay, PostgreSQL job leases with heartbeat and
dead-lettering, audit-log writes atomic with business mutations, MFA/TOTP,
per-token model usage accounting, GitHub Actions release gate.

**`internal/automation/jarvis/` is the most valuable asset in the repository.**

| File | Lines | What it gives us |
|---|---|---|
| `tools.go` | 8.7k | 12-tool registry, typed `read`/`propose`/`request`. Prohibited-prefix denylist covering `pay_`, `charge_`, `create_order_`, `delete_`, `sign_`, `terminate_worker_` |
| `policy.go` | 9.3k | Policy engine. Maker-checker separation, self-approval prevention, approval state machine |
| `context.go` | 12.6k | Context assembler with cross-property denial |
| `schema.go` | 10.5k | Tool schema persistence |
| `tools_test.go` + `context_test.go` | 51k | Extensive tests |

`ToolCallInput.ValidateScope` rejects any `tenant_id` or `property_id` in model
arguments that does not match the run context. `ApprovalRequest.Decide` returns
`ErrPolicySelfApproval` when approver == requester.

> **The requirement "it should not make the payment, just say it did and ask for
> confirmation" is already enforced in code with tests.** We surface it. We do
> not build it.

### Frontend — `ComfortCurators` @ `origin/dev`

Vite 8 + React 19 + React Router 7 + TanStack Query + Tailwind 4. 37 source
files, 9 completed phases with logs. Design system in `src/index.css` (1219
lines) with correct tokens: `--paper: #faf9f7`, `--ink: #000000`, `--red:
#ff0000`, all radii `0px`, real easing curves. Four fonts wired. Lenis smooth
scroll and blend-difference cursor present.

---

## 2. Defect register

Each entry is a concrete, located problem. Phase assignments in
`ORCHESTRATION.md`.

### DEF-01 — Policy engine is not wired into the runner ⚠️ blocks D2

`internal/platform/app/app.go:487-488` registers only `NewContextAssembler` and
`NewHandler`. `internal/automation/runner.go:99-113` claims a run, makes exactly
one `provider.Call(...)`, and completes. It never consults `PolicyEngine`, never
parses tool calls, never creates an `ApprovalRequest`.

`grep` confirms `NewPolicyEngine` is referenced only in
`internal/automation/evaluation/scenarios.go` and tests.

**Today, model output lands in `output_data` and nothing happens to it.**

### DEF-02 — HTTP provider cannot authenticate

`internal/automation/http_provider.go:106` sets only `Content-Type`. There is no
`Authorization` header, so it cannot reach `api.deepseek.com`. It targets the
local `model-stub` (`app.go:625`, default host `model-stub`).

### DEF-03 — Provider request shape is one-shot

`http_provider.go:73-81` builds a single `user` message. No system prompt, no
`tools` array, no `tool_choice`, no `stream`. `chatResponse` reads
`choices[0].message.content` only — `tool_calls` is not modelled.

### DEF-04 — No streaming anywhere

`grep -rln "websocket|text/event-stream|Flusher"` over `internal/` returns
nothing. A terminal that prints line by line needs SSE.

### DEF-05 — DeepSeek has no pricing entry

`internal/automation/pricing.go:19-28` lists only OpenAI and Anthropic pairs.
Every DeepSeek run therefore reports `usage_known: false` and `usage_minor: 0`.
Correct behaviour by design (`usageForTokens` never fabricates), but it means the
demo cannot show its own cost.

### DEF-06 — Login does not redirect

`src/routes/login.tsx:73-90` calls `signIn`, sets local state, and renders
`<Link>`s. It never navigates. `entry-route.tsx:7` sends `guest` → `/login`,
which is a loop, because guest has no destination.

### DEF-07 — No route guards

`src/main.tsx:47-57` mounts every route unguarded. An `owner` session can open
`/ops/tickets` by typing the URL. Role-scoped tab access does not exist.

### DEF-08 — Six specced routes are not mounted

`SCREENS.md` specifies them; `main.tsx` has no entry for any:
`/properties/:id`, `/invoices`, `/documents`, `/ops/properties`, `/ops/workers`,
`/ops/calendar`.

**`/ops/calendar` is demo-critical.** It carries *Poll feed* and *Generate
turnover proposals* — the two buttons that drive the reservation → turnover →
ticket chain in the `PRD.md` walkthrough. The signature demo currently has no UI
for its middle step.

### DEF-09 — Native form controls

15 native `<select>` elements and 1 native `<dialog>`:

| File | Count |
|---|---|
| `src/routes/onboarding.tsx` | 10 `<select>` |
| `src/routes/ops-tickets.tsx` | 3 `<select>` |
| `src/routes/ops-ticket-new.tsx` | 2 `<select>` |
| `src/routes/package-shop.tsx:298` | 1 `<dialog>` |

Native selects render OS-chrome dropdowns with rounded corners and system fonts.
This is the single most visible violation of "nothing default".

### DEF-10 — Login misregistration effect

`src/routes/login.tsx:115-116` sets `data-copy` on the title, driven by
`index.css:814` (`content: attr(data-copy)`). This is the red offset ghost the
human dislikes. It is `ART-DIRECTION.md §7` *Misregistration*, correctly
implemented — but it is being removed from login by request. **Keep the CSS
utility; remove its use on `/login`.**

### DEF-11 — Dev debris on the login page

`login.tsx` renders a money-format proof panel, a session console showing token
state and property counts, and a *Force API error* button. These are Phase-1
acceptance instruments on what is now a production-facing first impression.
**Move to `/debug`. Do not delete** — they are how phases get verified.

### DEF-12 — No cookie slip

`ART-DIRECTION.md §9` specifies a torn-paper cookie slip in detail. It does not
exist in the codebase.

---

## 3. New work

### 3.1 Intro sequence — `/` (Phase 2)

Reference: ysl.com. Full-bleed, type-led, brief.

**The intro must do real work, not fake a wait.** It runs concurrently with:
minting the demo session, `GET /v1/properties` prefetch into the TanStack Query
cache, and font loading via `document.fonts.ready`.

**Rules**

- **Session-gated.** `sessionStorage.cc_intro_seen`. Plays once. A forced 5s
  gate on every visit to an operations tool is friction, not brand.
- **Skippable from frame one.** A mono `SKIP · ESC` affordance, top right.
  Escape key bound. Never trap someone who has seen it.
- **Floor and ceiling, not a fixed duration.** Minimum 1800ms so it does not
  flash; maximum 5000ms even if prefetch is still running. Resolve on
  `Promise.race([work, ceiling])`.
- `prefers-reduced-motion`: skip entirely, go straight to `/login`.

**Beats** — 4 slides, ~900ms each, driven by the existing
`--ease-expo-out` and `--ease-overshoot` tokens:

```
01  Full-bleed --ink. Instrument Serif, white, clipped reveal from baseline.
    "A property should feel"  /  italic: "handled."
02  Cut to --paper. 12-column rule grid draws in, column by column,
    stagger 0.02. Registration marks snap into the four corners.
03  Three mono counters resolve — properties, open tickets, curators on shift.
    Values come from the prefetched payload. NEVER count up to a fake number;
    cross-fade the resolved value in over 150ms (ART-DIRECTION §10).
04  Wordmark lands. Grain overlay fades to .035. Hand off to /login.
```

Beat 03 is the load-bearing one: it is the moment the intro stops being a splash
screen and becomes a status readout. If prefetch failed, render `—` in mono, not
a zero.

### 3.2 Login rework — `/login` (Phase 2)

Remove DEF-10 and DEF-11. What remains:

```
┌──────────────────────────────────────────────────────────┐
│ COMFORT CURATORS / ACCESS      NOIDA · IN    BUILD 08  │  mono 11px
├──────────────────────────────────────────────────────────┤
│                                                          │
│   Choose your                                            │  Instrument Serif
│   keys.                                                  │  clamp(48px,9vw,120px)
│                                                          │  italic on "keys."
│   Three roles. One tenant. No passwords.                 │  Archivo 16
│                                                          │
├────────────────┬────────────────┬────────────────────────┤
│ 01             │ 02             │ 03                     │  1px black rules
│ OWNER          │ STAFF          │ GUEST                  │  Archivo Black
│ Exceptions     │ Operations     │ Your stay              │
│ and approvals  │ and curation   │ and the store          │
│                │                │                        │
│ ENTER →        │ ENTER →        │ ENTER →                │  mono
└────────────────┴────────────────┴────────────────────────┘
      hover: translateY(-2px) + rule thickens to 2px. No shadow.
```

**Behaviour on select:** dummy-fill and proceed, per the human's instruction —
one click, no intermediate confirmation. `signIn(role, contact)` → on resolve,
`navigate(homeFor(role), { replace: true })`.

The card enters a `MINTING…` state during the request. If `signIn` rejects,
the card returns to rest and the API's own message surfaces in a toast. Do not
navigate on failure.

### 3.3 Role routing (Phase 2)

Single source of truth, `src/lib/auth/roles.ts`:

```ts
export const homeFor = (role: Role) =>
  role === "owner" ? "/dashboard"
  : role === "staff" ? "/ops/tickets"
  : "/stay";

export const navFor = (role: Role) => ...   // drives the tab bar
export const allows  = (role: Role, path: string) => boolean;
```

`<RequireRole allow={["owner"]}>` wraps route elements. On violation: redirect to
that role's own home, **not** to `/login` — bouncing a signed-in user to a login
page reads as a bug.

`EntryRoute` becomes: no token → `/` (intro) → `/login`; token → `homeFor(role)`.

**Tab access by role**

| Role | Tabs |
|---|---|
| `owner` | Dashboard · Properties · Package · Invoices · Documents |
| `staff` | Tickets · Properties · Workers · Calendar |
| `guest` | Stay · Store *(single stack, no top nav)* |

Curator's `/jobs` stack stays under `staff` — the backend has three roles, not
six, and `PRD.md` already commits to that answer if asked.

### 3.4 Custom primitives — kill every default (Phase 1)

`src/components/ui/`. Nothing in the app may use a native `<select>`,
`<dialog>`, `confirm()`, or `alert()` after this phase.

| Component | Replaces | Notes |
|---|---|---|
| `Select` | 15 native `<select>` | Listbox pattern. `role="listbox"`, roving tabindex, type-ahead, Escape closes. Panel is a 1px-rule paper block, no radius, no shadow |
| `Modal` | `<dialog>` @ `package-shop.tsx:298` | **Off-centre, angled, no grey scrim** (`ART-DIRECTION §9`). Carries a mono index — `POPUP 03 / QUICK VIEW` |
| `CookieSlip` | nothing (DEF-12) | Torn right edge via `clip-path`. `ACCEPT ALL` and `NECESSARY ONLY` **equally prominent** — dark patterns are illegal in several jurisdictions and read as cheap. Tears away on accept |
| `ConfirmBlock` | `confirm()` | Used by the Superhost terminal. See §3.6 |
| `Popover` / `Tooltip` | native `title` | Same paper-block family |

Accessibility is not optional: every one of these needs focus trapping where
modal, `aria-expanded`/`aria-controls`, and full keyboard operation. A custom
control that cannot be driven by keyboard is worse than the native one it
replaced.

### 3.5 Superhost agent — backend (Phase 3) ⭐ the hard phase

Fixes DEF-01 through DEF-05. This is where D2 is honoured.

**3.5.1 Provider (`http_provider.go`)**

- `Authorization: Bearer <CC_MODEL_API_KEY>` from env. **Never logged, never
  returned in an error body.** Redact on marshal failure paths.
- Add `system` message from the prompt template; add `tools` array built from
  `jarvis.AllowedToolNames()` → full JSON schema per tool.
- Model `tool_calls` in `chatChoiceMessage`.
- Add `stream: true` support with an SSE reader.
- **Default model `gpt-5.6-luna`, base URL `https://api.openai.com` (D5).**
  The provider already targets `/v1/chat/completions`, which is Luna's native
  shape — no request rewriting needed. DeepSeek stays configured as the
  fallback provider via `CC_MODEL_FALLBACK_*`; it speaks the same wire format,
  so failover is a base-URL and key swap, nothing more.

**3.5.2 Tool loop (`runner.go`)**

```
claim → assemble context → call model
  ↓
for each tool_call in response (max 6 iterations, hard stop):
    decision := policy.Evaluate(ctx, toolCallInput)
    ├─ PolicyDenied            → emit denial event, feed refusal back to model
    ├─ PolicyApprovalRequired  → create ApprovalRequest, emit event, PAUSE run
    └─ PolicyAllowed           → execute read tool, feed result back
  ↓
complete with usage
```

The iteration cap is a hard stop, not a suggestion. An unbounded loop against a
paid API in a live demo is how you get a surprise bill on stage.

**Every policy decision writes an `agent_run_event`.** The terminal renders
those events verbatim. This is what makes the deny visible.

**3.5.3 Conversation surface**

`POST /v1/superhost/threads`, `POST /v1/superhost/threads/{id}/messages`,
`GET /v1/superhost/threads/{id}/stream` (SSE).

The existing `POST /v1/jarvis/runs` (renamed) stays for the async property-agent
path. The thread endpoints are a new conversational layer over the same store.

**3.5.4 Pricing (`pricing.go`)**

Verified 2026-08-09. Add, preserving the existing minor-unit convention
(one millionth of a currency unit, expressed per 1K tokens — check your arithmetic
against the existing `openai/gpt-4o` row at $2.50/1M → `2500`):

```go
// OpenAI GPT-5.6 Luna — the runtime model (D5): input $0.20/1M, output $1.20/1M.
"openai/gpt-5.6-luna":        {InputPer1KMinor: 200, OutputPer1KMinor: 1200, Currency: "USD"},
// DeepSeek fallback: input $0.14/1M, output $0.28/1M.
"deepseek/deepseek-v4-flash": {InputPer1KMinor: 140, OutputPer1KMinor: 280, Currency: "USD"},
// DeepSeek: input $0.435/1M, output $0.87/1M.
"deepseek/deepseek-v4-pro":   {InputPer1KMinor: 435, OutputPer1KMinor: 870, Currency: "USD"},
```

DeepSeek Flash has a cache-hit input tier at $0.0028/1M — a 50× reduction. The
table has no cache-tier concept; **do not invent one.** Bill at miss rate;
under-reporting cost is the safe direction and `usage_known` stays honest.

A Superhost conversation on Luna is roughly 48k input / 2.4k output → about
**1.2 cents**. Cost is not a reason to pick a weaker runtime model.

**3.5.5 Prompt scoping**

The system prompt is a governed artifact at
`internal/automation/superhost/prompt/v1.md`, versioned via the existing
`prompt_template_version` field. It must state: the agent proposes and never
executes; it has exactly the listed tools; it must not claim an action
succeeded; when a task needs authority it must say plainly what it wants to do
and stop.

**Voice constraint — "very simple language", per the human.** Short sentences.
No jargon. No emoji (`ART-DIRECTION §12`). The terminal is the one place where
the house voice is *plain* rather than dry-editorial, because someone is being
asked to approve a real action.

**3.5.6 Demo resilience — non-negotiable**

Every scripted beat carries a canned response keyed by intent. On provider
timeout (20s) or error, the terminal renders the canned line **and a visible
mono marker**: `OFFLINE FALLBACK · 03`. It never silently pretends to be live.
An investor who spots a fake is a lost investor; one who sees a labelled
fallback sees an engineer who planned for a bad hotel wifi.

### 3.6 Superhost terminal — frontend (Phase 4)

D1's governed exception. **A single block on an otherwise paper page.**

```css
.superhost-terminal {
  --phosphor:      #00FF66;
  --phosphor-dim:  #00994d;
  background: var(--ink);
  color: var(--phosphor);
  font-family: var(--font-meta);   /* JetBrains Mono */
  border: 1px solid var(--ink);
  border-radius: 0;
}
```

**Scoping rule, enforced in review:** `--phosphor` is declared *inside*
`.superhost-terminal` and nowhere else. It is not a root token. No other
component may reference it. This is what keeps D1 an exception rather than a
second theme.

**Behaviour**

- Mounts as a block on `/dashboard`, `/ops/tickets`, `/ops/tickets/:id`,
  `/stay`. It is part of the page, not a floating chat widget.
- Lines print with a ~12ms/char typewriter reveal, capped at 400ms per line.
  `prefers-reduced-motion`: print instantly.
- A block cursor blinks at 530ms.
- Streams `agent_run_event`s from SSE. Renders `> ` for agent lines,
  `$ ` for the operator's.
- **Policy denials render in `--red` inside the terminal** — the one place red
  and green coexist, and it is deliberate: the machine's own refusal.

**ConfirmBlock** — when the stream carries an `approval_required` event:

```
> i can raise a turnover ticket for gomti nagar, 11:00–14:00.
> i have not done it yet. it needs your ok.

  [ CONFIRM ]   [ NOT NOW ]              APPROVAL 04 / OPERATIONS
```

Buttons are `--phosphor` on `--ink`, inverting on hover. `CONFIRM` posts the
approval decision; the terminal then prints the *server's* result — never an
optimistic success line.

### 3.7 Guest portal — `/stay` (Phase 5)

One page, per the human. Sections: current stay, house guide, request help
(creates a ticket), **the store**.

**Store, under D3.** Providers are named in plain mono text — `INSTAMART`,
`ZEPTO`, `BLINKIT` — as selectable targets. **No logos, no scraped copy, no
brand colours.** Catalog is ours: real Noida items, ₹ prices in minor units
through the existing `<Money>`.

```
internal/procurement/store/
  provider.go      // StoreProvider: Search, Quote, PlaceOrder
  mock.go          // deterministic, seeded catalog
  // real adapters drop in here behind the same interface
```

`PlaceOrder` on the mock returns a `mock_order` marked as such in the response
body. The Superhost never calls it directly — `create_order_` and `place_order_`
are already in `prohibitedToolNamePrefixes` (`tools.go:224`). The agent proposes;
the guest confirms; the *application* orders. **Demo this deny explicitly.**

### 3.8 Expansion plan — `/expansion` (Phase 6)

The pitch as a page. Zero dependencies beyond design tokens, which makes it the
cleanest parallel workstream in the build.

Three stages, editorial layout, one hard rule per section boundary.

| Stage | Thesis | Revenue |
|---|---|---|
| **01 Comfort Curators** | Owners hand us the property; we run it | Owner management fees · property packages from the Chinese warehouse channel · maintenance |
| **02 Superhost OS** | The agent loop, unbundled from property management into the home | Subscription · finance and legal *preparation* · pantry and inventory · negotiated provider terms |
| **03 Curators Crew** | Contract work platform — delivery through accounting and legal | Placement margin · partnered equipment finance · the training data flywheel |

**Three framings the page must get right** — these were flagged as pitch risk:

1. **Equipment finance is partnered, never in-house.** Structured consumer
   credit in India needs an NBFC licence or a licensed partner. The page says
   *"equipment financing via a licensed NBFC partner"* and names the structure.
   Do not write "installment plan" unqualified.
2. **Stage 2 finance and legal is *preparation*, not practice.** Filing and
   representation are reserved to CAs and advocates. The page says the system
   assembles and a licensed professional decides.
3. **The authority model is the moat — lead with it.** Points 1 and 2 are not
   apologies. They are the same propose-approve architecture already in
   `policy.go`, which is exactly what lets the company enter regulated
   adjacencies *without becoming the regulated entity*. Machine prepares,
   licensed human decides, every decision audited. Give this its own section
   with a diagram of the real tool-call flow.

Charts: build with the `dataviz` skill conventions, mono axis labels, no
gradients, `--red` for one series maximum.

---

### 3.9 Control handover — Superhost drives the page (Phase 4) ⭐ D6

The demo moment. Superhost takes the wheel and visibly operates the interface,
then **hands it back the instant money is involved.**

#### 3.9.1 The model never gets the DOM

There is no `document`, no selector, no XPath, no `eval`. Elements **opt in**
declaratively and the model may only name registered IDs:

```tsx
<Select
  data-agent="ticket-type"
  data-agent-actions="focus set"
  data-agent-label="Ticket type"
/>
```

`AgentSurface` is a client-side registry mapping stable IDs → live elements. The
model receives only the ID list *for the current route*, injected into its
context each turn. **An element that is not registered does not exist to
Superhost** — that is the primary containment mechanism, and it is structural
rather than a rule the model is asked to follow.

**The entire intent vocabulary:**

| Intent | Effect |
|---|---|
| `ui.focus` | Move focus to a registered element |
| `ui.set_value` | Set a form value (fires React's synthetic change) |
| `ui.click` | Activate a registered control |
| `ui.scroll_to` | Bring an element into view |
| `ui.open_panel` | Open a registered disclosure |

Five intents. Nothing else is implementable without a spec change. There is no
generic escape hatch, and an agent must not add one.

The driver validates every intent before applying it: the ID exists on the
current route · the action is in that element's `data-agent-actions` · the
element is visible, mounted, and enabled. **A failed intent prints its failure
in the terminal. It never silently no-ops** — a silent no-op is how a demo
starts lying.

#### 3.9.2 Grant, visibility, revoke

Control is **never** assumed. It is granted by an explicit human click:

```
  [ HAND OVER CONTROL ]                         CONTROL 01 / GRANT
```

While control is live the page carries an unmissable frame — a 2px
`--phosphor` inset border and a fixed mono strip:

```
▌ SUPERHOST HAS CONTROL · 00:42 REMAINING · 7/25 ACTIONS · ESC TO REVOKE
```

**Revoke is instant and always available:** `ESC`, clicking the strip, or *any*
real user interaction with a non-agent element. The human is never a passenger.

**Budgets, because this runs live on stage:**

| Budget | Value | Why |
|---|---|---|
| Session TTL | 90s | A wedged loop cannot hold the page |
| Action cap | 25 | Hard stop, not a warning |
| Min spacing | 250ms | The audience must *see* each step |

Before each action, a `--phosphor` ring animates to the target over ~400ms on
`--ease-expo-out`, then the action fires. **Intent is visible before effect.**
That gap is the whole demo: the audience watches the machine decide, and has
time to react before it acts.

#### 3.9.3 The payment boundary — three independent gates

The requirement is that control **stops and is invalidated** at payment. Three
gates, none of which trusts the model, and none of which depends on the others:

**Gate 1 — Backend policy (already built).** `prohibitedToolNamePrefixes` in
`tools.go:224` blocks `pay_`, `charge_`, `create_order_`, `place_order_`,
`transfer_`, `disburse_`. `LookupTool` returns `ErrToolProhibited` before the
call is ever dispatched. This is existing, tested code.

**Gate 2 — Structural unreachability.** Anything inside `<PaymentBoundary>` is
**never registered** in `AgentSurface`. `PaymentBoundary` strips `data-agent*`
attributes from its whole subtree on mount. The model is not *refused* the pay
button — it is never told one exists.

**Gate 3 — Session invalidation.** If a control session so much as reaches
toward payment — an intent resolving into a payment subtree, or the route
changing into a payment step while control is live — the session is
**terminated, not skipped**:

1. `AgentSurface` registry is torn down
2. The control token is invalidated — single-use, **not resumable**
3. The frame turns `--red`
4. The terminal prints, in plain language:

```
> i can't take this step. paying is yours.
> i've handed control back.

                                    CONTROL REVOKED / PAYMENT BOUNDARY
```

Resuming requires a **fresh explicit grant** from the human. There is no
auto-resume, and adding one is a spec violation.

#### 3.9.4 Why this is the strongest thing in the demo

Do not treat the payment stop as a limitation to be apologised for. Rehearse it
as a deliberate beat: let Superhost fill the cart, drive the checkout, and then
**visibly refuse and hand back** — while the terminal narrates why.

That beat demonstrates, in about eight seconds, the thing that actually makes
this company investable in regulated adjacencies: **the machine prepares, a
human decides, and the boundary is enforced in three independent places rather
than promised in a prompt.** It is the same architecture as `policy.go`, made
visible.

An agent that can do everything is a liability. An agent that provably stops is
a product.

---

## 4. Cross-cutting rules for every agent

Inherited from `POLICY.md` and `ART-DIRECTION.md §12`, restated because they are
the ones that get violated:

- **`index.css` is frozen after Phase 1.** Route-specific styles go in that
  route's own `.css` file. This is a hard orchestration constraint — see
  `ORCHESTRATION.md §3`.
- Money is always `formatMoney(minorUnits, currency)`. Never float maths.
- **Never compute cost client-side.** The server is authoritative
  (`PRD.md`, `INTEGRATION.md §6`).
- **Never animate a number's value.** Cross-fade over 150ms.
- Surface the API's own `message` verbatim. `request_id` in a dev-only corner.
- Every list needs three designed states: skeleton, written empty copy, populated.
- No emoji. No rounded corners. No shadows. No gradients.
- `--red` appears once or twice per screen, never more.
- Real Noida addresses, real ₹ prices, plausible names. Never "Property 1".
- Every phase writes `logs/phase-N-<slug>.md` using the `POLICY.md` template,
  and **stops** for human verification.
