# Phase P3.1 addendum — orchestrator review + fixes

- **Date:** 2026-08-09
- **Agent/model:** claude-sonnet-5 (orchestrator, direct edit) + opencode-go/gpt-5.6-luna (independent adversarial review)
- **Status:** complete

## What happened

P3.1 (opencode-go/deepseek-v4-pro) implemented DEF-02/DEF-03 in
`internal/automation/http_provider.go`. Before merge, the orchestrator
reviewed the diff directly and found one bug, then commissioned an
independent adversarial review (different model) of the full diff
including that fix.

## Bugs found and fixed (by the orchestrator, directly)

1. **Invalid JSON from `%q` system-prompt encoding.** The original code
   used `fmt.Sprintf("%q", req.System)` to build the JSON `content`
   field. Go's `%q` can emit escapes (e.g. `\v`) that are valid in Go
   string literals but not valid JSON string escapes — confirmed with a
   standalone repro where `json.Unmarshal` rejected `%q`'s output on a
   string containing a vertical tab. Fixed by switching to
   `json.Marshal(req.System)`.

2. **API key could leak through echoed upstream error bodies.** Flagged
   by the independent review: a misconfigured or malicious upstream
   endpoint that echoes the `Authorization` header back in its error
   body could leak `CC_MODEL_API_KEY` into a returned error, and from
   there into logs or a persisted `agent_runs` error column. Fixed by
   adding `redactKey()`, which strips any literal occurrence of the
   configured API key from both the parsed `chatErrorResponse.Error.Message`
   and the raw response body before either reaches a returned error.

3. **Defaulted model was not reflected in response/pricing metadata.**
   Flagged by the independent review: when `req.Model` was empty, `Call()`
   correctly defaulted the *outgoing* request to `gpt-5.6-luna`, but the
   *returned* `ProviderResponse.Model` and the `usageForTokens(...)`
   pricing lookup still used the original empty `req.Model`, producing
   blank model metadata and always-unknown pricing for defaulted calls.
   Fixed by using the resolved `model` local variable (which carries the
   default) in both places instead of `req.Model`.

## Independent review verdict

ISSUES FOUND (the three above, modulo #1 which the review saw already
fixed in the diff it was given). The review separately confirmed: the
API key is never directly logged; the `Authorization` header is only
sent when a key is present; the `tool_calls` JSON shape round-trips
correctly against the OpenAI-compatible format; and the `superhostTools()`
placement in `app.go` correctly avoids the `automation`↔`superhost`
circular import.

One review finding was **not** treated as P3.1-scope: `runner.go` never
populates `ProviderRequest.System` and never acts on `ProviderResponse.ToolCalls`
(tool-call turns complete with `Output == nil`). This is real, but it's
the tool-loop wiring that P3.3 (runner.go + PolicyEngine integration) is
already scoped to build — P3.1's job was the wire format on the provider
side, not the runner's consumption of it. Flagging here so P3.3's
dispatch prompt references it explicitly rather than rediscovering it.

## Test-failure discrepancy investigated and resolved

The independent review's own `go test ./internal/automation/...` run
(no `-p 1`) showed `TestMigrationBackfillJarvisToSuperhost`,
`TestRestartRecoversExpiredLease`, and `TestCompletePersistsTokenAccounting`
failing, alongside the already-known `internal/automation/superhost`
FK-constraint failures. This looked like a possible regression at first.

Investigated directly:
- Re-running the three disputed tests in isolation with `-p 1` against a
  fresh throwaway Postgres: `TestMigrationBackfillJarvisToSuperhost` and
  `TestCompletePersistsTokenAccounting` **passed** — confirming these were
  the previously-diagnosed cross-package parallelism false-failure pattern
  (packages under `internal/automation/...` racing each other against a
  shared throwaway Postgres when not run with `-p 1`), not a real defect.
- `TestRestartRecoversExpiredLease` genuinely failed even in isolation
  (`expected 1 recovered lease, got 0`). Checked out this test at commit
  `dee72bd` (rename: Housemaster -> Jarvis), the last commit before any
  orchestration dispatch work began this session, in a separate worktree,
  and reproduced the identical failure there. **Confirmed pre-existing,
  unrelated to P3.1 or any block this session** — not fixed as part of
  this block; noted here for whichever future block owns `store.go`'s
  lease-recovery path.

## Files added or changed (this addendum, on top of P3.1's own commit)

- `internal/automation/http_provider.go` — `%q`→`json.Marshal` fix,
  `redactKey()` + its two call sites, `model` (resolved) used instead of
  `req.Model` for response/pricing metadata.

## Open questions

- `TestRestartRecoversExpiredLease` is a real, pre-existing, reproducible
  failure unrelated to this session's work. Not triaged further here —
  flagging for whichever block next touches `internal/automation/store.go`.
- Per-tool JSON argument schemas (flagged by P3.1 itself) still don't
  exist — carried forward, not this block's scope.
