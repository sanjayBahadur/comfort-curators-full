# Design review — P9.21

## Overall assessment

The eight screens unmistakably belong to one product. Warm paper, true-black rules, mono metadata, tight Instrument Serif, and sparse Archivo Black form a coherent vocabulary. The strongest screens use the grid as content architecture rather than decoration. This does not need to become friendlier, rounder, softer, or more conventionally "SaaS."

The recurring weaknesses are more concrete: the global header has collisions, wrapping, and one false active state; provisional system language is exposed in user-facing places; dormant "not connected" and "select a property" states dominate too much prime space; and some long pages leave large areas that feel unresolved rather than deliberately empty.

## 01 — Login / role selection

### What is working

- "Choose your *keys*." is a strong first-frame headline. The italic word supplies the expensive house gesture, and the two-line break feels composed rather than ornamental.
- The three equal role columns are a genuinely useful grid. `01 / 02 / 03`, mono capability labels, and vertical rules make the choices legible without cards, shadows, or generic application chrome.
- The black-and-white Lucknow image gives the otherwise typographic page locality and texture. The centered `LUCKNOW · IN` in the header reinforces that grounding with very little visual weight.
- The descriptions and bottom-aligned `ENTER →` links share a baseline, so the lower half reads as one designed object rather than three independently placed blocks.

### What is weak or inconsistent

- "Three roles. One tenant. No passwords." straddles the paper/photo boundary. Its last words land over a busy, dark image and lose contrast. This looks accidentally positioned, not like a deliberate violation of the grid. Keep the sentence entirely on paper or give it a defined caption field over the image.
- The Staff and especially Guest descriptions are internal implementation notes, not product copy. "Verify the backend's third real role without implying a full portal" instantly tells an investor that this is a test harness. "Enter the shared staff authority…" has the same architecture-first tone. The Owner description is the only one that speaks to a person's actual job.
- The large role panels do not clearly read as click targets. Only the small `ENTER →` label looks interactive despite most of each column being empty. Make the whole panel's interaction explicit through a consistent hover/focus rule and cursor treatment, or give `ENTER` a stronger block/rule treatment.
- For a surface explicitly allowed the maximum collage level, the page delivers the high-fashion half but none of the controlled disruptive event: no red, marker, misregistration, tear, or registration detail. One functional red or misregistered cue around "keys" or the hovered role would express the full thesis without cluttering the grid.

## 02 — Owner dashboard

### What is working

- The information sequence is excellent: portfolio question → Superhost → property health → red exception → upcoming work → money/documents → operating standard. It follows the order of an owner's concerns.
- "Is everything fine?" is much better than a generic "Overview." The `2 MANAGED PROPERTIES` note provides a factual counterweight to the editorial question.
- The property cards are well built. Real images, readiness checks, next-order information, and strong property names are separated by rules rather than ornamental containers. They feel operational and tangible.
- The red `NEEDS YOUR ATTENTION` band is exactly where the scream belongs. Because it is reserved for the single exception, it carries real urgency.
- The black Superhost block reads as the governed machine-on-paper exception described in the art direction, not as a floating chat widget.
- Numbered section eyebrows provide useful wayfinding down a long page without adding more chrome.

### What is weak or inconsistent

- The terminal's default language—`NOT CONNECTED` and "choose a single property…"—makes the flagship capability look broken before it looks useful. There is no selector in the terminal itself. Present a deliberate dormant state such as `SELECT A PROPERTY TO OPEN THE THREAD`, with the selector/action inside the black component.
- The `THIS MONTH` / `DOCUMENTS` split creates the page's biggest visual defect. Twelve document rows force the narrow right rail to become very tall while the left two-thirds contains one centered sentence and acres of blank paper. The resulting 4,048px page feels unfinished rather than editorial. Decouple the column heights, cap the empty statement area, and show a short recent-document list with `VIEW ALL`.
- The narrow document rail makes titles wrap into dense three- and four-line blocks while their timestamps become nearly illegible. It is too much inventory for that width.
- The red attention band promises an actionable exception, but the following incident row is extremely compressed and offers no prominent next action. Worse, "Demo richness · Assess a reported balcony door latch issue" exposes internal demo language. Replace it with user-facing incident context and a clear `REVIEW INCIDENT →` action.
- `PROPERTY DETAILS` and `VIEW PACKAGE` are quieter than the status metadata around them. The screen communicates state well but does not make the next click equally clear.
- The lower governance section is strong, but the oversized monthly void pushes it so far down that many users will never encounter it. This is an information-layout problem, not merely a preference about scrolling.

