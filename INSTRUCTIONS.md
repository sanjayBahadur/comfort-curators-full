# Comfort Curators — Final Demo

This folder is a **self-contained copy** of both real repos, frozen at a
verified working state as of 2026-08-13. It does not depend on anything
outside this folder except Docker and Node.js — no other Comfort Curators
checkout on this machine needs to exist or be running for this to work.

```
final-demo/
  backend/     the Go API + worker + Postgres/MinIO stack (docker compose)
  frontend/    the React/Vite app (Superhost, dashboard, shop, invoices...)
  INSTRUCTIONS.md   this file
  EXCLUDED.md       exactly what was left out of this copy, and why
```

This whole sequence was smoke-tested end to end right before this copy was
handed to you: fresh containers, fresh database, real migrations, real
seed data, a real headless-Chrome login → dashboard walkthrough, zero
console errors. Follow it in order and it will match.

## 0. Requirements

- Docker + Docker Compose
- Node.js 20+ and npm
- Nothing else — no other local Postgres/MinIO, no global npm packages

## 1. Start the backend

```bash
cd backend
docker compose up -d --build
```

This builds and starts five containers: `postgres`, `minio`, `model-stub`,
`api`, `worker`. Migrations apply automatically on first boot — there is no
separate migrate step. Give it about 15–20 seconds, then confirm:

```bash
curl http://localhost:8080/health/live
# -> 200
```

`backend/.env` already carries the real DeepSeek key, so Superhost answers
with real reasoning out of the box, not the offline stub. **This file has a
live API key in it — keep this folder off any public git remote, or at
minimum keep `.env` out of it if you do push it somewhere.**

## 2. Start the frontend

```bash
cd frontend/app
npm install
npm run dev
```

Open **http://localhost:3000**. `app/.env.local` already carries the demo
tenant id the frontend needs to talk to the backend — nothing else to
configure.

## 3. Seed demo data

With both of the above running:

```bash
cd frontend/app
npm run seed
```

This populates 2 properties (Gomti Riverside 2BHK, Hazratganj Studio), the
62-item catalog, 3 workers, 9 tickets, 17 reservations, 12 documents, and
2 active packages — everything the demo screens expect to see. It's
idempotent: safe to re-run if you ever want a clean reset (stop the stack,
`docker compose down -v`, `docker compose up -d --build`, reseed).

## 4. Run the demo

Go to **http://localhost:3000/login** and pick a role card — no password,
this is a demo build. Suggested walkthrough:

1. **Owner** → `/dashboard` — real metrics, live map, mini calendar,
   property carousel, the task terminal (tickets awaiting approval, plus
   any pending purchase built live below).
2. **Owner** → `/properties/:id/package` — drag/click items into a cart.
   Superhost (bottom-right, "HAND OVER CONTROL") can now build this cart
   for you live if you ask it to — that's a real, working `ui_click` path,
   not scripted.
3. Back to **`/dashboard`** — the cart you just built shows up as
   **PENDING PURCHASE · NOT YET ACTIVATED** in the task terminal, live,
   before you ever pay. Activating the package clears it.
4. **`/invoices`** — pick a property. Note: this demo's seed data has
   **zero recorded charges on any property**, so every property currently
   shows the honest "No charges yet" empty state, not the new statement
   layout. If you want the populated invoice-statement look for the demo,
   ask for a charge to be seeded before showtime — it's a small, separate
   addition, deliberately not done here since it would mean inventing
   numbers that don't correspond to anything real.
5. **Staff** / **Guest** role cards — the other two demo tracks (dispatch,
   guest stay + ordering).

Superhost, everywhere it's mounted: try asking it to do something it has
a real tool for — "the AC in Hazratganj needs a look," "add a coffee
restock to the package" — it proposes/acts for real, not a canned reply.
Asking it to pay or check out will surface the one deliberate refusal —
that boundary is enforced in code, not just in the prompt.

## Stopping / restarting

```bash
cd backend && docker compose stop     # pause, keeps all data
cd backend && docker compose up -d    # resume where you left off
cd backend && docker compose down -v  # full reset, wipes the database
```

The frontend is a plain `npm run dev` process — `Ctrl-C` to stop it,
`npm run dev` to resume (no data to lose, it's stateless).

## If something doesn't come up clean

- `docker compose ps` — every service should say `healthy` (worker has no
  healthcheck defined, `Up` is enough for it).
- `docker compose logs worker --since 2m` — look for
  `"superhost system prompt loaded"` and `"agent run provider initialized"`
  near the top; if `model_url` isn't `https://api.deepseek.com`, `.env`
  didn't load — confirm you're running compose from inside `backend/`.
- Port conflicts: this stack wants `3000` (frontend), `8080` (api),
  `5432` (postgres), `9000`/`9001` (minio), `8081` (model-stub) all free
  on `127.0.0.1`. If another Comfort Curators checkout is running
  anywhere on this machine, stop it first (`docker compose stop` in that
  other checkout) — two stacks cannot hold the same ports at once.

See `EXCLUDED.md` for what was deliberately left out of this copy and why.
