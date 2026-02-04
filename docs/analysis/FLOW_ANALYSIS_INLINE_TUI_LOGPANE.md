# User Flow Analysis: Inline TUI Mode with Dedicated Log Pane

**Analyst:** Claude Code UX Flow Analysis
**Date:** February 4, 2026
**Specification:** FrankenTUI-inspired inline mode with log pane for Autarch TUIs
**Scope:** All user journeys, edge cases, gaps, and clarifying questions

---

## Executive Summary

The inline TUI mode with log pane feature adds real-time agent operation visibility to Autarch's existing TUI interfaces. While the feature has solid architectural foundation documents (INLINE_MODE_SUMMARY.md, INLINE_MODE_ARCHITECTURE.md, etc.), this analysis identifies **23 critical gaps** requiring clarification before implementation, organized into 5 categories:

1. **Flag Behavior & Activation** (3 gaps)
2. **Log Routing & Message Flow** (6 gaps)
3. **UI State Management & Transitions** (5 gaps)
4. **Terminal Recovery & Safety** (5 gaps)
5. **Performance, Memory & Resource Constraints** (4 gaps)

---

## Part 1: User Flow Overview

### Flow 1: First-Time User Enables Inline Mode

**Entry Point:** User runs tool with `--inline` flag

```
User runs: gurgeh --inline
    ↓
TUI initializes with log pane visible
    ↓
User sees split layout:
  ├─ Top: Main interface (list/detail/interview)
  ├─ Separator
  └─ Bottom: Empty log pane (ready for logs)
    ↓
User navigates/launches agent action
    ↓
Logs stream into pane in real-time
    ↓
User completes action → Logs persist in scrollback
    ↓
User exits (Ctrl+C) → Terminal restored to normal
```

**Duration:** Entire session (potentially 30+ minutes for sprint)
**Variants:** Each tool (Gurgeh, Coldwine, Pollard)
**Success Criteria:** Logs visible, terminal usable after exit

---

### Flow 2: User Toggles Log Pane Visibility Mid-Session

**Entry Point:** User presses hotkey (e.g., `L`) during active session

```
TUI displaying split layout with logs visible
    ↓
User presses `L` (toggle)
    ↓
Log pane collapses → Main view expands to full width
    ↓
User works with more screen space
    ↓
User presses `L` again
    ↓
Log pane reappears → Main view shrinks to make room
    ↓
Scroll position preserved? History still accessible?
```

**Duration:** Seconds per toggle
**Context:** During interview, research scan, or task definition
**Uncertainty:** Scroll position recovery, history access when hidden

---

### Flow 3: TUI Crashes During Agent Operation

**Entry Point:** Unexpected error in agent code

```
Agent spawned, logs streaming
    ↓
Agent encounters panic/error
    ↓
TUI receives error message
    ↓
TUI crashes (unhandled exception in Update)
    ↓
Terminal left in alternate screen mode (BROKEN STATE)
    ↓
User tries to type → No visible response
    ↓
User force-quits (Ctrl+C)
    ↓
Terminal restored? Or still broken?
```

**Duration:** Seconds before user notices
**Severity:** High — breaks user's terminal session
**Uncertainty:** Panic recovery mechanism coverage, signal handler reliability

---

### Flow 4: User Scrolls Through Large Log Buffer

**Entry Point:** Session with many operations (500+ log entries)

```
Log pane fills with entries (circular buffer at 500)
    ↓
User scrolls up to find earlier log
    ↓
Scroll to top of visible logs
    ↓
Top entries are from middle of operation (older entries dropped)
    ↓
User cannot see first action that triggered logs
    ↓
User tries to scroll past beginning
    ↓
What happens? Error message? Silent limit?
```

**Duration:** Throughout session
**Context:** Long-running sprints, multiple scans
**Uncertainty:** Circular buffer behavior when scrolling, UX feedback

---

### Flow 5: User Filters Logs by Level

**Entry Point:** User applies filter in log pane

```
Log pane shows all entries (debug, info, warn, error)
    ↓
User presses `F` (filter)
    ↓
Filter menu appears:
  ├─ All
  ├─ Error only
  ├─ Warn+Error
  └─ Info+Warn+Error
    ↓
User selects "Error only"
    ↓
Log pane updates, showing only errors
    ↓
Some entries disappear from view
    ↓
Does hidden filtered content still scroll exist below?
Or is it actually removed from the buffer?
    ↓
User changes filter back to "All"
    ↓
Do hidden entries reappear?
```

**Duration:** Seconds per filter toggle
**Context:** Rapid problem diagnosis during failures
**Uncertainty:** Filter implementation (view vs. buffer), state recovery

---

### Flow 6: User Exits TUI Mid-Operation

**Entry Point:** Ctrl+C pressed while agent actively logging

```
Agent spawning/running, logs flowing at high rate
    ↓
User presses Ctrl+C
    ↓
SIGINT signal received
    ↓
Bubble Tea cleanup begins
    ↓
Alternate screen disabled
    ↓
Cursor restored
    ↓
Scrollback visible on terminal
    ↓
User can run next command
```

**Duration:** <1 second
**Context:** Common exit flow
**Uncertainty:** Message buffering during shutdown, log loss prevention

---

### Flow 7: User Switches Between Tools Without Closing TUI

**Entry Point:** Multi-tool Bigend TUI with inline mode

```
Gurgeh tab active with logs streaming
    ↓
Logs accumulated in pane
    ↓
User presses Tab key (tool switch) or clicks Coldwine tab
    ↓
Coldwine view becomes active
    ↓
Log pane updates to show... what?
  ├─ Gurgeh logs still visible?
  ├─ New Coldwine logs only?
  ├─ Mixed timeline?
    ↓
User switches back to Gurgeh
    ↓
Previous Gurgeh logs still there?
Or cleared when switched away?
```

**Duration:** Throughout session
**Context:** Bigend multi-tool orchestration
**Uncertainty:** Per-tool vs. unified log buffer, state isolation

---

### Flow 8: User Resizes Terminal While Logs Streaming

**Entry Point:** Window resize during active logging

```
TUI running with active agent
    ↓
User resizes terminal window
    ↓
SIGWINCH signal received
    ↓
Bubble Tea recalculates dimensions
    ↓
Layout reflows:
  ├─ Sidebar width adjusted?
  ├─ Main view width adjusted?
  ├─ Log pane height adjusted?
    ↓
Existing log entries re-wrapped or reflow?
    ↓
Scroll position maintained or reset?
    ↓
Active log stream continues uninterrupted?
```

