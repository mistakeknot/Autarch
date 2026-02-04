# Executive Brief: Inline TUI Mode with Log Pane

**Analyst:** Claude Code UX Flow Specialist
**Date:** February 4, 2026
**Status:** Analysis Complete — 23 Gaps Identified, 5 Critical Blockers
**Decision Required By:** Before coding starts

---

## The Problem

Users of Autarch TUIs (Gurgeh, Coldwine, Pollard) cannot see agent output or operation progress in real-time. Logs are suppressed in TUI mode, creating a "black box" experience where users don't know if the agent is working or stuck.

**Impact:** Frustration, no visibility into multi-minute operations, inability to diagnose failures.

---

## The Proposed Solution

Add a dedicated **log pane** to the bottom of Autarch TUIs that displays real-time logs from agents and operations. Inspired by FrankenTUI's inline mode pattern.

### Architecture Overview
```
┌─────────────────────────────────────────┐
│ Sidebar | Main View (Gurgeh/Coldwine)  │
├─────────────────────────────────────────┤
│ Log Pane (scrollable, filterable)       │
│ • [agent] Generating requirements...    │
│ • [scan] Found 23 files                 │
│ • [system] Sprint phase advanced        │
└─────────────────────────────────────────┘
```

**Key Components:**
1. **LogMsg** — New message type for slog routing
2. **slog.Handler** — Captures logs and emits LogMsg
3. **LogPane** — Viewport-based scrollable component
4. **Recovery** — Panic handling + terminal restoration
5. **--inline flag** — Opt-in activation

---

## Analysis Findings

### What's Already Done ✅

1. **Architectural documentation** — 4 detailed reference docs exist
2. **Code patterns identified** — Copy-paste ready from TerminalPane
3. **Design decisions made** — Circular buffer (500 entries), Tokyo Night colors
4. **Risk assessment** — Low overall risk, all patterns exist in codebase
5. **Implementation roadmap** — 4 phases, 5–7 hours estimated

### What's Missing (and Must Be Clarified) ❌

**23 gaps identified across 5 categories:**

| Category | Gaps | Severity | Examples |
|----------|------|----------|----------|
| **Flag Behavior** | 3 | P1 | Is --inline global or per-tool? Default behavior? Persistence? |
| **Log Routing** | 6 | P1 | How are sources attributed? Buffer vs. channel semantics? |
| **UI State** | 5 | P1 | Per-tool buffers or unified? Filter state across toggles? |
| **Terminal Safety** | 5 | P1 | Where's the panic recovery? Signal handlers needed? |
| **Performance** | 4 | P2 | 500 entries enough? 100+ logs/sec lag? Memory impact? |

---

## Critical Blockers (Must Decide Before Coding)

### 1. Where Does `--inline` Live?

