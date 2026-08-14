# Security and Privacy Baseline

## Threat priorities

The highest-impact risks are cross-tenant access, property access-secret exposure, unauthorized financial action, forged or deleted evidence, malicious webhook replay, document leakage, worker over-privilege, prompt injection through external content, and untraceable model actions.

## Mandatory controls

- Deny by default. Enforce role, tenant, property, assignment, state, time window, risk, and monetary threshold.
- Hash one-time login tokens; expire and consume them atomically. Staff authentication is MFA-ready.
- Store only secure session cookies or appropriately protected bearer credentials. Apply rotation, revocation, CSRF, origin, and rate controls appropriate to the client.
- Encrypt access codes and high-sensitivity document fields separately. Use narrow decryption services, purpose checks, short-lived disclosure, and audit.
- Use private object storage, content-type checks, size limits, malware scanning, immutable hashes, signed short-lived access, and tenant-prefixed ownership validation.
- Verify webhook signatures, timestamps, source identity, replay windows, and idempotency before processing.
- Redact credentials, tokens, access material, identity documents, payment data, and message contents from logs and model prompts.
- Treat document text, messages, reservation notes, and model output as untrusted input. They cannot rewrite instructions, tool policy, or authorization.
- Require maker-checker separation for bank detail changes, large credits, sensitive access overrides, and other declared high-risk operations.
- Preserve audit records and evidence under explicit retention rules. Operational APIs do not hard-delete them.

## Development data

Use synthetic fixtures. Production exports are not permitted in the development harness. Secrets remain in local untracked environment or provider authentication stores. `.env.example` contains only local throwaway values.

## Security evidence

The L3 gate includes static analysis, dependency and container scanning, secret scanning, authorization/property tests, webhook replay tests, object ownership tests, prompt-injection boundary tests, and policy-invariant tests. Findings are triaged with a documented severity and disposition. A high or critical unresolved finding blocks readiness.

