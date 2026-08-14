# Phase P2.2 — login rework

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built

- Reworked `/login` into the three-column role access design with scoped page CSS.
- Selecting a role calls `signIn(role, contact)`, shows `MINTING…` while pending, and navigates with `navigate(homeFor(role), { replace: true })` after success.
- Failed sign-in requests reset the card through `finally`; `signIn` already surfaces the API message through the global toast.
- Removed the login title's `data-copy` usage and all production-facing debug debris.

## Files added or changed

- `app/src/routes/login.tsx`
- `app/src/routes/login.css`
- `logs/phase-P2-2-login-rework.md`

## Decisions I made

- Added `login.css` with unique `access-*` selectors so the page does not depend on or modify the global login styles and their retained `data-copy` utility.
- Kept the existing `roleOptions` data and contacts, restructuring only how it is presented.
- Did not add a second toast call because `signIn` calls `toast.error` for each API/configuration failure.

## What did NOT work

- No known failures.

## Deviations from the plan

- The dev instruments were cut from `/login` but not moved to `/debug`; that remains P2.5 work. The exact removed logic was:

```tsx
<aside className="login-proof" aria-label="Money formatting proof">
  <span className="login-stamp">NOT A REAL LOGIN</span>
  <p>OPERATING SAMPLE / INR</p>
  <Money value={45000} currency="INR" className="money-proof" />
  <small>Rendered from 45000 minor units</small>
</aside>
```

```tsx
const [activeRole, setActiveRole] = useState<Role | null>(() => getRole());
const [sessionNote, setSessionNote] = useState(() =>
  getToken() ? "Session restored from this tab" : "No active demo session",
);
const [propertyCount, setPropertyCount] = useState<number | null>(null);
const [shopPropertyId, setShopPropertyId] = useState<string | null>(null);
```

The removed session console displayed `activeRole`, `sessionNote`, and `propertyCount`, linked to the owner dashboard, operations desk, and inventory shop, and provided `Clear session`. The removed `forceApiError` function called `api("/v1/properties/not-a-real-property")` after checking `getToken()`, allowing P2.5 to restore these instruments on `/debug`.

## Open questions

- None.
