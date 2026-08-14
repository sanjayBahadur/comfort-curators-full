# Art Direction

**This supersedes the earlier `DESIGN.md`** (warm-green hospitality palette). That
direction was wrong for this brief and has been deleted. Design is the primary
attraction of this product — treat this document as a specification, not advice.

---

## 1. Reference analysis

Extracted from the live sites on 2026-08-08, not recalled.

| Site | Type | Palette |
|---|---|---|
| **justus-john.com** | Instrument Serif (roman + **italic**), Inter 400/500/600, **JetBrains Mono** | Paper `#faf9f7` |
| **bigsurceramics.com** | Monument Grotesk Plus, ABC Diatype Plus, Gravity, Alte Haas Grotesk, **Arial Black** | `#000` / `#fff` + electric `#FF0000`, `#e52bba`, `#468bf0` |
| **veryworkinprogress.com** | GeneralSans variable (+italic), **QJae** display, Instrument Sans, **Tabular Variable** | `#000` (147×) / `#fff` (74×) + `#f472b6` pink, `#2d4a7a` navy |

Its CSS bundle *was* recoverable on a second pass — full motion and
interaction forensics for all three sites are in
**[`INTERACTION.md`](INTERACTION.md)**. Note VWIP ships a dedicated **tabular
numeral** typeface, independently confirming the mono-for-numbers rule below.

**What the references agree on:**

1. **Warm paper, never pure white.** `#faf9f7`.
2. **A high-contrast display serif set against a neutral grotesk.** The serif
   carries the personality; the grotesk carries the information.
3. **Monospace for metadata** — the single most underused move in commercial UI
   and a huge part of the art-tech feel. SKUs, prices, timestamps, counts.
4. **Arial Black.** Deliberately "wrong", deliberately cheap-looking. This is the
   punk instinct: the typographic equivalent of a photocopied flyer.
5. **Colour is nearly absent, then screams.** Black and paper, and one saturated
   hit that means something.

## 2. The thesis

> **Punk collage** is chaotic, torn, urgent, disposable.
> **High fashion** is restrained, precise, expensive, permanent.

These fight. The resolution, and the whole art direction in one line:

> ### The grid is immaculate. The content violates it on purpose.

Restraint is the *system*; collage is the *event*. A torn-paper element only
reads as expensive when it sits on a page that is otherwise flawlessly aligned.
Collage everywhere is a student zine. Collage once, on a perfect grid, is 032c.

Lineage: *032c*, *Purple*, SSENSE editorial, Dazed, Cargo-built artist sites,
Yohji lookbooks, early Vetements. Not: grunge textures, Photoshop bevels,
"distressed" filters, anything that looks like a Canva template called PUNK.

**The reference sites are not maximalist.** They are 90% white space and
impeccable type, with a small number of violent moments. Copy that ratio.

## 3. Tokens

```css
:root {
  /* Paper — never pure white */
  --paper:        #FAF9F7;
  --paper-2:      #F2F0EC;   /* sunk wells, filter rail */
  --paper-3:      #E8E5DF;   /* borders, rules */

  /* Ink — TRUE black. Punk does not do soft grey. */
  --ink:          #000000;
  --ink-60:       rgba(0,0,0,.60);
  --ink-40:       rgba(0,0,0,.40);
  --ink-12:       rgba(0,0,0,.12);

  /* The scream. ONE accent. It means "act" or "this is alive". */
  --red:          #FF0000;

  /* Rare second hit — reserve for delight moments only (confetti, hover
     easter eggs, the cookie slip). Never for status. */
  --magenta:      #E52BBA;

  /* Status — desaturated so --red keeps its power */
  --ok:           #1A6B3C;
  --warn:         #8A6A00;
  --danger:       #FF0000;   /* danger IS the scream */

  --radius:       0px;       /* SHARP. see §4 */
  --rule:         1px solid var(--ink);
}
```

**No gradients. No shadows. No rounded corners. No dark mode.**

## 4. Sharp corners, hard rules

`--radius: 0`. Everything is a rectangle with a 1px black rule.