**Current Ambiguity:**
- Is it a global flag in `cmd/autarch/main.go` (affects all tools)?
- Or per-tool (each tool's CLI has it)?
- Or in Bigend only?

**Why It Matters:** Determines command-line parsing and propagation logic.

**Decision Needed:**
```bash
# Option A: Global (Autarch-wide)
autarch gurgeh --inline        # ✅ Works
gurgeh --inline               # ✅ Works

# Option B: Per-tool only
autarch gurgeh --inline       # ✅ Works
gurgeh --inline              # ❓ Does it work? Or ignored?

# Option C: Bigend-specific
autarch --inline              # Works for Bigend and sub-tools
gurgeh --inline              # Doesn't work (Gurgeh has no flag)
```

**Recommendation:** Option A (global flag in autarch CLI) for consistency.

---

### 2. How Is Source Attribution Handled?

**Current Ambiguity:**
- When `slog.Info("msg")` is called in agent code, how does LogMsg know it came from "agent"?
- Does LogHandler extract the logger name?
- Or do agents explicitly set context: `slog.With("source", "agent")`?

**Why It Matters:** Filtering and searching by log source depends on this contract.

**Decision Needed:**
```go
// Option A: Extract from logger name
slog.With("source", "arbiter").Info("proposing")
// LogMsg{Source: "arbiter", ...}

// Option B: Hardcode based on caller
// LogHandler looks at call stack, detects it's in gurgeh/arbiter
// LogMsg{Source: "gurgeh.arbiter", ...}

// Option C: Default to module/tool name
// No explicit source, defaults to "system" or "gurgeh"
// LogMsg{Source: "gurgeh", ...}
```

**Recommendation:** Option A (agents set explicit context). Cleaner, more explicit.

---

### 3. Panic in Agent Code — Who Recovers?

**Current Ambiguity:**
- If an agent panics, does the TUI's `defer recovery.Recover()` catch it?
- Or does the panic propagate and break the terminal?
- Should agents wrap themselves with recovery?

**Why It Matters:** Terminal state is unrecoverable if panic isn't caught.

**Decision Needed:**
```go
// Option A: TUI-level recovery (current proposal)
// main() has defer recovery.Recover()
// Catches panics in TUI Update(), but NOT in agent goroutines

// Option B: Agent-level recovery
// Each agent wraps itself: defer recovery.Recover()
// But agent runs in background, recovery doesn't help TUI

// Option C: Sub-process isolation
// Spawn agent in separate process, no panic affects TUI
// Requires IPC to get logs back
```

**Recommendation:** Option A + agent error handling (agents log errors, don't panic). Accept that hard agent crashes require manual `reset`.

---

### 4. Per-Tool or Unified Log Buffer in Bigend?

**Current Ambiguity:**
- Bigend has tabs: Gurgeh, Coldwine, Pollard
- Do they share one log pane at bottom?
- Or separate log pane per tab?
- If shared, how are logs from different tools mixed?

**Why It Matters:** Affects filtering, searching, and UX.

**Decision Needed:**
```
Option A: One shared log pane (unified buffer)
┌─────────────────────────────────────┐
│ [Gurgeh] [Coldwine] [Pollard]      │
├─────────────────────────────────────┤
│ • [gurgeh.arbiter] Proposing...     │
│ • [coldwine.interview] Generating... │
│ • [gurgeh.arbiter] Consistency ok   │
└─────────────────────────────────────┘
(All logs in timeline, filterable by [source])

Option B: Per-tab log pane (isolated buffers)
┌──────────────────┬──────────────────┐
│ [Gurgeh] [C]    │ [Gurgeh] [C]    │
├──────────────────┼──────────────────┤
│ Gurgeh Logs      │ Coldwine Logs    │
│ • Proposing...   │ • Generating...  │
└──────────────────┴──────────────────┘
(Each tool has its own pane, switch tabs = switch panes)
```

**Recommendation:** Option A (unified buffer with source filtering). Simpler, chronological view helpful for diagnosing cross-tool issues.

---

### 5. Panic Recovery — Explicit Signal Handlers Needed?

**Current Ambiguity:**
- Bubble Tea already handles SIGINT/SIGTERM and restores terminal
- Do we need explicit signal handlers on top of that?
- Or is Bubble Tea's cleanup sufficient?

**Why It Matters:** Over-engineered recovery could conflict with Bubble Tea.

**Decision Needed:**
```go
// Option A: Just Bubble Tea (minimal)
p := tea.NewProgram(m, tea.WithAltScreen())
p.Run()  // Bubble Tea handles cleanup on Ctrl+C

// Option B: Explicit signal handlers (belt-and-suspenders)
sigChan := make(chan os.Signal, 1)
signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
go func() {
    <-sigChan
    recovery.RestoreTerminal()  // Explicit recovery
    os.Exit(0)
}()
p := tea.NewProgram(m, tea.WithAltScreen())
p.Run()

// Option C: Explicit signal handlers + defer (both mechanisms)
defer recovery.Recover()
signal.Notify(...)  // as above
```

**Recommendation:** Option A for MVP (Bubble Tea is sufficient). Add Option B later if needed.

---

## Implementation Roadmap

### Phase 1: Messages & Handler (2 hours)
- Add LogMsg type to `internal/tui/messages.go`
- Implement slog.Handler in `pkg/tui/loghandler/`
- Wire handler into entry point
- **Done when:** `slog.Info()` calls produce LogMsg in Bubble Tea

### Phase 2: LogPane Component (3 hours)
- Create `pkg/tui/logpane/` copying TerminalPane pattern
- Implement filtering, scrolling, circular buffer
- Implement View() with Tokyo Night colors
- **Done when:** Logs appear in pane and scroll correctly

### Phase 3: Integration (1 hour)
- Add LogPane to App struct
- Wire into App.Update() and App.View()
- Test with actual agents
- **Done when:** Inline logs visible in Gurgeh/Coldwine

### Phase 4: Safety & Polish (2 hours)
- Add panic recovery to entry points
- Test Ctrl+C, window resize, high-frequency logging
- Color styling, help text updates
- **Done when:** All tests pass, terminal recoverable

**Total Effort:** 5–7 hours for MVP + polish

---

## Risk Assessment

| Risk | Impact | Mitigation |
|------|--------|-----------|
| **Message loss in high-frequency logging** | Missed error logs | Circular buffer, add drop indicator |
| **Panic in agent breaks terminal** | User frustration | TUI-level recovery + agent error handling |
| **Memory growth unbounded** | OOM crash | 500-entry limit, no variable sizing |
| **TUI lags at 100+ logs/sec** | Poor UX | No batching in MVP, add if observed |
| **Flag conflicts** | Confusion | Clear docs, single activation path |

**Overall Risk:** Low. All patterns exist in codebase, no novel architecture.

---

## Decision Summary

| Decision | Recommendation | Rationale |
|----------|---|----------|
| **--inline scope** | Global flag in autarch CLI | Consistency, easier to discover |
| **Source attribution** | Explicit slog context (agent sets it) | Clear contract, explicit |
| **Panic recovery** | TUI-level defer + agent error handling | Matches existing patterns |
| **Log buffer** | Per-tool in MVP (easier), unified later | Simplicity first |
| **Signal handlers** | Just Bubble Tea (MVP), explicit handlers later | Sufficient for startup |
| **Buffer size** | 500 entries (fixed) | ~250 KB memory, negligible cost |
| **Log persistence** | None in MVP | Can add export later |
| **Filtering** | View-level (not buffer) | Simpler, faster |

---

## Next Steps

### Immediate (Before Coding)
1. **Confirm 5 critical decisions** (30 min) — Use decisions above or override with your choices
2. **Document ADR** (30 min) — Create Architecture Decision Record
3. **Update implementation checklist** (30 min) — Refine phase breakdown with concrete tasks

### Week 1 (Implementation)
1. **Phase 1** — Implement LogMsg + LogHandler (2 hours)
2. **Phase 2** — Build LogPane component (3 hours)
3. **Phase 3** — Integrate into App (1 hour)
4. **Phase 4** — Safety hardening + testing (2 hours)

### Week 2 (Integration & Polish)
1. Test with all tools (Gurgeh, Coldwine, Pollard, Bigend)
2. Performance test at 100+ logs/sec
3. Manual Ctrl+C + panic recovery tests
4. Documentation updates

---

## Key Documents

| Document | Purpose | Length | When to Read |
|----------|---------|--------|--------------|
| [INLINE_MODE_QUICK_START.md](docs/solutions/INLINE_MODE_QUICK_START.md) | Copy-paste implementation | 3 pgs | During coding |
| [INLINE_MODE_SUMMARY.md](docs/solutions/INLINE_MODE_SUMMARY.md) | Architecture & patterns | 4 pgs | Before deciding |
| [INLINE_MODE_ARCHITECTURE.md](docs/solutions/INLINE_MODE_ARCHITECTURE.md) | Detailed design | 8 pgs | For deep review |
| [AUTARCH_TUI_PATTERNS_REFERENCE.md](docs/solutions/AUTARCH_TUI_PATTERNS_REFERENCE.md) | Code patterns | 12 pgs | During coding (reference) |
| [FLOW_ANALYSIS_INLINE_TUI_LOGPANE.md](FLOW_ANALYSIS_INLINE_TUI_LOGPANE.md) | This analysis | 25 pgs | For complete context |

---

## Bottom Line

**The feature is feasible and low-risk.** Architecture is solid, patterns exist, timeline is realistic (5–7 hours). Main blocker is clarification on 5 design decisions (30 min to decide).

**Recommendation:** Answer the 5 critical questions above, document in ADR, and proceed with Phase 1. The risk of delaying is higher than the risk of implementing with these choices.

---

**Prepared By:** Claude Code UX Flow Analysis
**Date:** February 4, 2026
**Status:** Ready for Decision Makers
**Next:** Schedule 30-min decision meeting with architects
