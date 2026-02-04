# Inline Mode & Terminal Logging - Document Index

**Objective:** Comprehensive analysis of TUI patterns in Autarch for implementing inline logging and centralized terminal writing

**Created:** February 4, 2026
**Status:** Analysis Complete - 4 detailed reference documents ready for implementation

---

## Documents Overview

### 1. **INLINE_MODE_QUICK_START.md** ⭐ START HERE

**Length:** 3 pages | **Read Time:** 10 minutes
**Audience:** Developers ready to code

**Contents:**
- The problem (logs disappear in TUI mode)
- 4-step solution (copy-paste ready)
- Quick testing checklist
- Common issues & fixes

**Use This When:**
- You're ready to start implementing
- You want a fast overview of what's needed
- You need exact code snippets to start with

**Key Section:** "4 Steps to Inline Logging" with code samples

---

### 2. **INLINE_MODE_SUMMARY.md** ⭐ OVERVIEW

**Length:** 4 pages | **Read Time:** 15 minutes
**Audience:** Architects, reviewers, decision makers

**Contents:**
- Key findings (5 major areas analyzed)
- Established patterns (ready to use)
- Implementation roadmap (4 phases)
- Risk assessment with mitigations
- Success criteria

**Use This When:**
- Planning the implementation sprint
- Reviewing architectural decisions
- Communicating status to stakeholders
- Need a 15-minute executive brief

**Key Section:** "Established Patterns (Ready to Use)" table

---

### 3. **INLINE_MODE_ARCHITECTURE.md** ⭐ DESIGN

**Length:** 8 pages | **Read Time:** 25 minutes
**Audience:** Architects, senior developers

**Contents:**
- Current TUI flow (text diagram)
- Proposed flow (with inline logging)
- Component changes (5 detailed sections)
- Data flow diagram
- File changes summary
- Testing strategy
- Rollout plan (4 phases)

**Use This When:**
- Designing the implementation before coding
- Understanding data flows
- Planning testing approach
- Need diagrams for documentation

**Key Section:** "Component Changes" (what gets added/modified)

---

### 4. **TUI_LOGGING_AND_INLINE_MODE_ANALYSIS.md** ⭐ DEEP DIVE

**Length:** 9 pages | **Read Time:** 40 minutes
**Audience:** Senior developers, architects, maintainers

**Contents:**
- Current logging patterns in detail
- Terminal cleanup mechanisms
- Existing inline/log routing attempts
- pkg/tui vs tool-specific TUI relationship
- Bubble Tea program configuration
- Established patterns (with code references)
- Implementation strategy (3 phases)

**Use This When:**
- Need to understand the full context
- Researching specific patterns
- Building the detailed implementation plan
- Writing documentation

**Key Section:** "7. Key Findings for Implementation"

---

### 5. **AUTARCH_TUI_PATTERNS_REFERENCE.md** ⭐ COPY-PASTE

**Length:** 12 pages | **Read Time:** Reference doc
**Audience:** Developers during implementation

**Contents:**
- 12 detailed pattern references with code
- Entry point pattern
- TUI program initialization
- Cleanup & defer pattern
- View interface contract
- Async message pattern
- TerminalPane component template
- Writer-based progress pattern
- Unified app structure
- Shared TUI styling
- Layout components
- Quick reference table

**Use This When:**
- You need exact code to copy (during implementation)
- You forgot how something works
- You're looking for a specific pattern
- You need file names and line numbers

**Key Section:** "Pattern Quick Reference" lookup table

---

## Recommended Reading Path

### For Implementation (2-4 hours of coding)
1. Start: **INLINE_MODE_QUICK_START.md** (10 min)
2. Reference: **AUTARCH_TUI_PATTERNS_REFERENCE.md** (keep open while coding)
3. Debug: Use specific sections from **TUI_LOGGING_AND_INLINE_MODE_ANALYSIS.md**

### For Planning (30 minutes)
1. Read: **INLINE_MODE_SUMMARY.md** (15 min)
2. Reference: "Implementation Roadmap" section
3. Check: Risk assessment table

### For Architecture Review (45 minutes)
1. Read: **INLINE_MODE_ARCHITECTURE.md** (25 min)
2. Review: Data flow diagram
3. Verify: File changes summary

### For Deep Understanding (1-2 hours)
1. Read: **TUI_LOGGING_AND_INLINE_MODE_ANALYSIS.md** (40 min)
2. Reference: **AUTARCH_TUI_PATTERNS_REFERENCE.md** (as needed)
3. Study: Code locations provided in each section

---

## Key Findings at a Glance

