# Phase UI 1.3.1 — Property handover dossier

- **Date:** 2026-08-13
- **Agent/model:** GPT-5
- **Status:** complete

## What I built
Recomposed the owner onboarding entry as a five-chapter editorial property dossier: Listing, Details, Authority, Documents, and Handover. The owner begins with a clearly marked dummy public listing, reviews a full-bleed property reveal and editable architectural specification, grants narrowly described permission, optionally selects export filenames as metadata references, and explicitly creates the real lead property and onboarding case.

The market outlook is a restrained typographic insert inside the listing dossier rather than an embedded terminal. The existing seven-step, server-persisted onboarding workspace remains unchanged and opens after the final handover.

## Files added or changed
`app/src/components/onboarding/PropertyHandover.tsx` — implements the dossier chapters, listing reveal, owner-controlled scope, document ledger, illustrative outlook, existing-case access, and final handover summary.

`app/src/components/onboarding/PropertyHandover.css` — defines the ruled editorial layout, image treatment, chapter transition, desktop index, responsive mobile index, and reduced-motion behavior.

The Documents chapter uses a large archival-paper drop zone with a document-sheet illustration, document-blue and manila accents, whole-zone native file input, drag-active treatment, supported formats, and selected-file count. It remains explicit that only filenames become evidence references.

`app/src/routes/onboarding.tsx` — replaces the conventional intake entry with the dossier and connects its final action to the existing property, onboarding-case, section, and evidence APIs. Direct visits no longer silently auto-resume an open case; saved records remain available in the first chapter.

`logs/phase-ui-1.3.1-property-handover-dossier.md` — records the phase boundaries and verification steps.

`logs/DECISIONS.md` — records the cross-phase design and integration boundary.

## Decisions I made
The visual metaphor is a property dossier accumulating evidence, not a SaaS wizard or operations terminal. Instrument Serif carries the declarations, JetBrains Mono identifies provenance and state, red appears only as an editorial registration mark, and green appears only on completed chapters.

The existing Gomti property photograph is reused as a clearly labeled demo-listing photograph. It is not claimed to come from Airbnb or to depict the entered URL.

The dummy URL is not fetched. Market figures are deterministic illustrative planning numbers that respond to capacity and are explicitly labeled as neither Airbnb data nor a valuation or promised rate.

The final action alone creates server state. It seeds property basics and goals, while optional export filenames become append-only document references. Authority, ownership verification, safety, inspection, package, and contract steps remain incomplete until handled in the existing onboarding workspace.

## What did NOT work
An earlier implementation paired the handover form with a persistent Superhost pricing terminal. It competed with the owner’s decision flow, made the page feel like an analytics demo, and was removed before this phase.

## Deviations from the plan
No live Airbnb or MCP connection was added because the repository exposes no verified connector or authority contract. The design demonstrates the handover protocol without claiming external access.

## New API knowledge
none

## How to verify (human runs these)
Run `cd ~/open-code-projects/ComfortCurators/app && npm run build`. Expected: TypeScript and the production Vite bundle complete successfully.

Run `cd ~/open-code-projects/ComfortCurators/app && npx vitest run`. Expected: 5 test files and 27 tests pass.

Sign in as an owner and open `/onboarding`. Expected: the Listing dossier appears rather than silently opening a saved case. Saved cases or an existing property remain accessible below the listing input.

Use the dummy URL and choose `FIND THE PROPERTY`. Expected: a full-width property reveal appears with an existing property photograph, editable specification sheet, and a clearly labeled demo market outlook. Changing maximum guests updates the illustrative midpoint.

Continue through Authority and Documents. Expected: continuation requires explicit permission; the screen excludes credentials, messages, payouts, and publishing control; selected files show as filename references with a statement that contents are not uploaded.

Choose `CREATE THE PROPERTY RECORD`. Expected: a lead property and server-backed onboarding case are created, then the existing onboarding workspace opens at Identity and authority. Listing basics are prefilled while authority, safety, inspection, package, and contract remain incomplete.

Resize below 1020px and 760px. Expected: the image and property record stack, the chapter index becomes a compact sticky horizontal rail, fields become single-column on mobile, and there is no horizontal page overflow.

## Open questions for the human

## What's next
Visually inspect all five chapters on the target 1920×1080 browser viewport before committing. Pay particular attention to the long Details chapter and mobile chapter-index behavior.
