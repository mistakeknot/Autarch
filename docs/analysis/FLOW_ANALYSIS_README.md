# Inline TUI Mode with Log Pane - Complete Flow Analysis

**Date:** February 4, 2026
**Status:** Analysis Complete — Ready for Implementation Decision
**Total Pages:** 45+ pages of comprehensive analysis
**Time to Review:** 30 min (brief) to 2 hours (complete)

---

## What's This?

Complete user experience flow analysis for the **inline TUI mode with dedicated log pane** feature in Autarch. This analysis identifies all possible user journeys, edge cases, gaps, and clarifying questions that need answers before implementation.

**Key Insight:** The architecture is solid and patterns exist in the codebase, but 5 critical design decisions need to be made before coding can proceed confidently.

---

## Three Documents, Three Purposes

### Document 1: FLOW_ANALYSIS_INLINE_TUI_LOGPANE.md (50 KB)

**Purpose:** Complete reference for developers and architects
**Length:** 25 pages
**Time to Read:** 45 minutes to 1.5 hours
**Best For:** Understanding the full scope before implementation

**Contains:**
- Executive summary (2 pages)
- 10 detailed user flows with diagrams (10 pages)
- Flow permutations matrix (1 page)
- 23 gaps organized by 5 categories (8 pages)
- 15 critical questions with priority levels (6 pages)
- Risk summary and testing strategy (3 pages)
- References to existing code patterns (1 page)

**Sections to Read First:**
1. Executive Summary (get oriented)
2. Part 4: Critical Questions (understand blockers)
3. Part 3: Missing Elements (understand gaps)
4. Part 1: User Flows (understand journeys)

**Use Case:** "I need to understand everything before I start implementing"

---

### Document 2: FLOW_ANALYSIS_EXECUTIVE_BRIEF.md (13 KB)

**Purpose:** Decision-focused summary for architects and decision makers
**Length:** 8 pages
**Time to Read:** 20-30 minutes
**Best For:** Making decisions about design