## 03 — Property detail

### What is working

- This is one of the most resolved screens. The wide property image, enormous serif name, full Lucknow address, and separate lifecycle-state rail establish identity and status immediately.
- Readiness is especially legible: three generous rows, small green markers, plain-language labels, and right-aligned states. It stays restrained and workmanlike.
- Property facts and emergency contact form a sensible evidence rail while compliance and lifecycle history occupy the wide reading column.
- "No compliance holds recorded." is a good empty state: specific, calm, and written in the house voice rather than saying "No data."
- The lifecycle line and square nodes create a print-like audit trail without relying on cards or icons.

### What is weak or inconsistent

- `PACKAGE` is marked as the active global-nav item on a property-detail page. That is a concrete orientation bug: the page locator says `03 / PROPERTY DETAIL` while the primary navigation tells the user they are in Package.
- Section numbering reads out of sequence in the two-column layout: `02` beside `04`, then `03` beside `06`, then `05`. If the numbers are wayfinding, make them follow the visible row-major reading order or explicitly scope numbering by column.
- `ACCESS METHOD — NOT RECORDED` is the most consequential missing fact in the side rail but offers no way to resolve it. Add a small `RECORD ACCESS METHOD →` action so the state reads as operational rather than terminal.
- All five lifecycle entries share the same timestamp, actor, and "Phase 2 demo scenario activation" explanation. Even if this is technically accurate fixture data, it visibly reads as seeded content and damages the credibility of an otherwise convincing record. Use plausible staged timestamps and event-specific descriptions in investor-facing data.
- Actor IDs and timestamps are so pale and small that the audit trail loses some of the evidentiary value the mono treatment is supposed to provide.
- After Emergency Contacts ends, the right column remains empty for most of the lifecycle timeline. The persistent vertical rule makes that void look like a missing panel. Let the timeline span the full width below the facts/contact row, or rebalance the side rail with related property evidence.

## 04 — Package shop / builder

### What is working

- The three-part working layout is excellent: filter rail, ruled product inventory, persistent cart. Importantly, the filters, `62 ITEMS`, and `02 / YOUR CART` do align to the same top baseline; the grid is doing real organizational work.
- Isolated product photography creates enough visual event without breaking the grid. Category, SKU, quantity, and price are correctly assigned to mono, while bold product names remain the scan anchor.
- The fixed cart is spacious, and `empty. *for now.*` is probably the cleanest direct expression of the written art direction in the set.
- Filters are coherent: checkbox counts align, the range is quantified, and category/label totals map convincingly to a real catalog.

### What is weak or inconsistent

- The core action is invisible. Product cells contain no `ADD`, plus control, quantity affordance, or selected state. A user can admire the catalog but cannot confidently infer how to build a package. Put a compact `ADD +` in a consistent bottom-right product-cell zone, with visible keyboard focus; use the single red marker/rule for the most recently added item.
- The price-range control looks like two unrelated native sliders stacked together. Draw it as one intentional range track with two handles and explicit current minimum/maximum values.
- At this width, the inventory is compressed between the 209px filter rail and a roughly 440px empty cart. Product titles wrap heavily while a third of the viewport carries no cart content. Let the empty cart use a narrower summary width and expand after the first item is added.
- The surface is assigned a high collage level but is almost entirely immaculate catalog grid. The cut-out product imagery helps, but one functional annotation—marking an active filter or newest cart addition—would add the intended print event while reinforcing interaction.
- The top-right utility is crowded immediately beside or beneath Superhost. The header does not reserve a clean track for both.

## 05 — Documents

### What is working

- This is a clean, honest pre-selection state. The bordered property selector reads as a control, and the serif "Select a property." message is restrained and appropriate.
- `NAME · TYPE · STATUS · UPLOADED` previews the eventual information model without fabricating document rows.
- The large `DOCUMENTS` shout and fine eyebrow form a strong entry point, while the ruled filter band makes the property scope clear.
- Beyond the issues below, this page is genuinely sound for an empty state. It does not need decoration merely to fill the space.

### What is weak or inconsistent

- "Select a property" is communicated three times: at the hero's right edge, inside the selector, and again in the central empty state. The repetition feels placeholder-like. Keep the selector label and one concise empty-state explanation; reserve the hero's right slot for a post-selection count/status or leave it intentionally blank.
- The selector occupies exactly half the ruled band and leaves an unexplained empty cell on the right. With no second control hinted there, it looks as if half a form failed to render. Span the selector across the useful width or give the second cell a defined purpose.
- As an Owner surface, the huge Archivo Black title is a noticeable voice change from the serif Dashboard and Property Detail titles. It still belongs to the product, but the semantic rule for serif versus shout titles is not obvious. A consistent rule—serif for entity/editorial pages, shout for work queues and indexes—would remove the ambiguity.

