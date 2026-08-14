# Phase P9.29 — Qutub Minar cutout review (independent)

Review-only dispatch. No assets were modified. Analysis performed with `jimp`
(read-only; script lived in `/tmp`, installed `jimp` in a throwaway dir).

**Note on paths:** the brief listed the assets at `src/assets/hero/kutub-cutouts/`;
in this repo the four PNGs actually live at
`app/src/assets/hero/kutub-cutouts/` (app workspace root). All paths below are
relative to the repo root.

Files reviewed:

- `app/src/assets/hero/kutub-cutouts/cutout-2.png` — 451×800 RGBA (597,846 B)
- `app/src/assets/hero/kutub-cutouts/cutout-3.png` — 230×800 RGBA (210,264 B)
- `app/src/assets/hero/kutub-cutouts/cutout-5.png` — 451×800 RGBA (255,014 B)
- `app/src/assets/hero/kutub-cutouts/cutout-6.png` — 257×800 RGBA (268,811 B)

All four pass two structural checks: **every visible pixel is grayscale**
(R=G=B, verified pixel-by-pixel — zero non-grayscale visible pixels in any
file), so there is no chromatic fringing at the silhouette edges; and none of
the files is a "giant untouched opaque rectangle" (each has a real transparent
background). The specific failures found are shape/content failures, below.

---

## 1. Per-file alpha statistics (computed, not eyeballed)

### cutout-2.png (451×800, 360,800 px)

| Metric | Value |
|---|---|
| Fully transparent (α = 0) | 24.41% (88,053 px) |
| Fully opaque (α = 255) | 46.04% (166,108 px) |
| Partial (0 < α < 255) | **29.55%** (106,631 px) |
| Opaque bounding box | `[0,0]`–`[450,799]` — **the entire frame** |
| Opaque connected components | **356** |
| Stray opaque pixels off the main body | **8,237 px** |
| Opaque pixels in left 10 cols | 3,353 |
| Opaque pixels in right 10 cols | 2,904 |
| Far-left opaque sample | `(0,0)` RGB `254,254,254` — near-white sky kept opaque |

Partial-alpha distribution across the frame (buckets
1–59 / 60–119 / 120–179 / 180–229 / 230–254): `31,393 / 20,051 / 19,929 /
18,958 / 16,300` — a broad haze field, **not** an edge feather.

### cutout-3.png (230×800, 184,000 px)

| Metric | Value |
|---|---|
| Fully transparent (α = 0) | 49.66% (91,375 px) |
| Fully opaque (α = 255) | 48.90% (89,985 px) |
| Partial (0 < α < 255) | 1.43% (2,640 px) |
| Opaque bounding box | `[10,0]`–`[196,799]` (187×800) |
| Opaque connected components | **1** |
| Stray opaque pixels | 0 |
| Column span of opaque | 81.3% of width, single contiguous band |
| Max column opacity | 800 rows (full height) |
| Blank margins | top 0, bottom 0, left 8, right 30 cols |

### cutout-5.png (451×800, 360,800 px)

| Metric | Value |
|---|---|
| Fully transparent (α = 0) | 72.32% (260,932 px) |
| Fully opaque (α = 255) | 20.29% (73,218 px) |
| Partial (0 < α < 255) | 7.39% (26,650 px) |
| Opaque bounding box | `[74,274]`–`[331,799]` (258×526) — bottom-anchored band |
| Opaque connected components | 11 (1 main + 10 strays) |
| Stray opaque pixels | 188 px |
| Top rows fully transparent | 240 (tower starts ~⅓ down the frame) |

Stray detail: main body 73,030 px; detached blob of **164 px at
`x[297..331] y[378..402]`, mean gray 58** (tower body nearby ≈ gray 70, so it
reads as a dark mark, not a cloud); plus a 13 px speck, a 4 px speck and six
single-pixel specks. 7,145 partial-alpha pixels lie outside the tower bbox
(alpha 1–3, gray ≈ 206 — very faint pale wisps, e.g. `x=348..359, y=240..253`
above the tower tip).

### cutout-6.png (257×800, 205,600 px)

| Metric | Value |
|---|---|
| Fully transparent (α = 0) | 49.59% (101,953 px) |
| Fully opaque (α = 255) | 49.13% (101,007 px) |
| Partial (0 < α < 255) | 1.28% (2,639 px) |
| Opaque bounding box | `[21,0]`–`[225,799]` (205×800) |
| Opaque connected components | **1** |
| Stray opaque pixels | 0 |
| Column span of opaque | 79.8% of width, single contiguous band |
| Max column opacity | 800 rows (full height) |
| Blank margins | top 0, bottom 0, left 18, right 29 cols |

---

## 2. What the masks actually look like (ASCII downsamples of the alpha channel)