**Contains:**
- Problem statement (why this feature matters)
- Solution overview (what we're building)
- Key findings (what we know)
- Critical blockers (what needs deciding)
- 5 specific decision points with options
- Implementation roadmap (timeline and phases)
- Risk assessment (what could go wrong)
- Bottom line (should we do this?)

**Sections to Read First:**
1. Critical Blockers (5 decision points)
2. Implementation Roadmap (understand timeline)
3. Bottom Line (the recommendation)

**Use Case:** "I need to decide if we should build this and what we should decide"

---

### Document 3: FLOW_ANALYSIS_VISUAL_GUIDE.md (21 KB)

**Purpose:** Quick reference during implementation
**Length:** 12 pages
**Time to Read:** 30 minutes (or browse as needed)
**Best For:** Developers during coding and testing

**Contains:**
- 18 detailed ASCII flow diagrams
- State machines for each major flow
- Message routing pipeline
- Buffer management visualization
- Focus/navigation model
- Performance profiles
- Keyboard interaction matrix
- Quick navigation index (find diagram you need)

**Diagrams Include:**
- Happy path: User enables inline mode (#1)
- Error path: Agent panics (#2)
- Terminal recovery (#3)
- Scrolling behavior (#4)
- Log filtering (#5)
- Pane toggling (#6)
- Concurrent logging (#7)
- Terminal resize (#8)
- Focus/navigation (#9-10)
- Buffer management (#10)
- Message flow (#11)
- And 7 more...

**Use Case:** "I need a visual explanation of how flow X works"

---

## Quick Navigation by Role

### I'm a Decision Maker (30 min)
1. Read: FLOW_ANALYSIS_EXECUTIVE_BRIEF.md (entire)
2. Decide: Answer the 5 critical blockers
3. Done: Share decision summary with team

---

### I'm an Architect (1-1.5 hours)
1. Read: FLOW_ANALYSIS_EXECUTIVE_BRIEF.md (entire)
2. Review: FLOW_ANALYSIS_INLINE_TUI_LOGPANE.md (Part 4: Questions)
3. Study: FLOW_ANALYSIS_VISUAL_GUIDE.md (focus on diagrams 1, 10, 11)
4. Assess: Risk summary in both documents
5. Done: Write ADR documenting decisions

---

### I'm Implementing (2-3 hours prep, then coding)
1. Skim: FLOW_ANALYSIS_EXECUTIVE_BRIEF.md (focus on decisions)
2. Study: FLOW_ANALYSIS_INLINE_TUI_LOGPANE.md (Parts 3-4, reference Part 5)
3. Reference: FLOW_ANALYSIS_VISUAL_GUIDE.md (keep open while coding)
4. Code: Use Phase 1-4 breakdown from executive brief
5. Test: Use testing strategy from full analysis

---

### I'm QA/Tester (1 hour)
1. Read: FLOW_ANALYSIS_EXECUTIVE_BRIEF.md (Risk Assessment + Bottom Line)
2. Study: FLOW_ANALYSIS_VISUAL_GUIDE.md (diagrams 1, 2, 3, 4, 15)
3. Reference: FLOW_ANALYSIS_INLINE_TUI_LOGPANE.md (Part 7: Testing Strategy)
4. Done: You have all test cases mapped

---

## Key Findings at a Glance

### What We Know (Strengths)
- Architecture is solid (matches existing patterns)
- Code patterns exist (TerminalPane, slog, messages.go)
- Low risk overall (no novel ideas)
- Clear implementation path (4 phases, 5-7 hours)
- Existing reference docs are comprehensive

### What We Don't Know (Blockers)
1. **Flag Scope:** Is `--inline` global or per-tool?
2. **Source Attribution:** How are log origins tracked?
3. **Panic Handling:** Where does recovery live?
4. **Buffer Model:** Per-tool or unified in Bigend?
5. **Signal Handlers:** Explicit needed or Bubble Tea OK?

### Gaps Identified (23 Total)

| Category | Count | Priority | Examples |
|----------|-------|----------|----------|
| Flag Behavior | 3 | P1 | Default? Persistence? Flag interaction? |
| Log Routing | 6 | P1 | Source contract? Channel semantics? |
| UI State | 5 | P1 | Per-tool buffer? Filter persistence? |
| Terminal Safety | 5 | P1 | Recovery location? Signal handlers? |
| Performance | 4 | P2 | Buffer size? Log rate ceiling? |

---

## The 5 Critical Blockers Explained

### Blocker 1: Flag Scope

**Question:** Where does `--inline` live?

**Options:**
- A) Global in `autarch` CLI (affects all tools)
- B) Per-tool (each tool has its own flag)
- C) Bigend-specific

**Impact:** Determines parsing logic, CLI behavior, documentation

**Recommendation:** Option A (global, consistent, easy to discover)

---

### Blocker 2: Source Attribution

**Question:** How do logs get tagged with their origin?

**Options:**
- A) Agent explicitly sets slog context: `slog.With("source", "agent")`
- B) Handler extracts from logger name
- C) Handler hardcodes based on call stack

**Impact:** Filtering and searching depends on this contract

**Recommendation:** Option A (explicit, clear contract)

---

### Blocker 3: Panic Recovery

**Question:** Where is `defer recovery.Recover()` placed?

**Options:**
- A) TUI entry point (catches TUI panics, not agent panics)
- B) Agent code (catches agent panics, but agent runs in bg)
- C) Both (nested recovery)

**Impact:** Whether terminal can be broken by agent errors

**Recommendation:** Option A + agent error handling (agents log, don't panic)

---

### Blocker 4: Log Buffer in Bigend

**Question:** One buffer or separate per tool?

**Options:**
- A) Unified buffer (one log pane, all tools mixed, chronological order)
- B) Per-tab buffer (each tool has own pane, isolated)
- C) No log pane in Bigend (only in individual tools)

**Impact:** UX for multi-tool orchestration

**Recommendation:** Option A (unified, simpler, better for cross-tool debugging)

---

### Blocker 5: Signal Handlers

**Question:** Do we need explicit signal handlers?

**Options:**
- A) Just Bubble Tea (minimal, sufficient)
- B) Explicit handlers on top (belt-and-suspenders)
- C) Both (defensive)

**Impact:** Code complexity, terminal safety

**Recommendation:** Option A for MVP (Bubble Tea is sufficient)

---

## Implementation Phases

### Phase 1: Messages & Handler (2 hours)
Create the data flow pipeline.

**What:**
- Add LogMsg type to messages.go
- Implement slog.Handler
- Wire into entry point

**Verify:**
- slog.Info() calls produce LogMsg in channel

**Files:**
- `internal/tui/messages.go` (modify)
- `pkg/tui/loghandler/handler.go` (new)
- `cmd/*/main.go` (modify)

---

