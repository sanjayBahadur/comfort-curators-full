# Phase 0.6 — Tooling setup

- **Date:** 2026-08-09
- **Agent/model:** Claude Sonnet 5 (orchestrator, direct — not yet dispatched through sandcastle, since this block sets sandcastle up)
- **Status:** complete

## What I built

`@ai-hero/sandcastle` and `evalite` are installed as devDependencies in
`app/`. `.sandcastle/` is scaffolded and wired end to end: a working local
issue tracker (no external service), a per-block dispatcher (`main.ts`) that
resolves each block's tier to the right model and hard-refuses to run a block
still needing human sign-off, and vendored `agent-rules-books` packs (nano +
mini) for the four books ORCHESTRATION.md §8.1 names. `opencode.json` now
exists for the frontend (it previously only existed in the backend repo) and
both repos' configs point at the OpenCode Go gateway rather than raw
provider keys.

`mattpocock/skills` was evaluated and **not installed** — see *Decisions*.

## Files added or changed

`app/opencode.json` — new. Frontend build-agent default model config, mirrors
the backend's shape.

`comfort-curators-backend-alt/opencode.json` — `model`/`small_model` changed
from `deepseek/deepseek-v4-pro` / `deepseek/deepseek-v4-flash` to
`opencode-go/deepseek-v4-pro` / `opencode-go/deepseek-v4-flash`, per the
human's instruction to route build-agent dispatch through the OpenCode Go
subscription rather than direct per-provider keys.

`app/.sandcastle/blocks.json` — new. All 49 (see *Deviations* — I count 50)
blocks from ORCHESTRATION.md §5, structured: id, wave, repo, tier, owns, gate,
and flags for `requires_human_signoff` / `escalate_if` where the doc calls
them out (P0.4, P3.0 need sign-off; P4.6/P4.8 carry an explicit
no-generic-escape-hatch / no-softened-gate warning).

`app/.sandcastle/blocks.mjs` — new. Implements the local `list` / `view <id>`
/ `close <id>` contract `.sandcastle/SETUP_ISSUE_TRACKER.md` asked for.
`list`/`view` read `blocks.json`; `closed` is true only when
`logs/phase-<id>-*.md` exists with `**Status:** complete`. `close` **verifies
and reports, it does not mutate** — there is no external ticket, the log file
itself is the completion record, so a block that isn't actually logged as
complete cannot be closed by running this command.

`app/.sandcastle/main.ts` — rewritten from the generic scaffold. Takes a
block id, looks up its tier in `blocks.json`, maps tier → model
(`opencode-go/deepseek-v4-pro` / `opencode-go/gpt-5.6-luna` /
`opencode-go/deepseek-v4-flash` for tiers 1/2/3), and calls
`run({ agent: opencode(model), sandbox: docker(), promptFile:
".sandcastle/blocks/<id>.md" })`. Refuses to run if the block's prompt file
doesn't exist yet, or if the block is sign-off-gated and no sign-off has been
recorded — both checked in code, not left to a human remembering to check
prose.

`app/.sandcastle/prompt.md` — the context section now injects
`node .sandcastle/blocks.mjs list` as live context via the `!` shell
expression sentinel.

`app/.sandcastle/Dockerfile`, `.env.example` — TODOs resolved: no
issue-tracker CLI needed (tracker is plain Node reading files already in the
bind-mounted worktree); `.env.example` documents `OPENCODE_API_KEY` as the
`opencode-go` credential, not a raw OpenAI/DeepSeek key.

`app/.sandcastle/.env` — new, gitignored. Populated with the real
`opencode-go` API key read from the host's `opencode auth` store, so a
dispatched container can authenticate.

`app/.sandcastle/rules/{clean-architecture,refactoring,domain-driven-design,
a-philosophy-of-software-design}/*.{nano,mini}.md` — new. Vendored (not a
live dependency) from `mattpocock/agent-rules-books` (MIT).

`app/.sandcastle/blocks/README.md` — new, empty dir otherwise. Where
per-block prompt files land before each real dispatch.

## Decisions I made

