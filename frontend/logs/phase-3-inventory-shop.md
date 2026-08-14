# Phase 3 — Inventory Shop

- **Date:** 2026-08-08
- **Agent/model:** GPT-5.6 Codex (orchestrator); DeepSeek V4 Flash, DeepSeek V4 Pro, Qwen 3.7 Plus, GPT-5.6 Luna (read-only specialists)
- **Status:** complete

## What I built
The centrepiece inventory shop now exists at `/properties/:propertyId/package`. It loads the full active catalog once, filters instantly from URL query state, supports search with the `/` shortcut, presents deliberate code-native product wells for an API with no image URL, and provides click quick-view plus a Shift+F10/right-click keyboard action menu without adding a visible Add button.

The screen uses dnd-kit with an 8px PointerSensor and a touch-exclusive 200ms TouchSensor. Catalog cards drag into the cart, repeated drops increment quantity, and cart rows drag back to the grid with a short torn exit. Quantity, expected monthly use, and package rules re-POST the whole immutable draft after 400ms. Abort controllers plus a monotonic request sequence reject stale responses; last-good totals remain visible; activation is possible only when the newest server draft signature matches the visible state. All setup/monthly figures are rendered from the API response in INR.

Desktop is a fixed 240px / fluid / 340px ruled window with centre scrolling. Below 1100px the filters become a top drawer and the cart becomes a visible bottom drop bar plus expandable sheet. A real headless-Chrome 390px touch profile completed a long-press drag into that bar.

The live acceptance harness passed all ten SHOP checks under the staff demo role: one catalog request across filtering, URL filter restoration after reload, three pointer drags, server-sourced cost responses, one package POST after five rapid quantity clicks, ACTIVE transition, INR decimals, 390px touch drag, zero radius/shadow, and grain opacity `0.035`.

After the human-approved demo-volume reset, the corrected seed completed twice: the first run created the exact Phase 2 scenario under the stable owner actor, and the second created nothing. Final owner-role browser acceptance then passed the complete ten-check SHOP gate. The owner returned both seeded properties; three pointer drags, API pricing, a five-click/one-POST debounce check, exact `₹2,700.00` formatting, activation, and a genuine 390px touch drag all passed. Computed styles found zero rounded shop elements, zero shadows, and grain opacity `0.035`.

## Files added or changed
`app/src/routes/package-shop.tsx` — owns catalog/filter/cart/rules state, drag sensors and drop zones, debounced server synchronization, activation, quick view, context menu, offline state, and mobile sheets.

`app/src/routes/package-shop.css` — implements the sharp ruled desktop/mobile composition, product/card/cart/cost states, three restrained delight moments, clip quick-view, torn removal, and reduced-motion handling.

`app/src/lib/api/shop.ts` — typed catalog, property, package-draft, and activation contracts over the shared API client.

`app/src/main.tsx` — registers `/properties/:propertyId/package`.

`app/src/routes/login.tsx` — exposes the first accessible property's shop link for non-guest demo roles.

`app/package.json` and `app/package-lock.json` — add `@dnd-kit/core`; the temporary Puppeteer acceptance dependency was installed without saving and pruned afterward.

`app/scripts/seed.ts` — creates fresh managed properties under `owner@demo.test`, switches back to staff for operational resources, and fails safely on legacy staff-owned data instead of creating duplicates.

`INTEGRATION.md` — records owner-authority visibility, immutable package IDs/HTTP 201, and the absent monthly-budget contract.

`logs/DECISIONS.md` — records draft-signature activation, touch sensor routing, budget honesty, and stable-owner seeding.

## Decisions I made
The builder starts empty even though Phase 2 has an active package. This makes the drag interaction and DRAFT→ACTIVE change honest and visible; the old active version stays untouched until activation supersedes it.

The current cart signature includes ordered item IDs, quantities, monthly-use values, and every backend-supported rule. A package ID is activation-eligible only after the server has returned that exact signature. This prevents a slow or failed save from activating stale pricing.

The optional monthly budget is labelled `DISPLAY LIMIT · NOT ENFORCED BY BACKEND`. It is kept out of the package POST because the halted contract has no such field.

