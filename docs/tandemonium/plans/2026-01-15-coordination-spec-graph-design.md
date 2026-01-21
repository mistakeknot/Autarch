# Coordination + Spec Graph Design (No Worktrees)

Date: 2026-01-15

## Goal
Make Tandemonium a low-cognitive-load, high-throughput task + agent manager for
solo devs working in a single repo without git worktrees. Integrate directly
with Praude as the source of truth for PRDs, CUJs, and evidence-backed market
context. Preserve traceability while allowing fast execution.

## Cross-Repo Coordination Note
This design depends on the Praude PRD schema and validation behavior.
When changing this document, update the corresponding Praude design doc:
`/Users/sma/praude/docs/plans/2026-01-15-prd-schema-cuj-validation-design.md`.

## Architecture Overview
Tandemonium runs as a single coordinator process with four core modules:

1) Coordination Layer (feature parity with MCP Agent Mail, built-in):
- Messages, threads, ack-required, inbox state, search
- File reservations with leases and conflict reporting
- Append-only events log for auditability
- Maintain wire-level compatibility with MCP Agent Mail where practical
- No external MCP Agent Mail installation required for users

2) Hybrid Orchestration:
- Lock Service: advisory reservations with optional hard block
- Patch Queue: agent changes submitted as patches with metadata
- Apply Worker: serial patch applier + minimal checks

3) Spec Graph:
- Reads PRDs directly from `.praude/specs/`
- Indexes CUJs, requirements, evidence, and file hints
- Maps tasks to CUJ IDs + PRD revision hashes
- Praude authoring flows: `praude interview` (PRD creation) and
  `praude suggestions review/apply` for section updates

4) Hot File Discovery:
- Incremental risk scoring (git history + static analysis)
- Tiered file list for drift policy + lock suggestions

## Data Flow
1) Task start:
- Load PRD + CUJ context from Praude
- If no PRD exists, prompt to run `praude interview`
- Infer likely files and auto-suggest a lockset
- Reserve locks (advisory), then launch agent

2) Agent work:
- After-write updates (debounced) adjust hot-file scoring silently
- Patch submitted to queue, validated against reservations

3) Apply:
- Apply Worker merges patches serially
- On conflict, requeue as needs-resolution
- On success, update CUJ coverage and task status

4) Drift handling (hybrid default):
- Non-blocking by default
- Auto-block if Tier 1 file or critical CUJ touched
- Release gate requires all drift resolved

## Storage
- `state.db` (SQLite):
  - messages, mailboxes, reservations, patches, drift_events, cuj_links
- `.tandemonium/queue/`: patch bodies + metadata
- `.tandemonium/coord/events.jsonl`: append-only audit log

## Spec Graph + Praude Integration
- Treat `.praude/specs/` as canonical
- Watch for local file updates (no remote sync)
- Record PRD revision hash per task (pin until user re-pins)
- Evidence refs point to `.praude/research/`
- Suggestions are reviewed/applied via `praude suggestions review/apply`
- Optional future RPC, but not required for MVP

## Hot File Discovery
Signals (weighted):
- Git churn, dependency centrality, entrypoint proximity
- Spec relevance (Praude file hints and CUJ links)
- Docs criticality (ADR or architecture docs)

Triggers (default):
- Task start
- PRD update
- Task completion
- After-write (silent, thresholded, debounced)

Outputs tiers:
- Tier 1: block drift
- Tier 2: warn + ack
- Tier 3: ignore

## UI Surfaces
- Agent dashboard: status, blockers, last output
- Lock map: who holds what, lease expiry
- Patch queue: priority, risk, status
- Drift queue: accept or reject drift
- CUJ coverage: progress and gaps

## UI Sketch (Fleet View)
```
Tandemonium / Tasks (CUJ: all) | filter: all | search: -
--------------------------------------------------------
TASKS *                                DETAILS *
TYPE PRI ST  ID     TITLE              [Details] [Term]
--------------------------------------------------------
> CUJ-001: Onboarding
  - tsk_123  Add signup flow
  - tsk_124  Email verify
  CUJ-002: Checkout
  - tsk_201  Cart summary

DETAILS TAB
ID: tsk_123
Title: Add signup flow
Status: [RUN]
Primary CUJ: CUJ-001
Secondary CUJs: CUJ-003
Session: [RUN] working

Summary
...

Acceptance Criteria
...

Recent Activity
...

STATUS: ready
KEYS: n new task, s start, t terminal, r review, / search, tab focus
```

Right-pane sub-tabs (single visible at a time):
- Details (default)
- Terminal (per-task tmux attach)
- Locks
- Queue
- Drift
- CUJ

## UI Sketch (Review Mode)
```
REVIEW - tsk_123: Add signup flow

SPEC + CUJ ALIGNMENT
Primary CUJ: CUJ-001 (Onboarding)
Secondary: CUJ-003
Alignment: in-scope
Drift: none

SUMMARY
...

FILES CHANGED
- internal/auth/signup.go +120 -20
- internal/ui/onboarding.go +45 -10

TESTS: 8 passed

ACCEPTANCE CRITERIA
- ...

[d]iff  [a]pprove  [f]eedback  [r]eject  [e]dit story  [b]ack
```

Suggested key additions:
- `t` toggle terminal (fleet view)
- `]` / `[` cycle right-pane sub-tabs

## Task Terminal Pane
- Each task has a toggleable terminal pane attached to its agent session.
- `t` toggles terminal visibility for the selected task.
- `Ctrl+O` toggles interactive mode (stdin passthrough).
- Terminal view is single-session at a time to keep UI simple.
- Output is still logged to `.tandemonium/sessions/<id>.log` for audit.

## Agent Workspace Contract (No Worktrees)
- Agents are patch-only by default and never write to main.
- Each agent operates in a scratch clone (shared objects) to generate patches.
- Apply Worker is the only writer to main and runs patches sequentially.

## Focus Mode (Mandatory Patch on Exit)
- Focus Mode pauses Apply Worker and takes an exclusive lock on main.
- User edits/tests live in main for fast debugging.
- Exiting Focus Mode always produces a patch (or explicit discard) and resumes Apply Worker.

## Error Handling
- Stale locks: auto-expire; manual override in TUI
- Patch conflicts: requeue with diff summary
- Spec missing/invalid: degrade gracefully with warnings
- Invalid evidence refs: warn unless hard policy enabled

## Testing
- Unit tests: lock leasing, drift policy, hot-file scoring
- Integration tests: patch queue + apply worker
- Spec Graph tests: PRD parsing, CUJ mapping, hash updates

## Open Questions
- Patch format: start with raw git diff, move to structured patches later
- Minimum checks on apply: lint only vs targeted tests
- Default lock enforcement: advisory vs hard block
- Focus Mode UX: how to surface patch preview before enqueue
- Scratch clone cleanup policy (TTL vs size cap)
