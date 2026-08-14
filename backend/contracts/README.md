# Protected Backend Contracts

These files are the implementation boundary between independent planning, implementation workers, and black-box acceptance.

- `api/openapi.yaml`: minimum external REST surface and command conventions.
- `database/table_ownership.yaml`: authoritative module ownership, records, and cross-cutting data constraints.
- `events/catalog.yaml`: durable domain and integration event names plus envelope.
- `agents/state_machines.yaml`: durable Jarvis, Hermes, job, approval, and tool-call states.
- `acceptance/named_behaviors.yaml`: 55 named behaviors recovered from the prior scope.
- `acceptance/oracle.yaml`: the real observation required for each behavior.
- `acceptance/fixture_protocol.md`: the synthetic black-box test bootstrap contract.

The contracts are a floor, not permission to broaden V0. An implementation may add internal types and module-local behavior only when it remains inside the frozen requirements. Public API additions or semantic changes require a human-approved contract revision.