Touch-generated pointer-down is filtered from PointerSensor listeners so the 200ms TouchSensor owns touch gestures rather than racing the general pointer sensor. The visible mobile bottom bar is its own drop zone; the closed sheet is `display:none` so it cannot become an invisible oversized target.

Product image wells use deterministic category initials and SKU marks. The catalog has no image field, and inventing remote product photography would be less honest and less reliable than a deliberate code-native graphic system.

## What did NOT work
The first live drag test found both drop hooks outside their own DndContext: draggable children registered, but the cart/grid hooks did not. Moving each hook into a rendered `DropArea` fixed pointer collision and three successive drops passed.

The first mobile automation resized the viewport but retained a desktop pointer profile. A touch-enabled Chrome profile then revealed that PointerSensor claimed touch-generated pointer events before TouchSensor. Routing touch exclusively to TouchSensor produced a completed 200ms long-press drop.

The first mobile assertion accepted whitespace and captured while the drag overlay was still present. The corrected acceptance required the explicit bottom-bar text `1 ITEM` and absence of `.shop-drag-overlay` before capture.

GPT-5.6 Luna's final screenshot review could not open `/tmp` under its OpenCode sandbox. The orchestrator inspected both original-resolution screenshots directly; Luna's earlier CSS plan was still used. DeepSeek Pro and Qwen returned PASS with only non-blocking polish, which was applied where useful.

The pre-reset owner role returned zero properties. Read-only backend inspection and live role comparison established that Phase 2's staff actor received the owner-authority grant. The seed was corrected, the human approved the one-time demo reset, and two clean consecutive runs proved the corrected owner-first sequence and idempotency.

The final owner acceptance's temporary Chromium harness could not deliver Shift+F10 or the dedicated Context Menu key through headless automation, although that keyboard path had already passed in the earlier staff-role live run and is role-independent. The owner rerun therefore exercised the ten checks required by `SHOP.md §11` without rounding this automation limitation into a product failure.

## Deviations from the plan
Monthly budget is display-only because the verified package API has no budget field. Sending an invented property would violate the integration policy.

No role deviation remains: the final ten-check acceptance ran through the real `owner@demo.test` session against the corrected owner-visible seed data.

## New API knowledge
Creating a property grants its `owner_authority_id` to the actor making that request. Owner property lists are filtered by those grants; staff lists are tenant-wide. Package draft creation returns HTTP 201 and every POST has a new immutable package ID. The package contract has no monthly-budget field. All three details are now in `INTEGRATION.md`.

## How to verify (human runs these)
1. Run `cd ~/comfort-curators-frontend/app && npm run seed` twice. Expected with the current data: both runs complete with nothing under `created`, and the two active owner-visible properties plus all other seeded totals remain unchanged.
2. Run `npm run lint && npm run build`. Expected: both exit zero.
3. Open `http://127.0.0.1:3000/login`, choose OWNER, and click `OPEN INVENTORY SHOP`. Expected after the reset: two authenticated properties and a direct shop link.
4. Filter a category and inspect Network. Expected: instant card/result changes, a `category=` URL parameter, and no catalog refetch caused by the filter. Reload; the category remains selected.
5. Drag three distinct cards to the cart. Expected: three rows, a settling cost panel, then INR setup/monthly totals from an HTTP 201 package response.
6. Click one quantity `+` five times quickly. Expected: exactly one package POST about 400ms after the final click and updated server totals.
7. Activate. Expected: DRAFT becomes red ACTIVE with a short misregistration flicker and red left rules on rows.
8. At 390px, long-press a card for at least 200ms and drag it to the bottom bar. Expected: the bar changes from `0 ITEMS` to `1 ITEM`; tapping it opens the cart sheet.
9. Focus a card and press Shift+F10. Expected: an action menu with ADD TO CART and QUICK VIEW; arrow keys move between actions and Escape returns focus.
10. Inspect the page. Expected: no rounded corners or shadows, hard black rules, warm paper, and subtle grain.

## Open questions for the human

## What's next
Phase 3 is complete. The human instructed the orchestrator to proceed after the reset, so begin Phase 4 from `PHASES.md`: build the staff operations ticket list/detail/create/dispatch flow, write its phase log, and stop at the next manual boundary.