## 06 — Staff jobs / ticket queue

### What is working

- The ops treatment correctly turns collage down. Full-width rules, clear filters, a black table header, and mono status/ID fields prioritize work over personality.
- The table structure is excellent: job type, property, requested window, status, assignee, age, then a consistent row arrow. Staff can compare tickets horizontally without opening each one.
- Restrained colored status squares work better than colored pills here and preserve red's authority.
- "WORK ARRIVED. WHO IS DOING IT?" provides concise wayfinding, and `10 TOTAL` gives the huge title a useful operational counterweight.

### What is weak or inconsistent

- The primary work does not appear soon enough. Header, hero, filters, and a tall disconnected terminal consume almost the entire first 900px, so someone arriving to triage sees essentially no job data above the fold. Keep the required terminal on the page, but collapse its dormant state to roughly 120–160px, move it below the first queue rows, or make it explicitly expandable.
- Again, `NOT CONNECTED` is the most prominent machine status and makes the feature look unavailable. Because the actual dependency is property scope, place a selector or `SCOPE THREAD` action inside the terminal.
- Ten rows at this height make the queue needlessly long for an operations surface. Tighten vertical padding while preserving practical pointer/touch targets; the two-line type/ID cell can remain readable at a materially denser rhythm.
- Ticket IDs are too light to function as useful identifiers. They can remain secondary, but should meet the same readable ink-60 standard as other metadata.
- `NEW TICKET` and `ACCESS DESK` sit in a sparse second navigation row that reads like accidental flex wrapping, not an intentional sub-navigation band.

## 07 — Ops calendar

This review accepts that a separate calendar rebuild is already underway. The criticism here concerns the current hierarchy and dependency states, not the absence of a conventional calendar grid.

### What is working

- The workflow is clearly decomposed into `01 FEED HEALTH`, `02 RESERVATIONS`, and `03 TURNOVER PROPOSALS`. Numbering, rules, and right-edge state labels preserve the ops grammar.
- One property selector at the top correctly establishes the scope for everything below.
- The explanatory lines distinguish what each dependent section will contain, which is useful in principle.

### What is weak or inconsistent

- The same giant `SELECT A PROPERTY.` shout appears three times after the user has already encountered the selector. Repeating it in roughly 300px-tall panels makes the 1,639px page feel like three copies of a placeholder. Keep one unified empty state below the selector; let each section use a compact one-line `AWAITING PROPERTY SCOPE` state.
- `CALENDAR`, the three section names, and the three empty prompts all use heavy Archivo Black at competing sizes. There is no quiet level in the hierarchy. Keep page/section shouts, but use the serif empty-state voice or small mono dependency text inside sections.
- `GENERATE TURNOVER PROPOSALS` is a pale salmon block while unavailable. It reads simultaneously as disabled and as the only accent CTA. When unavailable, make it a neutral outlined control with an explicit dependency; once enabled, use the true red token rather than an off-token pinkish red.
- The same wrapped second-row header issue from Ticket Queue appears here, with `ACCESS DESK` stranded below the main navigation.

## 08 — Expansion / pitch, first slide

### What is working

- As a first impression, this is polished and confident. "The property is the *proving ground*." is memorable, and the italic phrase carries the thesis rather than serving as decoration.
- The asymmetric composition is disciplined: massive headline left, small explanation block low-right, and a tiny progress rail at the edge. The negative space feels intentional and expensive.
- "The agent company hiding inside property operations" quickly frames the category, while the body explains the loop from approved intent to physical work to returned evidence. It is substantially more specific than generic AI pitch copy.
- `EXPANSION / 00`, `00 / THESIS`, progress marks, and the scroll/swipe instruction convincingly signal a multi-slide narrative without cluttering the first slide.

### What is weak or inconsistent

