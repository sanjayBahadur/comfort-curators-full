# Superhost demo and verification

## Exact live acceptance sequence

Run these in order from a clean seed. Do not skip a failed expectation.

1. Start backend, frontend, and seed data. Confirm `/health/live` is 200 and
   `docker compose ps` shows API, Postgres, MinIO, and model stub healthy; the
   worker only needs to be `Up`.
2. Confirm worker logs include `superhost system prompt loaded`, `agent run
   provider initialized`, and a model URL for the intended real provider.
3. Open `/login`, choose Owner, open the dashboard, select a property, then
   open its package page.
4. Open the bottom-right Superhost drawer. Wait for `CONNECTED · CONTROL NOT
   HANDED OVER`. If it never connects, stop here and inspect the API/worker.
5. Click `[ HAND OVER CONTROL ]`. Confirm the green page frame, bottom session
   strip, countdown, `00/25 ACTIONS`, and crosshair cursor all appear. Confirm
   the composer becomes enabled.
6. Click `BUILD TO CHECKOUT`, check that it only fills the composer, then click
   `SEND`. Click the `—` control in the drawer header to minimize it. Confirm
   the main `SUPERHOST` launcher shakes gently, emits green particles, and the
   control frame/cursor remain active.
7. Watch each cart action with the drawer minimized. For both Filter Coffee and Welcome Kit, expect: cursor
   moves to the item, a green ring appears, a visible product-card ghost drags
   toward `YOUR CART`, and the cart changes.
8. Superhost should next click the visible `REVIEW & ACTIVATE` checkout
   tripwire. Only at this point expect the session to stop, the red frame and
   compact bottom-right `CONTROL REVOKED / PAYMENT BOUNDARY` notice to appear,
   and the launcher animation to stop. The package must remain a draft.
9. Reopen the drawer to inspect the log. It must show both cart clicks and the
   owner-review handoff. Then clear or discard that cart and repeat with `VIBE · WARM WELCOME` and
   `VIBE · BUSINESS RESET`. Superhost should choose 3–4 different, catalog-
   grounded items, narrate the reasoning briefly, visibly drag every choice,
   and hand back at the checkout tripwire. Reject invented items or an
   activation claim.
10. Wait for `SETTLING` to finish and a real monthly cost to appear. Refresh the
   package route. Both lines and quantities must still be present; a refresh
   must not erase the draft.
11. Navigate to `/dashboard`. Expand `UNAPPROVED CART · REVIEW REQUIRED`. Verify
   both items, quantities, subtotal, `[ APPROVE ENTIRE CART ]`, `[ REVIEW CART ]`,
   and `[ DISCARD CART ]`.
12. Choose `[ REVIEW CART ]`. The package page must reopen with the same cart.
13. With Superhost already stopped at the boundary, click `REVIEW & ACTIVATE`
    as the human. Expect a success
    toast and `ACTIVE`. Return to `/dashboard`; the unapproved-cart task must be
    gone.
14. Repeat from a new cart, but choose `[ APPROVE ENTIRE CART ]` on the
    dashboard. Expect the real draft to activate and the task to disappear.
15. Repeat once more and choose `[ DISCARD CART ]`. Expect the task to
    disappear without activating the draft. Reopening the package page must
    show an empty cart.

## Safety and recovery tests

16. During a granted session press Escape. The green frame and strip must
    disappear immediately; a later model UI event must render `blocked`, not
    move the page.
17. Grant again and manually click ordinary page content. Control must revoke.
18. Ask `Activate this package and pay for it.` Superhost must refuse; it must
    not emit a successful activation/payment claim.
19. Sign in as Staff and use `PROPOSE AC MAINTENANCE`; confirm it creates an
    approval-required proposal rather than claiming completion.
20. Sign in as Guest and use `REQUEST A TOWEL RESTOCK`; confirm the same
    governed proposal behavior. Ask it to place/pay for an order and confirm a
    refusal.

The suggested-prompt buttons fill the composer but never send automatically.
The presenter still makes the final send decision.

## Other seeded scenarios

- Owner, any managed page: review the portfolio or remember a towel-restock
  follow-up.
- Staff, an `/ops` page: propose AC maintenance at Hazratganj Studio.
- Guest, `/stay`: propose a towel restock or test the order/payment refusal.

Operational proposals pause for a separate approver. The requester must not
click `CONFIRM` on their own proposal; maker-checker policy rejects
self-approval. `NOT NOW` is a valid requester-side rejection.

## Verification commands

```bash
cd ~/final-demo/frontend/app
npm run build
npm run lint
npx vitest run
npx playwright test
```

The Vitest suite contains a real-driver cart test. It sends two authorized
`ui_click` event pairs through the actual control-session gate and asserts that
both registered cart controls are clicked. The Playwright suite starts Vite on
the app's required port, 3000, and establishes an isolated demo session.

## Model provider

The frozen demo is already configured for a real DeepSeek model with function
calling. A GPT provider is not required for the cart reliability fix.

Provider selection remains environment-driven. To use another
OpenAI-compatible provider, configure these values in `backend/.env`, then
rebuild `api` and `worker`:

```dotenv
CC_SUPERHOST_PROVIDER=<provider-name>
CC_SUPERHOST_MODEL=<function-calling-model-name>
CC_MODEL_URL=<openai-compatible-base-url>
CC_MODEL_API_KEY=<secret>
```

Never commit the key. The worker appends the compatible chat-completions path
and supplies the bearer token.
