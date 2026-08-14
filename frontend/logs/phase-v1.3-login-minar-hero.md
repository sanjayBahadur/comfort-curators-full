# Phase v1.3 — Login Minar hero

- **Date:** 2026-08-12
- **Agent/model:** Codex GPT-5
- **Status:** partial

## What I built
Replaced the login page's four fragmented Kutub cutouts with the exact Minar image supplied at `~/Downloads/minar.png`. The image is rotated exactly 90 degrees counterclockwise so the main minaret points left and fits the original hero box from its vertical side without cropping or distortion. The role/tenant/password deck is removed, its layout footprint is retained with wrapper padding, and the headline is enlarged exactly 20%. The image itself is now a replay key: clicking it clears the existing intro-seen session flag and re-enters the original intro route.

## Files added or changed
`app/src/assets/hero/minar-login-hero.png` — lossless 1536x1024 left-facing source-preserved project asset.

`app/src/routes/login.tsx` — renders the restored source image as an accessible intro-replay button and removes the supporting role/tenant/password deck.

`app/src/routes/login.css` — fits the image button exactly to the rendered PNG's 3:2 box, anchors it to the right, enlarges the headline by 20%, and replaces the removed deck's vertical footprint with responsive wrapper padding.

`app/src/components/difference-cursor.tsx` — accepts a positive per-element `data-cursor-scale` override; the hero replay key requests `3.8`, exactly twice the standard interactive scale of `1.9`.

## Decisions I made
Used a deterministic lossless rotation so the image remains exact. Rotating the project asset back and comparing it with the Downloads source produced an absolute pixel error count of `0`. The image uses `height: 100%`, `width: auto`, and `object-fit: contain`; the source image in Downloads remains unchanged.

The intro is replayed through the existing `/` entry route instead of copying its timers or animation state. The image control is a native button with a transparent background, no border or padding, an approximate architectural polygon hit area that excludes much of the PNG background, a descriptive accessible name, and a keyboard focus outline. Its custom cursor scale is `7.6`, four times the standard interactive scale of `1.9`.

On desktop, the login page is a `100svh` three-row grid: 64px header, a flexible hero, and role cards clamped between 250px and 304px. Hero padding, removed-deck compensation, and card spacing compress with viewport height so the page does not create document scrolling; the mobile layout remains content-sized and scrollable.

## What did NOT work
Earlier generated color-grade and transparent-cutout variants were rejected and removed from the final page at the user's direction. Automated browser screenshot verification could not run in the managed sandbox: starting a local Vite listener failed with `listen EPERM`, and the installed Playwright package had no matching browser binary.

## Deviations from the plan
None. This is a user-directed v1.3 final UI-polish task rather than a numbered phase from `PHASES.md`.

## New API knowledge
None.

## How to verify (human runs these)
1. Run `cd ~/open-code-projects/ComfortCurators/app && npm run dev`.
2. Open `http://localhost:3000/login` at desktop width. Expect the exact Downloads Minar image on the right, rotated left and fitted exactly to the hero's height.
3. Confirm the supporting “Three roles. One tenant. No passwords.” line is gone, “Choose your keys.” is 20% larger, and the hero/role-card boundary retains the spacing it had when the supporting deck existed.
4. Hover the architectural portions of the Minar image with a fine pointer. Expect the square difference cursor to become exactly four times as large as it does over a conventional button; much of the empty/background area outside the approximate polygon should not activate it.
5. Click the Minar image. Expect the original four-beat entry sequence to replay and return to `/login` when complete; keyboard focus plus Enter/Space must also activate it.
6. Resize below 700px. Expect the image to retain its full vertical fit without stretching or cropping.
7. At desktop widths above 700px, test short and tall viewport heights. Expect the header, hero, and all three role cards to fit within one viewport without page-level scrolling.
4. Run `npm run build && npm run lint && npx vitest run`. Expect all commands to exit successfully; lint currently reports eight pre-existing warnings.

## Open questions for the human
1. Does the desktop/mobile focal crop match your preferred view of the architecture? Recommendation: verify visually in the already-running host browser and adjust only `object-position` if needed.

## What's next
Complete the human visual check, then continue v1.3 final UI polishing from the next requested screen.
