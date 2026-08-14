# Phase P9.4 — seed realistic demo documents

- **Date:** 2026-08-10
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built
Added an idempotent `seedDocuments(properties)` step to `scripts/seed.ts`. It reads the property-scoped document collection before creating records, derives each title's property name from the returned `service_address`, and records created/reused document counts.

## Documents created (property, title, document_type — full list)
- Gomti Riverside 2BHK, Owner Service Agreement — Gomti Riverside 2BHK, `agreement`
- Gomti Riverside 2BHK, Annual Compliance Certificate — Gomti Riverside 2BHK, `compliance_cert`
- Gomti Riverside 2BHK, FY 2026 Property Tax Record — Gomti Riverside 2BHK, `tax_document`
- Hazratganj Studio, Home Insurance Policy — Hazratganj Studio, `insurance_policy`
- Hazratganj Studio, Move-in Inspection Report — Hazratganj Studio, `inspection_report`
- Hazratganj Studio, Registered Property Deed — Hazratganj Studio, `property_deed`

## Files added or changed
- Changed `app/scripts/seed.ts`
- Added `logs/phase-P9-4-seed-documents.md`

## Whether you ran the seed live or only type-checked it, and why
The seed was attempted live with `npx tsx scripts/seed.ts`, but preflight failed with `fetch failed` because no reachable backend/feed was available in the sandbox. After installing the locked dependencies with `npm ci`, `npm run build`, `npm run lint`, and a standalone TypeScript check for `scripts/seed.ts` passed. The second live run could not be performed for the same unavailable-service reason.

## Decisions I made
- Used the seed script's existing `request<T>()` helper and direct `POST /v1/documents` call, matching the surrounding seed conventions.
- Seeded three metadata-only documents per property with six distinct allowed document types.
- Matched existing documents by exact generated title, and added the generated title to the in-memory set after creation so a run cannot duplicate within its own request sequence.
- No file upload, object storage, or document-version logic was added.

## What did NOT work
- Live backend verification and the requested two-run duplicate proof were unavailable because the backend and Vite demo feed were not reachable.
- `AGENTS.md` was not present anywhere under the workspace, so the supplied clean-architecture mini rules and task constraints were used.

## Open questions
- None for this block.
