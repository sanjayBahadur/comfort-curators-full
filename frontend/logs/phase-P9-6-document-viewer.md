# Phase P9.6 — document viewer (modal + mini-modal variants)

- **Date:** 2026-08-10
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** partial

## What I built

Added a document viewer to the existing `/documents` table. Clicking the document name opens a full metadata modal; a separate `PREVIEW` control opens a compact quick-peek popover.

The viewer shows only data that exists in `OwnerDocumentData`. It does not present a file preview, thumbnail, download action, or open-file action because this build stores metadata only.

## Full modal vs. mini popover — what each shows, and why the split

The full modal shows title, humanized type, status, version/current version, created and updated timestamps, optional expiry, and the raw document id. It also states that no file is stored. The mini popover is intentionally scan-sized and shows only title, type, status, and created date.

The full modal uses the existing `Modal` primitive. The quick peek uses the existing `Popover` primitive. They have independent triggers and state.

## Whether you added an image/icon, and if so its source + license

No image or icon was added. A typographic type label and the existing status mark fit the restrained document route better and avoid implying that a file or thumbnail exists.

## Files added or changed

- `app/src/routes/documents.tsx`
- `app/src/components/documents/DocumentViewer.tsx`
- `app/src/components/documents/DocumentViewer.css`

## What I verified live vs. only build/lint-checked

The Vite app responded successfully at `http://localhost:3000`.

`npm run build` passed after installing the locked dependencies with `npm ci`.

`npm run lint` passed with existing warnings in unrelated files only.

The document interactions were not live-verified because the requested Playwright `channel: "chrome"` browser could not launch in this sandbox.

## Decisions I made

- The document name is the full-modal trigger, preserving the existing table and making the primary record action discoverable.
- `PREVIEW` is a separate small trigger so the two variants are not conflated.
- The route's existing `uploadedLabel` formatter is passed into the viewer and reused for every displayed timestamp.
- All five requested status values have distinct status-mark styling.

## What did NOT work

Playwright failed to launch the required real Chrome channel: `Chromium distribution 'chrome' is not found at /opt/google/chrome/chrome`. No `google-chrome`, `chromium`, or `chromium-browser` executable was available. Therefore I could not live-check focus movement, Escape handling, focus return, independent popover behavior, or seeded records with and without `expires_at`.

## Open questions

None for the implementation. A Chrome installation or browser-enabled environment is needed to complete the interaction checklist.
