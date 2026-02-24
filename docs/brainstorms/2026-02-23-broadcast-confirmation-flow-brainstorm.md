# Broadcast Confirmation Flow Brainstorm

**Bead:** iv-1pkt
**Date:** 2026-02-23

## What We're Building

A 3-phase confirmation flow for broadcast actions in the Autarch command palette:
**Select Command -> Select Target -> Confirm -> Execute**

When a command is marked as a broadcast action, selecting it in the palette transitions through target selection (All/Claude/Codex/Gemini with live pane counts) then a confirmation screen before executing.

## Why This Approach

- **Accidental broadcast prevention**: Sending prompts to multiple agent panes is destructive (interrupts running work). The NTM reference implementation skips confirmation — we add it.
- **Live pane counts**: Target selection shows "All agents (3)" not just "All agents". Requires pane enumeration by agent type from Bigend's tmux client.
- **Palette-level, not view-level**: Broadcast commands appear in the unified palette regardless of active tab. Any view can trigger broadcast. This matches how `unified_app.go` already aggregates commands from all views.

## Key Decisions

1. **Always confirm** — every broadcast action gets a confirmation screen showing command name, target, and pane count breakdown. Enter confirms, Esc goes back to target selection.

2. **Extend existing Palette** — add `Phase`, `Target`, `paneCounts` fields to `internal/tui/palette.go`. One component with phase-aware Update/View methods. No separate BroadcastPalette.

3. **Bigend tmux client for pane detection** — add `GetAgentPanes(session) ([]AgentPane, error)` to `internal/bigend/tmux/client.go`. Detects agent type from pane title convention.

4. **Scope: all views via unified palette** — broadcast commands are registered at the palette level (in `unified_app.go`), not per-view. Views keep their existing single-action commands unchanged.

## Architecture

### Phase Flow

```
PhaseCommand (existing)         PhaseTarget (new)           PhaseConfirm (new)
┌─────────────────────┐        ┌─────────────────────┐     ┌─────────────────────┐
│ > fuzzy search...   │ enter  │ Select target:      │ 1-4 │ Command: X          │ enter
│                     │ ────→  │                     │ ──→  │ Target: All (3)     │ ────→ Execute
│ 1. Run Research     │        │ 1. All agents (3)   │      │ Claude(1) Codex(1)  │
│ 2. Send Prompt      │        │ 2. Claude (1)       │      │                     │
│ 3. Refresh Sessions │        │ 3. Codex (1)        │      │ enter confirm       │
│                     │        │ 4. Gemini (1)       │      │ esc back  q cancel  │
│ esc close           │        │ esc back            │      │                     │
└─────────────────────┘        └─────────────────────┘     └─────────────────────┘
```

Non-broadcast commands skip PhaseTarget/PhaseConfirm entirely (existing behavior).

### Command Extension

```go
type Command struct {
    Name        string
    Description string
    Action      func() tea.Cmd
    Broadcast   bool           // NEW: enables target+confirm phases
}
```

When `Broadcast=true`, `Enter` in PhaseCommand transitions to PhaseTarget instead of calling Action directly.

### Pane Detection

```go
// internal/bigend/tmux/client.go
type AgentPane struct {
    ID        string
    AgentType string  // "claude", "codex", "gemini", "user"
    Title     string
}

func (c *Client) GetAgentPanes(session string) ([]AgentPane, error)
```

Agent type detected from pane title format (same convention as NTM: title contains agent identifier).

### Palette Pane Count Refresh

Pane counts are fetched asynchronously when the palette enters PhaseTarget (via tea.Cmd). Cached for the palette session — refreshed each time palette opens.

## Open Questions

- What specific broadcast commands ship initially? Candidates: "Send Prompt to Agents", "Stop All Agents". Or just mark existing per-view commands as broadcastable?
- Should the confirmation screen show a text preview of what will be sent (for prompt commands)?

## Reference

- NTM palette: `research/ntm/internal/palette/model.go` — Phase/Target enums, paneCounts, viewTargetPhase rendering
- Autarch palette: `internal/tui/palette.go` — current single-phase palette (218 lines)
- Coldwine confirm: `internal/coldwine/tui/model.go` — PendingApproveTask y/n pattern
