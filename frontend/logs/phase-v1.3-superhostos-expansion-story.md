# v1.3 — Final UI polish wrap-up

- **Date completed:** 2026-08-13
- **Branch:** `v1.3`
- **Status:** complete
- **Scope:** login/access UI, intro replay behavior, and the public expansion story

## Delivered changes and effects

### Login and access

- Replaced the fragmented login artwork with the supplied Minar PNG, rotated 90 degrees counter-clockwise and fitted to the existing vertical media box without changing the page background.
- Turned the Minar artwork into an accessible replay control with an approximate image-shaped hit area and enlarged difference cursor response.
- The replay action now uses `/?replay=intro`, allowing the Comfort Curators entry sequence to restart even if an authentication token is still present. Normal `/` routing remains unchanged.
- Removed the “Three roles. One tenant. No passwords.” line, restored the missing layout footprint with responsive spacing, and increased “Choose your keys.” by 20%.
- Rebalanced the desktop login grid to remain within one viewport and retain natural document flow on smaller media formats.

**Effect:** the login page now has one intentional visual focal point, preserves the black-and-white editorial system, avoids desktop page overflow, and provides a reliable way to replay the existing intro.

### Expansion story

- Replaced the previous short three-company pitch with an eleven-slide Comfort Curators / SuperhostOS narrative covering property operations, verified evidence, bounded agent authority, the Human Capability Index, capability matching, the company system, market context, compounding, and the closing vision.
- Reused the established `/debug` terminal language and existing documentary assets. The image was removed from slide 03 and moved to the final slide to support the close without crowding the operating-system example.
- Restored the strongest material from the previous page: the operating beachhead, policy/authority boundary, Crew execution role, and `Operate → Learn → Deliver → Compound` model.
- Rebuilt the three-company relationship as a CSS-grid diagram with equal company partitions, dedicated arrow columns, parallel opposing SuperhostOS/Crew relationships, and an independent verified-outcomes return lane. It no longer relies on SVG-positioned text.
- Removed the global Superhost launcher from `/expansion` only, preserving it everywhere else.
- Added linked source notes for the India tourism-employment figure, gig-workforce projection, and expected skills disruption.

**Effect:** the expansion page now tells a complete, evidence-backed product story with clearer company boundaries and operational credibility. Slides 04, 07, 09, and 10 use bounded editorial grids instead of distributing spare viewport height through flexible rows, reducing collisions and unintentional dead space.

### Responsive behavior

- Desktop chapters remain exactly one viewport high with keyboard, wheel, and progress navigation.
- Short-height desktop rules reduce type and component heights before content can collide.
- An intermediate 861–1180px layout protects smaller laptops and landscape tablets from inheriting wide-screen geometry.
- At 860px and below, fixed slide heights and scroll snapping are removed; diagrams, policy steps, metrics, and sources become natural stacked content.
- Reduced-motion and print behavior remain supported.

**Effect:** the presentation is designed around the reviewed 1920×1080 form factor while retaining explicit fallbacks for compact desktop, tablet, and mobile layouts.

## Files in the final expansion commit

- `app/src/routes/expansion.tsx`
- `app/src/routes/expansion.css`
- `app/src/routes/login.tsx`
- `app/src/routes/entry-route.tsx`
- `app/src/components/superhost/GlobalSuperhost.tsx`
- `logs/phase-v1.3-superhostos-expansion-story.md`

The Minar asset, login styling, image key, and cursor behavior were saved earlier on this branch in commit `b6ff2f2a`.

## Verification

- `npm run build` — passed
- `npm run lint` — passed with eight pre-existing warnings outside the changed expansion/login files
- `npx vitest --config vitest.config.ts run` — 27 tests passed
- `git diff --check` — passed

Browser screenshots could not be automated inside the managed sandbox because local browser execution and listener access were restricted. Visual refinement was therefore performed against the user-supplied 1920×1080 screenshots.

## Unaffected areas

- No backend, API contract, authentication implementation, database, property operations, shopping, or role-dashboard behavior was changed.
- Existing unrelated untracked workspace files were left untouched and are not included in the version commits.