### Phase 2: LogPane Component (3 hours)
Build the viewport-based log display.

**What:**
- Create LogPane copying TerminalPane pattern
- Implement filtering (by level, source)
- Implement scrolling and circular buffer
- Add colors (Tokyo Night palette)

**Verify:**
- Logs appear in pane
- Scrolling works
- Filtering works
- Buffer respects 500-entry limit

**Files:**
- `pkg/tui/logpane/pane.go` (new, ~200 lines)

---

### Phase 3: Integration (1 hour)
Wire LogPane into the app.

**What:**
- Add LogPane to App struct
- Wire Update() and View()
- Test with real agents
- Add toggle/filter keybindings

**Verify:**
- Logs appear inline during real operations
- No layout conflicts
- Views still work normally

**Files:**
- `internal/tui/app.go` (modify)
- `internal/{tool}/tui/model.go` (modify)

---

### Phase 4: Safety & Polish (2 hours)
Harden, test, document.

**What:**
- Panic recovery
- Signal handling
- Performance testing
- Help text, documentation

**Verify:**
- Ctrl+C restores terminal
- 100+ logs/sec doesn't crash
- All tools work (Gurgeh, Coldwine, Pollard, Bigend)
- Tests pass

**Files:**
- `pkg/tui/recovery/recovery.go` (new)
- `cmd/*/main.go` (modify)
- `docs/tui/SHORTCUTS.md` (update)

---

## Risk Assessment

### High-Risk Items

| Risk | Probability | Severity | Mitigation |
|------|-------------|----------|-----------|
| **Message loss at high rate** | Medium | High | Buffer + drop indicator |
| **Agent panic breaks terminal** | Low | High | TUI recovery + agent error handling |
| **Memory growth unbounded** | Low | High | Circular buffer with limit |

### Medium-Risk Items

| Risk | Probability | Severity | Mitigation |
|------|-------------|----------|-----------|
| **TUI lag at 100+ logs/sec** | Medium | Medium | No batching in MVP, add if needed |
| **Scroll/filter state lost** | Low | Medium | Persist state separately |

### Overall Assessment

**Low Risk** — All patterns exist in codebase, no novel architecture

---

## Testing Strategy

### Unit Tests
- LogHandler converts slog → LogMsg
- LogPane filtering works correctly
- Circular buffer behaves
- Scroll bounds enforcement

### Integration Tests
- Gurgeh: logs appear during interview
- Coldwine: progress logs show inline
- Bigend: multi-tool logs appear
- Terminal: Ctrl+C restores usable state

### Manual Tests
1. Run `gurgeh --inline`, start interview, verify logs
2. Filter logs, toggle pane, verify state persists
3. Resize terminal, verify logs reflow
4. Ctrl+C, verify terminal usable
5. High-frequency logging (100+ logs/sec), check lag

---

## Files to Reference While Implementing

| Pattern | File | Purpose |
|---------|------|---------|
| Message types | `internal/tui/messages.go` | Template for LogMsg |
| TerminalPane | `internal/bigend/tui/terminal.go` | Copy LogPane from here |
| View interface | `pkg/tui/view.go` | Contract for views |
| Colors | `pkg/tui/styles.go` | Tokyo Night palette |
| Cleanup | `cmd/autarch/main.go:130-150` | Defer pattern |
| slog setup | `cmd/bigend/main.go:40-50` | Logger config |

---

## Outstanding Questions Table

All 15 questions from the full analysis, with status:

| # | Question | Priority | Status |
|---|----------|----------|--------|
| 1.1 | Location of defer recovery.Recover() | P1 | **DECISION NEEDED** |
| 1.2 | --inline scope (global/per-tool) | P1 | **DECISION NEEDED** |
| 1.3 | Source attribution contract | P1 | **DECISION NEEDED** |
| 1.4 | Agent panic handling | P1 | **DECISION NEEDED** |
| 1.5 | Log buffer model (Bigend) | P1 | **DECISION NEEDED** |
| 2.1 | Filter state persistence | P2 | Pending |
| 2.2 | Drop indicator UI | P2 | Pending |
| 2.3 | View vs. buffer filter | P2 | Pending |
| 2.4 | Log pane focus model | P2 | Pending |
| 2.5 | High-rate throttling | P2 | Pending |
| 3.1 | Log history persistence | P3 | Nice-to-have |
| 3.2 | Configurable colors | P3 | Nice-to-have |
| 3.3 | Copy log text | P3 | Nice-to-have |
| 3.4 | Clear logs command | P3 | Nice-to-have |
| 3.5 | Collapsible pane | P3 | Nice-to-have |

