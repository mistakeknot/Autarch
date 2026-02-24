# Autarch TUI End-to-End User Experience Review

**Reviewer:** UX Specialist (CLI/TUI)
**Date:** 2026-02-07
**Scope:** End-to-end workflow from "I have an idea" to "tasks are being monitored"
**Codebase snapshot:** commit 926aa28 (main)

---

## 1. Executive Summary

The Autarch TUI aims to unify four tools (Gurgeh, Coldwine, Pollard, Bigend) into a single tabbed interface for AI-assisted product development. The **Gurgeh onboarding flow** (kickoff, sprint, spec summary, epic/task generation) is the most developed path and provides a genuinely impressive guided experience for PRD creation. However, the **end-to-end journey breaks down badly once a user finishes onboarding** and tries to use the other three tabs.

The core problem: Gurgeh is a deep, multi-phase, agent-driven workflow. The other three tabs (Bigend, Coldwine, Pollard) are read-only data browsers with placeholder commands. The TUI promises a unified workflow but delivers one functional pipeline and three essentially empty shells. A user who completes a PRD sprint and wants to generate tasks, run research, or monitor agents will hit dead ends in every other tab.

**Verdict:** The onboarding-to-spec flow is strong. Everything after that is a dead-end experience. The gap between documented workflows and implemented TUI features is the largest UX problem.

---

## 2. Workflow Journey Map

### What Works

**Step 1: Launch and Kickoff (Good)**
- `./dev autarch tui` starts the unified shell. The Gurgeh tab auto-initializes with the kickoff view.
- The kickoff view has clear affordances: chat panel with placeholder text ("Describe what you want to build..."), scan shortcut (Ctrl+S), recent projects list.
- The doc panel provides contextual tips and shortcuts without overwhelming the user.
- `/scan` slash command is discoverable alongside the Ctrl+S keybinding.
- Source: `/root/projects/Autarch/internal/tui/views/kickoff.go` lines 76-100 (NewKickoffView), lines 118-127 (seedChat).

**Step 2: Codebase Scan (Good)**
- Ctrl+S triggers an exploration scan via Claude Code. Progress is reported in the chat panel and log pane auto-shows.
- The log pane auto-hides after 3 seconds, which is a nice touch for avoiding clutter.
- Scan results auto-populate the doc panel with vision, problem, users, platform, and language.
- Source: `/root/projects/Autarch/internal/tui/views/gurgeh_onboarding.go` lines 868-922 (scanCodebase).

**Step 3: Sprint Flow (Good)**
- After scan, the sprint view launches with 8 phases (Vision through Acceptance Criteria).
- The phase sidebar shows progress. The chat panel supports `/accept`, `/vision`, `/problem`, etc. for phase navigation.
- Sprint state persists to disk and can be resumed.
- The doc panel shows the current phase draft with evidence and quality scores.
- Source: `/root/projects/Autarch/internal/tui/views/sprint_view.go` lines 18-75 (SprintView struct and constructor).

**Step 4: Tab Navigation (Good)**
- Ctrl+Left/Right cycles tabs. `/big`, `/gur`, `/cold`, `/pol` switch directly.
- The tab bar is always visible in the header.
- The command palette (Ctrl+P) lists all available commands with fuzzy search.
- Source: `/root/projects/Autarch/internal/tui/unified_app.go` lines 416-421 (tab cycling), lines 321-334 (slash commands).

### What Breaks

**Step 5: Post-Sprint Handoff (Broken)**
- AGENTS.md documents handoff options after sprint completion: "Press R: Run full research (Pollard scan), Press T: Generate tasks (Coldwine), Press E: Export (markdown/JSON)".
- In reality, the sprint completion handler (`handleSprintComplete` at line 595 of gurgeh_onboarding.go) transitions to SpecSummaryView. The SpecSummaryView's "generate epics" callback triggers `SpecAcceptedMsg`, which calls `generateEpicsWithAgent`. This uses the coding agent to generate epics, then transitions to EpicReviewView.
- **The R/T/E keybindings documented in AGENTS.md are not implemented in the onboarding flow.** The actual post-sprint path is: sprint complete -> spec summary -> generate epics -> epic review -> generate tasks -> task review -> onboarding complete.
- There is no "run Pollard research" action from within the TUI sprint or spec summary views.

