# Phase P9.18 — expansion pitch rewrite

## Why this phase exists

The previous expansion pass treated `/expansion` as a conventional long page with gentle section reveals. That did not give the pitch its intended weight. This phase changes the route into a full-viewport, scroll-to-switch presentation and rewrites the narrative around the actual company thesis: Superhost is the deep-tech core; Comfort Curators is its operating beachhead; Curators Crew is the planned physical execution network.

## Content changes

- Replaced the equal-weight “three business lines” framing with one compounding operating loop.
- Rewrote the opening around the property operation as Superhost's proving ground.
- Positioned Comfort Curators as the current operating business that supplies revenue, operating context, and real edge cases while the agent is hardened.
- Positioned Superhost as the long-term autonomous property-operations thesis, with explicit language about what is built and where its authority stops.
- Positioned Curators Crew as a planned contract-labor distribution layer, not a currently scaled platform.
- Rewrote the authority slide from general regulated-adjacency language to the real runner and policy model documented in the backend: allowlisted tools, policy evaluation, read-only operations, scoped user-started UI control, approval-required proposals, denials, and audit events.
- Preserved both SVG unit-economics charts and their numeric data. Their labels now describe beachhead / agent / network layers, and the surrounding copy continues to mark the figures as illustrative rather than audited results or forecasts.

The factual boundary was checked against these read-only backend sources:

- `internal/automation/superhost/prompt/v1.md`
- `internal/automation/superhost/tools.go`

## Presentation and motion changes

- Installed `animejs` 4.5.0 through npm. The package includes its own TypeScript declarations, so no separate `@types/animejs` package was added.
- Rebuilt the route as six semantic `<section>` slides in a dedicated `100dvh` scroll container.
- Added native mandatory vertical scroll snap with `scroll-snap-stop`, preserving ordinary wheel, trackpad, touch, and scrollbar navigation.
- Marked the nested presentation scroller with `data-lenis-prevent` so the already-global window Lenis instance does not consume its wheel/touch input.
- Added Arrow Up/Down and Page Up/Down navigation using `scrollIntoView`; the handler ignores form/editable targets and does not intercept normal wheel/touch scrolling.
- Added both a visible `ESC / EXIT` link and Escape-key routing back to `/login`.
- Rebuilt the progress rail as labelled, keyboard-focusable buttons with `aria-current` instead of an `aria-hidden` decoration.
- Used the anime.js v4 API directly: `createTimeline`, timeline `add`, `stagger`, named easings, and `revert` cleanup.
- Added per-slide entrance and exit timelines. Text and metadata enter in a 70 ms editorial stagger; rules draw from the left; authority nodes and connectors assemble in sequence; chart bars wipe from their real SVG origins and labels follow. Exit uses a short reverse stagger before the next snap settles.
- Used an `IntersectionObserver` rooted to the presentation scroller to select and replay the active slide choreography.

## Accessibility and fallback

- Every slide remains ordinary DOM content with a real `h1` or `h2` and an `aria-labelledby` relationship.
- The implementation is not a JS-only carousel: sections remain scrollable, searchable, printable, and reachable if JavaScript animation does not run.
- `prefers-reduced-motion: reduce` stops the animation/observer/keyboard-snap wiring at the React effect boundary.
- The matching CSS fallback removes the fixed-height scroller, scroll snapping, fixed header, progress rail, transitions, and animations, producing a fully visible static document. Print uses the same static path.

## Art-direction audit

- Kept the existing paper, ink, red, font, rule, and easing tokens.
- Red is reserved for the Superhost-core mark, the policy-denial branch, and the existing chart key.
- Introduced no gradients, shadows, rounded corners, new palette, or generic SaaS card styling.
- Final grep over `expansion.tsx` and `expansion.css` found no `border-radius`, `box-shadow`, `linear-gradient`, or `radial-gradient` declarations.

## Static verification

- `npm run build` — passed.
- `npm run lint` — passed with no warning originating from the final expansion implementation. The repository still reports its pre-existing warnings in unrelated Superhost/test/context files.
- Browser launch was intentionally not attempted, per the phase constraint. The orchestrator should verify viewport fit, snap feel, wheel/trackpad/touch behavior, keyboard navigation, progress state, and reduced-motion behavior in the independent live-browser pass.
