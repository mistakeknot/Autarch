# PRD: Broadcast Confirmation Flow

## Problem
The Autarch command palette executes actions immediately on Enter — there's no safety net for broadcast actions that send to multiple agent panes. Accidental sends interrupt running work across all agents.

## Solution
Add a 3-phase confirmation flow to the existing palette: when a command is marked as broadcast, selecting it transitions through target selection (with live pane counts) and a confirmation screen before executing.

## Features

### F1: Phase-aware Palette
**What:** Extend `internal/tui/palette.go` with Phase enum (Command/Target/Confirm), Target enum (All/Claude/Codex/Gemini), and phase-specific Update/View methods.
**Acceptance criteria:**
- [x] Non-broadcast commands work exactly as before (no regression)
- [x] Broadcast commands transition Command -> Target -> Confirm -> Execute
- [x] Esc goes back one phase (Target->Command, Confirm->Target)
- [x] q/ctrl+c closes palette from any phase

### F2: Target Selection with Live Pane Counts
**What:** Target selection phase shows numbered options (1-4) with live agent pane counts fetched from tmux.
**Acceptance criteria:**
- [x] Shows "All agents (N)" with total count
- [x] Shows per-type counts: "Claude (N)", "Codex (N)", "Gemini (N)"
- [x] Pane counts fetched async when entering target phase
- [x] Graceful degradation when tmux unavailable (show targets without counts)

### F3: Confirmation Screen
**What:** Final confirmation showing command name, selected target, and pane count breakdown. Enter executes, Esc goes back.
**Acceptance criteria:**
- [x] Shows command name, target name, and count
- [x] Enter triggers the broadcast action
- [x] Esc returns to target selection
- [x] Action receives target info (which agent types to send to)

### F4: Pane Detection
**What:** Add `GetAgentPanes()` to Bigend tmux client to enumerate panes by agent type.
**Acceptance criteria:**
- [x] Returns list of AgentPane with ID, AgentType, Title
- [x] Detects Claude/Codex/Gemini/User from pane title
- [x] Returns empty list (not error) when tmux unavailable

## Non-goals
- Custom prompt input (send arbitrary text to agents) — deferred
- Per-pane targeting (send to specific pane, not type group) — deferred
- Broadcast history/undo — deferred

## Dependencies
- Existing `internal/tui/palette.go` (218 lines, well-scoped)
- Existing `internal/bigend/tmux/client.go` (tmux session management)
- NTM reference: `research/ntm/internal/palette/model.go` (Phase/Target pattern)

## Open Questions
- None remaining after brainstorm decisions
