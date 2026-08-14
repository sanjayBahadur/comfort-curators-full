# Phase P9.33 — Flywheel loop fix

## Outcome

The red training-signal route no longer crosses the black Crew-to-Superhost
leg, and the moving packet is now a flat black-and-white hourglass.

## Geometric diagnosis and fix

The black leg runs from `A=(575,382)` to `B=(790,220)`, with vector
`(215,-162)` and length `sqrt(215^2 + 162^2) = 269.2` viewBox units. The
original red route ran from `R1=(565,406)` to `R2=(742,246)`, vector
`(177,-160)`. Solving the two segment equations showed that they intersected
inside both segments at approximately `(674.4,307.2)` (black parameter about
`0.54`, red parameter about `0.62`). This is the source of the tangled graph
reading visible near the animated packet.

The red route now runs from `R1=(540,430)` to `R2=(755,268)`, vector
`(215,-162)`, exactly parallel to the black leg. Its offset from both black
endpoints is `(-35,+48)`, so each endpoint clearance is
`sqrt(35^2 + 48^2) = 59.4` viewBox units, versus the original clearances of
`sqrt(10^2 + 24^2) = 26.0` at the lower endpoint and
`sqrt(48^2 + 26^2) = 54.6` at the upper endpoint. The parallel route cannot
cross the black leg and is 59.4 units from the packet at the corresponding
black-corner waypoints `(575,382)` and `(790,220)`.

The arrow marker uses a 12 by 12 triangle with its tip at local `x=12` and
`refX=10`. SVG aligns the path endpoint with marker local `x=10`, placing the
shaft inside the triangle; the tip extends 2 marker units beyond the endpoint
by design. The marker was therefore not changed. SVG paint order also already
puts the packet after all routes, so it paints above an overlap rather than
behind one.

## Packet icon

The circle and crossing line were replaced by a 16 by 24 viewBox-unit
hourglass path centered at `(330,150)`. Its explicit paper fill and ink
stroke preserve the page's hard-edged, flat visual language without the old
red ball or white line. The existing animation waypoints, timing, and opacity
loop are unchanged.

## Verification

- `npm run build` — passes.
- `npm run lint` — exits successfully with eight pre-existing warnings in
  unrelated files.
- `git diff --check` — passes.
- Diff audit for `border-radius`, `box-shadow`, `linear-gradient`, and
  `radial-gradient` — no matches.
