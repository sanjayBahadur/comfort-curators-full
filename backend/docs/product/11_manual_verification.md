# Manual Verification Handoff

Do not begin this checklist until the harness state is `MANUAL_INSPECTION`, the sealed L4
certificate verifies, and `./cc app` starts the exact locked images without rebuilding.

## Owner API flow

Using the repository's localhost API client and synthetic data, onboard an owner and property,
resolve a remediation item, accept a quote and contract, approve one repair expense, review an
incident, inspect supporting evidence, and read the monthly report. Confirm that routine internal
noise is absent from the owner exception response. V0 is backend-only, so this verifies API behavior,
not a user interface that the package does not deliver.

## Curator API and offline-sync flow

Using the localhost API client, accept a turnover, retrieve route and access instructions, advance
the checklist, upload synthetic evidence, record stock and expense, queue a local draft, replay it,
resolve a version conflict, and complete the job. Confirm idempotency and visible conflict behavior.

An Android device is optional and is not a release requirement for this backend-only deliverable.
If a separate test client is later supplied, connect it over USB with
`adb reverse tcp:8080 tcp:8080`; do not change the API's loopback binding or expose a LAN port.

## Recovery flow

Restore a disposable backup and object set using the runbook. Start the recovered stack, run the
smoke workflow, verify audit continuity and object ownership, and record recovery time plus
deviations.

## Decision

Record pass, fail, evidence, operator, timestamp, and follow-up task for each flow. A failure returns
the affected section to development. Manual approval does not waive a failed automated gate. This
handoff approves a localhost release candidate only; production deployment remains a separate human
decision.

