# Visual Flow Guide: Inline TUI Mode with Log Pane

**Purpose:** Quick reference diagrams and state transitions for inline mode
**Audience:** Developers, architects, QA testers
**Format:** ASCII diagrams, tables, state machines

---

## 1. Happy Path: User Enables Inline Mode

```
User runs: gurgeh --inline
    |
    v
TUI initializes
    |
    +---> Create slog.Handler
    |
    +---> Create LogPane component
    |
    +---> Create LogMsg channel
    |
    v
Render layout:
  +─────────────────────────┐
  │ Sidebar | Main View     │  ← User starts interaction
  ├─────────────────────────┤
  │ Log Pane (empty)        │  ← Ready for logs
  └─────────────────────────┘
    |
    v
User initiates agent action (interview, scan)
    |
    v
Agent code runs:
    slog.Info("generating requirements")
    slog.Warn("no results found")
    slog.Error("api timeout")
    |
    v
slog.Handler.Handle() for each call
    |
    +---> Extract level, message, timestamp
    |
    +---> Create LogMsg{Level, Source, Message, Time}
    |
    v
Send LogMsg via non-blocking channel
    |
    v
TUI.Update() receives LogMsg
    |
    +---> LogPane.Update(LogMsg)
    |
    +---> Append to entries[] slice
    |
    +---> Update viewport
    |
    v
TUI.View() renders:
  +─────────────────────────┐
  │ Sidebar | Main View     │
  ├─────────────────────────┤
  │ • [agent] generating... │  ← RED: error
  │ • [scan] found 23       │  ← YELLOW: warn
  │ • [agent] complete      │  ← WHITE: info
  └─────────────────────────┘
    |
    v
User continues interaction (editing, scrolling, filtering)
    |
    v
User exits: Ctrl+C
    |
    +---> SIGINT received
    |
    +---> Bubble Tea begins cleanup
    |
    +---> Alternate screen disabled
    |
    +---> Cursor restored
    |
    v
Terminal restored to normal
User can type next command
```

---

## 2. Error Path: Agent Panics

```
Agent code runs in goroutine
    |
    v
Unexpected error: panic("nil pointer dereference")
    |
    v
Panic propagates (NOT caught by TUI recovery)
    |
    v
Goroutine exits
    |
    v
Agent error is NOT logged (panic kills execution)
    |
    v
TUI is still running (unaffected by goroutine panic)
    |
    v
User notices: No progress update for 10 seconds
    |
    v
User can:
  • Continue with manual input
  • Press Ctrl+C to exit (terminal restored cleanly)
  • Or wait for agent timeout

OUTCOME: TUI survives, terminal usable
(Agent's mistake ≠ terminal broken)
```

---

## 3. Terminal Safety: Panic Recovery

```
Normal TUI operation
    |
    v
Unexpected error in Update() or View()
    |
    v
defer recovery.Recover() catches panic
    |
    +---> RestoreTerminal()
    |     ├─ Exit alt screen mode
    |     └─ Restore cursor visibility
    |
    v
Print error to stderr:
"PANIC: runtime error: slice bounds out of range"
    |
    v
Exit process (status 1)
    |
    v
Terminal is clean (no alt screen mode left)
User can run `reset` if needed (but usually not)
```

---

## 4. Scrolling Through Logs

```
Log pane with 150 entries visible (of 500 total)
Scrolling position: bottom (latest entries)
    |
    v
User presses 'k' (scroll up)
    |
    v
Viewport shifts up by ~1-10 lines
    |
    +---> Older entries become visible
    |
    +---> Newest entries scroll out of view
    |
    v
LogPane.viewport.LineUp() or similar
    |
    v
Render only visible lines (lazy rendering)
    |
    v
Display updates:
  ┌─────────────────┐
  │ [scan] query #1 │  ← User scrolled up, now visible
  │ [scan] query #2 │
  │ [scan] query #3 │
  │ ... 7 more ...  │
  │ [agent] done    │  ← Latest entry (no longer at bottom)
  └─────────────────┘
    |
    v
User presses 'k' again
    |
    v
Scroll continues upward
    |
    v
Eventually reach top of buffer (entry 1)
    |
    v
User tries to scroll past top
    |
    v
No change (scroll position clamped)
    |
    └─-> Stays at top entry
```

