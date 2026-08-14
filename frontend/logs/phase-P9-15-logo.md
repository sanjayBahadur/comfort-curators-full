# Phase P9.15 — logo (full + short marks)

- **Date:** 2026-08-10
- **Agent/model:** gpt-5.6-sol (codex, medium effort)
- **Status:** complete

## What I built (describe both marks — concept, not just "see attached")

The full mark is a pure-type, two-line `COMFORT / CURATORS` lockup. It treats the product name itself as the identity: dense uppercase forms, tight spacing, compact leading, a shared left edge, and an intentionally shorter first line. It does not add an unrelated hospitality icon because the existing product already establishes its identity through large Archivo Black wordmarks.

The short mark is a `CC` monogram optically spaced inside a sharp black square. Paper-coloured letterforms produce maximum contrast at browser-tab scale. Its spacing is deliberately looser than the full lockup so the two counters remain separate after rasterization at 16–24px.

## Typography/geometry decisions, and how they trace back to the real design system

- Both marks derive from the repository's real `--font-shout`, Archivo Black, matching `.intro-wordmark` rather than introducing another typeface.
- The full lockup keeps the existing uppercase, tightly tracked, industrial wordmark language and the two-line construction already used in the intro.
- The short mark uses only `--ink` black and `--paper` `#faf9f7`, with a square edge and no radius, shadow, gradient, texture, or extra iconography.
- Red is absent. The design system reserves red for action, danger, or a live accent; a permanent identity mark does not need to scream on every surface.
- I left the current context-qualified text headers (`/ OWNER`, `/ OPS`, etc.) intact. Replacing all of them with a tall two-line asset would lose useful context and force a layout change without improving those compact navigation surfaces.

## Portability approach per file (live text vs. outlined paths), and why

Both marks use outlined SVG paths, not live `<text>`. The outlines were generated from the repository's installed Archivo Black font asset and therefore render identically in an `<img>`, a raw SVG preview, an external document, or a favicon without relying on the parent page's web-font context. The SVGs retain accessible `<title>` and `<desc>` elements.

The full mark also declares intrinsic `512×174` dimensions and the short mark declares `32×32`, in addition to their matching `viewBox` values, so raw-image and favicon sizing is deterministic.

## Files added or changed

- Added `app/src/assets/brand/comfort-curators-full.svg` — full outlined wordmark.
- Added `app/src/assets/brand/comfort-curators-short.svg` — compact outlined monogram.
- Replaced `app/public/favicon.svg` with an exact copy of the short mark.
- Added `logs/phase-P9-15-logo-browser-proof.png` — Chrome proof showing the full mark plus the short mark at 24px and 96px.
- Added this log.
- Did not edit frozen `app/src/index.css`.

## What I verified live vs. build/lint-only

Live in real Google Chrome 150 (`channel: "chrome"`):

- Rendered the full SVG and short SVG together on the app's warm-paper background.
- Confirmed the short mark remains legible at exactly 24×24px, with separate `C` counters.
- Confirmed the live page still advertises `/favicon.svg`, Chrome fetched the new monogram with HTTP 200, and the source reported its intended intrinsic 32×32 dimensions.
- Confirmed the delivered short asset and public favicon are byte-identical by SHA-256.
- Saved the 1280×720 browser proof to `logs/phase-P9-15-logo-browser-proof.png`.

Build/lint and static checks:

- `xmllint --noout` passed for the full asset, short asset, and public favicon.
- `npm run build` passed.
- `npm run lint` passed with seven pre-existing warnings in unrelated React files and no errors.

## What did NOT work

- `AGENTS.md` does not exist in this worktree or its immediate parent, including hidden files, so it could not be read. `ART-DIRECTION.md` was read in full.
- The first Chrome launch was blocked by the filesystem/network sandbox. Re-running the same Playwright proof with approved browser permissions succeeded.
- Port 3000 was occupied during verification, so the current worktree was served on port 4173 for the proof.
- Headed Chrome also launched successfully, but the Wayland compositor denied an automated screenshot of browser chrome and its X11 root capture was blank. The saved proof therefore shows both marks in Chrome's rendered page area; favicon verification is based on the live `<link>` target and Chrome's successful HTTP fetch, not a captured tab-strip image.

## Open questions

None. The full mark is intentionally delivered as an asset rather than forced into compact, context-qualified application headers.