**Step 6: Coldwine Tab (Dead End)**
- After onboarding completes, the Coldwine tab shows a read-only epic/story/task browser.
- The "New Epic", "New Story", "New Task" palette commands all have `Action: func() tea.Cmd { return nil }` -- they are stubs that do nothing.
- Source: `/root/projects/Autarch/internal/tui/views/coldwine.go` lines 309-333 (Commands method with nil actions).
- The epics/stories/tasks shown come from `client.ListEpics("")` via the Intermute client. If the user just completed onboarding, these are stored in local state -- but there is no visible connection between "I just accepted tasks in onboarding" and "these tasks appear in the Coldwine tab."
- There is no way to start working on a task, change task status, create worktrees, or do anything the `docs/WORKFLOWS.md` describes for Coldwine (lines 153-225).

**Step 7: Pollard Tab (Dead End)**
- The Pollard tab shows a read-only insight browser. "Run Research" and "Link Insight" palette commands are stubs (nil actions).
- Source: `/root/projects/Autarch/internal/tui/views/pollard.go` lines 314-331 (Commands method with nil actions).
- Pollard has a rich CLI (`pollard scan`, `pollard report`, `pollard watch`), none of which is accessible from the TUI.
- The user cannot trigger a scan, configure hunters, generate reports, or link insights to specs from within the TUI.

**Step 8: Bigend Tab (Partially Implemented)**
- Shows sessions and ready tasks in a split pane. The task pane has actual selection behavior (Enter on a task calls `onTaskSelect`).
- "New Session" palette command is a stub.
- Source: `/root/projects/Autarch/internal/tui/views/bigend.go` lines 435-453 (Commands method, New Session is nil).
- Sessions load from `client.ListSessions("")` but there is no way to create, attach to, or kill sessions from the unified TUI. The documented workflow (WORKFLOWS.md lines 229-284) describes `a` to attach, `k` to kill, etc. -- these are from the old standalone Bigend TUI, not the unified tab.

---

## 3. Context Loss Points

### 3.1 Tab Switch During Sprint Discards State Silently

**Location:** `/root/projects/Autarch/internal/tui/unified_app.go` lines 621-643 (switchDashboardTab/switchToTab)

When a user is in the middle of a Gurgeh sprint (e.g., phase 4 of 8) and switches to Pollard via `/pol` or Ctrl+Right, the tab switch happens immediately. The sprint state is preserved in the Orchestrator (it auto-saves), but there is no visual confirmation that the sprint is paused, no breadcrumb showing "you were in phase 4 of Vision", and no easy way to get back to exactly where you were.

Switching back to Gurgeh (`/gur`) returns to the GurgehView, but whether it resumes the in-progress sprint or shows the kickoff/browser depends on the `showBrowser` flag state. If onboarding was still in progress (`showBrowser == false`), it should resume. If onboarding completed and `showBrowser == true`, the user sees the spec browser with no indication that a sprint was in progress.

**Impact:** Medium. Users may lose their place in a 20-40 minute sprint flow.

### 3.2 No Cross-Tab References

There are no cross-references between tabs. A spec in Gurgeh does not link to its epics in Coldwine or its research insights in Pollard. A user who creates a PRD and then switches to Coldwine has no way to see which tasks came from which spec.

The data model supports these links (e.g., `Insight.SpecID` exists in the Pollard view at `/root/projects/Autarch/internal/tui/views/pollard.go` line 251), but there is no navigation action to follow the link.

### 3.3 Onboarding Completion Transition is Jarring

**Location:** `/root/projects/Autarch/internal/tui/views/gurgeh_onboarding.go` lines 581-593 (handleTasksAccepted)

When the user accepts tasks in the final onboarding step, `OnboardingCompleteMsg` fires. The GurgehView sets `showBrowser = true` and re-emits the message. The UnifiedApp receives it but does nothing (`return a, nil` at line 442 of unified_app.go). The user is now looking at the Gurgeh spec browser, which shows specs loaded from the Intermute client. If the Intermute store is empty or slow, the user sees "No specs found" immediately after completing a full sprint.

There is no congratulatory message, no summary of what was created, no "next steps" guidance, and no automatic switch to the Bigend or Coldwine tab where the generated work would be actionable.

### 3.4 Agent Selection State is Global but Invisible Per-Tab