`#` ≈ α>240 · `+` ≈ 120–240 · `.` ≈ 40–120 · `:` ≈ 1–40

```
cutout-2 (451×800)                     cutout-5 (451×800)
OPAQUE CONTENT SPANS THE WHOLE         CLEAN NARROW BAND, bottom-anchored
FRAME — noise + clouds at both         (top 240 rows empty, stray blobs
edges, disconnected blobs above        circled below are the 164px/13px
and beside the tower.                  specks — not visible at this scale)
                                  ......
+ + . : : # +:... etc, full width       :                    :##:
```
(Both files rendered at 60×64; cutout-2 shows opaque and half-alpha material
across all 60 columns including the far-left and far-right edge columns;
cutout-5 shows a single tapering vertical band around columns 10–44 of 60.)

cutout-3 and cutout-6 both render as a single, narrow, centered, slightly
tapering vertical band with a wide base, 0–800 px tall, fluting visible as
internal tonal bands, and a ~3px partial-alpha feather at the silhouette edge
(partial α ≈ 1.3–1.4% of pixels, consistent with a thin edge feather, not a
halo).

---

## 3. Findings

1. **cutout-2.png — broken.** This is the failure mode the brief warned about,
   in its "scattered noise" form rather than the "giant rectangle" form. The
   opaque region is not a tower silhouette: opaque/partial content covers the
   whole frame (opaque bbox == full 451×800 canvas), 356 disconnected opaque
   components, 8,237 stray opaque pixels, and near-white sky retained as fully
   opaque even at the extreme frame corner `(0,0)` (RGB 254,254,254). The
   29.55% partial-alpha population is a broad haze band (roughly even across
   all alpha values 1–254), not a silhouette-edge feather — so on a paper
   background this will composite as a big soft grey smudge field around a
   tower, not a clean cutout. Disconnected opaque clouds float above the tower
   (e.g. 789 px at `x[281..322] y[39..94]`, 539 px at `x[32..65] y[0..31]`,
   410 px at `x[222..249] y[0..21]`).
2. **cutout-5.png — minor defects.** Silhouette shape is correct (single
   dominant narrow vertical band, bottom-anchored, 258×526 opaque bbox, top
   240 rows clean). Caveats: (a) a detached **164 px dark blob** at
   `x[297..331] y[378..402]` (gray ≈ 58) sitting beside the tower's lower
   right — dark enough to read as a stray mark on paper; (b) a 13 px and a
   4 px speck plus six single-pixel opaque specks; (c) ~7,145 very faint
   partial pixels (α 1–3, gray ≈ 206) outside the tower, incl. some above the
   tip and at frame edges — effectively invisible at α≤3 but present.
3. **cutout-3.png — clean.** Single component, zero strays, thin feather,
   correct slender-tower proportion (187 px wide × 800 tall).
4. **cutout-6.png — clean.** Single component, zero strays, thin feather,
   correct slender-tower proportion (205 px wide × 800 tall).

Nothing else wrong: no color fringing anywhere (verified zero non-grayscale
visible pixels per file); no corrupt/tiny images; aspect ratios are sensible
for a tall minaret; all four are true RGBA PNGs with real transparency.

---

## 4. Verdicts

| File | Verdict | Caveat / reason |
|---|---|---|
| `cutout-2.png` | **Broken — needs a redo** | Cutout did not isolate the tower: opaque+partial content covers the full 451×800 frame, 356 disconnected opaque components (8,237 stray px), clouds and near-white sky (RGB 254 at the corner) kept fully opaque, and a 29.6% partial-alpha haze field. Will composite as scattered noise/smudge, not a minaret silhouette. Regenerate with a tighter sky-distance threshold and a connected-main-body guard (drop components disconnected from the dominant tower mass). |
| `cutout-3.png` | **Usable as-is** | None. |
| `cutout-5.png` | **Usable with a caveat** | Detached 164 px dark blob (gray 58) at `x[297..331] y[378..402]`, a 13 px and 4 px speck plus six 1-px specks; faint α1–3 wisps above the tower tip and near frame edges (effectively invisible). If the blob is intended tower detail, ship as-is; otherwise it should be dropped. |
| `cutout-6.png` | **Usable as-is** | None. |

### Recommended action (for the orchestrator — not performed here)
Re-run `app/scripts/cutout-kutub.mjs`'s logic for **source-2** only with (a) a
sky-distance cutoff that no longer keeps near-white sky (the corner sample
`RGB 183,205,241` left 254-white opaque), (b) a `softSkyDistance`/ramp that
produces a thin edge feather instead of a whole-frame haze band, and (c) a
connected-component pass that keeps only the dominant tower mass. Optionally
trim the 164 px blob from cutout-5. No changes were made to any repo file other
than this log.
