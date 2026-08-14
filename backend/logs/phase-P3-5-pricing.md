# Phase P3.5 — pricing.go DeepSeek + Luna entries (DEF-05)

- **Date:** 2026-08-09
- **Agent/model:** opencode-go/deepseek-v4-flash
- **Status:** complete

## What I built
Added three rows to `internal/automation/pricing.go`'s `modelPricing` table:
`openai/gpt-5.6-luna`, `deepseek/deepseek-v4-flash`, and
`deepseek/deepseek-v4-pro`, matching the existing per-1K-token minor-unit
convention. Verified arithmetic against the `openai/gpt-4o` reference row
($2.50/1M → 2500 minor/1K): minor per 1K = (price per 1M / 1000) × 1e6.

Added `TestPricingLunaAndDeepSeekRowsKnown` in `usage_test.go`, which calls
`usageForTokens(provider, model, 1000, 1000)` for all three new pairs and
asserts `known=true`, currency `USD`, and the exact computed minor-unit cost.

## Files added or changed
- `internal/automation/pricing.go` — three new `modelPricing` rows + comments.
- `internal/automation/usage_test.go` — new test for the three new rows.

## Decisions I made
- Key format: confirmed the real convention matches the map keys shown. The
  only call site is `http_provider.go:217`, which calls
  `usageForTokens(req.Provider, model, ...)`; `priceForModel` joins them as
  `provider + "/" + model` (`pricing.go:33`). Provider and model are used
  verbatim (lowercase, slash-separated), so `"openai/gpt-5.6-luna"`,
  `"deepseek/deepseek-v4-flash"`, `"deepseek/deepseek-v4-pro"` are exactly
  right; no key-format change needed.
- Luna is the runtime default model (`http_provider.go:130` defaults an empty
  model to `gpt-5.6-luna`), so its row also fixes pricing for defaulted runs.
- Did NOT add a cache-tier concept for DeepSeek Flash's $0.0028/1M cache-hit
  input tier — the table has no such concept and the task forbids inventing
  one. Billing at the miss rate (input $0.14/1M) under-reports cost, which is
  the safe direction; `usage_known` stays honest either way.
- Kept the round-half-up math in `usageForTokens` untouched; only table rows
  were added.

## What did NOT work
Nothing — `go build ./...`, `go vet ./...`, and
`go test -p 1 ./internal/automation/...` all pass.

## Deviations from the plan
None. `http_provider.go`, `runner.go`, and `contracts/` were not touched.

## Open questions
- None.
