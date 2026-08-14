# Phase P9.24 — Expansion flywheel

## Outcome

The expansion page is now organized around one compounding operating system instead of three sequential company descriptions. The existing, honest business-stage copy remains close to verbatim, but it is supporting detail beneath an early full-slide flywheel that establishes how the businesses depend on one another.

## Flywheel design

The new slide `01 / THE LOOP` uses a hard-rule, ink-on-paper triangular diagram with three labeled company nodes:

- **Comfort Curators / property + real estate** outputs real operations: revenue, operating data, and edge cases.
- **Superhost OS / deep tech** receives that signal, is trained and hardened by it, and outputs bounded, approved intent.
- **Curators Crew / HR + labor** routes approved work to people in the physical world, then outputs completion proof and exceptions.

Three black directional paths make the primary circuit explicit. A fourth red return path carries Crew's training signal directly back to Superhost. Every path has an on-diagram label describing what moves and what it does: `DATA + REVENUE / TRAINS + FUNDS`, `APPROVED WORK / ROUTES TO PEOPLE`, `EVIDENCE + EXCEPTIONS / RETURNS TO OPERATIONS`, and `TRAINING SIGNAL / HARDENS THE AGENT`.

The center is a black registration-block reading `EACH TURN / CHEAPER / BETTER / FASTER`, qualified by `MORE CONTEXT / MORE TRUST / LESS FRICTION`. This is intentionally qualitative; no invented metrics, scale claims, revenue figures, or forecasts were added.

## Motion to verify in a live browser

The page keeps its existing scroll-snap and `IntersectionObserver` mechanism. When the flywheel slide crosses the active threshold:

1. The heading reveals with the existing expo-out language.
2. The three company nodes land in sequence.
3. The four SVG routes draw in sequence using normalized `pathLength` and animated `strokeDashoffset`.
4. The black compounding core lands after the routes establish the system.
5. A red “work packet” begins after the diagram has formed and continuously travels clockwise around all three sides of the loop. It pauses at the corners so the exchange between nodes reads as a handoff, then disappears briefly before the next turn.

Leaving the slide reverts both the entrance timeline and the repeating packet timeline. Re-entering reconstructs the diagram from a clean state. All motion is implemented with `animejs` timelines.

On each company-detail slide, a small recurring triangular loop remains in the left rail. `CC`, `OS`, and `HR` stay connected by directional rules while the current company's node is filled red. This is the recurring visual cue that each detailed thesis is one node of the same system, not a separate business pitch.

The final slide replaces the previous illustrative percentage and rupee charts with an unquantified causal sequence: `OPERATE → LEARN → DELIVER → COMPOUND`. It closes with an explicit maturity note distinguishing what operates today, what is being built, and what is planned.

## Responsive and reduced-motion behavior

At mobile widths, the wide triangular canvas becomes a legible vertical directional system. Company nodes, edge labels, and downward arrows retain the full causal chain; the large desktop SVG routes are hidden because they would be illegible when compressed.

When `prefers-reduced-motion: reduce` matches, the React animation effect returns before creating any anime.js timeline. CSS also removes all animations and transitions, restores all reveal content and SVG path strokes, disables scroll snapping/smooth behavior, and hides only the decorative moving packet. The complete labeled diagram remains visible as a static system; there is no blank animation-dependent state.

## Verification performed

- `npm run build` — passes.
- `npm run lint` — exits successfully; it reports only seven pre-existing warnings in unrelated shared/Superhost files.
- `git diff --check` — passes.
- Added-line audit for `border-radius`, `box-shadow`, `linear-gradient`, and `radial-gradient` — no matches.
- Final `expansion.tsx` and `expansion.css` were read back in full after implementation.