---

## 5. Filtering Logs by Level

```
Log pane with 500 entries (all levels)
Current filter: Show All
    |
    v
User presses 'F' (filter)
    |
    v
Filter menu appears:
  ┌─────────────────────┐
  │ Filter by level:    │
  │ • All               │  ← currently selected
  │ • Error only        │
  │ • Warn + Error      │
  │ • Info + Warn + Err │
  └─────────────────────┘
    |
    v
User selects "Error only"
    |
    v
LogPane.filterLevel = "ERROR"
    |
    v
LogPane.Update() rebuilds viewport
    |
    +---> Iterate entries[]
    |
    +---> Include only entries where level == "ERROR"
    |
    +---> Hide others (view filter, not buffer filter)
    |
    v
Viewport re-renders:
  ┌─────────────────────────┐
  │ • [agent] connection    │ ← ERROR only
  │ • [scan] timeout        │
  │ • [system] fatal error  │
  │ 47 entries hidden       │
  └─────────────────────────┘
    |
    v
User presses 'F' again
    |
    v
Filter menu:
  ┌─────────────────────┐
  │ • All               │
  │ • Error only        │  ← currently selected
  │ • Warn + Error      │
  │ • Info + Warn + Err │
  └─────────────────────┘
    |
    v
User selects "All"
    |
    v
LogPane.filterLevel = ""
    |
    v
All entries visible again:
  ┌─────────────────────────┐
  │ • [agent] starting      │ ← INFO
  │ • [scan] query found 15 │ ← INFO
  │ • [agent] connection    │ ← ERROR
  │ • [scan] timeout        │ ← ERROR
  │ ... more entries ...    │
  └─────────────────────────┘
```

---

## 6. Toggling Log Pane Visibility

```
Logs visible:
  ┌──────────────────────────┐
  │ Sidebar | Main View (70%)│
  ├──────────────────────────┤
  │ Log Pane (30% height)    │
  │ [agent] generating...    │
  └──────────────────────────┘
    |
    v
User presses 'L' (toggle)
    |
    v
LogPane visibility = hidden
    |
    v
Layout recalculates:
  ┌──────────────────────────┐
  │ Sidebar | Main View      │
  │         (100% height)    │
  │                          │
  │ [User can work with more │
  │  screen space]           │
  └──────────────────────────┘
    |
    v
User presses 'L' again
    |
    v
LogPane visibility = shown
    |
    v
Layout recalculates:
  ┌──────────────────────────┐
  │ Sidebar | Main View (70%)│
  ├──────────────────────────┤
  │ Log Pane (30% height)    │
  │ [agent] generating...    │  ← Scroll position restored?
  └──────────────────────────┘
    |
    v
Question: Was scroll position preserved?
  Option A: Jump to latest (auto-scroll behavior)
  Option B: Restore to line 120 (user was viewing)
```

---

## 7. Concurrent Logging (Multi-Source)

```
Coldwine TUI, interview + concurrent scan
    |
    v
Both agents logging simultaneously:
    |
    +--- Agent A (interview)         --->  slog.Info("generating...")
    |                                       slog.Info("structuring...")
    |
    +--- Agent B (scan)              --->  slog.Info("querying...")
    |                                       slog.Info("parsing...")
    |
    v
slog.Handler.Handle() called for each
(Handler is thread-safe, uses non-blocking channel)
    |
    v
Messages enter channel (possibly out of order)
    |
    v
TUI.Update() processes channel
    |
    +---> Order: Determined by when channel is read
    |
    +---> May NOT be chronological if agents log very fast
    |
    v
Viewport renders:
  ┌─────────────────────────────┐
  │ • [interview] generating    │  ← Time: 12:34:56.001
  │ • [scan] querying           │  ← Time: 12:34:56.002
  │ • [interview] structuring   │  ← Time: 12:34:56.003
  │ • [scan] parsing            │  ← Time: 12:34:56.004
  └─────────────────────────────┘
    |
    v
Question: Are logs ordered by:
  Option A: Channel receive order (may not be chronological)
  Option B: Timestamp (true chronological order)
```

