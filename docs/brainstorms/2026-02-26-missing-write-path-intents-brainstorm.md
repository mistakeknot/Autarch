# Brainstorm: Missing Write-Path Intents in TUI

**Bead:** iv-ssc4s
**Date:** 2026-02-26
**Status:** brainstorm

## Problem Statement

The Autarch vision doc (v1.1, §"Write-path contract") defines a **Minimal Intent Contract (v1)** with exactly four write-path operations that TUI apps submit to the OS (Clavain):

| Intent | App Action | OS Response |
|--------|-----------|-------------|
| `start-run` | User clicks "Start Sprint" in Bigend or Coldwine | OS creates run via `ic`, applies routing policy, returns run ID |
| `advance-run` | User clicks "Advance" or auto-advance triggers | OS evaluates gates via `ic`, advances if passing, returns result |
| `override-gate` | User clicks "Override" on a failed gate | OS records override via `ic` with reason, advances, returns result |
| `submit-artifact` | Tool generates a spec section (Gurgeh) or research result (Pollard) | OS registers artifact via `ic`, returns artifact ID |

**Current state:** Two of four are wired. Two are not.

## What's Implemented

### `start-run` — WIRED (Coldwine only)

- `ColdwineView` fires `cc.SprintCreate(ctx, goal)` via clavain client when user triggers sprint from epic context
- `/sprint create <goal>` slash command in `SprintCommandRouter` also calls `cc.SprintCreate()`
- Falls back to direct `ic.RunCreate()` if clavain-cli unavailable
- **Not wired in Bigend** — vision says "Bigend or Coldwine" but only Coldwine has it

### `advance-run` — WIRED (Coldwine only)

- Keypress `a` in Runs mode triggers `advancePhase()` → `cc.SprintAdvance()`
- Auto-advance after dispatch completion via `tryAutoAdvance()` → same path
- `/sprint advance` slash command in chat router
- Correctly avoids double-advance trap: uses `ic.RunStatus()` (read) after `cc.SprintAdvance()` (mutation)
- **Not wired in Bigend** — dashboard shows runs but can't advance them

## What's Missing

### `override-gate` — NOT WIRED

**API layer exists but is a stub:**
- `pkg/clavain/gate.go` → `GateOverride()` returns `ErrUnavailable` with TODO
- `pkg/intercore/operations.go` → `ic.GateOverride()` is defined (delegates to `ic gate override` CLI)

**No TUI surface:**
- No keybinding, button, or message type for gate override in any view
- No `gateOverrideMsg` or similar Bubble Tea message
- When a gate blocks advance in Coldwine, the user sees the block but has no TUI action to unblock it — must drop to CLI

**Impact:** A user watching a sprint in Coldwine TUI hits a failed gate and must exit to CLI (`ic gate override`) to unblock. This breaks the "TUI is a complete surface" promise.

### `submit-artifact` — NOT WIRED

**API layer exists:**
- `pkg/clavain/artifact.go` → `SetArtifact(beadID, artifactType, path)` defined
- `pkg/intercore/operations.go` → `ArtifactAdd(runID, phase, path, artifactType)` defined

**What exists instead — local-only:**
- Coldwine's `recordRunLogArtifact()` emits a local `RunArtifactAdded` event — never reaches the kernel
- Gurgeh generates spec sections as YAML but never registers them with `ic artifact add`
- Pollard produces research results that never reach the kernel's artifact registry
- `SprintAdvance()` accepts an optional `artifactPath` but no TUI call site passes one

**Impact:** Artifacts produced by Gurgeh and Pollard are invisible to the kernel. Gates that check `artifact_exists` can't see them. The kernel-driven lifecycle is blind to what the TUI tools produce.

## Approach Options

### Option A: Wire Both Intents in Coldwine First (Minimal)

Focus on where the existing infrastructure is strongest:

1. **`override-gate` in Coldwine Runs mode:**
   - Add keypress `o` (or similar) when viewing a blocked run
   - Implement `clavain.GateOverride()` (replace stub)
   - Show confirmation dialog with reason input (override requires a reason per vision)
   - Return `gateOverrideMsg` with result