| Finding | Status | Impact | Action |
|---------|--------|--------|--------|
| **Logging Suppressed** | ✅ Found | slog disabled in TUI | Create custom handler |
| **Terminal Cleanup** | ✅ Found | Auto-handled by Bubble Tea | Add panic recovery |
| **No Log Routing** | ✅ Confirmed | Logs disappear | Add LogMsg type |
| **View Interface** | ✅ Found | Ready to compose | Build on existing patterns |
| **Message System** | ✅ Found | Async working | Extend with LogMsg |
| **Component Template** | ✅ Found | TerminalPane reusable | Copy pattern for LogPane |

---

## Implementation Breakdown

### Phase 1: Infrastructure (2 hours)
- Add LogMsg to messages.go
- Implement slog.Handler
- Create LogPane component
- **Files:** 3 new + 1 modified

### Phase 2: Integration (1 hour)
- Wire LogHandler into Run()
- Add LogPane to App
- Test basic flow
- **Files:** 1 modified

### Phase 3: Hardening (1 hour)
- Add signal handlers
- Implement panic recovery
- Performance testing
- **Files:** 2 modified

### Phase 4: Polish (1 hour)
- Styling (colors, icons)
- UI filtering
- Documentation
- **Files:** 2 modified

**Total Time:** 5-7 hours for MVP + polish

---

## Verification Checklist

After implementing, verify:

- [ ] **Logs appear inline** - Agent output visible in real-time during interview
- [ ] **Logs scroll** - Auto-scrolling to newest log entry
- [ ] **Terminal restores** - Ctrl+C brings terminal back to normal
- [ ] **No crashes** - Panic in agent doesn't break TUI
- [ ] **Performance ok** - 500+ log entries doesn't lag
- [ ] **All tools work** - Gurgeh, Coldwine, Pollard all log
- [ ] **Tests pass** - go test ./...

---

## File Location Summary

All documents are in: `/root/projects/Autarch/docs/solutions/`

| Document | Filename | Size |
|----------|----------|------|
| Quick Start | INLINE_MODE_QUICK_START.md | 9 KB |
| Summary | INLINE_MODE_SUMMARY.md | 8 KB |
| Architecture | INLINE_MODE_ARCHITECTURE.md | 10 KB |
| Deep Dive | TUI_LOGGING_AND_INLINE_MODE_ANALYSIS.md | 10 KB |
| Reference | AUTARCH_TUI_PATTERNS_REFERENCE.md | 12 KB |
| Index | INLINE_MODE_INDEX.md | This file |

**Total:** ~59 KB of analysis

---

## Key Code References

### Where Logging Happens
- `cmd/bigend/main.go:40-48` - Current slog setup
- `cmd/autarch/main.go:95-99` - Unified TUI entry
- `internal/tui/app.go:364-369` - App.Run() function

### Where to Add Code
- `internal/tui/messages.go` - Add LogMsg (NEW)
- `pkg/tui/loghandler/` - Create handler (NEW)
- `pkg/tui/logpane/` - Create component (NEW)
- `internal/tui/app.go` - Wire it together

### Patterns to Copy From
- `internal/bigend/tui/terminal.go` - TerminalPane pattern (viewport)
- `pkg/tui/view.go` - View interface contract
- `internal/tui/messages.go` - Message type examples
- `internal/coldwine/cli/init_flow.go` - Progress callback pattern

---

## Decision Points Made

1. **Keep slog suppressed at process level** - Don't spam logs to stdout
2. **Use Bubble Tea message system** - Type-safe, async-friendly
3. **LogPane as reusable component** - Goes in pkg/tui for all tools
4. **Circular buffer (500 entries)** - Memory-bounded, auto-rotate
5. **Non-blocking channel** - Drop if full, don't block agent
6. **Automatic scroll-to-bottom** - Better UX for log tail following

---

## Related Projects/Docs

- **Intermute:** For domain API (Spec, Insight, etc.)
- **CLAUDE.md:** Global instructions and conventions
- **AGENTS.md:** Development guide for tools
- **docs/ARCHITECTURE.md:** System-wide architecture

---

## Questions?

For specific pattern details: See **AUTARCH_TUI_PATTERNS_REFERENCE.md**
For implementation guidance: See **INLINE_MODE_QUICK_START.md**
For architectural rationale: See **INLINE_MODE_ARCHITECTURE.md**

---

## Document Maintenance

**Last Updated:** February 4, 2026
**Status:** Complete analysis, ready for implementation
**Next Update:** After Phase 1 implementation, document learnings

When updating, maintain these sections in all docs:
- Problem statement (context)
- Solution overview (why this works)
- Code references (where in codebase)
- Implementation steps (how to build it)
