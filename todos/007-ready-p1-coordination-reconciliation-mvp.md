---
status: ready
priority: p1
issue_id: "007"
tags: [coordination, events, reconcile, signals, artifacts]
dependencies: []
---

# Coordination reconciliation + awareness MVP

Deliver the hybrid reconciliation MVP (specs/tasks + events), unified signals/events panel, run artifact schema, event query CLI, docs updates, and golden path smoke test.

## Problem Statement

Coordination state is split across files, event spine, and optional Intermute. There is no reconciler, conflicts are silent, and signals/events are not visible in the unified UI. This reduces reliability and observability for the system.

## Findings

- File-first specs/tasks and event spine coexist without reconciliation.
- Signals are emitted, but there is no unified UI panel for visibility.
- No artifact schema exists for run outputs.
- Docs do not clearly state local-only defaults or schema evolution rules.

## Proposed Solutions

### Option 1: Implement reconciliation MVP + unified panel + docs (recommended)

**Approach:** Follow `docs/plans/2026-01-30-coordination-reconciliation-plan.md`.

**Pros:** High leverage, improves reliability and visibility.
**Cons:** Touches multiple packages.
**Effort:** Multi-day
**Risk:** Medium

### Option 2: Do only docs + UI

**Approach:** Skip reconciliation MVP and only document policy + add panel.

**Pros:** Faster.
**Cons:** Panel has limited data and reconciliation remains missing.
**Effort:** 1-2 days
**Risk:** Medium

## Recommended Action

Execute the plan in `docs/plans/2026-01-30-coordination-reconciliation-plan.md` with reconciliation MVP first, then UI panel, artifacts, CLI queries, docs, and golden path script.

## Technical Details

**Plan:** `docs/plans/2026-01-30-coordination-reconciliation-plan.md`

**Key areas:**
- `pkg/events/` reconciliation + conflict log
- `internal/tui/` signals/events view
- `pkg/contract/` run artifacts
- `cmd/autarch/` query utilities

## Acceptance Criteria

- [ ] Reconciliation MVP emits events for spec/task changes and logs conflicts.
- [ ] Unified signals/events panel works offline and live (Intermute WS).
- [ ] Run artifacts captured locally with event metadata.
- [ ] Event query CLI available for golden path checks.
- [ ] Schema versioning + local-only policy documented.
- [ ] Golden path smoke script runs locally.

## Work Log

### 2026-01-30 - Plan approved

**By:** Codex

**Actions:**
- Created ready todo from plan

**Learnings:**
- Reconciliation needs explicit idempotency and conflict logging

### 2026-01-30 - Reconciliation MVP implementation

**By:** Codex

**Actions:**
- Added reconciliation tables and cursor/conflict helpers in `pkg/events`
- Implemented project reconcile runner with spec/task scanning
- Added `autarch reconcile` CLI command
- Added reconciliation tests for idempotency and task status transitions
- Updated plan progress and task scope
- Ran `GOCACHE=/tmp/go-build go test ./pkg/events -run Reconcile`

**Learnings:**
- File-based tasks use `pending/in_progress/blocked/completed` status names

### 2026-01-30 - Signals/events panel

**By:** Codex

**Actions:**
- Added unified Signals view with filters and sidebar categories
- Wired Intermute WS refresh with event spine fallback
- Added reconcile conflict listing helper
- Updated dashboard tab order to include Signals
- Ran `GOCACHE=/tmp/go-build go test ./internal/tui/...`

**Learnings:**
- Intermute events can trigger refresh without hard dependency

### 2026-01-30 - Run artifact schema + capture hook

**By:** Codex

**Actions:**
- Added `RunArtifact` to `pkg/contract`
- Added `run_artifact_added` event + emitter
- Captured session log as a run artifact on task start (symlink into `.autarch/artifacts/<run-id>/`)
- Ignored `.autarch/` in `.gitignore`

**Learnings:**
- Coldwine task start already produces a stable session ID usable as run ID

### 2026-01-30 - Event query CLI

**By:** Codex

**Actions:**
- Added `autarch events query` and `autarch events since` commands
- Added project/time/type filters with event spine output
- Ran `GOCACHE=/tmp/go-build go test ./cmd/autarch`
