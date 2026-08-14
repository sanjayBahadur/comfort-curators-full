# Cross-phase decisions

Append one line per decision that affects more than one phase. Never rewrite.

Format: `DATE | PHASE | decision — reason`

```
2026-08-08 | P0 | Next.js rewrite proxy instead of backend CORS — backend is halted; proxy needs no backend change and avoids preflight entirely
2026-08-08 | P1 | Session token in an httpOnly cookie, not localStorage — keeps it unreachable from page scripts
2026-08-08 | P3 | Package cost always read from the API response — server computes it and is authoritative; never recompute client-side
2026-08-08 | P0 | Use the current create-next-app output (Next.js 16.2.3) instead of pinning Next.js 15 — PHASES.md explicitly requires create-next-app@latest; lint, production build, and rewrite proxy were verified on the generated version
2026-08-08 | P0 | Switched Next.js 16 -> Vite 8 + React Router 7 — app is 100% client-side; user decision after weighing a verified Next Phase 0 against SSR/hydration risk in the motion-heavy Phase 0.5
2026-08-08 | P0 | vite server.host: true — Vite binds localhost only, so the backend container could not fetch /demo.ics; Phase 2's reservation chain would have failed silently
2026-08-08 | P0 | vite server.allowedHosts includes host.docker.internal — Vite 403s unknown Host headers (DNS-rebinding protection)
2026-08-08 | P0 | ApiError uses explicit fields, not TS parameter properties — the Vite react-ts template sets erasableSyntaxOnly
2026-08-08 | P0.5 | Radix primitives, NOT shadcn/ui — shadcn's skin is rounded corners + soft shadows, which ART-DIRECTION forbids; we want its a11y, not its looks
2026-08-08 | P1 | Token in memory + sessionStorage, not httpOnly cookie — no server in a Vite SPA, and the issuing endpoint hands any role to anyone with no credential
2026-08-08 | P0 | OpenCode audits use the read-only `plan` agent with orchestrator verification; direct DeepSeek Flash and Qwen 3.7 Plus are fallbacks for transient OpenCode Go Flash/Kimi tool-loop failures — delegated output is advisory until checked against source and live acceptance
```
2026-08-08 | P0.5 | Global motion uses one cleaned-up Lenis lifecycle plus a square difference-blend cursor, both disabled when accessibility or input mode requires it — later screens inherit consistent motion without violating the zero-radius art direction
2026-08-08 | P1 | The global API client surfaces the backend `message` verbatim through unstyled Sonner and retains `request_id` for diagnostics — every later phase gets useful errors without per-screen paraphrasing
2026-08-08 | P2 | Seed idempotency is check-then-create for every resource, including address-matched properties — the halted backend accepts property `idempotency_key` but does not use it
2026-08-08 | P2 | Demo workers receive availability plus backend skill aliases alongside their human-readable skills — real dispatch rejects otherwise plausible workers without both
2026-08-08 | P2 | Reservation-chain acceptance uses the poll result and persisted proposal collection — the poll itself creates proposals, so a subsequent explicit generation is correctly idempotent
2026-08-08 | P3 | The package shop starts as a fresh empty draft and activates only the server version whose saved signature matches the visible cart and rules — existing active seed packages remain untouched until the owner deliberately replaces one
2026-08-08 | P3 | Touch pointer-down is withheld from PointerSensor so touch gestures use the specified 200ms TouchSensor; mouse and pen keep the 8px PointerSensor threshold
2026-08-08 | P3 | Monthly budget is visibly captured but not submitted or described as enforced — the halted package API has no budget field
2026-08-08 | P2 | Managed properties must be created by the stable demo-owner actor before switching back to staff — property creation grants its supplied owner authority to the creating actor, so staff-created seed properties are invisible to owner sessions
2026-08-08 | P4 | The operations queue fetches each accessible property's tickets and merges them client-side — the halted backend returns an empty unfiltered ticket collection
2026-08-08 | P4 | Dispatch ranking is eligible-first then score-descending in the client — the candidates endpoint returns checks and scores in database order
2026-08-08 | P4 | Current assignees come from the assignment subresource and duplicate offers are disabled — assignment creation does not update the ticket status, assigned_to, or state history
2026-08-08 | P4 | A fresh ticket is explicitly advanced draft → proposed → approved → scheduled before dispatch — each transition remains visible in the real state history
2026-08-08 | P5 | Owner attention uses the reporting exception feed and filters it again to owner-visible property IDs — routine internal workflow stays off the calm exception dashboard
2026-08-08 | P5 | “This month” uses the period-bounded `/v1/reporting/property-contribution` endpoint — `/v1/reports/property-contribution` is an all-time collection and cannot truthfully support a monthly label
2026-08-08 | P5 | Package cost uses only the explicit active version and its server-provided monthly value — a newer draft is not active and client recomputation can drift
2026-08-08 | P5 | Superhost operating standards remain qualitative until real aggregate measurements exist — the halted backend exposes no response, turnover-window, resolution-time, or rework KPI
2026-08-08 | P6 | Onboarding resume is list → owner-property filter → full case + progress GET, with no wizard truth in browser storage — list rows are shallow and generic onboarding reads are only tenant-scoped
2026-08-08 | P6 | Seven owner-facing steps group the backend's 15 fixed checklist keys — server progress remains authoritative without exposing a twenty-field backend model as the navigation
2026-08-08 | P6 | Autonomy persists as `service_preferences.automation_limits` marker `autonomy_level:<value>` — unknown fields are discarded and no backend behavior consumes the marker
2026-08-08 | P6 | Package, contract, onboarding, and property lifecycle stay visibly separate — only an existing reviewed draft may be accepted, and finishing intake does not imply activation or automation
2026-08-08 | P6 | New-property retries reuse an owner-visible normalized address match before POST — the halted property create route has no idempotency behavior
2026-08-08 | P7 | Curator jobs derive from non-declined assignment subresources and per-property ticket queries — the backend has no curator role or tenant-wide ticket list
2026-08-08 | P7 | Completion sends metadata-only SHA-256 evidence and uses the backend evidence gate before `evidence_submitted` — MinIO upload is explicitly out of scope
2026-08-08 | P7 | Seeded tickets receive two idempotent checklist rows when absent, while no-checklist historical jobs stay visibly uncompletable — the curator must never invent operational work
2026-08-08 | P8 | Jarvis is a read-only URL-linked drawer because the backend exposes run GET/events but no run discovery endpoint — the frontend never fabricates or starts agent activity
2026-08-08 | P8 | Seed reset is a visible copy-command control, not a browser mutation — the backend is halted and reseeding belongs to the operator terminal workflow
2026-08-09 | P0.6 | mattpocock/skills evaluated, not installed — its useful skills (`/implement`, `/code-review`) are exactly the work ORCHESTRATION.md §6 forbids the orchestrator from doing, and its implementing-agent-facing skills are Claude-Code-native so the opencode()-driven sandcastle agents could never reach them regardless
2026-08-09 | P0.6 | Build-agent dispatch (both repos) routes through the `opencode-go/*` model prefix, not direct `deepseek/*`/`openai/*` keys — human instruction to rely on the OpenCode Go subscription; verified via `opencode models` CLI output that `gpt-5.6-luna` only exists under that prefix, not the free `opencode/*` tier
2026-08-09 | P0.6 | Sandcastle's "issue tracker" is local (blocks.json + logs/*.md Status field), not GitHub Issues — human chose this over standing up 49 real GitHub issues on a live repo
2026-08-13 | UI 1.3.1 | Owner listing handover is an editorial dossier without an embedded terminal; Airbnb lookup and market outlook remain explicitly labeled demos until a verified connector exists
