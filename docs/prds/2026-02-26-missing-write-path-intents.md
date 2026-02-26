# PRD: Complete Write-Path Intent Contract in Coldwine TUI

**Bead:** iv-ssc4s
**Date:** 2026-02-26
**Status:** prd
**Priority:** P1
**Complexity:** 3/5

## Problem

The Autarch vision doc (v1.1, §"Write-path contract") defines a Minimal Intent Contract with four write-path operations. Only two are wired in the TUI:

| Intent | Status | Where |
|--------|--------|-------|
| `start-run` | Wired | Coldwine: epic context + `/sprint create` |
| `advance-run` | Wired | Coldwine: keypress `a`, auto-advance, `/sprint advance` |
| `override-gate` | **Not wired** | `pkg/clavain/gate.go` returns `ErrUnavailable` |
| `submit-artifact` | **Not wired** | `pkg/clavain/artifact.go` implemented but no TUI call sites |

**User impact:** When a sprint gate blocks in the TUI, the user must exit to CLI (`ic gate override`). When Coldwine advances phases, the kernel doesn't know what artifacts were produced — gates checking `artifact_exists` are blind.

## Scope

Complete the 4/4 intent contract in Coldwine. Out of scope: wiring `start-run` into Bigend, wiring `submit-artifact` into Gurgeh/Pollard (separate follow-up beads).

## Feature 1: Gate Override Intent

### What

Add an `override-gate` action to Coldwine's Runs mode. When a run is blocked by a gate, the user can press `o` to override with a reason.

### Implementation

**`pkg/clavain/gate.go`** — Replace the stub with a fallback to `ic.GateOverride()`:

```go
func (c *Client) GateOverride(ctx context.Context, beadID, reason string) error {
    // Delegate to ic gate override (clavain-cli doesn't wrap this yet)
    return c.ic.GateOverride(ctx, beadID, reason)
}
```

Wait — the clavain client doesn't hold an intercore client reference. The pattern used by `advancePhase()` is: try `cc.SprintAdvance()`, fall through to `ic.RunAdvance()` on error. So the TUI should follow the same pattern:

```
cc.GateOverride() → if ErrUnavailable → ic.GateOverride() fallback
```

But `cc.GateOverride()` currently *always* returns `ErrUnavailable`, so the TUI must handle the fallback. Two options:

**Option A:** Fix the clavain stub to shell out to `ic gate override` (the clavain client already has `execText` which can call any binary). This keeps the OS-layer routing.

**Option B:** Have the TUI fall through to `ic.GateOverride()` when `cc.GateOverride()` returns `ErrUnavailable`. Same pattern as advance.

**Decision:** Option B (fallback pattern). Matches the existing clavain→ic fallback used by advance. When clavain-cli eventually adds `gate-override`, the stub can be replaced without TUI changes.

**TUI changes:**

1. New message type in `run_actions.go`:
   ```go
   type coldwineGateOverrideMsg struct {
       runID  string
       reason string
       err    error
   }
   ```