---

## 8. Terminal Resize During Logging

```
TUI running, logs streaming, user resizes terminal
    |
    v
SIGWINCH signal received
    |
    v
Bubble Tea handles SIGWINCH
    |
    +---> Recalculate width/height
    |
    +---> Call all views' Resize() or similar
    |
    v
LogPane.viewport recalculates
    |
    +---> New width/height from Bubble Tea
    |
    +---> Re-wrap log text to new width
    |
    +---> Adjust scroll position
    |
    v
New layout:
  Before: 80 cols          After: 120 cols
  ┌──────────┐            ┌──────────────────────┐
  │ Sidebar  │            │ Sidebar | Main View  │
  │          │  ──────>   ├──────────────────────┤
  │ Main     │            │ Log Pane (wider)     │
  │          │            │ • [agent] long text  │
  ├──────────┤            │   wraps differently  │
  │ Logs     │            └──────────────────────┘
  └──────────┘
    |
    v
Agent continues logging uninterrupted
```

---

## 9. State Transitions: Focus Model

```
Tab navigation between regions:
    |
    v
Sidebar focused:
    • 'j'/'k' navigate items
    • 'enter' select/expand
    • 'Tab' move to Main View
    |
    v
Main View focused:
    • 'j'/'k' navigate main content
    • 'enter' perform action
    • 'Tab' move to Log Pane (if visible)
    |
    v
Log Pane focused (if visible):
    • 'j'/'k' scroll logs
    • 'F' filter
    • 'C' clear (maybe)
    • 'Tab' move to Sidebar
    |
    v
If Log Pane hidden:
    • Tab cycles: Sidebar -> Main View -> Sidebar
    • Log Pane cannot be focused
    • User must press 'L' to show pane
```

---

## 10. Buffer Management: Circular Buffer

```
Empty buffer
entries[] = []
    |
    v
Agent logs entry 1
entries[] = [Entry1]
    |
    v
Agent logs entries 2-500
entries[] = [Entry1, Entry2, ..., Entry500]
    |
    v
Buffer at capacity (500 entries)
maxEntries = 500
    |
    v
Agent logs entry 501
    |
    v
LogPane.Update():
  if len(entries) > maxEntries {
      entries = entries[1:]  // Drop oldest
  }
    |
    v
entries[] = [Entry2, Entry3, ..., Entry501]
    |
    v
Entry1 is LOST (oldest entry dropped)
    |
    v
Agent continues logging
entries[] maintains 500-entry window
```

---

## 11. Message Flow: slog → LogMsg → Viewport

```
Agent code:
    slog.Info("message", "key", "value")
    |
    v
slog default logger routes to Handler
    |
    v
Handler.Handle(ctx, Record):
    ├─ Extract level (INFO)
    ├─ Extract message ("message")
    ├─ Extract time (now)
    ├─ Extract/infer source ("gurgeh.arbiter")
    └─ Create LogMsg{...}
    |
    v
Non-blocking send to channel:
    select {
    case logMsgChan <- msg:
        // Success
    default:
        // Buffer full, drop? Or handle error?
    }
    |
    v
TUI Update() loop:
    for {
        msg := <-logMsgChan
        app.Update(msg)
    }
    |
    v
App.Update(msg):
    case logMsg := msg.(type):
        logPane.Update(logMsg)
    |
    v
LogPane.Update(logMsg):
    ├─ Check filter (show this entry?)
    ├─ Append to entries[]
    ├─ Maintain circular buffer
    └─ Update viewport
    |
    v
TUI.View():
    Render logPane.View():
    ├─ Format entry as string
    ├─ Color by level (red/yellow/white)
    └─ Include timestamp
```

---

## 12. Error Logging: How Errors Are Surfaced

