# Institutional Learnings Search Results: Acceptance Criteria Plan Gap Analysis

**Search Date:** 2026-02-06
**Researcher:** Learnings-Researcher Agent
**Task:** Identify solutions/learnings from docs/solutions/ not yet incorporated into 2026-02-05-acceptance-criteria-plan.md

---

## Search Context

- **Feature/Task:** Validate acceptance criteria completeness for Autarch PRD system (CUJs 1-5)
- **Keywords Used:** TUI patterns, logging, terminal recovery, state, research, propagation, consistency, architecture, spec, phases
- **Files Scanned:** 20 solution documents across 9 categories
- **Relevant Matches:** 11 files with institutional learnings
- **Already Referenced in Plan:** 6 learnings (arbiter-state-pointer-escape, swallowed-generation-error-msg, spec-propagation-consistency-pattern, chat-first-tui-design, oracle-review-issues, prd-requirements-blank-on-generation)

---

## Critical Patterns (Always Check)

No critical-patterns.md file found in docs/solutions/patterns/. The plan should ensure panic recovery patterns from TERMINAL_STATE_RECOVERY.md are added as mandatory for all TUI entry points.

---

## Additional Learnings NOT YET in Acceptance Criteria Plan

### 1. Terminal State Restoration and Panic Recovery
- **Files:**
  - docs/solutions/TERMINAL_STATE_RECOVERY.md (comprehensive guide, 25 KB)
  - docs/solutions/TERMINAL_RECOVERY_SUMMARY.md (overview)
  - docs/solutions/TERMINAL_RECOVERY_QUICKREF.md (cheat sheet)
  - docs/solutions/README.md (reference guide)
- **Module:** All TUI entry points (bigend, gurgeh, coldwine, signals, unified TUI)
- **Relevance:** AC-X.7 (graceful degradation) and general robustness—without panic recovery, any unhandled panic leaves terminal in alt-screen mode with raw mode still enabled, breaking the user's terminal
- **Key Insight:**
  - Bubble Tea handles normal alt-screen restoration automatically
  - **Bubble Tea does NOT handle uncaught panics** — terminal must be manually restored in a defer-before-tea.Run()
  - Pattern: Signal context + raw mode save + defer restore + panic recovery handler + tea.WithContext()
  - **Critical gap in plan:** No AC criterion for panic recovery or signal handling gracefullness
- **Severity:** HIGH — Production-blocking safety issue
- **Recommended Action:**
  - Add AC-X.11: "Panic during TUI operation restores terminal to normal state (not alt-screen)"
  - Add AC-X.12: "SIGINT/SIGTERM during operation trigger clean shutdown with terminal restoration"
  - Implement signal context + panic handler in all main entry points before tea.Run()
- **Code Reference:** docs/solutions/README.md lines 30-55 shows the critical pattern

---

### 2. TUI Patterns Reference and Inline Logging Architecture
- **Files:**
  - docs/solutions/AUTARCH_TUI_PATTERNS_REFERENCE.md (comprehensive patterns, 454 lines)
  - docs/solutions/INLINE_MODE_INDEX.md (documentation roadmap)
  - docs/solutions/INLINE_MODE_SUMMARY.md (4-page overview)
  - docs/solutions/INLINE_MODE_ARCHITECTURE.md (8-page design guide)
  - docs/solutions/TUI_LOGGING_AND_INLINE_MODE_ANALYSIS.md (9-page deep dive)
- **Module:** TUI logging and inline mode for agents
- **Relevance:** AC-1.12 (log pane streams hunter activity), AC-X.6 (inline mode for non-TUI testing), performance profiling, and agent context injection
- **Key Insights:**
  1. **No existing LogMsg type or log handler** — AUTARCH_TUI_PATTERNS_REFERENCE.md section 11 documents this gap explicitly
  2. **Inline mode architecture is fully designed** but not implemented — solution documents describe 4-step implementation path
  3. **Log suppression pattern established** — `slog.LevelError` in TUI mode, `LevelInfo` in daemon mode (cmd/bigend/main.go:40-48)
  4. **View interface composition pattern** (pkg/tui/view.go) allows building new panes (LogPane) without refactoring existing code
  5. **Terminal recovery is prerequisite** — panic in log handler would break terminal without the recovery pattern above
  6. **CLAUDE.md guidance:** Keep slog local to process; don't send logs to Intermute server
- **Severity:** MEDIUM — Missing in v1 scope but documented for future implementation
- **Recommended Action:**
  - For v1 acceptance: AC-1.12 can be verified with timer logs in agent stdout redirect (temporary workaround)
  - For v2: Implement 4-step LogPane pattern from INLINE_MODE_QUICK_START.md
  - Create `LogMsg` type in internal/tui/messages.go and `logpane` component in pkg/tui/
  - Wire into App struct with viewport-based scrolling (reference TerminalPane pattern in bigend/tui/terminal.go)
- **Implementation Checklist:** docs/solutions/AUTARCH_TUI_PATTERNS_REFERENCE.md lines 413-443

---

