# Comfort Curators App — Frontend Architecture

**Backend:** `comfort-curators-backend-alt`, Go, `http://127.0.0.1:8080`.
340 routes. Verified running 2026-08-08.
**Status:** design ready to implement. Nothing built yet.

---

## 1. The one hard blocker, and the fix

**The backend sends no CORS headers, and its `OPTIONS` preflight returns 401.**
Verified live:

```
$ curl -i -X OPTIONS http://127.0.0.1:8080/v1/properties -H "Origin: http://localhost:3000" ...
HTTP/1.1 401 Unauthorized
$ curl -D- http://127.0.0.1:8080/health/ready -H "Origin: http://localhost:3000" | grep -i access-control
(nothing)
```

`RequireAuthByDefault` does not exempt `OPTIONS`, so even adding CORS headers
naively would still fail preflight. A browser app on another port cannot call
`:8080`.

**Fix: same-origin dev proxy. Zero backend changes.** The backend is halted; we
honour that. Vite's dev proxy makes the browser believe the API is same-origin,
so no preflight is ever issued.

```ts
// vite.config.ts
export default defineConfig({
  server: {
    port: 3000,                 // MUST stay 3000 — the iCal feed URL depends on it (§6)
    strictPort: true,
    host: true,                 // REQUIRED — Vite binds localhost only; the backend
                                // CONTAINER must reach us. Verified 2026-08-08.
    allowedHosts: ["host.docker.internal", "localhost"],
                                // Vite 403s unknown Host headers (DNS-rebinding
                                // protection). The backend polls us as
                                // host.docker.internal, so it must be allowed.
    proxy: {
      "/api": {
        target: "http://127.0.0.1:8080",
        changeOrigin: true,
        rewrite: (p) => p.replace(/^\/api/, ""),
      },
    },
  },
});
```

The app calls `/api/v1/properties`. Same origin, no CORS, no preflight.
Use the same proxy block in `vite preview` for a built demo.

**Production path (post-halt, ~20 lines):** a CORS middleware applied *outermost*
in `internal/platform/app/app.go` — outside `RequireAuthByDefault` — that
short-circuits `OPTIONS` with a `204` before the auth gate runs. Order matters;
inside the gate it will 401 forever. Do not do this before the demo.

## 2. Stack

| Concern | Choice | Why |
|---|---|---|
| Build / dev | **Vite 8** | Instant HMR. No SSR, so no hydration class of bug — which matters because GSAP, Lenis and a custom cursor are exactly what breaks under SSR. |
| UI | **React 19 + TypeScript** | — |
| Routing | **React Router 7** (declarative) | Four portals as nested route trees. Well-represented in model training data; App Router idioms are not. |
| Server state | **TanStack Query 5** | Everything here is server state. Caching, refetch, mutation invalidation for free. |
| Styling | **Tailwind 4** | — |
| Accessible primitives | **Radix UI**, unstyled | Only where a11y is genuinely hard: Dialog, Popover, Select, Tooltip, Checkbox, Switch. See §2.1. |
| Drag & drop | **dnd-kit** | Accessible, pointer + touch, keyboard fallback |
| Motion | **GSAP + Lenis + SplitType** | Reverse-engineered from the references — `INTERACTION.md §2` |
| Validation | **Zod**, boundary only, ~10 shapes | Contract covers 65 of 340 routes, so responses are unverified by definition |
| Toasts | **sonner** | Standalone, no shadcn dependency |
| Icons | **lucide-react** | |
| Client state | React state + Context (cart) + URL (filters) | No Redux, no Zustand. There is very little client state. |
| Auth | Bearer token in memory + `sessionStorage` | §5 — and why the earlier httpOnly design was over-engineering |

### 2.1 Why not shadcn/ui

shadcn is Radix plus an opinionated Tailwind skin — and that skin is **rounded
corners, soft shadows, muted greys**: precisely the friendly-SaaS language
`ART-DIRECTION.md §4` forbids. You would spend the build fighting its defaults,
and an agent will keep drifting back to them.

What we actually need a library for is **accessibility**, not looks: focus traps,
escape handling, scroll lock, keyboard navigation, collision-aware positioning.
That is Radix, and Radix ships unstyled.

```bash
npm i @radix-ui/react-dialog @radix-ui/react-popover @radix-ui/react-select \
      @radix-ui/react-tooltip @radix-ui/react-checkbox @radix-ui/react-switch
```

