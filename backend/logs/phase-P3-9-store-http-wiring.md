# Phase P3.9 — HTTP wiring for procurement/store (orchestrator-identified gap)

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built
Added authenticated guest-facing HTTP endpoints for store catalog search, quote calculation, and human-confirmed order placement. The mock provider is composed through the `StoreProvider` interface, and order responses preserve `is_mock: true` and the `mock_order_` identifier.

## Files added or changed
- `internal/procurement/store/handler.go`
- `internal/procurement/store/handler_test.go`
- `internal/platform/app/app.go`
- `logs/phase-P3-9-store-http-wiring.md`

## Decisions I made
- Used `GET /v1/store/catalog`, `POST /v1/store/quotes`, and `POST /v1/store/orders` to keep the guest store separate from B2B `/v1/procurement/...` routes.
- Required an authenticated tenant subject for all endpoints, required `property_id` for catalog search, and rejected order tenant mismatches before calling the provider.
- Kept provider response shapes direct so mock metadata is not renamed or stripped.
- Registered no store routes as Superhost tools. The requested search of `internal/automation/superhost` found no `store.` package reference; remaining matches are unrelated agent-run store calls.

## What did NOT work
The environment does not provide `rg`, so the boundary check used the repository Grep tool instead. The equivalent search passed.

## Deviations from the plan
None.

## Open questions
The real store adapter and `/stay` frontend integration remain future work.