The agent selector (`agentSelector`, `selectedAgent`) is maintained at the UnifiedApp level and propagated to views. But the selected agent is not shown in the tab bar or footer in a way that makes it clear which agent is active. The footer shows keybinding hints but not the current agent name.

Source: `/root/projects/Autarch/internal/tui/unified_app.go` lines 733-739 (renderFooterContent -- no agent name).

---

## 4. Dead-End States

### 4.1 Coldwine "No Epics Found" With No Creation Path

When the Coldwine tab loads and `client.ListEpics("")` returns empty, the user sees "No epics found" followed by "Create an epic to break down a spec into implementable work." But there is no way to create an epic from this view. The "New Epic" command is a stub.

Source: `/root/projects/Autarch/internal/tui/views/coldwine.go` lines 230-234.

**Suggestion:** Either hide the create prompt or wire the command to actually create an epic (even if it just opens a chat-driven flow).

### 4.2 Pollard "No Insights Found" With No Scan Path

When the Pollard tab loads empty, it says "No insights found" followed by "Run Pollard hunters to gather research insights." The "Run Research" command does nothing.

Source: `/root/projects/Autarch/internal/tui/views/pollard.go` lines 220-224.

**Suggestion:** The minimum viable action would be to execute `pollard scan` in a subprocess and stream results to the log pane, similar to how the kickoff view runs Claude Code scans.

### 4.3 Bigend "No Sessions Running" / "No Tasks Ready"

The Bigend tab shows both "No sessions running -- Start a task to launch an agent" and "No tasks ready -- Complete the onboarding flow to generate tasks." These messages reference features that don't work from within the view.

Source: `/root/projects/Autarch/internal/tui/views/bigend.go` lines 297-300, 372-375.

### 4.4 Help Overlay Refers to Non-Functional Keys

The help overlay (unified_app.go lines 743-812) lists bindings like `ctrl+g Agent selector` which opens the agent selector dropdown but the selector is not always initialized. If no agents are detected, pressing Ctrl+G does nothing with no feedback.

The global help also lists `/bigend, etc. Switch to tool by name` twice in different formats (`/big /gur etc.` and `/bigend, etc.`), which is redundant and slightly confusing.

### 4.5 Unknown Slash Commands Silently Ignored

When a user types a slash command that doesn't match any handler, the UnifiedApp silently ignores it:

```go
// Unknown command - ignore silently
return a, nil
```

Source: `/root/projects/Autarch/internal/tui/unified_app.go` line 341.

The user gets no feedback that their command was not recognized. A "Unknown command: /foo. Type / to see available commands." message in the chat panel would be more helpful.

---

## 5. Missing TUI Parity with CLI

This table shows features documented in CLI or AGENTS.md/WORKFLOWS.md that have no TUI equivalent:

| Feature | CLI Command | TUI Status |
|---------|------------|------------|
| Run Pollard scan | `pollard scan` | Not implemented (stub command) |
| Generate Pollard report | `pollard report` | Not implemented |
| Configure Pollard hunters | Edit `.pollard/config.yaml` | Not implemented |
| Competitor watch mode | `pollard watch` | Not implemented |
| List specs | `gurgeh list` | Implemented (spec browser in Gurgeh tab) |
| Export spec to briefs | `gurgeh export PRD-001` | Not implemented |
| Spec version history | `gurgeh history <id>` | Not implemented |
| Spec version diff | `gurgeh diff <id> v1 v2` | Not implemented |
| Feature prioritization | `gurgeh prioritize <id>` | Not implemented |
| Coldwine task status | `coldwine status` | Partially (read-only browser) |
| Start task (worktree) | `coldwine start <id>` | Not implemented |
| Complete task | `coldwine complete <id>` | Not implemented |
| Create epic from spec | Implicit in onboarding | Not available outside onboarding |
| Bigend web dashboard | `bigend` (web server) | N/A (different interface) |
| Bigend session attach | `a` key in old TUI | Not in unified TUI |
| Bigend session kill | `k` key in old TUI | Not in unified TUI |
| Spec multi-agent review | `gurgeh review PRD-001 --gaps` | Not implemented |
| Gurgeh API server | `gurgeh serve` | Not applicable (TUI concern) |
| Pollard API server | `pollard serve` | Not applicable (TUI concern) |

### Documentation-Reality Gaps