- The right-hand explanation is quiet enough to be mistaken for a footnote even though it contains the business mechanism. Increase its width or size slightly, or lead it with one short mono proof label, so the eye reliably reaches it after the headline.
- Three names appear immediately—Comfort Curators, Superhost, and "Crew"—without explaining their relationship. "Crew is designed to close the loop" introduces the least established name in the most explanatory paragraph. Either define the architecture in one phrase or avoid introducing Crew until the deck can explain it.
- The frame is so austere that, without one factual token, a first-time investor could read it as a creative-studio manifesto. A small, verified operating fact—only if supported—would ground the thesis without turning slide one into a metrics slide.
- The top-right `ESC…` utility is visibly squeezed or clipped by Superhost. That collision punctures an otherwise exceptionally controlled frame.
- The restraint works, so collage should not be added for its own sake. If a proof token is introduced, that is the appropriate place for one red registration or underline event.

## Cross-page consistency

### What holds together

- The screens feel like the same product at a systems level. Paper/ink, 1px ruled regions, hard corners, mono labels, serif editorial moments, and black shout moments are consistently recognizable.
- Centered header locators, outlined Back controls, numbered eyebrows, and top-right Superhost placement establish a repeatable shell.
- Density changes appropriately by surface: Owner is more editorial, Inventory is catalog-dense, and Ops is restrained. That variation is healthy rather than drift.
- Superhost's placement is highly consistent and gives the capability recognition across the product.

### Where it drifts or breaks

- The header is not robust. The Back control overlaps and clips the beginning of `COMFORT CURATORS` on several screens; Owner routes appear or disappear by page; Property Detail highlights the wrong route; Ops links wrap into an ambiguous second row; and right-side utilities are squeezed by Superhost.
- Although Superhost placement is consistent, the shell does not reserve space for it: it collides with `ESC` on Expansion and crowds the Package utility. Its green-outlined global button also puts phosphor green outside the terminal, despite the art direction explicitly scoping green to `.superhost-terminal`. Either formally amend that rule for the global CTA or render the header button in ink/red; do not leave it as an undocumented exception.
- Spacing rhythm is consistent at section entrances but not in empty states. Documents is calm; Calendar repeats giant empty panels; Dashboard contains a column-height-induced void; Jobs delays the core table with an oversized terminal. These feel less like deliberate surface variants than independent solutions to missing data.
- The typefaces are consistent but their semantics drift. Owner Dashboard and Property Detail use serif display titles; Documents, an Owner utility, switches to an Ops-like shout. Define when a title is editorial/entity-based versus a queue/index and apply that rule consistently.
- Navigation composition changes more than the content model explains. Package becomes a task-focused shell, Property Detail omits several Owner routes, and Documents restores others. Contextual reduction can work, but the invariant destinations and the active-route behavior need to remain predictable.

## Prioritized fixes

### 1. Rebuild the global header into fixed, non-overlapping tracks

Reserve explicit tracks for Back, brand/context, centered page locator, route navigation, utilities, and a fixed-width Superhost control. Do not let nav items wrap. Keep the invariant Owner destinations consistent, correct Property Detail's active state, move `NEW TICKET` and `ACCESS DESK` into an intentional subnav/menu, and guarantee Expansion and Package utilities a slot before Superhost. This is the highest-impact fix because the shell appears in nearly every screenshot and its collisions make an otherwise precise system look brittle.

### 2. Make dormant Superhost states look scoped rather than broken—and stop them hiding the work

Replace `NOT CONNECTED` with an explicit `SELECT PROPERTY / SCOPE THREAD` state, and put the dependency control inside the black component. On Ticket Queue, collapse the dormant terminal so several job rows are visible in the first viewport; expand it only after connection or a deliberate user action. Apply the same state language on Dashboard.

### 3. Remove exposed demo language and repair the login's user-facing credibility

Rewrite Staff and Guest around actual jobs rather than backend architecture; replace Dashboard's "Demo richness" incident copy; and give lifecycle events plausible, event-specific descriptions and timestamps. On Login, keep "Three roles…" wholly on a readable field and make each role panel clearly interactive. These changes directly improve a first-time visitor's or investor's belief that this is a real operating product.

### 4. Expose the Package Builder's core action

Add a consistent `ADD +` and quantity affordance to every product cell, with strong keyboard focus and a restrained selected/just-added state. Replace the stacked native-looking sliders with one deliberate two-handle range control. Allow the empty cart rail to start narrower and expand with content. The current screen looks excellent but does not yet explain its primary interaction.

### 5. Standardize empty-state height and dependency treatment

On Calendar, show the select-property dependency once and reduce each section to a compact awaiting-scope row. On Documents, remove the third duplicate instruction and either span the selector or use the second cell deliberately. On Dashboard, decouple or cap the `THIS MONTH` and Documents columns and show only recent documents. This removes the largest areas that currently look unfinished while preserving the product's intentional negative space.
