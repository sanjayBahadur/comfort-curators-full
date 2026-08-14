# Acceptance Fixture Protocol

Protected phase gates exercise the running Docker stack with synthetic data. The implementation must provide a Compose service named `acceptance-seed` that is unavailable in normal production profiles.

Running:

```bash
docker compose --profile acceptance run --rm acceptance-seed
```

must create two tenants, two owners, one operations supervisor, two Curators, one independent vendor, two properties, active and expired support grants, calendars, contracts, package data, stock locations, and deterministic credentials. It writes only this generated file:

```text
.runtime/acceptance-fixture.json
```

Required JSON keys:

```text
base_url
tenant_a.id
tenant_b.id
owner_a.token
owner_b.token
supervisor_a.token
curator_a.token
curator_b.token
vendor_a.token
property_a.id
property_b.id
active_support.token
expired_support.token
```

Rules:

- Fixtures are synthetic and deterministic for a clean database.
- The service refuses to run unless `CC_ENV=acceptance`.
- Fixture credentials are short-lived and never valid outside the acceptance stack.
- Tests call the real HTTP API and inspect PostgreSQL where database truth matters.
- Model scenarios use a deterministic local provider stub, including success, timeout, malformed output, duplicate delivery, prompt injection, and unavailable-provider modes.
- A gate always destroys its disposable volumes and repeats once from a new empty database.
- Test output is JUnit XML plus machine-readable evidence containing command, revision, timestamps, and observed result. Agents cannot satisfy a gate by creating the evidence file directly.


