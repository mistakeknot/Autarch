# Plan: Complete Write-Path Intent Contract in Coldwine TUI

**Bead:** iv-ssc4s
**PRD:** `docs/prds/2026-02-26-missing-write-path-intents.md`
**Date:** 2026-02-26

## Task 1: Add gate override message type and handler

**Files:** `internal/tui/views/run_actions.go`, `internal/tui/views/coldwine.go`

1. In `run_actions.go`, add:
   ```go
   type coldwineGateOverrideMsg struct {
       runID  string
       reason string
       err    error
   }
   ```

2. In `coldwine.go` Update() switch, add handler for `coldwineGateOverrideMsg`:
   - On success: set `v.statusMsg = "Gate overridden: <reason>"`, reload run detail
   - On error: set `v.statusMsg = "Override failed: <err>"`

**Test:** Verify message routing compiles. No functional test needed yet — action wired in Task 2.

## Task 2: Implement overrideGate() action with clavain→ic fallback

**Files:** `internal/tui/views/coldwine_mode.go`, `pkg/clavain/gate.go`

1. In `pkg/clavain/gate.go`, keep `GateOverride()` returning `ErrUnavailable` (unchanged — clavain-cli doesn't have this yet). The TUI handles the fallback.

2. In `coldwine_mode.go`, add:
   ```go
   func (v *ColdwineView) overrideGate(reason string) tea.Cmd {
       if v.iclient == nil || v.selectedRun < 0 || v.selectedRun >= len(v.runs) {
           return nil
       }
       run := v.runs[v.selectedRun]
       runID := run.ID
       ic := v.iclient
       cc := v.cclient
       return func() tea.Msg {
           ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
           defer cancel()
           // Try OS layer first
           if cc != nil {
               err := cc.GateOverride(ctx, runID, reason)
               if err == nil {
                   return coldwineGateOverrideMsg{runID: runID, reason: reason}
               }
               // Fall through to ic on ErrUnavailable
               if !errors.Is(err, clavain.ErrUnavailable) {
                   return coldwineGateOverrideMsg{runID: runID, reason: reason, err: err}
               }
           }
           // Fallback to kernel directly
           err := ic.GateOverride(ctx, runID, reason)
           return coldwineGateOverrideMsg{runID: runID, reason: reason, err: err}
       }
   }
   ```

3. Add keypress `o` in `handleRunsKeypress()` (after `"a"` for advance):
   ```go
   case "o":
       return v.overrideGate("Manual override from TUI")
   ```

**Test:** Unit test that `overrideGate()` produces a tea.Cmd (nil-safety). Integration test deferred to Task 6.

## Task 3: Add `/sprint override` chat command

**Files:** `internal/tui/views/sprint_commands.go`

1. Add case `"override"` to `handleSprint()`:
   ```go
   case "override":
       if len(args) < 2 {
           return "Usage: /sprint override <reason>"
       }
       reason := strings.Join(args[1:], " ")
       // Find active run
       run := r.getActiveRun()
       if run == nil {
           return "No active run"
       }
       // Try clavain, fallback to ic
       err := cc.GateOverride(ctx, run.ID, reason)
       if errors.Is(err, clavain.ErrUnavailable) {
           err = ic.GateOverride(ctx, run.ID, reason)
       }
       if err != nil {
           return fmt.Sprintf("Override failed: %s", err)
       }
       return fmt.Sprintf("Gate overridden: %s", reason)
   ```

2. Add `"override"` to the help text / command list.

**Test:** Verify command parsing handles missing reason gracefully.

## Task 4: Add `/sprint artifact` chat command

**Files:** `internal/tui/views/sprint_commands.go`

1. Add case `"artifact"` to `handleSprint()`:
   ```go
   case "artifact":
       if len(args) < 3 {
           return "Usage: /sprint artifact <type> <path>"
       }
       artifactType := args[1]
       path := args[2]
       // Resolve bead ID from active run
       beadID := r.getBeadID()
       if beadID == "" {
           return "No active sprint bead"
       }
       err := cc.SetArtifact(ctx, beadID, artifactType, path)
       if err != nil {
           return fmt.Sprintf("Artifact registration failed: %s", err)
       }
       return fmt.Sprintf("Registered %s artifact: %s", artifactType, path)
   ```

2. Add `"artifact"` to help text.

**Note:** `getBeadID()` may need to be added — check if `SprintCommandRouter` already resolves bead IDs from runs.

**Test:** Verify command parsing and error messages.

## Task 5: Wire SetArtifact into sprint creation success path

**Files:** `internal/tui/views/coldwine.go`

1. In the `sprintCreatedMsg` handler (around line 406), after successful creation:
   ```go
   case sprintCreatedMsg:
       if msg.err == nil && v.cclient != nil {
           // Register the sprint goal as a brainstorm-like artifact if we have a bead
           // This is best-effort — don't block on failure
           go func() {
               ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
               defer cancel()
               _ = v.cclient.SetArtifact(ctx, msg.beadID, "sprint-goal", msg.goal)
           }()
       }
   ```

   Wait — `sprintCreatedMsg` has `runID` and `goal` but not `beadID`. Check if we need to resolve it.

   Actually, the existing `sprintCreatedMsg` is produced by `cc.SprintCreate()` which returns a run ID, and then `cc.ResolveRunID()` maps bead→run. The bead ID is the sprint bead. Let me check the existing struct.

   If `sprintCreatedMsg` doesn't carry beadID, we can't call `SetArtifact` (which needs beadID). The sprint creation flow would need to be updated to carry the bead ID through. This may be more invasive than expected.

   **Decision:** Skip this task for v1. The `/sprint artifact` command (Task 4) covers explicit registration. Wiring auto-registration into creation requires structural changes to message flow.

**Revised:** Task 5 is deferred. Scope reduced to Tasks 1-4.

## Task 6: Manual verification

1. Build: `go build ./cmd/autarch/`
2. Run with a kernel database that has an active run with a blocked gate
3. Navigate to Coldwine Runs mode
4. Press `o` — verify override succeeds and status updates
5. Use `/sprint override <reason>` — verify chat command works
6. Use `/sprint artifact plan /path/to/plan.md` — verify registration

## Summary

| Task | Files | Effort |
|------|-------|--------|
| 1. Gate override message + handler | run_actions.go, coldwine.go | Small |
| 2. overrideGate() action + keypress | coldwine_mode.go | Medium |
| 3. /sprint override command | sprint_commands.go | Small |
| 4. /sprint artifact command | sprint_commands.go | Small |
| 5. ~~Auto-register on creation~~ | ~~coldwine.go~~ | Deferred |
| 6. Manual verification | — | Manual |

**Total: 4 code tasks, ~150 lines changed across 4 files.**