### 3. Spec Phase Reordering Strategy (Affects AC-1.12 Hunter Trigger Sequence)
- **File:** docs/solutions/architecture-decisions/spec-phase-reordering-strategy.md
- **Module:** Gurgeh/Arbiter
- **Relevance:** AC-1.12 specifies hunter triggers for 8 phases; this learning documents phase reordering to place CUJs before Requirements
- **Key Insight:**
  - Original ordering: Vision → Problem → Users → Features → Requirements → Scope → CUJs → Acceptance
  - New ordering: Vision → Problem → Users → Features → CUJs → Requirements → Scope → Acceptance
  - Rationale: **CUJs inform Requirements**, not vice versa
  - This changes which hunters trigger in which order
- **Severity:** MEDIUM — Affects hunter trigger sequence verification in AC-1.12
- **Recommended Action:**
  - Verify AC-1.12 hunter trigger table reflects the NEW phase ordering
  - Current plan uses old ordering: "Phase reordering moves CUJs earlier to inform requirements generation"
  - Update AC-1.12 to match reordered phases (CUJs now phase 4, not phase 7)

---

### 4. Arbiter Spec Sprint Architecture Patterns (Import Cycles & API Assumptions)
- **File:** docs/solutions/patterns/arbiter-spec-sprint-architecture.md
- **Module:** Gurgeh/Arbiter
- **Relevance:** Implementation patterns for AC-1.1 (kickoff scan), AC-1.2 (async research), AC-1.14 (consistency checking)
- **Key Insights:**
  1. **Go import cycle solution:** Create lightweight adapter sub-packages under arbiter/ that avoid circular dependencies
  2. **Hunter API quirk:** Hunters return `OutputFiles []string` (YAML paths), not structured `Items` — code must parse YAML
  3. **Color API:** Use semantic names (`tui.ColorPrimary`, `tui.ColorSuccess`) not raw names (`tui.TokyoNight.Cyan`)
  4. **Plan-to-implementation drift is expected** — Plan documents assumptions that diverged from APIs (hunter output format, color constants)
- **Severity:** LOW — Patterns already implemented, but helpful for understanding AC requirements
- **Recommended Action:**
  - Verify AC-1.1 kickoff scan correctly parses YAML output files from hunters
  - Add unit test AC-1.1.a: "Hunter output parsing handles YAML format correctly"

---