2. **`submit-artifact` in Coldwine sprint flow:**
   - After `SprintAdvance()`, pass artifact path if one was produced in the current phase
   - Call `cc.SetArtifact()` to register the artifact with the kernel
   - This makes the sprint lifecycle aware of what Coldwine produces

**Pros:** Smallest scope, completes the 4/4 contract in Coldwine.
**Cons:** Gurgeh and Pollard still don't register artifacts with kernel.

### Option B: Wire All Four Apps (Full Coverage)

Extend beyond Coldwine:

1. Everything in Option A, plus:
2. **`start-run` in Bigend:** Add "Start Sprint" action from project context in multi-project view
3. **`submit-artifact` in Gurgeh:** After each spec section is generated and saved, call `cc.SetArtifact()` to register it
4. **`submit-artifact` in Pollard:** After insight synthesis or watch results, register as discovery artifacts

**Pros:** Full vision compliance — every app that produces artifacts registers them.
**Cons:** Larger scope, Gurgeh/Pollard integration requires understanding their artifact lifecycles.

### Option C: Option A + Gurgeh Artifact Registration

A middle ground — wire the two missing intents in Coldwine, plus the highest-value cross-app integration:

1. Everything in Option A
2. **`submit-artifact` in Gurgeh spec sprint:** When a spec section completes, register it via `cc.SetArtifact(beadID, "spec-section", sectionPath)`. This enables `artifact_exists` gates for spec-to-execution transitions.

**Pros:** Closes the most valuable gap (spec→kernel visibility) without touching Pollard.
**Cons:** Pollard artifacts still local-only.

## Recommended Approach: Option A (Minimal)

**Rationale:** The bead says "two of four vision write-path intents missing from TUI" — the scope is completing the contract, not extending it to all apps. Options B/C are valuable follow-ups but are separate scope.

### Implementation Sketch

#### 1. `override-gate` (pkg/clavain + Coldwine TUI)

**clavain client:**
```go
// pkg/clavain/gate.go — replace stub
func (c *Client) GateOverride(ctx context.Context, runID, phase, reason string) error {
    _, err := c.execText(ctx, "gate-override", runID, phase, reason)
    return err
}
```

**Coldwine TUI:**
- New message type: `gateOverrideMsg { RunID, Phase, Reason, Err error }`
- In `coldwine_mode.go`, when run is in blocked state: keypress `o` → show reason input → fire `tea.Cmd` calling `cc.GateOverride()`
- On success: refresh run status (same as post-advance)
- Fallback: `ic.GateOverride()` if clavain-cli unavailable

#### 2. `submit-artifact` (Coldwine sprint flow)

**Coldwine TUI:**
- After successful `SprintAdvance()`, if the current phase produced a file artifact:
  - Call `cc.SetArtifact(beadID, artifactType, path)`
  - Artifact types: `"session-log"`, `"plan"`, `"review"` (map from phase)
- On `sprintCreatedMsg`: register the brainstorm doc if one exists
- On dispatch completion: register dispatch output as artifact

**No new message type needed** — artifact registration is a side-effect of existing advance/create flows.

## Dependencies

- `clavain-cli gate-override` subcommand must exist (check if it does)
- `ic gate override` must be functional (already defined in pkg/intercore)
- Coldwine must already have a `cclient` wired (confirmed: `coldwine.SetClavain(cclient)` in main.go)

## Open Questions

1. Should `override-gate` require a confirmation dialog, or is a keypress + reason sufficient?
2. What artifact types should Coldwine register? Session logs? Dispatch outputs? Just the phase artifact?
3. Should we also wire `start-run` into Bigend (the vision says "Bigend or Coldwine") or defer that?
4. Does `clavain-cli gate-override` exist, or does it need to be added to the clavain-cli binary?

## Success Criteria

1. All 4 vision write-path intents are callable from Coldwine TUI
2. `override-gate`: user can unblock a failed gate without leaving the TUI
3. `submit-artifact`: phase transitions in Coldwine register artifacts with the kernel
4. No regression in existing start-run and advance-run functionality