---

## Next Steps

### This Week: Decide (30 min meeting)
- Review FLOW_ANALYSIS_EXECUTIVE_BRIEF.md
- Answer 5 critical blockers
- Document decisions in ADR

### Next Week: Implement
- Phase 1-4 breakdown from roadmap
- Reference documents open during coding
- Manual testing per strategy

### Week After: Integrate & Test
- Test with all tools
- Performance validation
- Documentation updates
- Deploy to users

---

## How to Use These Documents

### Print or Bookmark
- Print FLOW_ANALYSIS_EXECUTIVE_BRIEF.md for decision meeting
- Keep FLOW_ANALYSIS_VISUAL_GUIDE.md open during coding
- Reference FLOW_ANALYSIS_INLINE_TUI_LOGPANE.md as needed

### Share with Team
- Decision makers: FLOW_ANALYSIS_EXECUTIVE_BRIEF.md
- Developers: All three documents
- QA: FLOW_ANALYSIS_VISUAL_GUIDE.md + testing section from full analysis
- Architects: All three + INLINE_MODE_*.md docs

### Link from Other Docs
- Update CLAUDE.md to mention these analyses
- Link from AGENTS.md in TUI section
- Reference in ADR when implemented

---

## Related Documentation

**Existing Analysis** (comprehensive architecture docs):
- `/root/projects/Autarch/docs/solutions/INLINE_MODE_SUMMARY.md` (4 pages)
- `/root/projects/Autarch/docs/solutions/INLINE_MODE_ARCHITECTURE.md` (8 pages)
- `/root/projects/Autarch/docs/solutions/INLINE_MODE_QUICK_START.md` (3 pages)
- `/root/projects/Autarch/docs/solutions/AUTARCH_TUI_PATTERNS_REFERENCE.md` (12 pages)
- `/root/projects/Autarch/docs/solutions/TUI_LOGGING_AND_INLINE_MODE_ANALYSIS.md` (9 pages)
- `/root/projects/Autarch/docs/solutions/INLINE_MODE_INDEX.md` (index document)

**These New Documents** (flow analysis):
- `/root/projects/Autarch/FLOW_ANALYSIS_INLINE_TUI_LOGPANE.md` (25 pages)
- `/root/projects/Autarch/FLOW_ANALYSIS_EXECUTIVE_BRIEF.md` (8 pages)
- `/root/projects/Autarch/FLOW_ANALYSIS_VISUAL_GUIDE.md` (12 pages)
- `/root/projects/Autarch/FLOW_ANALYSIS_README.md` (this document)

---

## Document Statistics

| Document | File Size | Pages | Read Time |
|----------|-----------|-------|-----------|
| Full Analysis | 50 KB | 25 | 45 min |
| Executive Brief | 13 KB | 8 | 20 min |
| Visual Guide | 21 KB | 12 | 30 min |
| README | 9 KB | 6 | 15 min |
| **Total** | **93 KB** | **51** | **~2 hours** |

---

## Version & Maintenance

**Created:** February 4, 2026
**Status:** Complete and Ready for Review
**Last Updated:** February 4, 2026
**Maintainers:** Claude Code UX Flow Analysis

**When to Update:**
- After implementation (document learnings, discoveries)
- When flow requirements change
- When new gaps are discovered

---

## Questions?

Refer to the appropriate document:
- **"What's in the analysis?"** → This README
- **"Should we build this?"** → FLOW_ANALYSIS_EXECUTIVE_BRIEF.md
- **"What are all the flows?"** → FLOW_ANALYSIS_INLINE_TUI_LOGPANE.md (Part 1)
- **"What gaps exist?"** → FLOW_ANALYSIS_INLINE_TUI_LOGPANE.md (Part 3)
- **"What do I need to decide?"** → FLOW_ANALYSIS_EXECUTIVE_BRIEF.md (Critical Blockers)
- **"How does [flow] work?"** → FLOW_ANALYSIS_VISUAL_GUIDE.md (use index)
- **"What should I test?"** → FLOW_ANALYSIS_INLINE_TUI_LOGPANE.md (Part 7)

---

**End of README**

All analysis documents are located in: `/root/projects/Autarch/`

Start with FLOW_ANALYSIS_EXECUTIVE_BRIEF.md if you're new to this analysis.
