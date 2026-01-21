# Implementation Plan: Coordination + Spec Graph (No Worktrees)

Date: 2026-01-15

## Scope
Implement a hybrid, no-worktree orchestration model, an embedded coordination
layer (feature parity with MCP Agent Mail), Spec Graph integration with Praude,
and hot-file discovery with event triggers. Reflect maintenance CUJ and
suggestions flow from Praude. Add per-task terminal pane.

## Implementation Checklist (Owners)
- [ ] (Tandemonium) Define patch metadata schema (base commit, files, CUJ IDs, risk tier, spec hash)
- [ ] (Tandemonium) Add DB migrations for `messages`, `mailboxes`, `reservations`, `events`
- [ ] (Tandemonium) Implement built-in coordination layer with MCP Agent Mail wire compatibility
- [ ] (Tandemonium) Build Spec Graph reader for `.praude/specs/` + spec hash pinning
- [ ] (Tandemonium) Add task fields: `prd_id`, `primary_cuj_id`, `secondary_cuj_ids`, `spec_hash`
- [ ] (Tandemonium) Implement suggestions index (map `PRD-###` to pending suggestions)
- [ ] (Tandemonium) Build patch queue storage + apply worker (3-way apply + guardrails)
- [ ] (Tandemonium) Implement scratch clone pool + cleanup policy
- [ ] (Tandemonium) Add Focus Mode (exclusive lock + mandatory patch on exit)
- [ ] (Tandemonium) Add terminal pane toggle + right-pane sub-tabs
- [ ] (Tandemonium) Hot-file scoring + tier persistence + after-write detection
- [ ] (Tandemonium) Tests: coord layer, spec graph, apply worker, focus mode, suggestions index

## Phase 1: Coordination Layer (Agent Mail Parity)
1) Data model
- Extend `state.db` schema with tables:
  - `messages`, `mailboxes`, `reservations`, `events`
- Add indexes for inbox lookup and path reservation conflicts

2) API surface (internal)
- Functions: `send_message`, `fetch_inbox`, `ack_message`, `reserve_paths`,
  `release_paths`, `search_messages`, `summarize_thread`
- Keep payloads minimal for TUI

3) MCP Agent Mail compatibility layer
- Map MCP Agent Mail message/lock semantics to Tandemonium storage
- Keep wire-level payloads compatible where practical
- Document supported endpoints and gaps
- Built-in by default; no external MCP Agent Mail install required

4) CLI parity
- `tand mail send`, `tand mail inbox`, `tand lock reserve`, `tand lock release`

## Phase 2: Hybrid Orchestration
4) Patch Queue
- Queue directory `.tandemonium/queue/`
- Patch metadata format (json) + raw git diff body

5) Agent workspace contract
- Agents operate in scratch clones (shared objects)
- Patch-only writes; main is write-protected except Apply Worker
- Maintain scratch pool with cleanup policy

6) Apply Worker
- Serial apply of patches
- Minimal checks (lint or targeted tests)
- 3-way apply + limited auto-rebase for low-risk patches
- Fail fast to Focus Mode for high-risk conflicts

7) Drift policy integration
- Non-blocking default
- Auto-block for Tier 1 files or critical CUJs
- Release gate enforcement

## Phase 3: Spec Graph (Praude Integration)
8) Spec reader
- Read `.praude/specs/` directly
- Build in-memory index of PRDs, CUJs, evidence refs
- Ensure maintenance CUJ is treated as a valid primary CUJ option

9) File watcher
- Watch for PRD changes and update graph
- Tasks pin spec hash until re-pinned

10) Task linkage
- Require task -> primary CUJ mapping (or maintenance CUJ)
- Store `prd_id`, `spec_hash`, `primary_cuj_id`, `secondary_cuj_ids`

11) Suggestions awareness
- Read `.praude/suggestions/` for pending updates
- Surface suggestion availability in TUI (informational only)
- Map suggestion files to PRD IDs via filename prefix (PRD-###)
- Link to Praude review/apply CLI: `praude suggestions review/apply`

## Phase 4: Hot File Discovery
12) Risk scoring
- Git churn + static analysis (imports/entrypoints)
- Spec relevance weighting
- Persist tiers in config or DB with expiry policy

13) Triggers
- Task start, PRD update, task completion
- After-write silent update (debounced, thresholded)
- After-write detection via file watcher + diff sampling

14) Tier outputs
- Tier 1 (block), Tier 2 (warn), Tier 3 (ignore)

## Phase 5: UI + UX
15) TUI panels
- Lock map, patch queue, drift queue
- CUJ coverage view
- Suggestions indicator (links to Praude)

16) Per-task terminal pane
- Toggle with `t` for selected task
- `Ctrl+O` toggles interactive mode
- One terminal visible at a time
- Session output still logged to `.tandemonium/sessions/<id>.log`

17) Drift resolution
- Accept drift (update spec hash + prompt Praude)
- Reject drift (generate corrective guidance)
- Prompt to re-run `praude interview` when PRD is missing

## Phase 6: Tests
18) Unit tests
- Lock lease rules, reservation conflicts
- Patch queue metadata validation
- Spec Graph parsing + hash updates

19) Integration tests
- Patch queue + apply worker
- Drift workflow
- Hot-file triggers
- Terminal toggle behavior (basic TUI tests)
- Focus Mode enter/exit (patch creation)
- Suggestions index (PRD mapping)

## Notes
- Default patch format: raw git diff
- Default validation: advisory locks (hard block optional)
- Update Praude design doc if schema changes
- Add DB migration step for new tables before shipping
