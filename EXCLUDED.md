# What was left out of this copy, and why

This folder is meant to *run the demo*, not preserve every artifact of how
the project was built. Everything below was deliberately excluded from the
`backend/` and `frontend/` copies. Nothing here is needed to run the app —
if you want any of it, it's still in the original checkouts:
`~/open-code-projects/comfort-curators-backend-alt` and
`~/open-code-projects/ComfortCurators`.

## Genuinely redundant / stray (not just "not needed here")

These aren't demo-vs-source-tree tradeoffs — they're leftover cruft in the
*original* checkout that doesn't belong to the running app at all:

- **`ComfortCurators/node_modules/`, `.next/`, `next-env.d.ts`,
  `.deepseek-ui-runner/`, `deepseek-ui-chunk-safe.sh`, `.cc-harness/`,
  `supabase/`, `tsconfig.tsbuildinfo`, `.env.cc`, `.env.cc.example`,
  root-level `.env.local`** — all remnants of an abandoned Next.js +
  Supabase rebuild attempt that the project moved off of. The real app is
  the Vite build under `app/`, tracked on the `dev` branch; none of this
  is imported by it or referenced anywhere in `src/`. **The root
  `.env.local` in particular held a live Supabase service-role secret for
  that abandoned attempt — it was not copied here, and if that credential
  is still live anywhere, it's worth rotating regardless of this copy.**
- **`tests/quality/reports/*.json`** (backend) — generated test-report
  output, already gitignored in the source repo, regenerated on every
  real test run.

## Left out because it's build tooling / agent orchestration, not the app

Both repos were built with a fair amount of multi-agent dispatch
infrastructure (Sandcastle, opencode, per-agent config). None of it runs
the product — it's how the product got written:

- `.sandcastle/` (both repos) — the dispatch pipeline and every per-block
  prompt file.
- `.beads/`, `.claude/`, `.codex/`, `.harness/`, `.opencode/`,
  `opencode.json`, backend root `package.json`/`package-lock.json` (that
  one exists *only* to give `.sandcastle/` an npm-resolvable
  `node_modules` — the backend itself is Go) — agent-tooling config.
  `frontend/app/opencode.json` was left in place (harmless, tiny) but is
  in this same category — the running app never reads it.
- `.github/` (backend) — CI workflow config; irrelevant to running the
  app locally.
- `reports/` (backend) — empty build-report scaffold directory.

## Left out because it documents the build process, not the product

These are real documents, not clutter — just written for the person
building the app, not the person demoing it:

- `ORCHESTRATION.md`, `PHASES.md`, `SKILLS.md`, `KICKOFF-PROMPT.md`,
  `POLICY.md`, `SETUP.md` (frontend) — the multi-agent build plan, phase
  gates, and agent working rules used to construct the app in the first
  place.
- `INTERACTION.md` (frontend) — forensic CSS/JS extraction notes from
  three reference sites, done as input to `ART-DIRECTION.md`. The
  synthesized result (`ART-DIRECTION.md`) is kept; the raw research notes
  are not.
- `DEMO-RUNBOOK.md` (frontend) — an earlier demo runbook, already flagged
  stale in this repo's own logs (references seed counts and a build
  process that no longer match). **This `INSTRUCTIONS.md` supersedes it.**

## Left out because it's pure version-control history

- `.git/` in both repos (487MB backend, even larger frontend once you
  count every sandcastle branch) — this copy is a working snapshot, not a
  clone. If you want history, `git log` in the original checkouts, or
  `git init` fresh in here if you want to start tracking changes made
  from this point forward.

## Kept, worth knowing about

- **`backend/.env`** — copied as-is, including the real DeepSeek key. This
  is what makes Superhost give real answers instead of the offline stub.
  Treat this folder's `.env` with the same care as the original.
- **`frontend/app/.env.local`** — copied as-is (just the demo tenant id,
  not a secret) — required, the app will not authenticate without it.
- **`logs/`** in both repos — kept. This is real project history (what
  was built, when, by which model, what was verified) — genuinely useful
  if anyone picks this up later, cheap in size (under 1MB combined).
- **`HANDOVER.md`** (backend) and the product-facing docs at the frontend
  root (`ART-DIRECTION.md`, `ARCHITECTURE.md`, `PRD.md`, `SCREENS.md`,
  `IMPLEMENTATION-SPEC.md`, `SHOP.md`, `INTEGRATION.md`, `README.md`,
  `CHANGELOG.md`) — kept. These describe the actual product (design
  system, architecture, screen inventory), not the process that built it.
- **`AGENTS.md`** (backend) — kept. Doubles as a reasonably good
  architecture/conventions summary even for a human reader, not just an
  agent.

## Known gap in the demo data itself (not a copy issue)

`/invoices` will show "No charges yet" for both seeded properties —
there are zero rows in the `charges` table anywhere in this seed. See
`INSTRUCTIONS.md` §4 step 4.