1. **WORKFLOWS.md** (lines 449-487) describes keyboard shortcuts `?`, `q`, `j/k`, `n`, `e`, `d`, `r`, `A`, `s`, `c`, `g`, `a`, `k`, `Tab` for various tools. The unified TUI uses a completely different keybinding model (no single-letter shortcuts; everything is Ctrl+ or slash commands). The documented shortcuts are from old standalone TUIs that are now deprecated.

2. **AGENTS.md** (lines 482-527) describes post-sprint handoff with "Press R/T/E" for research/tasks/export. This is not implemented; the flow auto-continues through epic/task generation.

3. **WORKFLOWS.md Workflow 5** (lines 287-314) describes a multi-step setup process using separate CLI commands. The unified TUI's `autarch setup` auto-runs on first launch, but the user still needs to run `gurgeh init`, `pollard init`, `coldwine init` separately -- none of which are available from the TUI.

---

## 6. Recommendations (Prioritized by User Impact)

### P0: Wire At Least One Action Per Tab (High Impact, Moderate Effort)

Each non-Gurgeh tab should have at least one functional action to prevent dead-end states:

- **Pollard:** Wire "Run Research" to execute `pollard scan` in a goroutine, streaming slog output to the log pane (same pattern as codebase scan in kickoff). This is the single highest-value addition because it closes the research gap in the end-to-end workflow.
- **Coldwine:** Wire "New Epic" to prompt for a spec ID and call the epic generation agent (same as the onboarding flow's `generateEpicsWithAgent`). Alternatively, show a "From spec..." chooser that lists available specs.
- **Bigend:** Show the most recent sprint or spec status even when no sessions are running. A static summary is better than an empty state.

### P1: Show "What To Do Next" After Onboarding (High Impact, Low Effort)

When `OnboardingCompleteMsg` fires, instead of silently switching to an empty spec browser:

1. Add a system message to the Gurgeh chat panel: "PRD complete. X epics and Y tasks generated."
2. Add a follow-up message: "Next steps: /cold to review tasks, /pol to run research, /big to monitor agents."
3. Consider auto-switching to the Bigend tab, which is the natural "mission control" view.

The code change is small -- add chat messages in the `OnboardingCompleteMsg` handler at `/root/projects/Autarch/internal/tui/views/gurgeh.go` line 130.

### P1: Show "Unknown Command" Feedback (High Impact, Low Effort)

In `/root/projects/Autarch/internal/tui/unified_app.go` line 341, instead of `return a, nil`, emit a message to the current view's chat panel:

```
Unknown command: /foo -- type / to see available commands
```

This applies to both the UnifiedApp handler and the view-specific `HandleSlashCommand` fallthrough.

### P1: Remove or Update Stale WORKFLOWS.md Shortcuts (High Impact, Low Effort)

The keyboard shortcuts cheat sheet in `docs/WORKFLOWS.md` lines 449-487 references single-letter shortcuts (`n`, `e`, `d`, `A`, `s`, `c`, etc.) that belong to deprecated standalone TUIs. The unified TUI uses Ctrl+ modifiers and slash commands exclusively. Update or remove this section to avoid misleading users who read the docs.

### P2: Add Spec-to-Tab Cross-References (Medium Impact, Moderate Effort)

When the Gurgeh spec browser shows a spec, add a "Related" section:
- "Epics: 3 (view in Coldwine)" -- clickable or with `/cold` hint
- "Research: 2 insights (view in Pollard)" -- with `/pol` hint
- "Sessions: 1 active (view in Bigend)" -- with `/big` hint

This requires querying the Intermute client for related entities, which the data model already supports (specs have IDs, epics have SpecID, insights have SpecID).

### P2: Prevent Sprint State Loss on Tab Switch (Medium Impact, Moderate Effort)

When the user switches tabs during an active sprint:
1. Show a brief confirmation: "Sprint in progress (phase 4/8). Switch tab? (Ctrl+Left/Right to switch, Esc to stay)"
2. Or, show a persistent indicator in the tab bar: "Gurgeh (sprint: 4/8)" so the user knows they can return.

This was flagged by the original flux-drive review (issue checklist item in the navigation plan at `/root/projects/Autarch/docs/plans/2026-02-05-unified-tui-navigation-design.md` line 26).

### P2: Remove Placeholder Commands from Palette (Medium Impact, Low Effort)

The command palette shows "New Epic", "New Story", "New Task", "New Session", "Run Research", "Link Insight" -- all of which do nothing. This is worse than not showing them at all, because it suggests functionality that doesn't exist and erodes user trust.

Either:
- Remove stub commands until they are implemented, OR
- Change stubs to show a "Not yet implemented" message in the chat panel

### P3: Add Signal Notification Badge to Tab Bar (Low Impact, Low Effort)

The signals overlay (`/sig`) is the only way to see cross-tool signals. There is no badge or indicator in the tab bar when new signals arrive. A simple count like "Signals (3)" in the footer or a dot on the header would help discoverability.

This was also flagged in the navigation plan issues checklist (line 29).

### P3: Improve Footer Help Density (Low Impact, Low Effort)

The footer currently shows:
```
/big /gur /cold /pol /sig  ctrl+l logs  ctrl+p palette  ctrl+, settings  /help  ctrl+c*2 quit
```

This is dense but acceptable at 80+ columns. At narrower terminals, it may truncate. Consider showing only the most essential shortcuts in the footer and deferring the rest to `/help`:

```
/ commands  ctrl+left/right tabs  ctrl+p palette  ? help  ctrl+c*2 quit
```

The view-specific ShortHelp (from `currentView.ShortHelp()`) is prepended to this global help, which can make the footer very long. At 80 columns, the combined string from the sprint view would be well over 100 characters.

Source: `/root/projects/Autarch/internal/tui/unified_app.go` lines 733-739.

---

## Terminal-Specific Notes

### Color Accessibility

The Tokyo Night color palette is well-chosen for modern terminals. The code uses `lipgloss` styles throughout, which degrade to nearest colors on 16-color terminals. However, the `categoryIcon` function in Pollard uses emoji characters (line 193-206 of pollard.go: `"*"`, `"*"`, etc.) that may not render correctly in all terminal fonts. The `signalSeverityIcon` function in the overlay uses `!!` and `! ` text, which is a good accessible fallback.

### Screen Real Estate

The layout math subtracts fixed amounts for header (3 lines), footer (3 lines), and optional log pane (10 lines). At the minimum 80x24 terminal size, with log pane visible, content gets 24 - 3 - 3 - 10 = 8 lines. The 3-pane shell layout (sidebar + doc + chat) divides this further. At 80 columns, each pane would be roughly 25 characters wide, which is very tight for the spec details or chat messages.

Source: `/root/projects/Autarch/internal/tui/unified_app.go` lines 662-667 (height calculation).

### Inline vs Fullscreen

The `--inline` mode is a thoughtful addition for preserving scrollback. Log entries are dumped to stdout after exit (lines 920-925 of unified_app.go). This is valuable for debugging but the inline mode does not show the tab bar header in the same styled way (since there's no alternate screen for full-height rendering).

### Copy-Paste

The fullscreen TUI (default mode) uses the alternate screen, which makes copy-paste of output difficult. The inline mode preserves scrollback but the log dump at exit is minimal. There is no "export to clipboard" or "copy spec to clipboard" feature within the TUI. Given that spec content is the primary output of the tool, this is a meaningful gap. A `/export` slash command that writes the current spec to stdout or a file would help.

---

## Summary

| Aspect | Rating |
|--------|--------|
| Gurgeh onboarding/sprint flow | Strong |
| Tab navigation and slash commands | Strong |
| Cross-tab workflow continuity | Broken |
| Coldwine TUI functionality | Stub only |
| Pollard TUI functionality | Stub only |
| Bigend TUI functionality | Partial |
| Documentation accuracy | Out of date |
| Error feedback | Poor (silent failures) |
| Progressive disclosure | Good in onboarding, absent elsewhere |

**Overall UX impact:** The onboarding flow is a standout feature -- genuinely impressive guided PRD creation with AI assistance. But the end-to-end promise of "idea to monitored tasks" is not delivered. Three of four tabs are decorative. The top 3 changes for maximum user impact:

1. **Wire Pollard "Run Research" to an actual scan** -- closes the biggest gap in the documented workflow
2. **Show next-step guidance after onboarding completes** -- prevents the "now what?" moment
3. **Remove or disable stub commands in the palette** -- stops suggesting functionality that doesn't exist