Everything else in the inventory — card, badge, separator, skeleton, table,
tabs — is a `div` with a 1px black rule under this art direction. Hand-roll them
into `components/ui/`. It is less work than restyling shadcn, and the components
come out matching the design instead of approximating it.

### 2.2 Component inventory

| Component | Built on | Notes |
|---|---|---|
| `<Money value currency />` | — | The **only** place money is formatted. Tabular numerals. |
| `<StatusDot status />` | — | Word + dot from one status→token map. Never colour alone. |
| `<EmptyState title description action />` | — | Every list uses it. Real copy, never "No data". |
| `<PageHeader title subtitle actions />` | — | Mono section index + display title |
| `<Rule />` `<Card />` `<Badge />` `<Skeleton />` `<Table />` `<Tabs />` | — | A `div` with a 1px black rule. Hand-roll; ~15 lines each. |
| `<Button />` | — | Solid black / 1px outline. Two variants, no more. |
| `<Dialog />` `<Sheet />` | Radix Dialog | Focus trap, escape, scroll lock |
| `<Select />` `<Dropdown />` | Radix Select / Popover | Keyboard nav, collision-aware placement |
| `<Tooltip />` | Radix Tooltip | |
| `<Checkbox />` `<Switch />` | Radix | 12px square, 1px rule, solid black when on |
| `<Cursor />` | — | Blend-difference, lerp-followed — `INTERACTION.md §4.1` |
| `<Marquee />` | — | 30s linear infinite — `INTERACTION.md §4.4` |
| `<Reveal />` | GSAP + ScrollTrigger | Clip-path wipe, expo-out — `INTERACTION.md §4.3` |
| `<SplitText />` | SplitType + GSAP | Char stagger `0.02` — `INTERACTION.md §4.8` |
| Shop: `<ItemCard>` `<FilterRail>` `<Cart>` `<CostPanel>` | dnd-kit | `SHOP.md` |

**No OpenAPI codegen.** The contract documents 65 of 340 routes; generated
clients would cover 19% of what we need and silently miss the rest. Hand-write a
typed client for the ~45 endpoints the demo touches. Shapes are in
`INTEGRATION.md`, all verified against the running backend.

## 3. Structure

```
index.html
vite.config.ts
public/
  demo.ics                  # the iCal feed the BACKEND polls — SETUP.md §6
src/
  main.tsx                  # Query + Router + Lenis providers
  routes.tsx                # route tree: owner / ops / curator
  styles/
    tokens.css              # ART-DIRECTION.md §3
    grain.css               # ART-DIRECTION.md §7
  routes/
    owner/    dashboard.tsx  onboarding.tsx  shop.tsx  invoices.tsx  documents.tsx
    ops/      properties.tsx tickets.tsx  ticket-detail.tsx  workers.tsx  calendar.tsx
    curator/  jobs.tsx  job-detail.tsx
    login.tsx  debug.tsx
  lib/
    api/
      client.ts             # fetch wrapper: base path, auth header, envelope, errors
      types.ts              # hand-written, INTEGRATION.md is the source
      catalog.ts  properties.ts  packages.ts  tickets.ts  dispatch.ts
      onboarding.ts  workers.ts  billing.ts  superhost.ts
    auth/session.ts         # token store + role switcher
    money.ts                # formatMoney — the ONLY money formatter
    motion/  lenis.ts  cursor.ts  reveal.ts
  components/
    shop/                   # palette, grid, cart, cost panel
    ui/                     # hand-rolled primitives + Radix wrappers
scripts/
  seed.ts                   # idempotent demo data — INTEGRATION.md §11
```

## 4. API client

Every write returns the envelope `{ id, version, data }`. Every error returns
`{ code, message, request_id }`. Both verified live.

```ts
// src/lib/api/client.ts
import { getToken } from "../auth/session";

export type Envelope<T> = { id: string; version: number; data: T };

// Explicit fields, NOT TS parameter properties: the Vite react-ts template
// enables `erasableSyntaxOnly`, which rejects `constructor(readonly x: T)`.
export class ApiError extends Error {
  status: number; code: string; requestId: string;
  constructor(status: number, code: string, message: string, requestId: string) {
    super(message);
    this.name = "ApiError";
    this.status = status; this.code = code; this.requestId = requestId;
  }
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const token = getToken();
  const res = await fetch(`/api${path}`, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...init.headers,
    },
  });
  const body = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new ApiError(res.status, body.code ?? "UNKNOWN",
                       body.message ?? res.statusText, body.request_id ?? "");
  }
  return body as T;
}

/** Writes return {id,version,data}; most reads return the payload directly. */
export const unwrap = <T,>(e: Envelope<T>): T => e.data;
```

