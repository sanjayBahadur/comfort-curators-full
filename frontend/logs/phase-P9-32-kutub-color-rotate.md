# P9.32 Kutub Color + Rotate

Generated new full-color PNG cutouts for sources 1, 2, 5, and 6. Sources 3 and 4 and all retired grayscale outputs were not processed. The mask uses broad top/side sky sampling, RGB-distance thresholding, largest-component selection, and a thin adjacent edge feather. RGB data is retained; no grayscale conversion is applied. Jimp rotation is counterclockwise 90 degrees after resizing and the verification pass confirms the PNGs retain alpha.

## Verification

| File | Output | Transparent | Partial alpha | Opaque | Components | Bounding box | Color spot-check |
| --- | --- | ---: | ---: | ---: | ---: | --- | --- |
| `cutout-1-color.png` | 800x450 | 71.44% | 0.56% | 28.00% | 1 | `[184,85,799,384]` | 100,748 opaque pixels with unequal RGB; examples `[34,42,51]`, `[44,49,58]`, `[48,55,59]` |
| `cutout-2-color.png` | 800x451 | 80.39% | 0.82% | 18.78% | 1 | `[287,114,799,354]` | 67,765 opaque pixels with unequal RGB; examples `[205,184,173]`, `[170,143,126]` |
| `cutout-5-color.png` | 800x451 | 78.35% | 0.43% | 21.21% | 1 | `[272,125,799,379]` | 76,531 opaque pixels with unequal RGB; examples `[206,213,211]`, `[155,148,144]` |
| `cutout-6-color.png` | 800x450 | 74.67% | 0.66% | 24.67% | 1 | `[27,110,746,339]` | 88,796 opaque pixels with unequal RGB; examples `[79,97,68]`, `[72,80,7]` |

The bounding boxes are substantially smaller than their canvases, and all partial-alpha fractions remain below the 1-2% thin-feather target. The rotated dimensions confirm the alpha-bearing images were rotated rather than flattened.

## Checks

- `node scripts/verify-kutub-color.mjs` passed for all four outputs.
- `npm run build` passed.
- `npm run lint` passed.