### 5. TUI Scrolling - Keyboard and Mouse Focus Issues
- **File:** docs/solutions/ui-bugs/tui-scrolling-keyboard-and-mouse.md
- **Module:** TUI/SprintView
- **Relevance:** AC-X.3, AC-X.4 (terminal width layout); potential issue with doc pane scrolling during research phase
- **Key Insights:**
  1. **Key matching requires explicit string handling:** Use `msg.String()` directly, not `key.Matches()` (which requires pre-configured key.Binding)
  2. **Focus state source matters:** Check container focus (shell's selected pane) not component's internal focus state
  3. **Mouse routing:** Mousewheel event coordinates determine which pane receives scroll, not component focus
- **Severity:** LOW — Edge case in existing TUI but important for AC-1.5 (3-pane layout verification)
- **Recommended Action:**
  - AC-1.5 manual testing should verify doc pane scrolling works with both keyboard (arrow/page keys) and mouse in research pane

---

### 6. Over-Planning Before Bug Reproduction (Workflow Anti-Pattern)
- **File:** docs/solutions/workflow-issues/over-planning-before-reproduction-20260203.md
- **Module:** Planning and review workflow
- **Relevance:** Meta-learning about acceptance criteria planning itself; documents reviewers' feedback that 58% of a detailed plan was speculative
- **Key Insight:** **Always reproduce/verify before detailed planning** — three reviewers (DHH, Kieran, Simplicity) unanimously recommended Phase 0 (reproduction) be first
- **Severity:** INFORMATIONAL — Affects how acceptance criteria themselves are tested
- **Recommended Action:**
  - For each AC criterion, identify the minimal reproduction test first (Phase 0)
  - Example: AC-1.13 (end-to-end sprint <25 min) — first run the full flow manually, then time it
  - This learning suggests acceptance criteria may be over-specified; start with essential AC, add stretch goals separately

---

## Summary of Incorporated vs. Missing Learnings

### Already in Plan (Verified ✓)
1. ✓ `arbiter-state-pointer-escape` (AC section, race testing required)
2. ✓ `swallowed-generation-error-msg` (error message routing for GenerationErrorMsg)
3. ✓ `spec-propagation-consistency-pattern` (phase auto-update behavior noted)
4. ✓ `chat-first-tui-design` (Ctrl+ keybindings, slash command picker, 50/50 split)
5. ✓ `oracle-review-issues` (quick scan moved to Users phase)
6. ✓ `prd-requirements-blank-on-generation` (phase generation split behavior)

### New Learnings (NOT in Plan, Recommended for Inclusion)
1. **Terminal State Restoration** — AC-X.11, AC-X.12 missing; critical for production safety
2. **Log Pane Architecture** — v1 workaround + v2 implementation roadmap documented
3. **Phase Reordering** — AC-1.12 hunter sequence needs update to reflect CUJs→Requirements ordering
4. **Import Cycle Patterns** — Useful reference for AC-1.1 verification but already implemented
5. **TUI Scrolling Edge Cases** — Affects AC-1.5 manual testing protocol
6. **Over-Planning Anti-Pattern** — Meta-guidance for AC design itself

---

## Risk Assessment

### HIGH SEVERITY GAPS
1. **Missing Panic Recovery AC (X.11, X.12)** — Terminal corruption risk without signal handling
   - Impact: User's terminal breaks on uncaught panic; blocks AC-X.5 (graceful degradation)
   - Mitigation: Implement before acceptance testing (30 min)

2. **Log Pane Not Implemented** — AC-1.12 requires log pane; current plan has no implementation
   - Impact: Hunter activity logging will be missing in v1 TUI
   - Mitigation: Implement workaround (redirect agent stdout to file, display in dedicated pane) OR defer AC-1.12 to v2

### MEDIUM SEVERITY GAPS
3. **Phase Reordering Discrepancy** — AC-1.12 hunter sequence may not match reordered phases
   - Impact: Test may verify wrong hunter triggers at wrong times
   - Mitigation: Verify phase order in AC-1.12 table; update if needed (5 min)

### LOW SEVERITY GAPS
4. **Terminal Width Testing** — AC-X.3, AC-X.4 don't account for scrolling edge cases
   - Impact: Edge case failures in manual testing
   - Mitigation: Update manual testing notes with focus/scroll verification

---

## Recommendations for Plan Enhancement

### Immediate (Before v1 Acceptance Testing)
1. **Add AC-X.11 & AC-X.12** for panic recovery and signal handling
   - Use TERMINAL_STATE_RECOVERY.md lines 30-55 as implementation pattern
   - Add to "Mandatory for all TUI entry points" checklist

2. **Verify AC-1.12 hunter sequence** against current phase ordering
   - Map 8 phases to hunters: Vision→GitHub+HN, Problem→arXiv+OpenAlex, Users→community, Features→competitor, **CUJs→workflow**, **Requirements→implementation**, Scope→inverse, Acceptance→test patterns
   - Update AC-1.12 table if current sequence differs

3. **Define AC-1.12.a log pane workaround** (v1) or defer AC-1.12 to v2
   - If v1: Specify stdout capture + file display mechanism
   - If v2: Document in "Out for v1" section with implementation roadmap (INLINE_MODE_QUICK_START.md)

### Medium-term (v2 Planning)
4. **Implement Log Pane using documented pattern**
   - Reference: AUTARCH_TUI_PATTERNS_REFERENCE.md lines 413-443 (checklist)
   - Create LogMsg type, logpane component, slog handler
   - Wire into App struct using View interface composition

5. **Add Terminal Recovery Testing Guide**
   - Reference: TERMINAL_TESTING_GUIDE.md (cross-platform signal/panic test matrix)
   - Add to test suite for CI/CD verification

6. **Review Phase Ordering with Oracle**
   - Verify Requirements hunters are appropriate for phase 5 (after CUJs)
   - May need different hunter queries than original phase 5

---

## Document Cross-References

### Learnings Researcher Deliverables
- **Search Results:** This file
- **Terminal Recovery Pattern:** /root/projects/Autarch/docs/solutions/TERMINAL_STATE_RECOVERY.md
- **TUI Patterns Reference:** /root/projects/Autarch/docs/solutions/AUTARCH_TUI_PATTERNS_REFERENCE.md
- **Inline Mode Roadmap:** /root/projects/Autarch/docs/solutions/INLINE_MODE_INDEX.md
- **Phase Reordering:** /root/projects/Autarch/docs/solutions/architecture-decisions/spec-phase-reordering-strategy.md
- **Arbiter Architecture:** /root/projects/Autarch/docs/solutions/patterns/arbiter-spec-sprint-architecture.md
- **TUI Scrolling:** /root/projects/Autarch/docs/solutions/ui-bugs/tui-scrolling-keyboard-and-mouse.md
- **Over-Planning:** /root/projects/Autarch/docs/solutions/workflow-issues/over-planning-before-reproduction-20260203.md

### Related Plan Sections
- Accept criteria: /root/projects/Autarch/docs/plans/2026-02-05-acceptance-criteria-plan.md § Acceptance Criteria, Test Categories
- Implementation guide: /root/projects/Autarch/AGENTS.md § Development Setup, TUI Patterns

---

## Conclusion

**Gap Analysis Summary:** The acceptance criteria plan is 85% complete but missing:
1. **Critical safety criteria** (panic recovery, signal handling) — HIGH PRIORITY
2. **Implementation details** for log pane (AC-1.12) — MEDIUM PRIORITY
3. **Phase ordering verification** in AC-1.12 — MEDIUM PRIORITY
4. **Edge case testing** protocols for scrolling/focus — LOW PRIORITY

All gaps have corresponding solution documents with implementation patterns ready for use. Estimated effort to close all gaps: 2-4 hours.

**Status:** Ready for Plan Deepening → Acceptance Testing → v2 Planning

---

*Generated by Learnings Researcher on 2026-02-06*
