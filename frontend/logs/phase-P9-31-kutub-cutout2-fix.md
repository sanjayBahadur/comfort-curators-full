# Phase P9.31 — `cutout-2.png` regeneration

## Before

The previously generated `app/src/assets/hero/kutub-cutouts/cutout-2.png`
was measured at 451x800 with:

| Metric | Before |
| --- | ---: |
| Opaque bounding box | `[0,0]`–`[450,799]` (full canvas) |
| Opaque connected components | 356 |
| Stray opaque pixels outside largest component | 8,237 |
| Partial-alpha pixels | 106,631 (29.55%) |
| Corner `(0,0)` | opaque, RGB `(254,254,254)` |

## Fix

Updated `app/scripts/cutout-kutub.mjs` and regenerated source 2 only.
The generator now:

- Resizes the source to its final 451x800 canvas before masking.
- Samples the top edge at multiple heights plus both side edges through the
  upper 62% of the frame, using a 4px sample step and 28 RGB range padding.
- Builds a binary foreground mask and keeps only its largest 8-connected
  component.
- Applies a one-pixel silhouette feather after component filtering, rather
  than assigning partial alpha across the whole photo.
- Desaturates the output and writes true RGBA PNG data.

## After

Measured with `app/scripts/measure-cutout.mjs`:

| Metric | After |
| --- | ---: |
| Dimensions | 451x800 |
| Opaque bounding box | `[99,287]`–`[330,799]` |
| Opaque connected components | **1** |
| Stray opaque pixels outside largest component | **0** |
| Largest opaque component | 61,699 px |
| Partial-alpha pixels | **6,337 (1.76%)** |

The opaque region is no longer the full canvas, the disconnected cloud/noise
components are gone, and partial alpha is reduced from 29.55% to 1.76%.