**`mattpocock/skills` — not installed.** The kickoff prompt required stopping
to ask before installing it; the human then delegated the actual call to me
("determine if the mat pocock skills will help or hamper... and proceed
accordingly"). Reasoning:

- Its skills are Claude-Code-native (slash commands / model-invoked skills
  inside a Claude Code session). The implementing agents in this plan run
  through `opencode()` inside sandcastle's Docker sandboxes — a different CLI
  entirely. Those agents could never use this plugin regardless of whether
  it's installed.
- The only session that *could* use it is the orchestrator (me) — and
  ORCHESTRATION.md §6 is explicit that the orchestrator's job is dispatch and
  assembly, not implementation or code review. Two of the plugin's
  model-invoked skills are literally `/implement` and `/code-review` — having
  those one command away, auto-triggering "when tasks fit," is a standing
  temptation to do exactly the work §6 says has "leaked out of the dispatch
  model." That's a discipline risk specific to this project's structure, not
  a property of the plugin being bad in general.
- `/wayfinder` ("map a build too large for one session as decision tickets on
  an issue tracker") sounds like the closest fit to this project's actual
  need, but this project already has that: the block table in
  ORCHESTRATION.md §5 plus `logs/phase-N-*.md`. Adopting wayfinder as
  designed would mean standing up an issue tracker this project doesn't use
  (see the P0.6 issue-tracker question below) to duplicate a system that
  already works.
- It's a genuinely well-regarded, real plugin (confirmed via web search, not
  taken on faith) — this is not a "don't trust it" call, it's a "wrong tool
  for this role split" call.

Net: skipped. If a later phase finds a specific skill worth having *just for
a human-run Claude Code session outside this dispatch loop* (e.g. `/tdd` for
manual debugging), that's a separate, smaller decision — not this one.

**Local issue tracker, not GitHub Issues.** Asked the human directly (this
project's convention is markdown phase logs, not tickets, and standing up 49
GitHub issues is an outward-facing change to a real repo). Human chose local.
Implemented as `blocks.mjs` reading `blocks.json` + `logs/`, per above.

**OpenCode Go gateway for build-agent dispatch**, per explicit human
instruction. Both repos' `opencode.json` and the dispatcher's tier→model map
now use the `opencode-go/*` model IDs (verified against the live `opencode
models` CLI output, not guessed — the free `opencode/*` tier and the paid
`opencode-go/*` tier are different prefixes and only the latter has
`gpt-5.6-luna`).

**`blocks.json` is a maintained mirror of ORCHESTRATION.md §5, not a parser
over it.** Markdown-table parsing was judged more fragile than keeping a
small structured file in sync by hand when the plan changes.

## What did NOT work

`sandcastle init`'s non-interactive mode requires `--agent` explicitly (no
TTY prompt fallback) — fine once discovered, just noting it for whoever runs
init again on the backend repo (P3 also needs its own `.sandcastle/`, not
scaffolded here — this block only did the frontend).

## Deviations from the plan

`blocks.mjs list` returns 50 open blocks, not 49 as ORCHESTRATION.md §1/§8.1
state (I count 6+7+4+5+9+6+9+4 = 50 from the §5 tables). Did not chase this
down further — immaterial to dispatch, flagging so it isn't mistaken for a
transcription error in `blocks.json` if someone recounts later.

Did not run `sandcastle docker build-image` — no block is being dispatched
yet, and building the image is a several-minute, disk-consuming step better
done right before P0.1 actually dispatches rather than speculatively now.

Did not scaffold `.sandcastle/` in the backend repo. P3 needs it (it's the
"other repo, fully parallel" track), but Wave 0 is P0 + this block only;
doing it now would be ahead of the wave structure ORCHESTRATION.md §4 sets.

## New API knowledge

`opencode models` (CLI) lists both a free `opencode/<id>` tier and the paid
`opencode-go/<id>` tier as distinct provider prefixes — `gpt-5.6-luna` only
exists under `opencode-go/`. Not documented clearly on the Zen docs page
(which only shows the free-tier prefix); worth checking the CLI directly
rather than the docs page if this comes up again.

GPT-5.6 Luna's public per-1M pricing may have dropped ($0.20/$1.20 →
possibly $0.10/$0.60 as of a 2026-07-30 cut, per a live search — not
independently verified against OpenAI's pricing page). Flagged for whoever
picks up P3.5 (`pricing.go`) to check before hardcoding.