**Duration:** Seconds
**Context:** Common during long operations
**Uncertainty:** Log pane viewport recalculation, content reflow

---

### Flow 9: Network/API Error During Agent Operation

**Entry Point:** External service (Intermute, Pollard, etc.) fails

```
Agent running, logging progress
    ↓
External API returns 500 error
    ↓
Agent logs error:
  "error: failed to fetch from intermute: connection refused"
    ↓
Error appears in log pane with level=ERROR, color=red
    ↓
Does the error also trigger a TUI-level error state?
Or just logged?
    ↓
Does the main view show error separately?
Or only in logs?
    ↓
Can user retry from logs view, or must use main interface?
```

**Duration:** 1-5 seconds for error to appear
**Context:** Real-world production issues
**Uncertainty:** Error routing (logs vs. UI state), retry flow

---

### Flow 10: Concurrent Operations (Scan + Interview)

**Entry Point:** User starts interview while Pollard scan runs (Coldwine)

```
Coldwine TUI open, task interview active
    ↓
User triggers:
  ├─ CUJ interview (logs: "generating questions...")
  └─ Concurrent Pollard scan (logs: "querying OpenAlex...")
    ↓
Log pane receives entries from both:
  • [interview] Generating user journey...
  • [scan] OpenAlex: found 15 papers
  • [interview] Structuring acceptance criteria...
  • [scan] Cross-referencing with GitHub...
    ↓
Timeline interleaved (chronological order)?
Or separated by source?
    ↓
Scroll position updates for each log?
Or batch updates?
    ↓
User tries to scroll while both active
    ↓
What's the behavior? Race condition possible?
```

**Duration:** Throughout operation
**Context:** Real-world multi-task usage
**Uncertainty:** Concurrency safety, log ordering, scroll behavior under load

---

## Part 2: Flow Permutations Matrix

### Dimension 1: Activation Mode

| Mode | Entry Point | Default Log Level | Pane Visibility | Use Case |
|------|-------------|-------------------|-----------------|----------|
| **--inline** | CLI flag | INFO | Always visible | Default for logging |
| **Default (no flag)** | Normal TUI | ERROR | Hidden | Less verbose |
| **--inline --verbose** | Flag combo | DEBUG | Always visible | Deep debugging |
| **--inline --quiet** | Flag combo | WARN | Always visible | Minimal noise |

**Gap:** Are all four combinations tested? What's the default when no flag given?

---

### Dimension 2: Tool/Context

| Tool | Log Sources | State Persistence | Multi-window? |
|------|-------------|-------------------|---------------|
| **Gurgeh** | agent, arbiter, interview, scan | Per-spec history? | Single window |
| **Coldwine** | interview, scan, task-gen | Per-task history? | Single window |
| **Pollard** | hunter, scan, report-gen | Per-run history? | Single window |
| **Bigend** | aggregator, discovery, tmux | Unified log stream | Multi-tab |

**Gap:** Is log history persisted per-tool? Unified across tools in Bigend?

---

### Dimension 3: User State

| State | Inline Active | Log Buffer | Focus | Action |
|-------|---------------|-----------|-------|--------|
| **First-run** | Must opt-in (--inline) | Empty | Main view | See logs appear |
| **Returning** | Persists from prior session? | Carry over? | Last position? | Resume where left |
| **Error state** | Still active? | Error logs added? | Auto-scroll to error? | Diagnosis aid |
| **Completed** | View historical logs? | Persist after exit? | Can export? | Audit trail |

**Gap:** What state persists between sessions? Is logs file saved?

---

### Dimension 4: Terminal Context

| Context | Alt Screen | Scrollback | Signal Handlers | Recovery |
|---------|-----------|-----------|-----------------|----------|
| **Normal exit (Ctrl+C)** | Disabled | Visible | SIGINT | Automatic |
| **Panic in TUI** | Still active | Lost? | Not triggered? | defer recovery.Recover() |
| **Panic in agent** | Still active | Captured? | Not agent's problem | Logs show error? |
| **Kill -9** | Broken | N/A | Not triggered | Manual terminal fix |

**Gap:** How are panics in the agent code handled? Can users recover?

---

### Dimension 5: Log Volume/Performance

| Volume | Duration | Entries/Sec | Behavior | Risk |
|--------|----------|-------------|----------|------|
| **Light** | 5 min | <1 | Smooth | None |
| **Medium** | 20 min | 10–50 | Circular buffer rotation | Oldest entries drop |
| **Heavy** | 45+ min | 100+ | Potential lag | TUI responsiveness? |
| **Burst** | Seconds | 500+ | Channel buffer full | Drop messages? |

**Gap:** What's the actual performance at 100+ logs/sec? Does TUI lag?

---

## Part 3: Missing Elements & Gaps

### Category A: Flag Behavior & Activation

#### Gap A1: Default Behavior Without `--inline` Flag

**Description:** Specification states "opt-in activation" via `--inline` flag, but unclear what happens without it.

**Current Ambiguity:**
- Is log pane completely hidden without the flag?
- Are logs still being captured internally but not displayed?
- Does absence of `--inline` suppress the log handler entirely (for performance)?
- Can users enable inline mode mid-session if they start without the flag?

**Impact:** Users cannot decide between logging levels without restarting TUI.

**Assumptions I'd Make:**
- Without `--inline`, log pane is absent and slog handler is NOT created
- Log output goes nowhere (matches current behavior)
- Flag is must be set at startup; no runtime toggle

**Examples:**
```bash
gurgeh                    # Current behavior—no logs
gurgeh --inline          # New behavior—logs visible
gurgeh --inline --debug  # Logs at debug level?
```

**Question:** Should log level be adjustable without restarting, or is restart acceptable?

---

#### Gap A2: Interaction with Other Flags

**Description:** How does `--inline` compose with existing CLI flags?

**Current Ambiguity:**
- What if user runs `gurgeh --inline --verbose`? (Hypothetical; `--verbose` may not exist)
- Does `--inline` override/interact with env var LOG_LEVEL?
- Precedence: CLI flag > env var > default?
- Can user do `gurgeh --inline --log-level=debug`?

**Impact:** Unexpected log output if flags conflict.

**Assumptions I'd Make:**
- Precedence: CLI flag > env var > default (standard Go practice)
- `--inline` just enables the pane; separate flag controls level

**Examples:**
```bash
LOG_LEVEL=debug gurgeh --inline  # debug logs shown?
gurgeh --inline --log-level=warn  # warn+ logs shown?
```

**Question:** What's the exact precedence and composability of flags?

