# Standard Gastown Operating Model

Comfort Curators uses Gastown as the sole development orchestrator. Project-specific code controls contracts, phase gates, and release evidence, but it does not replace Gastown's worker lifecycle.

## Standard components

| Responsibility | Standard component |
|---|---|
| Work graph and dependencies | Beads |
| Background dispatch | Gastown capacity scheduler and deferred sling contexts |
| Coding workers | Polecats |
| Worker health and nudges | Witness |
| Integration and merge sequencing | Refinery and merge queue |
| Cross-task coordination | Convoys and Gastown mail |
| OpenAI runtime | Built-in `codex` agent preset |
| DeepSeek runtime | Built-in `opencode` agent preset |

There is no custom Python dispatcher, custom OpenCode launcher, or Bubblewrap worker namespace.

## Provider allocation

- DeepSeek V4 Pro through Gastown's built-in `opencode` preset is the default implementation runtime.
- Codex with GPT-5.6 Sol is reserved for 11 high-risk tasks covering the repository spine, concurrency and durability, security and identity, dispatch state machines, access custody, financial maker-checker controls, typed AI policy, hardening, and release traceability.
- Mayor and Witness use OpenCode so DeepSeek handles coordination, nudges, and routine diagnosis.
- Refinery uses Codex for integration, merge repair, and the final code-quality choke point.
- The per-task provider is stored in the protected plan and passed through ordinary `gt sling --agent` contexts.

This sends 38 of 49 implementation tasks to DeepSeek while keeping GPT usage concentrated where a subtle mistake would poison many downstream tasks.

## Phase flow

1. `./cc run --workers auto` starts Gastown, creates the current phase integration branch, creates standard deferred sling contexts for the phase tasks, and resumes the scheduler.
2. Gastown dispatches dependency-ready Beads to Polecats.
3. Polecats work in Gastown-managed worktrees and submit through the normal merge queue.
4. Witness observes workers and Refinery integrates completed work.
5. When all implementation Beads in the phase are closed, `./cc approve` reruns the phase acceptance gate twice against the exact integration commit.
6. Human approval lands that exact commit to protected `main` and creates `verified/phase-N`.
7. The next phase is scheduled through ordinary Gastown sling contexts.

## Operator commands

```bash
./cc auth
./cc run --workers auto
./cc
./cc watch
./cc problems
./cc stop
./cc fix
./cc approve
```

`--workers auto` is the default. It selects phase capacities of 3, 2, 3, 3, 4, 2, and 4. Manual values from 1 through 8 are accepted, as is `--workers max`. The scheduler remains blocker-aware, so capacity is a ceiling rather than a command to spawn useless workers.

