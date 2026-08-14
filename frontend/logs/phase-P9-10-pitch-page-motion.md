# Phase P9.10 — pitch page motion pass

- **Date:** 2026-08-10
- **Agent/model:** gpt-5 (codex, medium effort)
- **Status:** complete

## What I built

A scroll-driven reveal system for the `/expansion` pitch page that gives each major section a fade+translateY entrance as it scrolls into the viewport, provides a fixed right-edge stage progress indicator that highlights the current section, and animates the two SVG bar charts with a clip-path wipe revealing bars from left to right with staggered timing.

No content, copy, numbers, section order, or claims were changed. This is a pure motion/presentation pass.

## Mechanism used for scroll reveals (and why)

**IntersectionObserver + CSS transitions gated by `data-revealed` attributes.**

Each major section (`expansion-intro`, `expansion-stage`, `authority-model`, `unit-economics`) received a `data-expansion-section` attribute (values 0–5). A single `IntersectionObserver` in `useEffect` watches all six sections. When a section crosses 15–30% visibility:

1. It sets `data-revealed="true"` on the intersecting element, which triggers CSS opacity+translateY transitions (`700ms var(--ease-expo-out)`) from hidden to visible.
2. Child elements within each section stagger in with a `70ms` delay per element (matching the `0.07s` stagger from INTERACTION.md's reference-site forensics).
3. The observer tracks all simultaneously intersecting sections and picks the one whose top edge is closest to (but not above) the viewport top as the "active" stage for the progress indicator.

**Why IntersectionObserver over Lenis hooks or `@scroll-timeline`:**
- The app already has Lenis for smooth scrolling, but Lenis doesn't provide section-visibility callbacks — it's a scroll animation driver, not a visibility detector.
- `@scroll-timeline` / `animation-timeline: scroll()` has uneven browser support and would require a fallback anyway.
- IntersectionObserver is universally supported, doesn't trigger on every scroll frame, and plays naturally with the existing CSS transition system already used throughout the app (easing tokens from `index.css`, no new curves).
- This approach maintains continuous scroll — everything remains reachable by normal scrolling, search, screen readers, and print.

## How the charts animate

The two existing SVG bar charts use `clip-path: inset(0 100% 0 0)` → `clip-path: inset(0 0 0 0)` transitions triggered when the unit-economics section receives `data-revealed="true"`.

- Each bar row wipes in with staggered delays (300ms, 380ms, 460ms per row within each SVG group).
- Chart labels (`economics-bar-labels text`, `contribution-labels text`) fade in at `900ms` delay — after the bars finish.
- The existing `<rect>` elements, their `x`/`y`/`width`/`height` attributes, and the SVG viewBox are all unchanged. I used `clip-path` rather than `transform: scaleX` to avoid SVG coordinate-system quirks with non-zero-origin rects.

## How prefers-reduced-motion is respected

Two layers:

1. **CSS layer**: `@media (prefers-reduced-motion: reduce)` block sets all animated properties to their final visible state — `opacity: 1`, `transform: none`, `clip-path: none`, `transition: none` — regardless of `data-revealed` attribute state. This already existed in the original CSS and was extended for the new reveal classes.

2. **JS layer**: On mount, the `useEffect` checks `window.matchMedia("(prefers-reduced-motion: reduce)")`. If matched, it immediately sets `data-revealed="true"` on all sections and skips the IntersectionObserver entirely — no observer, no intersection map, no stage tracking. Content is fully visible with zero animation.

The existing `@media (prefers-reduced-motion: reduce)` block at line 586 of expansion.css was preserved and extended.

## Files added or changed

- **`app/src/routes/expansion.tsx`** — Added `useState`/`useEffect` imports, IntersectionObserver logic, `data-expansion-section` attributes on 6 sections, and stage progress indicator JSX (a fixed right-edge nav with mono dot labels, `aria-hidden="true"`).
- **`app/src/routes/expansion.css`** — Added ~150 lines: section reveal base states + transitions, nth-child stagger delays, chart clip-path wipe-ins with row stagger, authority-flow diagram fade-in, chart-container fade-in, stage progress indicator styles (fixed position, dot + label, active state with scale 1.8 via `--ease-overshoot`), responsive hide below 860px, and extended `@media (prefers-reduced-motion: reduce)`.
- **`logs/phase-P9-10-pitch-page-motion.md`** — This file.

No other files were touched. `src/index.css` and `src/main.tsx` are unchanged. The Lenis smooth-scroll instance in `src/components/smooth-scroll.tsx` is untouched — this system layers on top of Lenis, not within it.

## What I verified live vs. build/lint-only

- **`npm run build`** — passes (TypeScript compilation + Vite production build, 239 modules).
- **`npx oxlint`** — passes (only pre-existing warnings in unrelated files, none in expansion.tsx or expansion.css).
- **Built CSS** — confirmed presence of `data-revealed` (34 matches), `clip-path` (27), `stage-progress` (12), and `prefers-reduced-motion` (20) in the production CSS bundle.
- **Built JS** — confirmed `data-expansion-section` attribute string is present in the minified JS bundle, meaning the React render includes it.
- **Live browser test** — NOT performed. The sandbox cannot launch Chromium headless: `libnspr4.so` and other system libraries are missing and `apt-get` requires root. The build and code review constitute the full verification for this pass.

## Decisions I made

1. **clip-path over transform for SVG bars**: Even though `transform: scaleX` is simpler, SVG coordinate space makes `transform-origin` unreliable for rects positioned at different x-offsets. `clip-path: inset()` works per-element and produces the same visual result (a left-to-right bar grow) without coordinate-system debugging.

2. **stagger timing 70ms**: Matches the reference sites' `stagger: 0.07` value from INTERACTION.md §3, not an arbitrary choice. Applied to sibling children within each section (not to deeply nested descendants, to keep stagger sensible).

3. **Stage progress indicator over page-level background shift**: ART-DIRECTION.md mandates sharp corners, no gradients, no dark mode. A per-stage background tint or rotation would violate those rules. A right-edge mono dot progress indicator is consistent with the existing "registration marks" and mono-metadata conventions, uses `--ease-overshoot` for the dot scale, and stays out of the reader's content flow.

4. **Hidden below 860px**: The stage progress indicator is `display: none` below 860px viewport width. On narrow screens, a fixed right-edge element crowds the content — the existing responsive breakpoints already collapse the two-column grid, and the progress dots would overlap the text.

5. **No deep stagger on nested children**: Only direct children of sections get staggered. Deeper nesting (e.g., individual chart bars, auth-flow nodes) have their own dedicated transitions. This avoids a dozen staggered children competing visually.

## What did NOT work

- **Browser launch in the sandbox**: Missing system libraries for Chromium headless (libnspr4, libnss3, etc.). Root permissions unavailable. Verified via build output inspection and code review instead.
- **Lenis-driven reveals**: Lenis exposes `lenis.on('scroll', ...)` but section-in-viewport detection from scroll position requires manual intersection math with `getBoundingClientRect`, which IntersectionObserver already does more efficiently and without per-frame callbacks.

## Open questions

- The stage progress indicator uses `activeStage === i` for exact-match activation. This means only one dot is highlighted at a time. Should earlier (already passed) stages stay dimly lit as a "progress trail"? I chose single-highlight because multiple lit dots read as "you can click these" — but these are not clickable, so a single active marker is less ambiguous. Worth a design review.
- The chart bar stagger delays (300/380/460ms per row) are currently hardcoded by `:nth-child` indices. If the SVG bar data ever changes (more or fewer bars per row), these will need manual adjustment. A JS-driven stagger (setting `transition-delay` via a data attribute) would be more resilient but is currently overkill for fixed chart data.