Rounded corners and soft shadows are the visual language of friendly consumer
SaaS. This is not that. Hard edges plus warm paper is what makes the reference
sites feel like printed objects rather than web pages — the single highest-
leverage decision in this document.

The only exceptions: circular avatars, and the hand-drawn annotation marks in §7.

## 5. Typography

All free, all on Google Fonts or Fontsource.

```bash
npm i @fontsource/instrument-serif @fontsource-variable/archivo \
      @fontsource/archivo-black @fontsource-variable/jetbrains-mono
```

| Role | Face | Usage |
|---|---|---|
| **Display** | `Instrument Serif` | Page titles, hero, pull quotes. Often **italic**. Large — 48–120px. Tight leading (0.95). |
| **Punk hit** | `Archivo Black` | Cookie banner, popups, empty states, price shouts. `text-transform: uppercase`, `letter-spacing: -0.02em`. Sparingly. |
| **UI / body** | `Archivo` variable | Everything functional. 14–16px. |
| **Metadata** | `JetBrains Mono` | SKUs, prices, IDs, timestamps, counts, filter labels, status. `text-transform: uppercase`, `letter-spacing: 0.08em`, 11–13px. |

**Rules**

- Display serif and Archivo Black **never appear in the same block.** Pick one
  voice per moment.
- Everything that is a *number or a code* is mono. Prices, quantities, SKUs,
  ticket IDs, dates. This alone does most of the work.
- Set display type **tight**: `line-height: 0.95`, `letter-spacing: -0.03em`.
- Italic Instrument Serif is the house accent. Use it for one word in a heading,
  for empty-state copy, for the cart total label. It is the expensive gesture.

```css
h1 { font-family: 'Instrument Serif'; font-size: clamp(40px, 7vw, 96px);
     line-height: .95; letter-spacing: -.03em; }
.meta { font-family: 'JetBrains Mono'; font-size: 11px;
        text-transform: uppercase; letter-spacing: .08em; color: var(--ink-60); }
.shout { font-family: 'Archivo Black'; text-transform: uppercase;
         letter-spacing: -.02em; }
```

## 6. Layout — the immaculate grid

12 columns, 24px gutter, hard `1px solid black` rules between major regions.
Visible grid lines are part of the aesthetic — do not hide the structure.

- Rules are **black and 1px**, never grey and never 2px.
- Sections are separated by rules, not by whitespace alone.
- **Registration marks** (small crop-mark crosses) in page corners — a printed-
  matter signal that costs nothing and reads as expensive.
- Numbered sections in mono: `01 / INVENTORY`, `02 / YOUR CART`.

## 7. Collage mechanics

How to build punk collage in CSS without it looking like clipart. **All of these
are CSS/SVG — no image assets required.**

**Grain.** A tiled SVG `feTurbulence` at `opacity: .035`, `mix-blend-mode:
multiply`, fixed over the whole page. This one effect does more than everything
else combined — it makes the screen feel like paper.

```css
body::after {
  content:''; position:fixed; inset:0; pointer-events:none; z-index:9999;
  opacity:.035; mix-blend-mode:multiply;
  background-image:url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='.8' numOctaves='4'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E");
}
```

**Halftone.** `repeating-radial-gradient` dot screen over images or as a fill
for accent blocks. Dot size 3px, spacing 4px.

**Torn edge.** `clip-path: polygon(...)` with 20–30 slightly irregular points
along one edge. Write one utility class, reuse it. Never tear all four edges.

**Tape.** A rotated rectangle, `rgba(0,0,0,.06)`, 2px blur, `rotate(-3deg)`,
sitting across a corner. Two per page maximum.

**Rotation.** Two bands, corrected against the references (VWIP uses -12° and
-15° on decorative marks):
- **±3°** for anything containing text you must read — cards, panels, cart rows.
- **−12° to −15°** for stickers, badges, seals, tape — decorative marks, not content.

**Misregistration.** The punk-print signature: duplicate a headline in `--red`,
offset it `2px, -2px`, `mix-blend-mode: multiply`, place it behind. Looks like a
misprinted second pass. Use on **one** headline per page.

