# Phase P9.30: Kutub Hero Integration

## Composition

The former `login-strip.webp` band was removed and replaced with a static,
decorative cluster contained by `.access-intro` (`overflow: hidden`). The
cluster is anchored to the bottom-right and layered behind the headline.
All five images retain their native proportions through `height: auto` and
`object-fit: contain`.

- `cutout-1.png`: `120px` wide, `right: 4%`, bottom aligned, opacity `.58`.
  This is a small outer-right tower that establishes the cluster edge.
- `cutout-3.png`: `82px` wide, `right: 25%`, `bottom: -8%`, opacity `.42`.
  This is the smallest and faintest rear tower, adding height variation.
- `cutout-4.png`: `204px` wide, `right: 32%`, `bottom: -3%`, opacity `.82`.
  This is the largest/frontmost anchor for the loose skyline silhouette.
- `cutout-5.png`: `142px` wide, `right: 58%`, `bottom: -10%`, opacity `.5`.
  This adds a mid-scale, partially cropped layer toward the cluster's left.
- `cutout-6.png`: `92px` wide, `right: 76%`, `bottom: -4%`, opacity `.68`.
  This is a narrow, taller counterpoint at the left edge.

At screens up to `700px`, the cluster scales to `72%` width and `62%` height,
with each image reduced proportionally so the role-card grid remains below the
intro without layout flow from the art.

## Verification

- `npm run build`: passed. Vite emitted only the existing chunk-size warning.
- `npm run lint`: passed with exit code 0. Output contains existing warnings in
  unrelated files; no warning points to this change.
- Diff grep for `border-radius`, `box-shadow`, `linear-gradient`, and
  `radial-gradient`: no matches.
- `.access-role-card` desktop `min-height: 304px` and padding
  `24px 28px 28px` are unchanged; the mobile `min-height: 250px` is also
  unchanged.
- The five images are absolutely positioned inside `.access-intro`, whose
  existing minimum height and border remain in place. `overflow: hidden`
  prevents the artwork from entering the role-card section.
- `cutout-2.png` is not imported or referenced.