---

#### Gap A3: Persistence Across Sessions

**Description:** Does the `--inline` flag affect subsequent invocations?

**Current Ambiguity:**
- If user runs `gurgeh --inline`, does next invocation remember the flag?
- Is there a `.gurgeh/config` that stores "always use inline"?
- Can user set a default in global config (`~/.config/autarch/autarch.toml`)?

**Impact:** Friction if users want inline as default but must specify flag every time.

**Assumptions I'd Make:**
- Flag is per-invocation only
- No config persistence across sessions
- Users must type `--inline` each time

**Examples:**
```bash
gurgeh --inline                    # Session 1: enables inline
gurgeh                             # Session 2: inline NOT enabled
echo 'inline = true' >> ~/.config/autarch/autarch.toml  # Persist default?
```

**Question:** Should users be able to set `--inline` as a persistent default?

---

### Category B: Log Routing & Message Flow

#### Gap B1: Log Source Attribution

**Description:** How are logs tagged with their source (agent, scan, system, etc.)?

**Current Ambiguity:**
- Does the slog context automatically capture source, or must agents explicitly set it?
- If agent logs `slog.Info("generating...")`, how do we know it's from the "agent"?
- Are we parsing logger names, or using slog context attributes?
- How do we distinguish between:
  - `slog.Info("...")` in Gurgeh's arbiter
  - `slog.Info("...")` in Pollard's hunter
  - `slog.Info("...")` in Coldwine's interview

**Impact:** Log filtering/searching by source is broken if attribution is unclear.

**Assumptions I'd Make:**
- LogHandler extracts source from slog Record's logger name (e.g., `"gurgeh.arbiter"`)
- Or, agents explicitly set slog context: `slog.With("source", "agent")`
- Source defaults to "system" if not provided

**Examples:**
```go
// In arbiter.go
logger := slog.With("source", "arbiter")
logger.Info("proposing requirements")  // → LogMsg{Source: "arbiter", ...}

// In hunter.go (Pollard)
logger := slog.With("source", "hunter")
logger.Info("found 15 papers")  // → LogMsg{Source: "hunter", ...}
```

**Question:** What's the contract for how agents provide source attribution?

---

#### Gap B2: Non-Blocking Send Semantics

**Description:** When the log buffer is full, what happens?

**Current Proposal:** Non-blocking send with drop-oldest behavior.

**Current Ambiguity:**
- Which messages are dropped? Oldest? Or newest incoming?
- Is there user feedback when a message is dropped?
- Can the drop be observed/logged?
- What's the buffer size? (Proposal: 500 entries, but is this bytes or count?)

**Impact:**
- User may miss critical error logs if buffer fills
- No visibility into dropped messages = silent data loss

**Assumptions I'd Make:**
- 500 is entry count, not byte size
- Oldest entries dropped when buffer reaches capacity
- Drop is silent (no visual warning in pane)
- Once an entry is dropped, it cannot be recovered

**Examples:**
```
Circular buffer at capacity (500 entries)
    ↓
New log arrives: LogMsg{Level: "error", Message: "critical failure"}
    ↓
Oldest entry (may be important!) is dropped
    ↓
New error added
    ↓
User scrolls to find old error: NOT THERE
    ↓
User has no idea it was dropped
```

**Question:** Should dropped messages trigger a visual indicator (e.g., "[... 47 messages dropped ...]")?

---

#### Gap B3: Channel vs. Viewport Buffering

**Description:** Is buffering at the channel level or viewport level?

**Current Ambiguity:**
- LogMsg flows: agent → slog handler → non-blocking send to channel
- What happens to a message that lands while the TUI is updating?
- Is the channel buffered (current proposal: yes, 100 entries)?
- Or is buffering only in the LogPane's entries[] slice?
- Can a message be lost between channel send and LogPane.Update()?

**Impact:** Messages could be silently lost if synchronization is wrong.

**Assumptions I'd Make:**
- Channel has a small buffer (100 entries?)
- If channel full, message is dropped (non-blocking)
- LogPane reads from channel and appends to entries[]
- No message should be lost between handler and viewport

**Examples:**
```go
// In app.Update(), we receive from channel
case msg := <-logMsgChan:
    logPane.Update(msg)

// If channel is buffered (100), up to 100 messages can queue
// If more than 100 arrive before Update() reads, extras are dropped
```

**Question:** Is the channel size configurable, or fixed at 100?

---

#### Gap B4: Async Agent Output Capture

**Description:** How are agent logs captured if agent runs in a goroutine?

**Current Ambiguity:**
- Agent code calls `slog.Info(...)` in a background goroutine
- Handler.Handle() is called from that goroutine
- Is the non-blocking send thread-safe?
- Can two agents log simultaneously?
- What if the TUI thread is updating LogPane while agent is logging?

**Impact:** Race conditions if concurrent logging is unsynchronized.

**Assumptions I'd Make:**
- Sending to a buffered channel is atomic in Go (safe)
- LogPane.Update() is only called from TUI thread
- No mutex needed if we only use channel + TUI's single-thread model

**Examples:**
```go
// Goroutine 1: TUI thread
func (app *App) Update(msg tea.Msg) {
    case logMsg := msg.(type):
        logPane.entries = append(logPane.entries, ...)  // TUI thread only
}

// Goroutine 2: Agent thread
func (agent *Agent) interview() {
    slog.Info("starting interview")
    // → handler.Handle() → send(LogMsg)  // Just sends to channel, no lock needed
}
```

**Question:** Is the channel-based approach sufficient, or do we need explicit synchronization?

---

#### Gap B5: Log Loss During Shutdown

**Description:** What happens to in-flight log messages when user exits?

**Current Ambiguity:**
- User presses Ctrl+C
- Bubble Tea shutdown begins
- Are pending log messages in the channel processed before exit?
- Can messages be lost if TUI stops before draining the channel?

**Impact:** Last few log messages may disappear, losing important final state.

**Assumptions I'd Make:**
- Shutdown is synchronous: Bubble Tea waits for Update() to finish
- Any pending messages in channel are lost
- No graceful shutdown of agent goroutines

**Examples:**
```
Agent logging at high rate
    ↓
User presses Ctrl+C
    ↓
Bubble Tea.Run() begins cleanup
    ↓
Does it process remaining channel messages? Or exit immediately?
```

**Question:** Should there be a graceful shutdown that drains pending messages?

---

#### Gap B6: LogHandler Integration with slog.Record Attributes

**Description:** How are structured slog attributes (key-value pairs) handled?

