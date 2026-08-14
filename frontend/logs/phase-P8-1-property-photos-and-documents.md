# Phase P8.1 — Property stock photos + demo document richness

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** partial

## What I built

- Added two real residential photographs as optimized WebP assets in
  `app/src/assets/properties/`.
- Added `app/src/lib/property-images.ts`, a pure frontend lookup used by both
  the owner dashboard cards and property detail hero.
- Wired photos into both property surfaces without adding a property image
  field to the API contract. Missing matches continue to render the existing
  text-only treatment.
- Part B was escalated: the seed script currently creates no documents and
  contains no document-creation API call, so no new seed/backend flow was
  invented.

## Photo sources and licenses (Part A)

- **Gomti Riverside 2BHK:** Unsplash image source URL:
  `https://images.unsplash.com/photo-1600585154340-be6161a56a0c?w=1400&q=85&fm=jpg`.
  Used under the Unsplash License, which permits downloading, modifying, and
  using Unsplash images for commercial and non-commercial purposes. License:
  `https://unsplash.com/license`.
- **Hazratganj Studio:** Unsplash image source URL:
  `https://images.unsplash.com/photo-1600607687920-4e2a09cf159d?w=1400&q=85&fm=jpg`.
  Used under the same Unsplash License: `https://unsplash.com/license`.

Both source images were downloaded, resized to 1000px on the long edge, and
converted to WebP. The photos were selected for plausible contemporary
residential interiors/exteriors rather than as literal claims that the source
photographs were taken in Lucknow.

## Files added or changed

- Added `app/src/assets/properties/gomti-riverside-2bhk.webp`.
- Added `app/src/assets/properties/hazratganj-studio.webp`.
- Added `app/src/lib/property-images.ts`.
- Changed `app/src/routes/dashboard.tsx` and `dashboard.css`.
- Changed `app/src/routes/property-detail.tsx` and `property-detail.css`.

## Decisions I made

- Matched by normalized `service_address.line1` plus postal code because the
  backend regenerates `prop_...` IDs on a fresh seed, while these demo
  addresses are the stable identity already used by `scripts/seed.ts`.
- Kept the lookup frontend-only, following the existing `catalog-images.ts`
  pattern exactly. No backend or contract files were changed.
- Kept photos inside the existing grid columns and made the image conditional,
  preserving the old render path when a property has no honest match.

## What did NOT work

- Unsplash's public HTML pages returned HTTP 401 in this environment, so the
  image CDN URLs and published Unsplash License page are recorded directly
  rather than relying on page scraping. The license terms were checked from
  the published Unsplash License reference.

## Open questions

- Part B cannot be completed within this block's scope: `app/scripts/seed.ts`
  has no `document`, `invoice`, or `createDocument` call. Adding realistic
  documents would require inventing a new seed call/API flow, which the task
  explicitly prohibits. The existing owner UI's `POST /v1/documents` call is
  not used by the seed script and does not change this finding.