2. New action function in `coldwine_mode.go`:
   ```go
   func (v *ColdwineView) overrideGate() tea.Cmd
   ```
   - Check `v.iclient` and selected run exist
   - Check run has a blocked gate (from detail panel's `Gate` field)
   - Prompt for reason via status line input (reuse composer pattern)
   - Call `cc.GateOverride()`, fall back to `ic.GateOverride()` on `ErrUnavailable`
   - Return `coldwineGateOverrideMsg`

3. Handle `coldwineGateOverrideMsg` in `handleColdwineGateOverrideMsg()`:
   - On success: set status message "Gate overridden: <reason>", reload run detail
   - On error: set status message "Override failed: <err>"

4. Keypress `o` in Runs mode (in `handleRunsKeypress`):
   - Only active when selected run has a blocked gate
   - Triggers `overrideGate()`

### UX Flow

1. User views run in Runs mode
2. Run shows "Gate blocked: artifact_missing — plan artifact required"
3. User presses `o`
4. Status line prompts: "Override reason: "
5. User types reason, presses Enter
6. Gate is overridden, run detail reloads
7. Status shows "Gate overridden: <reason>"

### Reason Input

The simplest approach: use a hardcoded reason for v1 ("Manual override from TUI"). The `/sprint override <reason>` chat command provides the free-text path. This avoids building a text input modal.

**Decision:** Hardcoded reason for keypress, free-text via `/sprint override <reason>` chat command.

## Feature 2: Submit Artifact Intent

### What

Register artifacts with the kernel when Coldwine produces them during sprint operations.

### Where Artifacts Are Produced

1. **Sprint creation** — brainstorm doc may exist
2. **Phase advance** — the phase may have produced an artifact
3. **Dispatch completion** — dispatch output files

### Implementation

The key insight: `cc.SetArtifact()` is already fully implemented. The work is adding call sites.

**Call site 1: `sprintCreatedMsg` handler** (in `coldwine.go`):
After successful sprint creation, if a brainstorm artifact path is known, register it:
```go
case sprintCreatedMsg:
    if msg.err == nil && msg.brainstormPath != "" {
        cc.SetArtifact(ctx, beadID, "brainstorm", msg.brainstormPath)
    }
```

However — sprint creation from the TUI doesn't produce a brainstorm doc. The brainstorm happens in Clavain CLI sessions. So this call site is low value.

**Call site 2: `coldwineAdvancedMsg` handler** (in `coldwine_mode.go`):
After successful advance, register the phase's artifact if one exists. The artifact path comes from the sprint's artifact registry (the advance already happened via `cc.SprintAdvance`, which registers artifacts itself in the CLI flow).

This is also low value — the CLI flow already registers artifacts.

**The real gap:** Artifacts produced *within* the TUI that never reach the kernel. This happens when:
- Coldwine creates a session log → `recordRunLogArtifact()` emits local event only
- Dispatch output files are written but not registered

**Call site 3: `recordRunLogArtifact`** (in `coldwine/tui/artifacts.go`):
This is dead code from the pre-unified TUI. In the current unified TUI, session logs are not produced by Coldwine — they're produced by the CLI agents that Coldwine dispatches.

**Revised assessment:** The `submit-artifact` gap is narrower than initially scoped. The primary gap is:

1. **Chat-initiated operations** — when the user runs `/sprint create <goal>` via chat, the sprint object exists but no artifact is registered. The fix: after sprint creation, if clavain-cli created a brainstorm doc, read it back and register it.

2. **Phase-specific artifacts** — when a user manually advances in the TUI, the OS layer (`cc.SprintAdvance`) already handles artifact registration in the CLI. But the TUI could pass artifact context that the CLI doesn't have.

**Decision:** Wire `cc.SetArtifact()` into the `/sprint` chat commands where artifact paths are available. Add `/sprint artifact <type> <path>` chat command for explicit registration. This covers the gap without inventing new artifact-production flows.

## Tasks

1. **Add `coldwineGateOverrideMsg` and handler** — new message type, handler function
2. **Implement `overrideGate()` action** — clavain→ic fallback pattern
3. **Add keypress `o` in Runs mode** — gated on blocked state
4. **Add `/sprint override <reason>` chat command** — in `SprintCommandRouter`
5. **Add `/sprint artifact <type> <path>` chat command** — explicit artifact registration
6. **Wire `SetArtifact` into `/sprint create` success path** — register brainstorm if available

## Success Criteria

1. All 4 vision write-path intents are callable from Coldwine TUI
2. Pressing `o` on a gate-blocked run overrides the gate with a status message
3. `/sprint override <reason>` provides free-text override reason
4. `/sprint artifact <type> <path>` registers artifacts with the kernel
5. No regression in existing start-run and advance-run functionality

## Out of Scope

- Gate override confirmation dialog (v1 uses hardcoded reason for keypress)
- Automatic artifact detection (user explicitly specifies path)
- Bigend start-run wiring
- Gurgeh/Pollard artifact registration
