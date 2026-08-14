# Phase P3.8 — procurement/store/ (StoreProvider + mock catalog, D3)

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/gpt-5.6-luna
- **Status:** complete

## What I built
Added an application-level `StoreProvider` boundary with `Search`, `Quote`, and `PlaceOrder` operations. Added a deterministic mock provider with a fixed Lucknow-relevant household procurement catalog covering `INSTAMART`, `ZEPTO`, and `BLINKIT`. Catalog prices use `billing.Money` with integer INR minor units. Search, quote IDs, and order IDs are deterministic, and mock orders are explicitly marked with `IsMock: true` and a `mock_order_` ID.

## Files added or changed
- `internal/procurement/store/provider.go`
- `internal/procurement/store/mock.go`
- `internal/procurement/store/provider_test.go`
- `logs/phase-P3-8-procurement-store.md`

## Decisions I made
- Reused the repository's `billing.Money` type rather than introducing another money representation.
- Required tenant and property scope when placing an order, matching the existing tenant/property conventions.
- Kept the package outside the Superhost tool registry. The existing `prohibitedToolNamePrefixes` and `IsToolProhibited` boundary rejects order-authority tool names; this service must be called by a human-confirmed guest-facing application flow, not by the Superhost agent.
- Derived quote and order identifiers from request content using SHA-256, with no randomness or clock-dependent behavior.

## What did NOT work
Nothing. `go build ./...`, `go vet ./...`, and `go test -p 1 ./internal/procurement/...` passed.

## Deviations from the plan
None.

## Open questions
Real courier/store adapters and the guest-facing `/stay` wiring remain future work, as intended for this block.
