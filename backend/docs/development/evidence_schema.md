# Release Evidence Schema

Phase 7 must produce three tracked JSON files. Certification rejects missing IDs, unknown test names, duplicate ownership, skipped behaviors, and unresolved high or critical security findings. Test names must exist either as Go test functions in the repository or as protected names in `contracts/acceptance/oracle.yaml`.

## Requirement evidence

Path: `docs/development/requirement-evidence.json`

```json
{
  "schema": "comfort-curators-requirement-evidence/v1",
  "requirements": [
    {
      "id": "TEN-001",
      "owner_task": "p2-tenancy",
      "tests": ["TestCCIAM001CrossTenantDenied"],
      "commands": ["go test ./internal/tenancy/... -run TestTenantIsolation"]
    }
  ]
}
```

There must be exactly one entry for each of the 146 normative requirement IDs. `owner_task` must match the unique owner in the protected plan. `tests` must be non-empty, must not cite a protected behavior from before the owner's implementation phase, and collectively must include all 55 protected behaviors. `commands` are retained for human traceability and must describe real deterministic commands.

## Launch acceptance evidence

Path: `docs/development/launch-evidence.json`

```json
{
  "schema": "comfort-curators-launch-evidence/v1",
  "areas": [
    {
      "area": 1,
      "name": "Tenant and property isolation",
      "tests": ["TestCCIAM001CrossTenantDenied"],
      "commands": ["tests/acceptance/run --phase 2"]
    }
  ]
}
```

There must be exactly one non-empty entry for each launch area 1 through 16 in the product requirements, and the complete mapping must use at least 16 distinct real tests.

## Security findings

Path: `docs/development/security-findings.json`

```json
{
  "schema": "comfort-curators-security-findings/v1",
  "scan_revision": "full Git commit",
  "findings": [
    {
      "id": "scanner identifier",
      "severity": "medium",
      "status": "resolved",
      "evidence": "local certification artifact or test"
    }
  ]
}
```

An empty findings list is allowed only when the local `security-scan` certification service succeeds. High or critical findings must be `resolved`, `fixed`, or documented `false_positive`; accepted-but-open findings block certification.

