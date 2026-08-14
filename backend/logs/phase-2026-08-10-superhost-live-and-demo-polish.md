# Session 2026-08-10 — Superhost goes live against a real model, plus demo-readiness polish

- **Date:** 2026-08-10
- **Agent/model:** Claude Sonnet 5, orchestrator session (direct work, not dispatched
  through `.sandcastle` — this was solo debugging/implementation, not a farmed-out
  block)
- **Status:** complete
- **Branch:** `orch/p0-rename-jarvis-superhost`
- **Commits:** `f29afe3` → `9072177` (8 commits, `git log --oneline f29afe3~1..9072177`)

This log exists because `HANDOVER.md` (written 2026-08-09) describes this branch's
"what's left" as Wave 3/4 frontend blocks (P4.5–P4.9, P7.1–P7.4) with **no backend
work planned**. All of that frontend work has since landed (see the frontend repo's
own `logs/` and this session's frontend log), and — not anticipated in that
handover — a full day was then spent here on the backend, because wiring Superhost
to a real model provider surfaced a chain of real bugs the model-stub had never
exercised. Read this before trusting `HANDOVER.md`'s "no remaining backend work"
line; it's out of date.

## What I built

### 1. Real model provider (`f29afe3`)

Superhost's provider was still `CC_SUPERHOST_PROVIDER=stub` by default — every
prior "verification" of Superhost this whole project had been against a canned
model-stub, never a real one. Wired `HTTPProvider` (already built in the P3.1
block) to a real DeepSeek endpoint (`CC_MODEL_URL=https://api.deepseek.com`,
model `deepseek-chat` — not `deepseek-reasoner`, which lacks function-calling
support). Immediately exposed two real OpenAI-compatible-contract bugs the stub
had never enforced:

- `messages[1]: content should be a string or a list` — `runner.go` was sending
  `run.InputData` (a raw JSON object) directly as a message's `content`. Fixed by
  sending a pre-built `Messages` array instead of the old `System`+`Input` fields.
- `invalid tool arguments: json: cannot unmarshal string into Go value of type
  map[string]interface{}` — `function.arguments` in a tool call is always a
  JSON-*encoded string* per the real spec, never a bare object; the stub had
  never produced this shape. Fixed via `NormalizeToolArguments` in `runner.go`.

### 2. Three more live-verified bugs (`1b23396`)

Fixing the contract bugs above let real runs actually execute far enough to hit
three more:

- **A `superhost_threads_pkey` duplicate-key race** on two concurrent
  `CreateThread` calls sharing an idempotency key — both got the same run ID from
  `Submit`'s own dedup, then raced to `INSERT` the same `thread_id`. The recovery
  path's own re-fetch then ALSO failed ("no rows in result set") because
  `agent_runs.idempotency_key` uniqueness wasn't actor-scoped the way
  `superhost_threads` was — root-fixed by combining `actorID + ":" + idempotencyKey`
  for `Submit()`'s own key.
- **Approval-required runs silently self-resolving** with a fabricated "Approved
  by human reviewer" text, with no human ever having approved anything.
  `RecoverExpiredLeases` was treating `waiting_for_approval` as an
  abandoned-worker state eligible for lease recovery using the short
  processing-lease deadline. Fixed by excluding that state from the recovery
  query. Found by reproducing the user's own reported "tries to restock and does
  nothing" complaint exactly.
- **Portfolio-scoped runs incorrectly denying any `property_id` tool argument** —
  `ValidateScope` treated an empty `propertyID` param as meaning "no property is
  ever valid here," even for a run deliberately scoped to the whole portfolio.
  Fixed by skipping that specific check when `propertyID == ""`, with a real
  tenant-ownership check added separately in `tool_executor.go` so this isn't a
  scope hole.

### 3. Real per-account identity, role-scoped tools, task ledger (`7c4f607`)

Extended Superhost from owner-only to all three roles (owner/staff/guest), each
with its own tool surface (`ToolAudience` became a `[]ToolAudience` slice per
tool rather than one value) and its own persistent memory: a new
`superhost_account_tasks` table (`AccountTaskStore`), scoped per `(tenant_id,
actor_id)`, holding a short list of things that account's Superhost has noted it
should follow up on. Explicitly framed in the system prompt as the account's own
working notes, not verified business state — real tickets/reservations elsewhere
in context remain the record of truth.

### 4. Portfolio-scoped threads (`3717c30`)

Before this, every Superhost thread was locked to exactly one property. Added
`ContextAssembler.AssemblePortfolio` (all of a tenant's properties in one
context) and thread-level portfolio mode (`property_id == ""` on the thread).
This is the fix for "the superhost should manage multiple properties in
parallel" — previously structurally impossible, not just a UX gap.

### 5. Kickoff suggestions + a major answer-rendering bug (`d15a19e`)

Added proactive "here's what you could ask" suggestions so a new user isn't
staring at a blank prompt. Live-verifying that feature surfaced the single most
significant bug of the whole session: **`AgentRunCompleted.v1` never carried the
model's actual answer text.** `store.go`'s `CompleteWithUsage` only ever put
usage stats in the event payload — the real text lived only in the
`output_data` DB column, never streamed to the frontend. Every prior
"verification" of a plain conversational Superhost response (no tool call) in
this whole project had actually been observing the literal fallback string "run
completed," not a real answer, and nobody had caught it because most manual
testing happened to involve a tool call, which has its own event carrying real
text. Fixed by including `outputText` in the completion event.

### 6. Talk like a person, not the database (`a684fd9`)

Added an explicit instruction (and worked wrong/right examples) telling
Superhost to replace raw ids (`tkt_...`, `prop_...`, `res_...`) with what they
actually are — a ticket's type + property, a property's real name — in its own
prose, while still using the exact id for tool arguments. Ids were leaking into
sentences shown to a human.

### 7. Two prompt refinements from live user feedback (`0c46fd8`)

- **Stop asking "which property?" by default.** In a portfolio-scoped thread,
  when exactly one property in context obviously matches what was asked (an
  in-progress ticket of the right type, the only one with a relevant stock
  signal, etc.), Superhost now proceeds with a stated assumption rather than
  stopping to ask first — reserving an actual clarifying question for genuine
  ambiguity.
- **Broadened the id-avoidance rule** to explicitly cover every id prefix, not
  just properties — a pasted user transcript showed a ticket id specifically
  still leaking through in parentheses, the same pattern the existing rule
  already forbade generically but hadn't made concrete for tickets.

### 8. Real live token streaming (`9072177`)

The single biggest architecture change of the day, and the direct answer to "I
want to even see it type live." Everything up to this point was fully
non-streaming: one blocking HTTP call to the model per turn, with the frontend's
`useTypewriter` faking a character-reveal *after* the complete response had
already arrived.

- `HTTPProvider.CallStream` — new method alongside the existing `Call`, sets
  `stream: true` (+ `stream_options.include_usage`) against the OpenAI-
  compatible endpoint, reads the SSE response as it arrives, and invokes an
  `onDelta(cumulativeText string)` callback on every chunk. Tool-call
  arguments also arrive fragmented in the same stream and are reconstructed
  by index, but only ever surfaced whole on the final `ProviderResponse` — a
  tool call isn't the "type it live" thing this exists for.
- New optional `StreamingProvider` interface; `Runner.callProvider` type-
  asserts the active provider against it and, when available, persists the
  growing text to a new `agent_runs.streaming_text` column (throttled to
  roughly 3 writes/sec) as deltas arrive.
- The SSE stream handler (`stream.go`) polls that column alongside its normal
  event-cursor polling and, when it's changed, writes a synthetic,
  **non-persisted** `AgentRunToken.v1` frame — never a row in
  `agent_run_events`, never advances the real cursor, purely a live view of
  state a real, persisted terminal event is about to supersede.

Live-verified against the real DeepSeek-backed worker across owner, staff, and
guest sessions: a response now visibly grows in the terminal as the model
actually generates it (watched via repeated screenshots across a several-second
generation), and settles cleanly to the complete final sentence with no
leftover fragment once the real `AgentRunCompleted.v1` event lands.

## Files changed

See each commit's own message for the exact file list — they're individually
detailed (this session kept commit messages as the primary record of *why*, not
just *what*). At a high level, everything above lives under
`internal/automation/` (`http_provider.go`, `provider.go`, `runner.go`,
`schema.go`, `store.go`, `stream.go`, `tool_loop.go`, `handler.go`,
`thread_store.go`, `tools.go`, `policy.go`, `models.go`, `context.go`,
`account_tasks.go`, `provider_config.go`) and
`internal/automation/superhost/prompt/v1.md`.

## Decisions I made

- **DeepSeek over other providers**, specifically `deepseek-chat` not
  `deepseek-reasoner` — the reasoner variant doesn't support function calling,
  which every Superhost tool call depends on.
- **`AgentRunToken.v1` is synthetic and never persisted.** Considered writing
  a real `agent_run_events` row per delta and rejected it — that's dozens of
  DB writes per response for pure display state that the real completion
  event immediately supersedes. Kept it as a poll-loop-only, non-cursor-
  advancing frame instead.
- **Streaming text is cleared, not left, after each call** (`callProvider`
  writes `""` to `streaming_text` when a streamed call finishes, success or
  failure) — prevents a stale mid-sentence fragment from lingering against
  that run's row.
- **Throttled streaming-text writes to ~300ms**, just under the SSE poll
  loop's own 500ms tick — writing faster buys nothing the poll loop can
  observe, only extra DB load.

## What did NOT work

- The first attempt at portfolio-mode tool scoping (`ValidateScope`) denied
  *any* `property_id` argument on a portfolio-scoped run, which was backwards —
  fixed in `3717c30`/`1b23396`, see above.
- `superhost_account_tasks` was first added as a new numbered migration file
  with `UUID` columns — discovered the numbered-migrations system isn't
  exercised by test setup (only `EnsureSchema`/`EnsureToolSchema` are) and
  `UUID` didn't match the rest of the codebase's `TEXT` id convention. Deleted
  the migration, moved the DDL into `schema.go`'s existing `EnsureToolSchema`
  with `TEXT` columns.

## Deviations from the plan

None of this was planned in `ORCHESTRATION.md` — `HANDOVER.md` explicitly says
"no remaining backend work is currently planned" for this branch. This entire
session is scope the orchestrator (previous instance of me, and this one) added
directly in response to live user testing revealing the model-stub had been
masking real bugs the whole project. Same pattern `HANDOVER.md §5` already
documents for `P3.9`/`P3.10` — found via live use, not spec-planned.

## Open questions / known gaps for whoever picks this up next

- **No separate test database.** `go test ./internal/automation/...` runs
  against the same Postgres the demo uses, and a run mid-session truncated
  the live demo data again (recovered via `npm run seed` in the frontend
  repo). Flagged repeatedly, not yet fixed. The right fix is a throwaway
  Postgres per test run, the same pattern `HANDOVER.md §2.4` already
  describes for dispatch verification — it just isn't wired into the normal
  `go test` invocation for this package yet.
- **UI-driven ticket creation via Superhost is not a complete path.** A
  `ui_click` on "open the new ticket form" gets stuck once the form's own
  fields aren't registered as UI surfaces — direct `propose_*` tool calls
  work end-to-end; walking a form via clicks does not, yet.
- **`HANDOVER.md`'s "Current state" and "What's left" sections are now
  stale** relative to this branch's actual HEAD — they describe a 2026-08-09
  checkpoint. A short pointer to this log (and the frontend's equivalent) was
  added at the top rather than rewriting the whole document, to keep this
  change small and reviewable; a fuller rewrite is still owed at some point.