**Current Ambiguity:**
- slog.Info("message", "key1", "val1", "key2", "val2") includes attributes
- Does LogMsg.Message contain the full formatted output, or just the message?
- Are attributes displayed separately, or merged into the message?
- What about nested attributes or complex values?

**Impact:** Log output may be unstructured or hard to parse.

**Assumptions I'd Make:**
- LogMsg.Message contains the formatted output string
- Attributes are rendered inline (not stored separately)
- Complex values are stringified

**Examples:**
```go
slog.Info("processing", "count", 42, "status", "complete")
// LogMsg{Message: "processing count=42 status=complete"} ?
// Or LogMsg{Message: "processing"} with separate attributes?
```

**Question:** How are slog attributes represented in the LogMsg?

---

### Category C: UI State Management & Transitions

#### Gap C1: Scroll Position When Toggling Pane Visibility

**Description:** When user collapses/expands log pane, what happens to scroll?

**Current Ambiguity:**
- Pane is visible and user has scrolled to line 100
- User presses 'L' to hide the pane
- Main view expands to fill the space
- User presses 'L' again to show pane
- Does the pane re-appear at line 100 (scroll preserved)?
- Or does it scroll to bottom (latest entries)?
- Or does it reset to top?

**Impact:** Confusing UX if scroll state is lost.

**Assumptions I'd Make:**
- LogPane stores scroll position separately from visibility
- When re-shown, scroll position is restored
- Or, always scroll to bottom on re-show (better UX for logs)

**Examples:**
```
User scrolls to line 100 in log pane
    ↓
Presses 'L' (hide)
    ↓
Presses 'L' (show)
    ↓
Scroll position restored? Or jump to newest?
```

**Question:** Should toggling pane visibility preserve scroll position or jump to latest?

---

#### Gap C2: Filter State Across Visibility Toggles

**Description:** When user hides/shows pane, does filter persist?

**Current Ambiguity:**
- User sets filter to "error only" (3 visible entries out of 50)
- User hides pane with 'L'
- User shows pane with 'L' again
- Does the "error only" filter still apply?
- Or does it reset to "show all"?

**Impact:** Filtering is fragile if state is not preserved.

**Assumptions I'd Make:**
- Filter state is independent of visibility
- Toggling visibility does NOT change the filter
- Filter persists until explicitly changed

**Examples:**
```
Filter: "error only" (3 entries visible)
    ↓
Hide pane (L)
    ↓
Show pane (L)
    ↓
Still showing "error only"? Or reset to "all"?
```

**Question:** Should visibility toggle preserve or reset the filter?

---

#### Gap C3: Multi-Tool Log Stream Isolation (Bigend)

**Description:** In Bigend's multi-tool TUI, how are logs isolated per tool?

**Current Ambiguity:**
- Bigend has tabs: Gurgeh, Coldwine, Pollard
- Each tool emits logs to the same slog handler
- Does Bigend have one shared log pane for all tools?
- Or separate log pane per tool tab?
- If shared, how are logs attributed to tools?
- Can user filter by tool?

**Impact:** Log pane becomes unusable if tool-specific logs are mixed without attribution.

**Assumptions I'd Make:**
- Bigend has ONE shared log pane at bottom
- All tools' logs appear in chronological order
- Source attribution includes tool name (e.g., "gurgeh.arbiter", "coldwine.interview")
- User can filter by tool using source filter

**Examples:**
```
Gurgeh tab logs:
  • [gurgeh.arbiter] Proposing requirements

Coldwine tab logs:
  • [coldwine.interview] Generating questions

Bigend log pane (shared):
  • [gurgeh.arbiter] Proposing requirements
  • [coldwine.interview] Generating questions
  • [gurgeh.arbiter] Consistency check passed
```

**Question:** Should Bigend have separate log panes per tool or one unified pane?

---

#### Gap C4: Log Pane Focus & Navigation Conflicts

**Description:** When log pane is focused, how do nav keys behave?

**Current Ambiguity:**
- TUI has multiple focusable regions: sidebar, main view, log pane
- User can 'Tab' between them
- When log pane is focused:
  - Do 'j'/'k' scroll logs, or do they navigate the main view underneath?
  - Does 'enter' do something in the log pane?
  - Can user copy log text?

**Impact:** Keyboard navigation is ambiguous and frustrating.

**Assumptions I'd Make:**
- When log pane is focused, 'j'/'k' scroll the log viewport
- Other keys (like 'n' for new spec) are ignored
- 'Tab' moves focus away from log pane
- No text selection in log pane (at least in MVP)

**Examples:**
```
Sidebar | Main View
   ↓    |    ↓
Focus: Sidebar
    ↓
Press 'Tab'
    ↓
Focus: Main View
    ↓
Press 'Tab'
    ↓
Focus: Log Pane
    ↓
Press 'j' (scroll logs down, not navigate main view)
```

**Question:** What's the focus model? And how do keybindings work when log pane is focused?

---

#### Gap C5: Transition to Error State

**Description:** When an agent error occurs, how does the TUI respond?

**Current Ambiguity:**
- Agent logs an error: `slog.Error("failed to connect", "err", someErr)`
- Error appears in log pane (red, level=ERROR)
- Does the main view also show an error state?
- Is there a modal overlay, or just a color change?
- Can user dismiss the error?
- Does the error affect the TUI's ability to accept more input?

**Impact:** Users may not realize an error occurred if it's only in logs.

**Assumptions I'd Make:**
- Error logs are displayed in red in the pane
- Main view status bar also shows an error indicator
- User can continue using TUI (not blocked)
- No modal overlay (too disruptive)

**Examples:**
```
Interview active
    ↓
Agent hits error, logs: "failed to fetch from intermute"
    ↓
Log pane shows error in red
    ↓
Main view status line also shows: "[ERROR] failed to fetch..."
    ↓
User can retry or continue editing manually
```

**Question:** Should errors in logs trigger a separate TUI-level error state?

---

### Category D: Terminal Recovery & Safety

#### Gap D1: Panic Recovery Coverage

**Description:** Which panics are caught and which leave the terminal broken?

**Current Ambiguity:**
- Proposal: defer recovery.Recover() in main()
- But where exactly does this defer go?
  - In `cmd/gurgeh/main.go`?
  - In `internal/gurgeh/cli/root.go`?
  - In `internal/tui/app.go` Run() function?
- Does it cover:
  - Panics in TUI Update()? (YES, in main defer)
  - Panics in agent goroutines? (NO, agent runs independently)
  - Panics in message handlers? (YES, if in TUI thread)

