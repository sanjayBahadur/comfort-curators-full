# Phase 6 — Owner Onboarding Wizard

- **Date:** 2026-08-08
- **Agent/model:** GPT-5.6 Codex (orchestrator); DeepSeek V4 Flash (API audit); Qwen 3.8 Max (UX/accessibility audit); Qwen 3.7 Plus (host browser acceptance); DeepSeek V4 Pro (read-only final review)
- **Status:** complete

## What I built
The owner onboarding wizard now exists at `/onboarding`, linked from the owner dashboard and protected by the real owner demo role. It presents the specified seven calm editorial steps: identity and authority, address and basics, documents, inspection evidence, service preferences with autonomy, active package, and contract/review.

Every data-bearing Continue writes real backend state. The seven screens group the backend's 15 persisted checklist keys, while the sidebar and review rail display progress from `GET /v1/onboarding/cases/{id}/progress`. On reload, the client lists cases, filters them to owner-visible properties, hydrates the newest open case, and restores its full server state. No onboarding case ID, progress, autonomy, or form payload is stored in browser storage.

Autonomy is a required native three-choice control and round-trips through the only retained backend carrier, `service_preferences.automation_limits`, as `autonomy_level:<value>`. It is displayed in the persistent summary and final review with the explicit statement that it changes no approval or operating behavior.

The package step reads the property's explicit active server package and exact monthly price, then links back to the Phase 3 builder. The contract step shows an accepted agreement or the exact terms of an existing draft before allowing acceptance. When no agreement exists, it says so and does not invent terms. Finishing the intake never claims to activate the property or enforce autonomy.

The start surface can use an existing owner-visible property or create a real lead property first. Because property creation is not idempotent, retries reuse a normalized owner-visible address match before POSTing.

Live browser acceptance created case `case_039b4456d5511559`, persisted all 15 server checks, reloaded to the final review, and recovered `autonomy_level:autonomous` from the backend. The 13/13 harness also verified no onboarding browser-storage keys, an exact active-package handoff, honest contract boundaries, zero radius/shadow, 1440px containment, 390px containment, and a clean runtime. Desktop and mobile screenshots were inspected; a mobile sticky-action overlap found visually was removed, and a focused responsive rerun passed with a static 68px action and no overflow.

## Files added or changed
`app/src/lib/api/onboarding.ts` — typed owner property, case, progress, evidence, inspection, package, agreement, property creation, section persistence, and explicit draft acceptance contracts.

`app/src/routes/onboarding.tsx` — owner gate, new/existing property preface, server resume, seven wizard steps, 15-key progress mapping, autonomy persistence, package handoff, and contract boundary.

`app/src/routes/onboarding.css` — medium-collage three-region editorial wizard and a readable, non-overlapping 390px flow.

`app/src/main.tsx` — registers `/onboarding`.

`app/src/routes/dashboard.tsx` — adds owner onboarding navigation and turns the no-property state into a live onboarding link.

`app/src/routes/dashboard.css` — styles the no-property onboarding action consistently.

`INTEGRATION.md` — records the live onboarding discovery, mutation, progress, autonomy, package, contract, and activation contracts.

`logs/DECISIONS.md` — records the server-resume, 7-to-15 mapping, autonomy carrier, aggregate boundary, and property retry decisions.

## Decisions I made
The wizard uses the owner-scoped start route rather than the generic case POST because it verifies owner role, property existence, and property visibility. Generic list results are filtered to the authenticated owner's property IDs before any case is displayed, then a full case GET supplies the actual saved content.

The seven product steps remain stable even though the backend exposes 15 progress keys. Document and inspection steps use the dedicated append-only evidence routes; service preferences collect the remaining typed operating inputs without exposing backend storage terminology as navigation.

Document and inspection controls explicitly record metadata references, not uploads. Browser-generated hashes cover those submitted references so the backend can preserve immutable evidence metadata without claiming MinIO content exists.

The wizard never creates commercial terms. An actual existing draft can be reviewed verbatim and accepted through the contract service; otherwise the screen remains an honest pending state. The onboarding case is not terminally activated by the Finish intake button.

## What did NOT work
The first Codex-side live curl could not reach ports 8080 or 3000 because that process is network-isolated. The acceptance stack was restarted without resetting volumes through the authorized OpenCode host, and all live checks ran there.

The first OpenCode Go DeepSeek Flash acceptance invocation failed before executing because the provider rejected its routed model name. Qwen 3.7 Plus ran the fixed harness successfully without changing product files.

The initial mobile design used a sticky Save/Finish row. Automated containment passed, but original-resolution screenshot inspection showed that the row obscured review content. Making the action static preserved the 68px touch target and removed the overlap; the focused responsive rerun passed.

The backend has no one-to-one seven-section onboarding API and no autonomy field. Unknown payload fields are discarded, so a top-level `autonomy_level` would not survive reload. The retained marker convention avoids that dead end.

## Deviations from the plan
The contract step accepts only real server-held terms. With the current seed there is no agreement to accept, so the completed intake truthfully shows `Not issued` instead of manufacturing a demo contract.

The Finish intake action does not call terminal onboarding activation or transition the property. Those are separate backend actions and the phase requirements do not provide authority to imply that an intake acknowledgement, package selection, or autonomy preference performs them automatically.

## New API knowledge
The owner-scoped case POST is safer than the generic route. Case-list rows are shallow, generic case routes are tenant-scoped, progress has 15 fixed keys, typed section PUTs accept only ten exact names, and contacts/evidence/inspections use dedicated endpoints. Autonomy survives only in a recognized field such as `automation_limits`. Property creation is not idempotent. Packages, agreements, onboarding, and lifecycle are independent property-linked aggregates. These findings are now in `INTEGRATION.md`.

## How to verify (human runs these)
1. Run `cd ~/comfort-curators-frontend/app && npm run lint && npm run build`. Expected: both commands exit zero.
2. Open `http://127.0.0.1:3000/login`, choose OWNER, then open `/onboarding`. Expected: the saved Phase 6 case resumes from the backend and shows `15 / 15`, status `ready`, and autonomy `autonomous`.
3. Click step `05`. Expected: Autonomous is selected and the disclosure says it does not change how work is approved or carried out yet.
4. Reload the page. Expected: the same case and progress return without restarting. Browser storage contains only the demo session token/role, not onboarding state.
5. Click step `06`. Expected: Hazratganj Studio's active package is shown with its server-priced monthly cost and a real link to the package builder.
6. Click step `07`. Expected: the current seed says no agreement is issued, does not offer fake acceptance, and states that finishing intake is not contract acceptance or property activation.
7. Click `CHOOSE ANOTHER CASE`. Expected: existing properties can start a new server case; the lower form can create a real lead property and case, with a warning that the address is matched before retries.
8. Set the viewport to 390px. Expected: the numbered rail becomes horizontal, forms and review become one column, actions remain at least 44px high, and nothing overlaps or scrolls horizontally.

## Open questions for the human

## What's next
Phase 6 is complete. Stop at this boundary for manual approval. After approval, begin Phase 7 from `PHASES.md`; do not extend onboarding or build the curator portal before that approval.
