# Task-First Flow (Tasks-Only Fleet) Design

**Date:** 2026-01-13  
**Status:** Draft  

## Goal

Make the TUI feel active and “Clark-like” by making **tasks** the primary surface in the Fleet view, with direct actions to create, start, stop, and review work.

## Non-Goals (for this phase)

- Multi-pane split views (tasks + sessions)
- Advanced command palette actions
- Full PM/refinement flow
- Daemon/agent health dashboards

## UX Overview

### Fleet View (Tasks-Only)
- The main list is **tasks** from SQLite, not review queue.
- Each row shows:
  - Task ID
  - Title
  - Status (todo / in_progress / review / blocked / done)
  - Optional “session” badge if a tmux session exists

### Primary Actions (Footer)
- `[n]` New quick task (already implemented)
- `[s]` Start task (create worktree + tmux session, mark in_progress)
- `[r]` Review (enter review view if task is in review)
- `[x]` Stop task (stop tmux session; set status to blocked or todo)
- `[b]` Back (unchanged)

### Empty State
If no tasks, show CTA:
```
No tasks yet.
[n] new quick task  [i] init  [?] help
```

## Data Flow

### Load Tasks
- Query SQLite `tasks` table.
- Display list ordered by status + recent updated time (if we add a column later; for now, stable order).

### Start Task
1. Ensure repo initialized (`project.Init` if needed).
2. Create worktree + branch via `internal/git` (existing helpers).
3. Start tmux session (existing `internal/tmux`).
4. Update SQLite `tasks.status = in_progress`.
5. Add/Update `sessions` table.

### Stop Task
1. Stop tmux session (existing `internal/tmux`).
2. Update SQLite task status to `blocked` (or `todo` if we choose).

### Review
If `status == review`, enter existing Review view (already implemented).

## Integration Points

- `internal/tui/model.go`: add task list state, selection, and key handlers.
- `internal/tui/task_loader.go`: new loader for tasks from SQLite.
- `internal/storage/task.go`: already supports Insert/Get/Update; may add ListTasks.
- `internal/agent/launcher.go`: use to generate session IDs.

## Error Handling

- All action failures should set `StatusError` in TUI status line.
- Non-fatal errors should keep the UI responsive.

## Testing

- Unit tests for task loader ordering and TUI key handling.
- Stub task creator/runner in tests (as with quick tasks).

## UX Success Criteria

- Running `./dev` shows a populated, actionable task list after creating a quick task.
- Pressing `[s]` and `[x]` changes status and provides clear feedback.
- Review view remains unchanged and usable.