**Cut-out type.** Black text on a `--red` block, the block rotated 1.5°, clipped
with a slightly irregular polygon.

**Marker annotation.** Hand-drawn SVG circles, arrows and underlines (irregular
stroke, `stroke-linecap: round`) in `--red`, used to point at exactly one thing
per screen. This is the highest-charm, lowest-cost move available.

**Crossed-out text.** For struck prices or superseded values: real strikethrough
plus a hand-drawn SVG scribble over it.

## 8. Where collage is allowed

**This table is the most important thing in the document.** Getting it wrong
either makes the app boring or makes it unusable.

| Surface | Collage level | Why |
|---|---|---|
| Landing / login | **Maximum** | First impression. Go hard. |
| Cookie slip, popups | **Maximum** | §9 — designed to be screenshotted |
| Inventory shop + cart | **High** | The attraction. Grain, mono, hard rules, one marker annotation. |
| Owner dashboard | **Medium** | Grid, rules, mono metadata. No tearing. |
| Onboarding wizard | **Medium** | Editorial serif, generous, calm |
| Ops tickets & dispatch | **Low** | Someone is working. Grid, rules, mono. **No rotation, no tearing, no grain over tables.** |
| Curator mobile | **Low** | Outdoors, one-handed, in a hurry. Legibility wins. |

An operator under time pressure does not want art. The restraint here is what
makes the maximalism elsewhere read as *deliberate* rather than *default*.

## 9. Cookie slip and popups

Explicitly requested, and worth real effort: this is the first thing anyone sees.

### Cookie slip

Not a bar. **A torn paper slip**, bottom-left, entering at `-2deg` with a slight
overshoot (`cubic-bezier(.2,1.2,.3,1)`, 420ms).

```
┌───────────────────────────────╌╌╌┐   ← torn right edge
│  ██ WE USE COOKIES ██             │   Archivo Black, uppercase
│                                   │
│  Some are necessary. Some tell    │   Archivo 14px
│  us which towels you liked.       │   ← wry, human, specific
│                                   │
│  [ ACCEPT ALL ]  [ NECESSARY ]    │   black block / outlined
│  ─────────────────────────────    │
│  PRIVACY · 01                     │   JetBrains Mono 11px
└───────────────────────────────╌╌╌┘
     ▓ tape strip, rotated -6deg
```

- Header on a `--red` block, cut-out white type, block rotated 1.5°
- Halftone in the top-right corner
- `ACCEPT ALL` — solid black, white type. `NECESSARY ONLY` — 1px outline.
  **Both equally prominent.** Dark-pattern cookie banners are the opposite of
  premium, and in several jurisdictions the opposite of legal.
- On accept: the slip *tears away* — clip-path animates, slight rotate, fade.
  Do not just fade it.

### Popups

Same family. Rules for all of them:

- **Never centred with a grey scrim.** Off-centre, at an angle, over the live page.
- Entry: overshoot easing. Exit: tear or slide, never a plain fade.
- Every popup carries a mono index — `POPUP 03 / WELCOME`.
- **Maximum one popup per session.** Nothing destroys premium faster than a
  second modal.
- Dismissal is always obvious. Intrigue is not the same as trapping people.

## 10. Motion

**Curves and durations are specified in [`INTERACTION.md §3`](INTERACTION.md),
extracted from the reference sites' own stylesheets. Use those, not these.**
Summary: expo-out `cubic-bezier(.16,1,.3,1)` for reveals, overshoot
`cubic-bezier(.34,1.56,.64,1)` for objects being placed, durations 250–900ms,
stagger `0.07` for elements and `0.02` for characters.

- My earlier guess of `cubic-bezier(.2,1.2,.3,1)` was too soft — the references
  overshoot harder (1.56, and justus goes to 1.8). That punch matters.
- **Install Lenis.** Inertial smooth scroll is the single biggest perceptual
  difference between "nice" and "expensive" (`INTERACTION.md §8`).
- Hover on a card: `translateY(-2px)` **plus** the black rule thickening. No shadow.
- Page transitions: a paper-slide, not a fade.
- **Never animate a number's value.** Prices come from the server. Cross-fade the
  old and new value over 150ms. A count-up animation implies client-side maths
  and would be a lie (`INTEGRATION.md §6`).
