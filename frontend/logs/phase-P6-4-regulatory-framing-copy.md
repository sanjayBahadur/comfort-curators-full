# Phase P6.4 — regulatory framing copy pass

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-flash
- **Status:** complete

## What I built

A copy pass over the existing `/expansion` page tightening the two regulatory
framings flagged as pitch risk in `IMPLEMENTATION-SPEC.md` §3.8. No new
sections, no content removed, no chart numbers or `P6.3` illustrative
disclosure touched.

1. **Equipment finance is partnered, never in-house.** The Curators Crew
   revenue line "partnered equipment finance" now reads, in full, "equipment
   financing via a licensed NBFC partner". A new one-sentence note under the
   stage's revenue list names the structure explicitly: it is a partnership,
   the licensed NBFC partner holds the credit, and Comfort Curators provides
   placement and service — the credit never sits on our books.
2. **Stage 2 finance and legal is preparation, not practice.** Tightened the
   authority-model copy so the split reads unambiguously: the system
   *assembles documents, filings, and drafts*, and a *licensed professional
   files, represents, and signs*. The "machine prepares / licensed human
   decides / every decision audited" refrain (the moat framing from `P6.2`)
   was left intact and reads consistently with the tightened paragraph.
3. **Authority model as the moat** — left that section's argument, heading,
   and diagram untouched; only checked it still reads consistently.

## Files added or changed

- `app/src/routes/expansion.tsx`
- `app/src/routes/expansion.css`
- `logs/phase-P6-4-regulatory-framing-copy.md`

## Decisions I made

- Put the exact spec phrase "equipment financing via a licensed NBFC partner"
  in the stage revenue line itself (the natural place §3.8's table names it),
  and the structure sentence as a small mono footnote under that stage's
  revenue list rather than mixing a credit-structure sentence into the
  authority-model prose.
- Kept the stage-2 revenue label "finance and legal preparation" exactly as
  the spec table writes it; the preparation-not-practice precision lives in
  the authority-model copy paragraph, which now enumerates what the machine
  assembles versus what the licensed professional does.
- Left the `P6.2` moat heading/punchline ("The machine prepares. A licensed
  human decides. Every decision is audited.") unchanged — it is the section's
  stated point and matches §3.8 framing 3 verbatim.
- Added a `note?: string` field to the stage data type (with a `Stage` type
  in place of `as const`) so the footnote renders only where present; reused
  the page's existing mono/uppercase/muted footnote styling rather than
  inventing a new look.
- Did not touch `src/index.css` or any file outside `expansion.tsx` /
  `expansion.css`.

## What did NOT work

- The first `npm run build` attempt failed because `node_modules` was absent
  (`tsc: not found`). `npm ci` installed the existing lockfile; no manifest
  changes were made. Build and oxlint then passed.
- Dropping `as const` for the explicit `Stage` type was necessary to make
  `note` optional; a union-with-`as const` would have rejected `.note` on
  stages without it.

## Deviations from the plan

- None. Grep checks performed before editing confirmed the gaps: line 31
  "partnered equipment finance" (short label only), line 86 "finance and
  legal document preparation" with the assemble/decide split only implied.

## Open questions

- None. Verification greps: `grep -n "licensed NBFC partner"` matches in
  `expansion.tsx` (revenue line + note); no unqualified installment/loan
  language remains — searched for
  `installment|emi|equated|loan|credit` across the page and the only
  credit references are inside the qualified NBFC partnership sentence.
