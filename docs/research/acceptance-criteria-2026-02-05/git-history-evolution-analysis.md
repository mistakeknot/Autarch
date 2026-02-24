# Git history evolution analysis

**Source:** Agent result from session b817bfb0
**Task ID:** ac480e1
**Date:** 2026-02-05

---

Now I have a comprehensive picture. Let me compile the final analysis.

---

## Git History Analysis for Autarch Acceptance Criteria Plan

### Repository Overview

The repository spans **399 commits** over **16 days** (2026-01-21 to 2026-02-05), written by a single developer (`mistakeknot`, 395 commits, with 4 early commits under `mk`). This is an extremely high-velocity solo project with an average of 25 commits per day, and notable burst days: 64 commits on Jan 22 (initial buildout), 42 on Jan 26 (Intermute deep integration + Arbiter creation), and a massive **69 commits on Feb 4** (the exploration/Claude Code integration day).

---

### 1. Arbiter Evolution -- The Central Engine

The Arbiter (`/root/projects/Autarch/internal/gurgeh/arbiter/`) is the most architecturally significant module in the repository. Its history tells a clear story of rapid maturation under pressure.

**Timeline:**

| Date | Commit | Event |
|------|--------|-------|
| Jan 26 | `04965ef` | Core sprint state types created |
| Jan 26 | `8110d2a` | Sprint orchestrator added (the foundational commit) |
| Jan 26 | `2ac838a` | Pollard research integration into sprints |
| Jan 26 | `d32d081` | Real Pollard scanner, quality scoring, research TUI, integration tests |
| Jan 27 | `30f2514` | StartReview() for vision spec review sprints |
| Jan 27 | `f13e935` | research_phases.go added -- only commit ever touching this file |
| Jan 31 | `816c37c` | Chat-driven orchestrator methods |
| Jan 31 | `202628c` | Scan artifacts threaded through engine |
| Feb 1 | `a6b026d` | **FIX**: Thinking preamble separation |
| Feb 1 | `f406a0b` | **FIX**: Oracle architecture review findings (issues 3-6) |
| Feb 1 | `33dbc60` | **CRITICAL FIX**: State() pointer escape and concurrency races |
| Feb 1 | `dfc2cc7` | Tests added for phase transitions, blocker gating, confidence scoring |
| Feb 1 | `09ac111` | Pollard/Intermute research integration wired end-to-end |
| Feb 4 | `0988081` | GeneratePhase for Claude Code-powered transitions |
| Feb 4 | `94366fe` | **FIX**: Generate later PRD phases via Claude Code |
| Feb 4 | `f287b7b` | Propagate changes across all phases on iteration |
| Feb 4 | `a96a70e` | **REFACTOR**: CUJs phase moved before Requirements |
| Feb 4 | `cc3b855` | Cache all 8 phases in initial exploration |
| Feb 5 | `fe4cc34` | **REFACTOR**: Remove Ctrl+1-3 keybindings (freeing for tab switching) |
| Feb 5 | `ed9a976` | Reuse Claude Code session across sprint phases |

**Key findings:**

The most changed arbiter file is **`/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go`** at 23 commits -- the single highest-churn Go source file in the project. The `types.go` file has 12 commits. This is the beating heart of Gurgeh.

The **concurrency race fix on Feb 1** (`33dbc60`) is the most important single commit to understand. It touched 5 files, added 298 lines, and introduced `Clone()` methods with deep-copy semantics for `SprintState`, `SectionDraft`, `QuickScanResult`, and `VisionContext`. This directly addresses the acceptance plan's institutional learning about `-race` testing. The commit message itself documents the exact race: TUI goroutine reading state at ~60 FPS while `ProcessChatMessage`/`ChatAcceptDraft` goroutines mutate concurrently.

The **phase reordering on Feb 4** (`a96a70e`) is a clean 12-line-delta commit that rearranged the 8 phases. The new order (Vision, Problem, Users, Features+Goals, CUJs, Requirements, Scope, Acceptance) replaced the old order where CUJs came at position 7. This is relevant to AC-1.12 (hunter trigger sequence) because `research_phases.go` was written when CUJs was at position 7 and has **never been updated since its single commit on Jan 27**. The acceptance criteria should verify the phase-to-hunter mapping reflects the current ordering.