- `prefers-reduced-motion`: drop translation and rotation, keep opacity.

## 11. Voice

Confident, dry, specific, never cute. Lowercase is fine; exclamation marks are not.

| Instead of | Write |
|---|---|
| "No data available" | "Nothing here yet. Drag something in." |
| "Your cart is empty" | "empty. *for now.*" (italic serif on "for now") |
| "Loading..." | `LOADING · 03` (mono) |
| "Oops! Something went wrong" | The API's actual message, in mono |
| "Welcome back!" | "back again." |

Copy is part of the art direction. Generic microcopy undoes good typography.

## 12. Anti-slop checklist

Before handing back any screen:

- [ ] Paper is `#FAF9F7`, not `#FFFFFF`
- [ ] Black is `#000000`, not a soft grey
- [ ] **No rounded corners.** No shadows. No gradients.
- [ ] Grain overlay is present and subtle (≤4%)
- [ ] Every number, code, and timestamp is in JetBrains Mono
- [ ] `--red` appears **once or twice** on the screen, never more
- [ ] Rotation is within ±3°
- [ ] Rules are 1px and black
- [ ] Not using Inter as the display face
- [ ] No emoji anywhere in the UI
- [ ] Empty state has written copy, not "No data"
- [ ] Real Lucknow addresses, ₹ prices, plausible names — never "Property 1"
- [ ] Ops tables have **no** tearing, rotation, or grain

## 13. What to build first

**Lenis smooth scroll**, the **grain overlay**, the **blend-difference cursor**,
the **type scale**, and the **1px-rule grid**. Those five land most of the
aesthetic before a single component exists — see `INTERACTION.md §8` for the
full priority table with effort estimates. Build them in Phase 0
as a `/debug` style sheet you can look at, and get them signed off before
anything else — every later screen inherits them.

## 14. The Superhost terminal — a governed exception

Decision D1. The one machine surface on a paper page.

Everything else in this document is warm paper and printed matter — grain,
hard rules, torn edges, Instrument Serif. The terminal is the one place the
house voice stops being paper and becomes a machine: true black background,
phosphor green, JetBrains Mono. A desk-bound terminal, not a window into it.

It is deliberately **not** "dark mode". §3 says *no dark mode* and that line
still holds — there is no theme toggle, no second mode for the app, no
switch to throw. This is one scoped component carrying a terminal metaphor:
the machine is a thing on the page, and everything around it stays paper.

```css
.superhost-terminal {
  --phosphor:      #00FF66;
  --phosphor-dim:  #00994d;
  background: var(--ink);
  color: var(--phosphor);
  font-family: var(--font-meta);   /* JetBrains Mono */
  border: 1px solid var(--ink);
  border-radius: 0;
}
```

**Green is scoped to `.superhost-terminal` and nowhere else. No green on
`:root`. No green anywhere else on the page.**

`--phosphor` and `--phosphor-dim` are declared *inside* `.superhost-terminal`
and nowhere else. They are not root tokens — do not add them to the §3
`:root` block. No other component may reference them. If a later block ever
needs green outside this one component, that is not a request to loosen this
rule — it is the rule working as designed. This is what keeps D1 an exception
rather than a second theme, and it is enforced by grep at P4's gate:
`--phosphor` must appear nowhere outside `.superhost-terminal`.

**Where it appears:** per `IMPLEMENTATION-SPEC.md §3.6`, a block on the page
at `/dashboard`, `/ops/tickets`, `/ops/tickets/:id`, and `/stay`. It is part
of the page — a black rectangle sitting on paper — **not a floating chat
widget**.

**The one place red and green coexist:** policy denials render in `--red`
*inside* the terminal. This is deliberate — the machine's own refusal — and
it is the one documented exception to §12's "*`--red` appears once or twice
per screen, never more*", *inside this component specifically*. A later
reviewer should not flag it: a denial is the terminal refusing an action,
and it is read that way on purpose. Anywhere outside the terminal, the
anti-slop checklist still holds in full.