**Impact:** If an agent panics, the terminal may be left in a broken state.

**Assumptions I'd Make:**
- defer recovery.Recover() is in the TUI entry point (not in agent code)
- It catches panics in the TUI thread only
- Agent panics are NOT caught (agent's responsibility)

**Examples:**
```go
// In cmd/gurgeh/main.go
func main() {
    defer recovery.Recover()  // Catches panics in TUI

    m := tui.NewModel()
    p := tea.NewProgram(m)
    p.Run()  // If TUI panics here, recovery catches it
}

// In agent.go (other goroutine)
func (a *Agent) interview() {
    // If this panics, recovery doesn't catch it
    // But Bubble Tea's alt screen is already set up, so terminal is still broken
}
```

**Question:** Should panics in agent code trigger their own recovery, or is TUI-level enough?

---

#### Gap D2: Signal Handler Interaction with Bubble Tea

**Description:** When should explicit signal handlers be added?

**Current Ambiguity:**
- Bubble Tea's default handles SIGINT/SIGTERM → exits cleanly
- Proposal: add explicit signal handlers for terminal restore
- But Bubble Tea's cleanup already restores terminal
- So why add explicit handlers?
- What do they do that Bubble Tea doesn't?

**Impact:** Over-engineered cleanup that conflicts with Bubble Tea.

**Assumptions I'd Make:**
- Bubble Tea handles terminal restore on exit
- Explicit signal handlers are NOT needed in MVP
- They can be added later if we need custom cleanup logic

**Examples:**
```go
// Unnecessary (Bubble Tea does this)
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
go func() {
    <-sigChan
    recovery.RestoreTerminal()  // Bubble Tea already does this
    os.Exit(0)
}()

// Sufficient (just let Bubble Tea handle it)
p := tea.NewProgram(m, tea.WithAltScreen())
p.Run()  // Cleans up on exit
```

**Question:** Are explicit signal handlers needed, or does Bubble Tea suffice?

---

#### Gap D3: Stale Goroutine Cleanup

**Description:** What happens to agent goroutines when TUI exits?

**Current Ambiguity:**
- Agent spawns goroutines to run interview, scan, etc.
- User exits TUI with Ctrl+C
- Do agent goroutines continue running?
- Are they explicitly cancelled, or left to run?
- If left running, do they eventually exit, or leak?

**Impact:** Goroutine leaks could accumulate if sessions are short.

**Assumptions I'd Make:**
- Agent goroutines are NOT explicitly cancelled on TUI exit
- They continue running in the background
- Eventually exit when their work completes or context expires
- This is acceptable for MVP

**Examples:**
```
TUI running for 30 minutes
    ↓
Agent spawns 5 long-running scans
    ↓
User exits TUI after 5 minutes
    ↓
Scans continue in the background (goroutines not cancelled)
    ↓
Eventually complete and exit (memory freed)
    ↓
Or do they leak?
```

**Question:** Should TUI exit explicitly cancel agent goroutines?

---

#### Gap D4: Alt Screen State on Abnormal Exit

**Description:** Can the TUI's alt screen mode be left active if exit is abnormal?

**Current Ambiguity:**
- User runs `gurgeh --inline`
- TUI starts, alt screen enabled
- Process is killed with `kill -9` (or crashes hard)
- Alt screen mode is left active
- Terminal shows garbage or nothing
- User must manually reset with `reset` command

**Impact:** Users need to know to run `reset` to recover terminal.

**Assumptions I'd Make:**
- `kill -9` is unrecoverable (no cleanup possible)
- Graceful exit (Ctrl+C, panic recovery) restores terminal
- Hard crash leaves alt screen active

**Examples:**
```
Terminal state after kill -9:
- Alt screen still active
- Cursor hidden
- Terminal appears frozen or blank
- User types `reset<enter>`
- Terminal restored
```

**Question:** Should documentation warn about `kill -9` and recovery steps?

---

#### Gap D5: Ctrl+C Handling During Message Processing

**Description:** What if Ctrl+C is pressed while Update() is running?

**Current Ambiguity:**
- TUI is processing a LogMsg
- LogPane.Update() is appending to entries[] and recalculating viewport
- User presses Ctrl+C
- Bubble Tea signals exit
- Does Update() finish first, or is it interrupted?
- Can LogPane be left in an inconsistent state?

**Impact:** Potential data corruption or visual artifacts on exit.

**Assumptions I'd Make:**
- Bubble Tea waits for Update() to finish before exiting
- Ctrl+C is queued and processed after current Update() completes
- No interruption mid-Update()

**Examples:**
```
LogPane.Update(logMsg) running
    ↓
    Appending to entries[]
    ↓
    Recalculating viewport layout
    ↓
User presses Ctrl+C
    ↓
Ctrl+C is queued (not processed yet)
    ↓
Update() finishes
    ↓
Next Update() receives Ctrl+C message
    ↓
Bubble Tea cleanup begins
```

**Question:** Does Bubble Tea guarantee that Update() completes before exit?

---

### Category E: Performance, Memory & Resource Constraints

#### Gap E1: Circular Buffer Sizing

**Description:** Is 500 entries the right size?

**Current Ambiguity:**
- Proposal: 500-entry circular buffer
- At what capacity does the oldest entry get dropped?
- Is this 500 entries (count) or 500 KB (bytes)?
- What's the memory impact? (Likely <1 MB per entry)
- Is this configurable?

**Impact:** If too small, users lose important logs; if too large, memory bloat.

**Assumptions I'd Make:**
- 500 is entry count, not byte size
- Each entry is ~500 bytes (level, source, message, timestamp)
- Total memory: 500 * 500 = 250 KB (negligible)
- Size is hardcoded in MVP (not configurable)

**Examples:**
```
500 entries * ~500 bytes = 250 KB
• negligible memory impact
• 30-minute session at 10 logs/sec = 18,000 entries (only 500 visible)
• oldest entries continuously dropped
```

**Question:** Is 500 entries sufficient, or should it be adjustable?

---

#### Gap E2: Update Rate Ceiling

**Description:** How many log entries per second can the TUI handle?

**Current Ambiguity:**
- Agent logs at high rate: 100 messages/sec
- TUI thread processes LogMsg and updates viewport
- Viewport recalculation (re-wrapping text) is expensive
- At what rate does the TUI lag?
- Should there be a throttle or batch updates?

**Impact:** TUI becomes unresponsive during high-volume logging.

**Assumptions I'd Make:**
- No batching in MVP
- Each LogMsg triggers a viewport update
- TUI can handle ~50 logs/sec comfortably
- At 100+ logs/sec, there may be visible lag

**Examples:**
```
Agent logging at 10 logs/sec → smooth TUI
Agent logging at 50 logs/sec → still responsive
Agent logging at 100+ logs/sec → noticeable lag
```

**Question:** Should there be a throttle or batch update mechanism?

---

#### Gap E3: Scrolling Performance with Large Buffer

**Description:** Does scrolling through 500 entries lag the TUI?

**Current Ambiguity:**
- LogPane contains 500 entries
- User scrolls up with 'k' (10 lines per keystroke)
- Does the viewport need to re-render all 500 entries?
- Or does it use lazy rendering?
- At what point does scrolling become slow?

**Impact:** Scrolling becomes unusable if viewport rendering is inefficient.

**Assumptions I'd Make:**
- Viewport renders only visible lines (lazy rendering)
- Scrolling is efficient (O(1) to change scroll position)
- Text re-wrapping happens on demand
- No performance issue expected

**Examples:**
```
500 entries, viewport height = 5 lines
    ↓
User presses 'k' (scroll up)
    ↓
Viewport shifts by ~1 line
    ↓
Only the newly visible line is re-rendered
    ↓
Previous rendering cached?
```

**Question:** Is the viewport rendering lazy, or does it re-render all 500 entries on each scroll?

---

#### Gap E4: Memory Impact of High-Frequency Filtering

**Description:** Does filtering by level require rebuilding the buffer?

**Current Ambiguity:**
- LogPane has 500 entries (all levels)
- User applies filter: "error only" (50 entries match)
- Is this a view filter (hides entries) or a buffer filter (removes entries)?
- If view filter: no memory cost, but scrolling shows gaps?
- If buffer filter: must rebuild on each filter change?

**Impact:** Filtering either wastes memory (duplicate buffer) or is slow (rebuild on each change).

**Assumptions I'd Make:**
- Filtering is a view filter (not a buffer rebuild)
- All 500 entries stored in buffer
- Viewport only renders entries matching the filter
- Changing filter is fast (O(n) scan, but not expensive)

**Examples:**
```
Buffer: [entry1(INFO), entry2(ERROR), entry3(INFO), entry4(ERROR), ...]
    ↓
Filter: "error only"
    ↓
Viewport renders: [entry2(ERROR), entry4(ERROR), ...]
    ↓
Scrolling shows only errors (gaps where other levels hidden)
    ↓
User can see that entries are filtered, not removed
```

**Question:** Is filtering a view-level operation or a buffer-level operation?

---

## Part 4: Critical Questions Requiring Clarification

### Priority 1: CRITICAL (Blocks Implementation)

#### Q1.1: What's the exact location of `defer recovery.Recover()`?

**Why It Matters:** Determines whether panics in TUI thread are caught.

**Blocking Aspect:** Cannot write safe code without knowing where recovery is installed.

**Assumptions if Not Answered:**
- Place it in `cmd/gurgeh/main.go` and other entry points
- Assume it doesn't cover agent goroutines

**Clarification Needed:**
```
Q: Should recovery.Recover() be:
  a) In cmd/{tool}/main.go (covers tool entry point)
  b) In internal/{tool}/cli/root.go (covers CLI command handler)
  c) In internal/tui/app.go Run() (covers TUI initialization)
  d) All of the above (nested recovery)
```

---

#### Q1.2: Is `--inline` a global flag or tool-specific?

**Why It Matters:** Determines where the flag is parsed and how it's propagated.

**Blocking Aspect:** Cannot implement flag handling without knowing the scope.

**Assumptions if Not Answered:**
- Assume tool-specific (each tool handles its own `--inline`)
- Flag is parsed in `internal/{tool}/cli/root.go`

**Clarification Needed:**
```
Q: For Gurgeh, Coldwine, and Pollard:
  a) Each tool has its own --inline flag?
  b) Global flag in cmd/autarch/main.go that affects all tools?
  c) Flag in Bigend that affects all sub-tools?
```

---

#### Q1.3: What's the contract for source attribution in LogMsg?

**Why It Matters:** Determines how to filter/display logs by origin.

**Blocking Aspect:** Cannot implement filtering without knowing the source format.

**Assumptions if Not Answered:**
- Extract source from slog Record's logger name (e.g., `"gurgeh.arbiter"`)
- Or agents explicitly set slog context: `slog.With("source", "agent")`

**Clarification Needed:**
```
Q: For source attribution:
  a) Parse slog Record's logger name?
  b) Require agents to set slog context attributes?
  c) Use structured logging (slog.Attr)?
  d) Hardcode source in LogHandler based on who's calling?

Q: What should the source string look like?
  a) "gurgeh" (tool only)?
  b) "gurgeh.arbiter" (tool.module)?
  c) "agent" (semantic meaning)?
  d) Configurable?
```

---

#### Q1.4: How should panics in agent code be handled?

**Why It Matters:** Determines whether terminal state is recoverable.

**Blocking Aspect:** Cannot implement safety without panic handling in agents.

**Assumptions if Not Answered:**
- Panics in agents are NOT caught by TUI's recovery
- Terminal may be left in alt-screen mode
- User must type `reset` to recover

**Clarification Needed:**
```
Q: Should agent code:
  a) Have its own panic recovery (recover() in agent)?
  b) Be wrapped with recovery by the TUI?
  c) Not catch panics (accept they'll break terminal)?
  d) Run in a sub-process (isolated panic)?
```

---

#### Q1.5: Is the log buffer per-tool or unified (for Bigend)?

**Why It Matters:** Determines how Bigend displays logs from multiple tools.

**Blocking Aspect:** Cannot implement multi-tool logging without knowing the buffer model.

**Assumptions if Not Answered:**
- One unified buffer for all tools in Bigend
- All logs appear in one pane with source attribution
- User can filter by source to see tool-specific logs

**Clarification Needed:**
```
Q: For Bigend (multi-tool TUI):
  a) One shared log pane at bottom (unified buffer)?
  b) Separate log pane per tool tab (isolated buffers)?
  c) Log pane visible only for active tool tab?
  d) Tool switching clears the log pane?
```

---

### Priority 2: IMPORTANT (Significantly Affects Implementation)

#### Q2.1: Should the log filter state persist when toggling visibility?

**Why It Matters:** Affects UX consistency and state management complexity.

**Implementation Impact:** Requires separate tracking for filter vs. visibility state.

**Assumptions if Not Answered:**
- Filter state persists independently
- Toggling visibility doesn't change the filter
- User must explicitly change filter to see different logs

**Clarification Needed:**
```
Q: When user hides and re-shows log pane:
  a) Filter state persists (error-only filter still active)?
  b) Filter resets to "show all"?
  c) Scroll position also restored?
```

---

#### Q2.2: Should there be a UI indicator when messages are dropped from the buffer?

**Why It Matters:** Affects user awareness of data loss.

**Implementation Impact:** Requires tracking drops and rendering a visual indicator.

**Assumptions if Not Answered:**
- Drop is silent (no indicator)
- User has no way to know if messages were lost

**Clarification Needed:**
```
Q: When circular buffer drops oldest entries:
  a) Show "[... 47 entries dropped ...]" indicator?
  b) Silent drop (no feedback)?
  c) Highlight the "drop boundary" in a subtle color?
  d) Log a special "[BUFFER FULL]" entry?
```

---

#### Q2.3: Should filtering rebuild the viewport or just hide entries?

**Why It Matters:** Affects performance and UX.

**Implementation Impact:** View filter is simpler; buffer filter is more complex.

**Assumptions if Not Answered:**
- View filter (entries remain in buffer, viewport hides non-matching)
- Scrolling shows only matching entries (gaps where hidden)
- No visual indication of gaps

**Clarification Needed:**
```
Q: When filtering to "error only":
  a) Viewport hides non-error entries (view filter)?
  b) Remove non-error entries from buffer (buffer filter)?
  c) Copy non-error entries to a separate buffer (dual buffer)?
  d) Maintain an index of error entries (indexed filter)?
```

---

#### Q2.4: Should log pane focus block input to the main view?

**Why It Matters:** Affects navigation complexity.

**Implementation Impact:** Requires focus model that may conflict with existing TUI.

**Assumptions if Not Answered:**
- Log pane can be focused (Tab cycles through sidebar/main/logs)
- When logs focused, j/k scroll logs, other keys ignored
- Tab moves focus away from logs

**Clarification Needed:**
```
Q: When log pane is focused:
  a) j/k scroll logs, other keys ignored?
  b) All keys except Tab are ignored?
  c) Log pane is never focusable (always visible, no interaction)?
  d) Double-press L to focus/unfocus logs?
```

---

#### Q2.5: Should high-frequency logging (100+ logs/sec) be throttled?

**Why It Matters:** Affects TUI responsiveness and code complexity.

**Implementation Impact:** Batching logic adds complexity; no batching may cause lag.

**Assumptions if Not Answered:**
- No throttle in MVP
- TUI may lag at high rates
- Acceptable for initial version

**Clarification Needed:**
```
Q: For agents logging at 100+ entries/sec:
  a) No throttle (accept potential lag)?
  b) Batch updates (e.g., 10 entries per Update())?
  c) Drop messages if channel full?
  d) Limit log level (e.g., only ERROR at high rates)?
```

---

### Priority 3: NICE-TO-HAVE (Improves Clarity)

#### Q3.1: Should log history be persisted to disk between sessions?

**Why It Matters:** Affects auditability and long-term debugging.

**Not Blocking:** MVP can omit persistence; can be added later.

**Assumptions if Not Answered:**
- No persistence in MVP
- Logs are lost when TUI exits
- User can export manually if needed

**Clarification Needed:**
```
Q: When TUI exits:
  a) Logs discarded (in-memory only)?
  b) Saved to .gurgeh/logs/ directory?
  c) User can export with E key?
  d) Automatic export on exit?
```

---

#### Q3.2: Should log formatting/colors be configurable?

**Why It Matters:** Affects accessibility and personal preference.

**Not Blocking:** Reasonable defaults can be provided.

**Assumptions if Not Answered:**
- Fixed color scheme (Tokyo Night)
- Error = red, Warn = yellow, Info = white
- Not configurable in MVP

**Clarification Needed:**
```
Q: Log level colors:
  a) Tokyo Night palette (fixed)?
  b) Configurable in config file?
  c) Terminal-detected (auto-dark/light)?
  d) Accessibility high-contrast mode?
```

---

#### Q3.3: Should users be able to copy log text?

**Why It Matters:** Affects usability for error reporting.

**Not Blocking:** Can be added later if needed.

**Assumptions if Not Answered:**
- No text selection in MVP
- User must scroll to terminal and copy from alt-screen buffer
- Or export entire log

**Clarification Needed:**
```
Q: Log text selection:
  a) No selection (MVP minimal)?
  b) Mouse selection (copy to clipboard)?
  c) Keyboard selection (y/v keys like vim)?
  d) Export single entry (E on a log line)?
```

---

#### Q3.4: Should there be a "clear logs" command?

**Why It Matters:** Affects ability to focus on recent logs.

**Not Blocking:** Filter (error-only) provides similar functionality.

**Assumptions if Not Answered:**
- No clear command in MVP
- User can filter to reduce noise

**Clarification Needed:**
```
Q: Log pane commands:
  a) No clear (just filter)?
  b) Clear command (C key)?
  c) Clear with confirmation dialog?
  d) Auto-clear on new session?
```

---

#### Q3.5: Should log pane be collapsible to 0 height?

**Why It Matters:** Affects minimalism and screen space.

**Not Blocking:** Can keep a minimum height (1-2 lines).

**Assumptions if Not Answered:**
- Minimum height 1 line (shows label or collapsed indicator)
- Cannot fully hide (use --inline flag to opt-out instead)

**Clarification Needed:**
```
Q: Log pane visibility:
  a) Always visible (fixed minimum height)?
  b) Fully collapsible with L (toggle)?
  c) Collapsible with sliding animation?
  d) Only hidden if --inline flag not used?
```

---

## Part 5: Recommended Next Steps

### Before Implementation Starts

1. **Prioritize Gap Resolution** (2 hours)
   - Answer all Priority 1 questions (Q1.1–Q1.5)
   - Answer 2–3 Priority 2 questions based on design philosophy
   - Document decisions in an ADR (Architecture Decision Record)

2. **Create Implementation Checklist** (1 hour)
   - Map each answer to specific code locations
   - Break down the 4 phases (Messages, LogHandler, LogPane, Integration) into concrete tasks
   - Estimate effort for each task

3. **Review Existing Architecture Docs** (30 minutes)
   - Read INLINE_MODE_QUICK_START.md (copy-paste ready code)
   - Read AUTARCH_TUI_PATTERNS_REFERENCE.md (patterns to follow)
   - Identify reusable code from TerminalPane, messages.go, etc.

---

### During Implementation

4. **Phase 1: Messages & Handler** (2 hours)
   - Add LogMsg type to `internal/tui/messages.go`
   - Implement slog.Handler in `pkg/tui/loghandler/`
   - Wire into entry point (cmd/{tool}/main.go or internal/tui/app.go)
   - **Verify:** slog calls produce LogMsg in the channel

5. **Phase 2: LogPane Component** (3 hours)
   - Create `pkg/tui/logpane/` copying TerminalPane pattern
   - Implement Update(LogMsg) to append and maintain circular buffer
   - Implement View() to render with colors
   - Implement filtering (by level, by source)
   - **Verify:** Logs appear and scroll correctly

6. **Phase 3: Integration** (1 hour)
   - Add LogPane to App struct
   - Wire LogPane.Update() into App.Update()
   - Add LogPane to App.View() layout
   - **Verify:** Logs appear inline in running TUI

7. **Phase 4: Safety & Polish** (2 hours)
   - Add panic recovery to entry points
   - Test Ctrl+C handling
   - Test window resize
   - Test high-frequency logging
   - **Verify:** Terminal usable after exit

---

### After Implementation

8. **Integration Testing** (2 hours)
   - Test with Gurgeh (interview + logs)
   - Test with Coldwine (scan + logs)
   - Test with Pollard (hunter + logs)
   - Test with Bigend (multi-tool + logs)

9. **Documentation** (1 hour)
   - Update docs/tui/SHORTCUTS.md with log pane hotkeys
   - Update AGENTS.md with inline mode examples
   - Document the Q&A decisions in a new ADR

---

## Part 6: Risk Summary

### High-Risk Areas

| Risk | Probability | Severity | Mitigation |
|------|-------------|----------|-----------|
| **Message loss in high-frequency logging** | Medium | High | Batch updates, increase buffer size, add drop indicator |
| **Panic in agent leaves alt-screen broken** | Low | High | Explicit recovery in agent code or subprocess isolation |
| **Scroll position lost on filter toggle** | High | Low | Persist filter+scroll state separately |
| **Memory growth unbounded** | Low | High | Circular buffer with size limit, monitor with profiling |
| **TUI lag at 100+ logs/sec** | Medium | Medium | No batching in MVP; add throttle if lag observed |

---

## Part 7: Testing Strategy

### Unit Tests
- [ ] LogHandler.Handle() converts slog.Record → LogMsg
- [ ] LogPane filtering by level removes correct entries
- [ ] Circular buffer doesn't grow past 500
- [ ] LogPane scroll doesn't exceed bounds

### Integration Tests
- [ ] Gurgeh interview: logs appear in pane during agent run
- [ ] Coldwine scan: progress logs show inline
- [ ] LogPane toggle: visibility persists, filters persist
- [ ] Terminal: Ctrl+C restores terminal to usable state
- [ ] High load: 100 logs/sec doesn't crash TUI

### Manual Tests
1. Run `gurgeh --inline`, start interview, watch logs
2. Run `coldwine`, start scan, verify progress logs
3. Toggle log pane visibility multiple times
4. Filter logs, toggle visibility, verify filter persists
5. Press Ctrl+C, verify terminal is usable
6. Resize terminal mid-interview, verify logs reflow

---

## Appendix A: References to Existing Code

| Pattern | File | Lines | Purpose |
|---------|------|-------|---------|
| Message types | `internal/tui/messages.go` | 1–100 | Template for LogMsg |
| TerminalPane component | `internal/bigend/tui/terminal.go` | 1–200 | Template for LogPane |
| View interface | `pkg/tui/view.go` | 1–58 | Contract for views |
| Colors | `pkg/tui/styles.go` | 1–50 | Tokyo Night palette |
| Cleanup pattern | `cmd/autarch/main.go` | 130–150 | Defer pattern |
| slog setup | `cmd/bigend/main.go` | 40–50 | Logger configuration |

---

## Appendix B: Outstanding Questions Table

| # | Question | Priority | Category | Status |
|---|----------|----------|----------|--------|
| 1.1 | Location of `defer recovery.Recover()` | P1 | Terminal Safety | Pending |
| 1.2 | Is `--inline` global or tool-specific? | P1 | Flag Behavior | Pending |
| 1.3 | Contract for source attribution? | P1 | Log Routing | Pending |
| 1.4 | How to handle agent panics? | P1 | Terminal Safety | Pending |
| 1.5 | Per-tool or unified buffer in Bigend? | P1 | UI State | Pending |
| 2.1 | Filter state persist on visibility toggle? | P2 | UI State | Pending |
| 2.2 | UI indicator for dropped messages? | P2 | Log Routing | Pending |
| 2.3 | View filter or buffer filter? | P2 | Performance | Pending |
| 2.4 | Log pane focus model? | P2 | UI State | Pending |
| 2.5 | Throttle high-frequency logging? | P2 | Performance | Pending |
| 3.1 | Persist log history to disk? | P3 | Nice-to-have | Pending |
| 3.2 | Configurable log colors? | P3 | Nice-to-have | Pending |
| 3.3 | Copy log text? | P3 | Nice-to-have | Pending |
| 3.4 | Clear logs command? | P3 | Nice-to-have | Pending |
| 3.5 | Fully collapsible pane? | P3 | Nice-to-have | Pending |

---

## Conclusion

The inline TUI mode with log pane feature has **strong architectural foundation** from existing documents (INLINE_MODE_SUMMARY.md, INLINE_MODE_ARCHITECTURE.md, AUTARCH_TUI_PATTERNS_REFERENCE.md). However, **23 specific gaps** require clarification before implementation can proceed confidently.

**Critical blockers** (5 questions) are in the areas of:
- Flag scope and entry point
- Log source attribution contract
- Panic handling strategy
- Multi-tool buffer model
- Terminal recovery coverage

**Key recommendations:**
1. Answer all Priority 1 questions before coding
2. Document decisions in an ADR for future reference
3. Start with Phase 1 (LogMsg + LogHandler) to validate the basic flow
4. Run manual tests early (don't wait for full implementation)
5. Monitor performance at high log rates (100+ entries/sec)

The implementation can be broken into 4 manageable phases (2–3 hours each) with clear success criteria at each phase. Total estimated effort: **5–7 hours for MVP + polish**, assuming all questions are answered upfront.

---

**Document Created:** February 4, 2026
**Analysis Scope:** User journeys, edge cases, gaps, and clarifying questions
**Audience:** Implementation team, architects, decision makers
**Next Action:** Review Priority 1 questions and provide answers