The **Oracle review fix** (`f406a0b`) and **thinking preamble fix** (`a6b026d`) on Feb 1 show an external review (Oracle = cross-AI GPT review) surfaced architectural issues that required immediate fixes. This is a pattern of external review driving quality.

**Arbiter test coverage**: Tests (`orchestrator_test.go`, `types_test.go`) were added Feb 1 alongside the race fix -- meaning tests came **6 days after the orchestrator was created**. The test commit `dfc2cc7` added phase transition, blocker gating, and confidence scoring tests. There is no test for `research_phases.go` or `intermute.go` in the arbiter.

---

### 2. TUI Churn -- The Most Active Subsystem

The TUI has the highest file churn in the entire repository. The top 5 most-changed files are all TUI-related:

| File | Commits | Risk Level |
|------|---------|------------|
| `/root/projects/Autarch/internal/tui/views/kickoff.go` | 42 | **Very High** |
| `/root/projects/Autarch/internal/tui/unified_app.go` | 37 | **Very High** |
| `/root/projects/Autarch/internal/tui/views/sprint_view.go` | 23 | **High** |
| `/root/projects/Autarch/internal/tui/messages.go` | 15 | **Medium** |
| `/root/projects/Autarch/pkg/tui/chatpanel.go` | 11 | **Medium** |

**Fix-after-fix chains (indicates fragility):**

The most revealing pattern is the **doc panel scrolling saga on Feb 4**, a classic fix chain:

1. `dc062f4` -- feat: add keyboard shortcuts for doc panel scrolling
2. `cd54c84` -- **fix**: enable doc panel scrolling with arrow keys and j/k
3. `02d7b2d` -- **fix**: remove j/k/g from doc panel scrolling
4. `96a927c` -- feat: add ctrl+j/k for doc panel scrolling
5. `4157d2d` -- **fix**: use ctrl+n/p instead of ctrl+j/k for scrolling
6. `0a573ae` -- **fix**: use shell focus state for mouse wheel scrolling

Six commits to get scrolling right. This pattern indicates the TUI keybinding system is difficult to get right on first attempt -- there are constant conflicts between chat input, panel navigation, and terminal sequences. The acceptance criteria for AC-X.3 (layout rendering), the institutional learning about Ctrl+ prefixes only, and AC-1.5 (3-pane layout with slash command picker) should all be tested thoroughly.

Similarly, the **ANSI-aware rendering saga** across Jan-Feb shows multiple fixes:
- `a50675c` -- fix: handle ANSI escape codes in split layout padding
- `ccc9419` -- fix: constrain ChatPanel output to panel width
- `75bf58f` -- fix: ensure Composer width respects panel constraints
- `a53e718` -- fix: properly truncate lines in split layout
- `1f38f3f` -- fix: use ANSI-aware string operations in help overlay
- `cba7e13` -- fix: apply ANSI-aware overlay fix to app.go, document solution

This shows ANSI handling is a recurring pain point. The solution was documented in `docs/solutions/` -- evidence that the team captures institutional knowledge from repeated fixes.

**Feb 4 was the highest-risk day for TUI**: 24+ commits touching `internal/tui/` in a single day, including scan refactoring, exploration integration, chat-first workflow, inline mode, slash commands, and more. This kind of concentrated change day often leaves subtle integration issues.

---

### 3. Intermute Integration History -- Young and Shallow

The Intermute integration has a very clear layered history:

| Date | Commit | Layer |
|------|--------|-------|
| Jan 24 | `0a1927d` | **Layer 1**: Client wiring into all 4 modules (223 lines, mostly boilerplate) |
| Jan 26 | `0da5243` | Reservation and inbox count methods added to `pkg/intermute` |
| Jan 26 | `a65df98` | **Layer 2**: Deep integration (sessions, sync, publisher -- 1,129 lines with tests) |
| Jan 26 | `3237851` | Spec generation gap analysis subagents |
| Jan 27 | `f13e935` | **Layer 3**: Coordination infrastructure (2,835 lines, massive commit, includes `research_phases.go`, `SaveRevision`, `pkg/intermute/client.go`) |
| Jan 28 | `531e5ce` | **FIX**: Bound Intermute heartbeat and LLM summary (guardrails fix) |
| Feb 1 | `09ac111` | Pollard/Intermute research integration wired end-to-end in arbiter |
| Feb 3 | `7844873` | Coldwine emits task.blocked events to Gurgeh |

**Critical observations:**

1. The **`pkg/intermute/client.go`** was created in a single massive commit (`f13e935`, 120 lines in that file, 2,835 total). This is the shared Intermute client -- it has the `Reserve()` method at line 940. It has **never been modified since that commit**.

2. The **Coldwine local `ReservePaths`** in `/root/projects/Autarch/internal/coldwine/storage/coordination.go` uses **exact string matching** on paths (line 258: `WHERE path = ?`). The acceptance plan correctly identifies this as a critical gap -- glob patterns like `internal/auth/**/*.go` are stored as literal strings. Two agents can hold `internal/auth/**/*.go` and `internal/**/*.go` simultaneously without conflict detection.

3. The Coldwine-to-Intermute broadcaster (`/root/projects/Autarch/internal/coldwine/intermute/broadcaster.go`) has only **2 commits**: its creation (Jan 26) and the task.blocked events addition (Feb 3). It handles outbound events only, with no inbound Agent Teams integration, confirming the acceptance plan's Gap 2.

4. The **guardrails fix** (`531e5ce`) on Jan 28 is the only bug fix in the entire Intermute integration history. It bounded heartbeat and LLM summary operations -- a performance constraint, not a logic bug. This means the integration layer has been written once and essentially untouched, which for code this young (10 days old) suggests it has not been exercised under real concurrent workloads.

---

### 4. Recent Commit Patterns -- Fix Density Analysis

Across all 399 commits, I count approximately **38 fix commits** (commits with `fix(` prefix). The distribution is telling:

| Area | Fix Count | Notes |
|------|-----------|-------|
| TUI (keybindings, layout, rendering) | ~22 | By far the most fragile area |
| Arbiter (state, generation, phases) | ~5 | Concentrated on Feb 1 after Oracle review |
| Exploration (evidence parsing, logs) | ~4 | All on Feb 4 during integration |
| Kickoff (UX details) | ~3 | Minor UI corrections |
| Sprint (Esc behavior, scan) | ~2 | Behavioral refinements |
| Guardrails (Intermute bounds) | ~1 | Single performance fix |
| Docs | ~1 | Jekyll processing |

**Pattern: Fix clustering reveals integration boundaries.** Fixes cluster on specific dates corresponding to integration days:
- **Feb 1**: 5 fixes (arbiter race, Oracle findings, ANSI overlay, error message swallowing) -- post-review cleanup day
- **Feb 4**: 9 fixes (scrolling, evidence parsing, name fallback, phase generation) -- massive integration day

The **error message swallowing fix** (`8752350`) on Jan 31 is directly called out in the acceptance plan's institutional learnings: "Parent views must pass error messages to children. `GenerationErrorMsg` from arbiter must reach SprintView's chat panel, not be swallowed." This was a real bug that was fixed and then documented as a pattern to watch for.

There are **zero fix commits** in:
- `pkg/signals/broker.go` (1 commit total, never fixed because barely used)
- `internal/coldwine/intermute/` (no fixes at all)
- `internal/gurgeh/specs/evolution.go` (1 commit, `SaveRevision` has the non-atomic write issue the plan identifies but it has never been fixed)
- `pkg/intermute/client.go` (1 commit, never modified)

The absence of fixes in these areas is not reassurance -- it indicates low exercise. The acceptance criteria should treat these as **untested code paths**.

---

### 5. Key Contributor and Architectural Intent

