# Phase 0.5 — Design foundation

- **Date:** 2026-08-08
- **Agent/model:** GPT-5.6 Codex (orchestrator); DeepSeek V4 Flash/Pro, Qwen 3.7 Plus, GPT-5.6 Luna (advisory audits)
- **Status:** complete

## What I built
The `/debug` route is now a responsive visual-foundation proof for Comfort Curators. It demonstrates the four local type families, exact paper/ink/accent tokens, subtle grain, hard-edged layout rules, a 12-column grid, interaction fields, motion easings, the live same-origin API seam, Lenis scrolling, and a square difference-blend cursor. Smooth scrolling and the custom cursor opt out for reduced-motion, touch, and keyboard use as appropriate.

## Files added or changed
`app/package.json` — adds local Fontsource packages and Lenis.

`app/package-lock.json` — locks the Phase 0.5 dependency graph.

`app/src/main.tsx` — loads the local fonts and global motion/cursor foundations.

`app/src/index.css` — defines the design tokens, grain, typography, grid, hard-edged components, responsive rules, cursor, and reduced-motion overrides.

`app/src/routes/debug.tsx` — replaces the Phase 0 proxy panel with the complete design-system proof while retaining live proxy output.

`app/src/components/smooth-scroll.tsx` — owns one Lenis lifecycle and disables it when reduced motion is requested.

`app/src/components/difference-cursor.tsx` — implements the square, lerped, difference-blend cursor with touch, keyboard, and reduced-motion safeguards.

`.gitignore` — keeps the superseded 700 MB Next.js archive and local secrets out of version control.

`logs/DECISIONS.md` — records the cross-phase motion and cursor decision.

## Decisions I made
All four fonts are bundled locally through Fontsource so the demo does not depend on a third-party font request. Tailwind 4 theme variables live in the CSS-native `@theme` block, matching the installed toolchain. The cursor is square rather than circular so the global zero-radius rule remains honest. Lenis and cursor behavior are isolated in small lifecycle components instead of being coupled to the debug route, because every later screen inherits them.

OpenCode models were used as read-only specialists: DeepSeek Flash extracted acceptance values, DeepSeek Pro reviewed motion risks, Qwen reviewed structure and final code, and Luna reviewed the desktop/mobile captures. The orchestrator checked all advice against the repository and rejected a Pro suggestion to treat Lenis as a singleton because effect cleanup already handles React Strict Mode, and rejected any circular cursor treatment because it conflicts with the sharp-corner art direction.

## What did NOT work
The first parallel headless-Chrome capture was interrupted before it produced files; rerunning the captures with distinct browser profiles succeeded. Kimi K2.7 Code was not used for this implementation because earlier repository-tool runs stalled; Qwen 3.7 Plus was the reliable code-review fallback. OpenCode Go Flash was intermittently unavailable, so the configured direct DeepSeek Flash provider handled its bounded checklist task.

## Deviations from the plan
None. The design foundation is implemented only on `/debug`; no Phase 1 product or authentication screen was started.

## New API knowledge
None. The existing authenticated `/api/v1/properties` proxy proof still returns the documented empty collection.

## How to verify (human runs these)
1. Run `cd ~/comfort-curators-frontend/app && npm run lint` — expected: ESLint exits successfully with no errors.
2. Run `cd ~/comfort-curators-frontend/app && npm run build` — expected: TypeScript and Vite production build complete successfully.
3. Run `cd ~/comfort-curators-frontend/app && npm run dev`, then open `http://localhost:3000/debug` — expected: the full paper-based style sheet renders with Instrument Serif, Archivo, Archivo Black, and JetBrains Mono; no Inter is used.
4. Scroll the page — expected: wheel scrolling has restrained inertia/glide.
5. Move the pointer between the paper and black cursor fields — expected: the square cursor stays visible and inverts against both fields.
6. Inspect the page at desktop and approximately 390 px wide — expected: no horizontal overflow or clipped type; layout rules remain square with no shadows, gradients, or rounded corners.
7. Enable the operating system/browser “reduce motion” preference and reload — expected: native scrolling is restored and the custom cursor is absent.
8. Inspect the page background — expected: paper is `#FAF9F7` and the fixed monochrome grain remains subtle at opacity `0.035`.

## Open questions for the human
1. GitHub authentication is currently invalid. Options: run `gh auth login -h github.com`, or provide a fresh project-scoped `GH_TOKEN`; recommendation: use `gh auth login` so the CLI can create and push the private repository without placing a token in project files.
2. The superseded Next.js app remains locally in `app-next-superseded/` and is excluded from Git. Options: retain it as a local reference, archive it elsewhere, or remove it after the Vite repository is safely published; recommendation: retain it until the first GitHub upload is confirmed, then remove it to recover roughly 700 MB.

## What's next
Wait for manual Phase 0.5 acceptance. After approval, begin Phase 1 exactly as specified in `PHASES.md`; do not reuse the debug style-sheet composition as a product screen. Once GitHub authentication is repaired, initialize and upload the current repository as private, then use feature branches and draft pull requests for later phases while committing every phase log.