```
Agent encounters error:
    err := someFunc()
    if err != nil {
        slog.Error("operation failed", "error", err)
    }
    |
    v
slog.Handler.Handle() called with level=ERROR
    |
    v
LogMsg created:
    LogMsg{
        Level: "ERROR",
        Source: "gurgeh.arbiter",
        Message: "operation failed error=<error string>",
        Time: now,
    }
    |
    v
Sent to viewport
    |
    v
LogPane.Update() receives, appends to entries[]
    |
    v
LogPane.View() renders in RED:
    [ERROR] operation failed error=...
    |
    v
QUESTION: Does main view also show error state?
    Option A: Only in logs (no separate error state)
    Option B: Log appears + status bar shows "[ERROR]"
    Option C: Log appears + modal error dialog (too disruptive)
```

---

## 13. Flag Interaction Precedence

```
User runs various command combinations:

Case 1: gurgeh (no flags)
    ├─ No --inline flag
    ├─ No slog.Handler created
    └─ Log pane absent
    |
Case 2: gurgeh --inline
    ├─ --inline flag present
    ├─ slog.Handler created
    └─ Log pane shown
    |
Case 3: LOG_LEVEL=debug gurgeh --inline
    ├─ --inline flag present
    ├─ slog.Handler created at DEBUG level
    └─ Debug + info + warn + error all shown
    |
Case 4: gurgeh --inline --log-level=error
    ├─ --inline flag present
    ├─ slog.Handler created at ERROR level
    └─ Only errors shown
    |
QUESTION: What's the exact precedence?
    Option A: CLI flag > env var > default
    Option B: CLI flag only (env vars ignored)
    Option C: Separate --log-level flag for inline mode
```

---

## 14. Multi-Tool Bigend: Log Stream Model

```
Bigend (unified TUI with tabs)
    |
    ├─ Gurgeh tab
    ├─ Coldwine tab
    └─ Pollard tab
    |
    v
Question: How are logs organized?
    |
    v
OPTION A: Unified buffer (one log pane at bottom)
    ┌──────────────────────────┐
    │ [Gurgeh] [Coldwine] [Poll]
    ├──────────────────────────┤
    │ • [gurgeh] proposing     │
    │ • [coldwine] generating  │
    │ • [gurgeh] consistency   │
    │ • [pollard] found 15     │  ← All tools in one pane
    └──────────────────────────┘
    Tool switch: Logs persist, source filter shows active tool
    |
    OPTION B: Per-tab buffer (separate panes)
    ┌──────────────┬──────────────┐
    │ [Gurgeh] [C] │ [Gurgeh] [C] │
    ├──────────────┼──────────────┤
    │ • [gurgeh]   │ • [coldwine] │
    │ • [gurgeh]   │ • [coldwine] │
    │ • [gurgeh]   │ • [coldwine] │
    └──────────────┴──────────────┘
    Tool switch: Pane changes
    |
    OPTION C: No log pane in Bigend (only in individual tools)
    ┌──────────────────────────┐
    │ [Gurgeh] [Coldwine] [Poll]
    ├──────────────────────────┤
    │ Active view (no logs)     │
    │ Logs only visible when    │
    │ running each tool alone   │
    └──────────────────────────┘
```

---

## 15. Shutdown Sequence

```
User presses Ctrl+C (normal exit)
    |
    v
SIGINT signal
    |
    v
Bubble Tea begins shutdown:
    1. Stop accepting input
    2. Call cleanup for each view
    3. Clear message queue
    4. Exit alt screen mode
    5. Restore cursor
    |
    v
QUESTION: Are pending LogMsg processed before exit?
    Option A: Drain channel (all pending logs shown)
    Option B: Discard channel (logs in flight lost)
    Option C: Graceful timeout (wait up to 1 sec)
    |
    v
Terminal restored
    |
    v
Scrollback visible:
    $ gurgeh --inline
    > [logs from session...]
    > [last log line]
    $  ← User can type here
```

---

## 16. Performance Profile: Log Rate vs. TUI Lag