The repository is effectively a **single-author project** (`mistakeknot`, 395 of 399 commits). The 4 `mk` commits are from the same person under a different git config.

Several commits are co-authored with `Claude Opus 4.5 <noreply@anthropic.com>`, indicating AI-assisted development. Notable AI-assisted commits:
- `a65df98` -- Intermute deep integration (1,129 lines)
- `33dbc60` -- State() pointer escape race fix (298 net new lines)
- `a96a70e` -- CUJs phase reordering

The commit message style follows **conventional commits** consistently: `type(scope): description`. The types used are `feat`, `fix`, `refactor`, `perf`, `test`, `docs`, `chore`. This is disciplined and searchable.

The **Oracle (cross-AI review)** pattern is visible in the history: commit `f406a0b` ("address Oracle architecture review findings") shows external GPT-5 review driving architectural corrections. This is a healthy quality gate that the acceptance plan should leverage.

---

### 6. Phase Ordering Changes -- Traced Through History

The 8-phase Arbiter Sprint has evolved:

**Original ordering** (Jan 26, `04965ef`): Created in `types.go` as core sprint state types. The initial order was:
1. Vision, 2. Problem, 3. Users, 4. Features, 5. Requirements, 6. Scope, 7. CUJs, 8. Acceptance

**Phase reorder** (Feb 4, `a96a70e`): CUJs moved from position 7 to position 5:
1. Vision, 2. Problem, 3. Users, 4. Features+Goals, 5. **CUJs**, 6. Requirements, 7. Scope, 8. Acceptance

The commit message explains the rationale: CUJs describe HOW users accomplish goals, they flow from Users+Features, and Requirements should derive FROM user journeys.

**Impact on acceptance criteria**: `research_phases.go` (created Jan 27 at `f13e935`, never modified) may still reference the **old phase ordering**. The plan's AC-1.12 maps specific hunters to specific phases. If `research_phases.go` uses phase indices rather than phase names, the reordering could break the hunter-to-phase mapping. This file has exactly 1 commit and 0 tests -- it deserves specific attention.

The exploration caching commits on Feb 4-5 (`cc3b855`, `ed9a976`) changed how phases are generated, caching all 8 phases during initial exploration. This is a recent performance optimization that fundamentally changes the phase transition flow -- phases are now pre-generated rather than generated on-demand. AC-1.1 timing targets should account for this: the structural scan is fast, but the initial exploration now front-loads all phase generation.

---

### 7. File Reservation Code -- Exact String Matching, Never Enhanced

**Creation**: The `ReservePaths` function in `/root/projects/Autarch/internal/coldwine/storage/coordination.go` was introduced in commit `99a806a` (Jan 21, "initial Vauxpraudemonium monorepo structure") -- it has been present since **day one** of the repository.

**Current implementation** (lines 239-289 of `coordination.go`): The function operates within a SQL transaction, iterating through requested paths and checking for conflicts via:
```sql
SELECT owner, exclusive FROM reservations WHERE path = ? AND released_ts IS NULL AND expires_ts > ?
```

This is **exact string matching**. The `WHERE path = ?` clause means `internal/auth/**/*.go` and `internal/auth/jwt/*.go` will **never conflict** despite the second being a strict subset of the first when interpreted as globs.

**Test coverage**: There are tests in `/root/projects/Autarch/internal/coldwine/storage/coordination_test.go` (`TestReservePathsConflicts` at line 90) and `/root/projects/Autarch/internal/coldwine/coordination/compat_test.go` (`TestReservePathsAndRelease` at line 100), but these only test exact-match scenarios (both test `"a.go"` conflicting with `"a.go"`).

**The Intermute side**: `pkg/intermute/client.go` line 940 shows the `Reserve()` method delegates to `c.base.Reserve()` which calls the Intermute service. The acceptance plan correctly identifies that the Intermute storage layer also does a simple INSERT with no overlap check.

**Bug fix history**: Zero. No fix commits have ever touched the reservation code. Combined with exact-match-only semantics, this means AC-4.2 ("Overlapping reservation request rejected with blocking task ID within 1 second") is fundamentally untestable in the current codebase. The plan's characterization of this as a **blocking gap** is well-supported by the git history.

