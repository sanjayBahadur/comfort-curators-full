# Demo Runbook

Verified working 2026-08-08. Do not refactor before the demo.

## Boot

```bash
cd ~/open-code-projects/comfort-curators-backend-alt
export CC_BUILD_TAGS=acceptance     # REQUIRED — enables the no-OTP login route
docker compose up -d --build api worker postgres minio model-stub
curl -s http://127.0.0.1:8080/health/ready
# {"status":"ok","checks":{"database":"ok","minio":"ok","model":"ok"}}
```

`CC_BUILD_TAGS=acceptance` compiles in `POST /auth/session/create`. Without it
that route does not exist and the only way in is the OTP flow, which needs a
real delivery channel. **A plain `docker compose up` will leave you unable to
log in.**

## Log in (no OTP, any role)

```bash
TOKEN=$(curl -s -X POST http://127.0.0.1:8080/auth/session/create \
  -H 'Content-Type: application/json' \
  -d '{"tenant_id":"11111111-1111-4111-8111-111111111111",
       "contact":"owner@demo.test","roles":["owner"]}' \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["session_token"])')

curl -H "Authorization: Bearer $TOKEN" http://127.0.0.1:8080/v1/properties
```

Valid roles: `owner`, `guest`, `staff`, `jarvis`. The user is created on first
login — there is no user seeding step. Use the same `tenant_id` for every actor
in the demo or they will not see each other's data (tenant isolation is real
and enforced from the session subject).

## Facts that matter

- **340 routes exist; the OpenAPI contract documents 65.** Use
  `docs/development/ROUTE_INVENTORY.md` — it lists all 340 with the handler
  file for each. For a payload shape, open the handler and read the request
  struct. Do not trust `contracts/api/openapi.yaml` for coverage.
- **There is no database seed.** Demo data must be created through the API.
  Write a seed script that logs in as staff and POSTs the demo scenario.
- Reads on an empty system return `200` with an empty list, not `404`.
- Auth is default-deny. Everything except `/health/live`, `/health/ready`,
  `/auth/otp/request`, `/auth/otp/verify`, `/auth/session/create`, and
  `/v1/communications/secure-links/redeem` requires a session.

## Teardown

```bash
docker compose down          # keep volumes
docker compose down -v       # wipe the database too
```
