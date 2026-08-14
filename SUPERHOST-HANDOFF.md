# Superhost demo handoff

This snapshot was updated on 2026-08-14 to make the owner package-building
demo observable, repeatable, and safe at the commercial boundary. The exact
presenter walkthrough is in `SUPERHOST-DEMO.md`.

## What changed

- Superhost can issue several UI actions in one turn without later actions
  being discarded as `too_fast`.
- Package catalog actions show a cursor/ring and a product-card ghost moving
  into the cart before the registered add control is invoked.
- The global Superhost drawer remains mounted when minimized, so its stream
  and UI driver keep running. The launcher shakes, glows, and emits particles
  while control is active in the background.
- The control lease is a bounded five-minute sliding window. Navigation and
  successful authorized actions preserve it; Escape, the manual strip,
  five idle minutes, the 25-action cap, and the payment boundary still stop it.
- Dashboard-to-package navigation keeps the same property scope instead of
  briefly switching to the portfolio thread.
- Cached thread events are terminal history only. Reopening the drawer cannot
  replay old `ui_click` actions or produce repeated stale `not_granted` lines.
- Package prompts cover a fixed cart plus warm-welcome and business-reset
  variants grounded in the seeded catalog.
- A model-visible `REVIEW & ACTIVATE` tripwire stops control and shows the red
  handoff. The real activation control remains outside the model's registered
  surface and is clicked by the human after handoff.
- Draft carts persist across refresh, create an `UNAPPROVED CART · REVIEW
  REQUIRED` dashboard task, and support review, full approval, or discard.
  Approval/activation removes the task.
- The payment warning is compact and bottom-right so it does not obscure the
  cart-building animation.

## Safety model

The visible checkout control available to Superhost is only a handoff
tripwire. It cannot activate a package or perform payment. The protected
commercial subtree remains absent from AgentSurface; after the tripwire
revokes control, the human-owned action is rendered.

Pending carts are stored in the current browser's local storage for this
demo. They are reliable across refreshes in the same browser but are not a
server-side, cross-device approval record.

## Run the demo

```bash
cd ~/final-demo/backend
docker compose up -d --build

cd ~/final-demo/frontend/app
npm install
npm run dev
```

If starting from an empty database, run `npm run seed` from
`frontend/app`. Open `http://localhost:3000/login`, sign in as Owner, select
Hazratganj Studio, and open its package page. Wait for product cards before
handing over control. Use `BUILD TO CHECKOUT`, press `SEND`, then minimize the
drawer to observe the work.

Fresh package actions should target IDs beginning with
`shop-catalog-add-`. A restored thread may show
`history restored: prior browser actions were not replayed`; this is expected
and confirms historical tool events are inert.

## Verification

```bash
cd ~/final-demo/frontend/app
npm run build
npm run lint
npx vitest run
npx playwright test --list
```

At handoff, the production build passes and Vitest reports 38 passing tests.
Lint completes with existing non-blocking React Fast Refresh/hook warnings.
The browser suite discovers two Chrome tests. A full headed browser run still
requires the local frontend/backend stack because this workspace cannot bind
the required browser and Docker sockets.

## Important files

- `SUPERHOST-DEMO.md` — exact presenter and acceptance sequence.
- `frontend/app/src/components/superhost/` — control session, drawer, cursor,
  driver, payment boundary, and terminal behavior.
- `frontend/app/src/routes/package-shop.tsx` — catalog/cart registration,
  persistence, and checkout handoff.
- `frontend/app/src/components/owner/OwnerTaskTerminal.tsx` — pending-cart
  review, approval, and discard lifecycle.
- `frontend/app/src/lib/superhost-demo-scenarios.ts` — seeded demo prompts.
- `frontend/app/src/__tests__/` — regression coverage for the driver, cart,
  payment boundary, navigation lease, persistence, and scenarios.

Never commit `backend/.env`; it contains a live model-provider credential.
The root `.gitignore` and nested repository ignores deliberately exclude it.
