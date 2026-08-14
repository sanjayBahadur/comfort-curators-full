# P9.34 Kutub Hero Reintegration

Replaced the five grayscale Kutub login cutouts with the four verified
full-color, counterclockwise-rotated landscape cutouts. The cluster remains
absolutely positioned inside `.access-intro`, which retains `overflow: hidden`.

## Final composition

The source images are used as-is, with `object-fit: contain`. Negative
`right` offsets keep each wide/base end at or beyond the intro's right edge,
while the tapering tips extend left into the composition.

| Image | Desktop width | Desktop right / bottom | Mobile width | Mobile right / bottom |
| --- | ---: | --- | ---: | --- |
| `cutout-1-color.png` | 360px | `-4%` / `7%` | 250px | `-8%` / `7%` |
| `cutout-2-color.png` | 300px | `-18%` / `40%` | 210px | `-24%` / `37%` |
| `cutout-5-color.png` | 400px | `-2%` / `-4%` | 280px | `-5%` / `-5%` |
| `cutout-6-color.png` | 260px | `-27%` / `20%` | 180px | `-34%` / `18%` |

The desktop cluster is `min(78%, 900px)` wide and `100%` high. At the
`700px` breakpoint it becomes `100%` wide and `78%` high. The role-card
`min-height` and padding declarations were not changed.

## Checks

- `npm run build` passed.
- `npm run lint` completed with existing warnings unrelated to this change.
- Old grayscale cutout filenames no longer appear in `src/routes/login.tsx`.
- The focused diff contains no border-radius, box-shadow, or gradient changes.