```
Log volume scenarios:

Light (1-10 logs/sec):
    Agent running interview
    ├─ TUI: No perceptible lag
    ├─ Channel: No backup
    ├─ Viewport: Smooth scrolling
    └─ User: "Responsive"
    |
Medium (10-50 logs/sec):
    Agent running + concurrent scan
    ├─ TUI: Still responsive
    ├─ Channel: < 10 msgs backed up
    ├─ Viewport: Occasional flicker
    └─ User: "Acceptable"
    |
Heavy (50-100 logs/sec):
    Multiple agents + high churn
    ├─ TUI: Noticeable lag on scroll
    ├─ Channel: 50+ msgs backed up
    ├─ Viewport: Re-wrapping all entries
    └─ User: "Sluggish"
    |
Burst (100+ logs/sec):
    Agent panic, 500 errors per second
    ├─ TUI: Very slow or unresponsive
    ├─ Channel: Full, messages dropped
    ├─ Viewport: Recalculating constantly
    └─ User: "Freezing"
    |
QUESTION: Is no batching acceptable for MVP?
    Yes if: Expected log rate < 50/sec
    No if: Need to support 100+ logs/sec
```

---

## 17. Focus & Keyboard Navigation Conflicts

```
Three focusable regions:
    1. Sidebar (navigate specs)
    2. Main View (edit spec)
    3. Log Pane (scroll logs)
    |
    v
When Sidebar focused:
    | 'j'/'k' | Scroll specs up/down |
    | 'enter' | Open spec           |
    | 'Tab'   | Next region         |
    |
When Main View focused:
    | 'j'/'k' | Navigate content   |
    | 'i'     | Enter edit mode    |
    | 'Tab'   | Next region        |
    |
When Log Pane focused:
    | 'j'/'k' | Scroll logs up/down      |
    | 'F'     | Open filter menu         |
    | 'Tab'   | Next region              |
    |
QUESTION: Can conflicts occur?
    Scenario: Main view has 'j' bound to "next feature"
    User focuses Log Pane
    User presses 'j'
    Result: Scroll log or navigate feature?

    Answer: Log Pane has focus, so 'j' = scroll
            Main View no longer receives 'j'
            (Bubble Tea ensures only focused region handles input)
```

---

## 18. Summary: All Flows at a Glance

| Flow | Entry | Duration | Success Criteria |
|------|-------|----------|-----------------|
| **1. Happy Path** | `gurgeh --inline` | 30+ min | Logs visible, terminal restored |
| **2. Error Recovery** | Agent panic | <1 sec | TUI survives, user can exit cleanly |
| **3. Scrolling** | 'k' key | <100ms | Smooth scroll, no lag |
| **4. Filtering** | 'F' key | <1 sec | Correct entries shown/hidden |
| **5. Toggle Visibility** | 'L' key | <100ms | Pane shown/hidden, scroll preserved |
| **6. Concurrent Logging** | Multi-agent | Continuous | Logs interleaved, no drops |
| **7. Resize** | Window size change | <1 sec | Layout reflows, logs re-wrap |
| **8. Panic Recovery** | Unexpected error | <1 sec | Terminal cleaned up, process exits |
| **9. Shutdown** | Ctrl+C | <1 sec | Logs drained, terminal restored |
| **10. Focus Navigation** | 'Tab' key | <100ms | Focus moves, keyboard works |

---

## Legend & Notation

```
┌─────┐                  Box: UI component
│     │
└─────┘

    |
    v                    Arrow: Data flow or state transition

 ─>                      Solid arrow: Process flow

Case 1:                  Branch: Different scenarios
Case 2:

[ERROR]                  Bracket + CAPS: Log level or status

// Comment             Code block: Pseudocode or implementation hint

Option A:              Decision point: Multiple valid choices
Option B:
Option C:

QUESTION:              Ambiguity: Needs clarification before coding
ASSUMPTION:
```

---

## Quick Navigation

| Question | See Diagram |
|----------|------------|
| How does a log get from agent to screen? | #11 |
| What happens if I press Ctrl+C? | #15 |
| How is the log buffer managed? | #10 |
| What if agents panic? | #2 |
| How do I filter logs? | #5 |
| What about performance at high rates? | #16 |
| How does focus work with logs? | #17 |
| What about resizing the terminal? | #8 |
| Can I hide the log pane? | #6 |
| How do multiple tools work in Bigend? | #14 |

---

**Created:** February 4, 2026
**Type:** Reference Guide
**Use:** Print or keep open during implementation
**Update:** Add actual screenshots after MVP complete
