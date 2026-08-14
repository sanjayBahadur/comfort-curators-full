# Fatal Flaw Recovery

A fatal flaw is a defect that invalidates a protected contract, tenant or property isolation, money correctness, access custody, audit integrity, migration safety, job idempotency, or several already-integrated modules.

## Required response

1. Stop dispatch for the affected phase and every downstream phase.
2. Record a P0 Bead with the failing protected scenario, first bad commit if known, affected requirements, and blast radius.
3. Identify the newest trustworthy `verified/phase-N` tag. Do not use an unverified integration branch as the recovery base.
4. Create `repair/phase-M/<short-description>` from that tag, where M is the first invalid phase.
5. Reopen or replace affected tasks and make every downstream open task depend on the repair gate.
6. Preserve the failed branch for evidence. Do not rewrite it or force-push over it.
7. Run the repaired phase gate plus regression gates for every earlier affected phase.
8. Land only after an independent reviewer confirms the original failure is reproduced before the fix and absent after it.
9. Create a new annotated verified tag. Record which prior tag it supersedes.

## Stop conditions

Escalate to the human overseer instead of continuing when:

- the contract itself is unsafe, contradictory, or legally uncertain;
- the recovery requires deleting or rewriting operational evidence;
- a migration cannot be forward-recovered without data loss;
- the flaw changes pricing, labor, access, privacy, financial, or AI authority policy;
- the test oracle is shown to be invalid; or
- the same fatal class recurs after one repair cycle.

Gas Town can isolate and schedule repair work, but it cannot decide that a business or legal contract should change. That boundary remains explicit.