**Error handling that matters for the demo:** a `VALIDATION_ERROR` names the
exact field (`invalid substitution policy: "allow_equivalent"`). Surface
`message` directly in a toast — it is genuinely useful text, not a stack trace.

## 5. Auth

There is no signup. `POST /auth/session/create` mints a session for any
tenant/contact/role **with no OTP and no credential check**, creating the user on
first call.

```ts
// src/lib/auth/session.ts
const TENANT = import.meta.env.VITE_DEMO_TENANT_ID;
let token: string | null = sessionStorage.getItem("cc_session");

export const getToken = () => token;

export async function signIn(role: "owner" | "staff" | "guest", contact: string) {
  const r = await fetch("/api/auth/session/create", {
    method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ tenant_id: TENANT, contact, roles: [role] }),
  });
  if (!r.ok) throw new Error("sign-in failed");
  const { session_token } = await r.json();
  token = session_token;
  sessionStorage.setItem("cc_session", session_token);
}

export function signOut() { token = null; sessionStorage.removeItem("cc_session"); }
```

> **On the earlier httpOnly-cookie design:** it was over-engineering, and dropping
> it is deliberate. Protecting a token whose issuing endpoint hands out any role
> to anyone with no credential is theatre — it added a server hop for no real
> security. Post-demo the fix is real OTP auth plus backend CORS, not cookie
> flags. Recorded in `logs/DECISIONS.md`.

Roles accepted by the backend: `owner`, `guest`, `staff`, `superhost`.

> **Every demo actor must share one `tenant_id`.** Tenant isolation is real and
> enforced from the session subject — actors in different tenants see nothing of
> each other's data, which looks exactly like a broken app.

> **`CC_BUILD_TAGS=acceptance` is required when building the backend**, or
> `/auth/session/create` is not compiled in and there is no way to log in.

## 6. Role reality — design around this

The backend has **three** roles: `owner`, `guest`, `staff`. The product defines
six. Curator, Operations supervisor, Vendor, and HR provider all collapse into
`staff`.

**Consequence for the frontend:** portal separation is a *UI* concern, not an
enforced one. The Curator app and the Ops console both authenticate as `staff`
and the backend will authorize both identically. Build the portals as separate
route groups with separate navigation, and do not claim they are access-isolated.

Do not build the HR portal. It cannot be secured today.

## 7. Data shape notes

- **IDs are prefixed strings**, not UUIDs: `prop_f13af52428579ba3`,
  `cit_aba44b275b239cca`, `pkg_8447af804d935ebb`, `tkt_c600ad8132664dee`.
  Type them as branded strings; never assume UUID format.
- **Money is minor units + currency**: `owner_price_minor_units: 45000` with
  `"INR"` = ₹450.00. Write one `formatMoney(minor, currency)` helper and use it
  everywhere. Never do float maths on these.
- **Empty collections return `200` with an empty array**, not `404`.
- **Optimistic concurrency**: writes return an incrementing `version`. Keep it
  and send it back on updates where the endpoint accepts it.

## 8. Seed data

There is **no database seed**. An empty backend renders an empty app.
`scripts/seed.ts` must run before every demo: log in as `staff`, then create the
scenario through the API. The verified call sequence is in `INTEGRATION.md §7`.

Make the seed **idempotent** — it will be run more than once, including once in
a panic five minutes before the demo.

## 9. Build order

| # | Deliverable | Why this order |
|---|---|---|
| 1 | Scaffold + Vite proxy + `api()` + browser session store + role switcher | Nothing else can be tested until a call succeeds |
| 2 | `scripts/seed.ts` | Every screen renders empty without it |
| 3 | **Package builder** | The centrepiece; backend already computes cost server-side |
| 4 | Ops: properties list → detail → tickets → dispatch | The operational spine |
| 5 | Owner dashboard | Highest polish-to-effort ratio |
| 6 | Curator job view | Read-mostly; cut first if time runs out |
| 7 | Superhost panel | Cosmetic without the agent loop; cut second |

## 10. Explicitly out of scope

HR portal (role model blocks it) · guest mini-store (deferred in the V0 freeze) ·
real OTP login · file upload to MinIO (stub the URL) · automatic package→ticket
triggering (the protocol engine does not exist — see PRD §Demo staging).
