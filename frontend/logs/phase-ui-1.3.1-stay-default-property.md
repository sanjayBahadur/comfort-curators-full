# Phase UI 1.3.1 — Stay default property

- **Date:** 2026-08-13
- **Agent/model:** GPT-5
- **Status:** complete

## What I built
Made `/stay` automatically select the first property returned for the account when the property list loads. This immediately enables the existing reservation, catalog, house-guide, request-help, and Superhost property context. If the account has no properties, the page remains unselected and preserves its existing empty guidance.

## Files added or changed
`app/src/routes/stay.tsx` — selects the first available property after a successful property-list query and repairs an invalid selection to the first valid property.

`logs/phase-ui-1.3.1-stay-default-property.md` — records the default-selection behavior and safety boundary.

## Decisions I made
The default is applied only after the property query succeeds and returns at least one item. No placeholder ID is created, and no dependent property request runs for an empty account. An ID that no longer exists in the loaded list is treated like no selection and safely replaced with the first valid property.

## What did NOT work
none

## Deviations from the plan
none

## New API knowledge
none

## How to verify (human runs these)
Run `cd ~/open-code-projects/ComfortCurators/app && npm run build`. Expected: TypeScript and the production bundle complete successfully.

Open `/stay` with an account containing properties. Expected: the first property is selected automatically and its stay, guide, store, and Superhost context begin loading.

Open `/stay` with an account containing no properties. Expected: `SELECT PROPERTY` remains visible and no property-scoped reservation or catalog request is made.

## Open questions for the human

## What's next
Visually verify the automatically selected property and its dependent sections before committing this phase.