---

### 8. Signal Broker -- Write-Once, Minimal Infrastructure

The signal broker at `/root/projects/Autarch/pkg/signals/broker.go` has **exactly 1 commit** in its entire history (`2f01d80`, Jan 27, "add local-only coordination APIs"). It was created as part of a 14-file commit. It is 116 lines of pure fan-out with:

- No deduplication (the `(spec_id, type, affected_field)` constraint mentioned in the PRD exists nowhere)
- No persistence (in-memory only)
- Silent signal dropping when subscriber buffers fill (lines 51-54: `select { case sub.ch <- sig: default: }`)
- No bridge to Intermute events

The acceptance plan's Gap 3 (signal deduplication) and Gap 4 (signal transport ambiguity) are fully confirmed by the git history. This code was written once as infrastructure scaffolding and never revisited.

---

### 9. SaveRevision -- Non-Atomic Writes Confirmed

The `/root/projects/Autarch/internal/gurgeh/specs/evolution.go` file has **exactly 1 commit** (`f13e935`, Jan 27). The `SaveRevision` function (lines 41-81) performs:

1. Line 47-48: Mutates `spec.Version` as a side effect **before** any file writes
2. Line 66: Writes snapshot YAML file
3. Line 76: Writes revision metadata YAML file

If the process crashes between the two `WriteFile` calls, you get a snapshot without revision metadata. If `spec.Version` was incremented but the first write fails, the version counter has been corrupted in memory. There is no write-to-temp-then-rename pattern, and no rollback on partial failure.

This has never been fixed because it has never been hit -- but under AC-4.4 (reservation auto-release + concurrent operations) and AC-1.15 (concurrent phase completions), it becomes a real risk. The acceptance plan correctly flags this.

---

### 10. Areas Requiring Special Attention from Acceptance Criteria

Based on the complete git history analysis, here is my prioritized risk assessment:

**Highest Risk (code exists but is fundamentally incomplete):**
1. **Glob overlap detection** -- exact string matching only, untestable for AC-4.2
2. **Signal deduplication** -- no implementation exists, AC-3.4a untestable
3. **Agent Teams bridge** -- no inbound integration, AC-4.8 has no mechanism
4. **Signal transport path** -- two separate systems, no bridge, AC-3.4 path undefined

**High Risk (code exists but untested under load):**
5. **SaveRevision atomicity** -- non-atomic dual writes, version side-effect mutation
6. **SQLite single-connection bottleneck** -- `MaxOpenConns(1)` at `/root/projects/Autarch/pkg/db/open.go:22` will serialize all concurrent agent writes
7. **Intermute integration** -- written once (Jan 24-27), single guardrails fix, essentially untouched for 9 days
8. **research_phases.go** -- 1 commit, 0 tests, potentially stale after phase reordering

**Medium Risk (active development, fix chains indicate ongoing refinement):**
9. **TUI keybinding system** -- 6-commit scrolling saga shows this requires careful testing
10. **unified_app.go** -- 37 commits, ~400 lines of transition handlers, identified for refactoring in Phase 2
11. **kickoff.go** -- 42 commits, the most changed file, but fixes are mostly UX polish
12. **ANSI rendering** -- documented solution exists but multiple files affected

**Lower Risk (recently stabilized, has tests):**
13. **Arbiter orchestrator** -- high churn (23 commits) but race condition fixed, tests added, recent commits are performance only
14. **Exploration/caching** -- very recent (Feb 4-5) but the performance optimization pattern is clean
15. **Sprint persistence** -- added Feb 4, auto-save and resume pattern is well-understood

**Key recommendation for the acceptance criteria**: The plan should add explicit smoke tests for the **research_phases.go phase-to-hunter mapping** given the phase reorder that happened after that file was written. It should also require integration tests that exercise the Intermute client under concurrent access, since the entire integration layer has essentially zero bug-fix history -- meaning it has never been stressed.