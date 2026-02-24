# Quality-Gate Review Synthesis — Thematic Work Lanes (v13 Schema)

**Date:** 2026-02-21
**Context:** 15 files changed across Go and Shell. Risk domains: database schema migration, CRUD store, CLI, shell scripting, TUI rendering. Feature: thematic work lanes for label-based sprint sequencing.

---

## Validation Report

**Agents Launched:** 3 (fd-architecture, fd-correctness, fd-quality)
**Agents Completed:** 3/3 (100%)
**Agents Failed:** 0
**Overall Verdict:** needs-changes

All three agent reports validated successfully:
- `fd-architecture.md` — Valid structure, Verdict: needs-changes
- `fd-correctness.md` — Valid structure, Verdict: needs-changes
- `fd-quality.md` — Valid structure, Verdict: needs-changes

---

## Verdict Summary

| Agent | Status | Verdict | Key Finding |
|-------|--------|---------|-------------|
| fd-architecture | NEEDS_ATTENTION | needs-changes | Dual membership sources (bd labels + lane_members) with no reconciliation (A2) |
| fd-correctness | NEEDS_ATTENTION | needs-changes | Early-return in Migrate skips explicit tx.Commit (C-01) |
| fd-quality | NEEDS_ATTENTION | needs-changes | `sql.ErrNoRows` compared with `==` instead of `errors.Is` (Q1) |

---

## Findings Inventory

### Critical Issues (P0/P1) — Must Fix Before Merge

#### Q1. HIGH: `sql.ErrNoRows` compared with `==` instead of `errors.Is`
- **Severity:** P1 (correctness, error handling regression)
- **Agent:** fd-quality
- **Files:** `infra/intercore/internal/lane/store.go:106,123`
- **Impact:** Direct `==` comparison fails when error is wrapped by middleware or driver layer, breaking error chain matching pattern. Project uses `errors.Is(err, sql.ErrNoRows)` elsewhere (`state/state.go:100`).
- **Fix:** Replace both comparisons with `errors.Is(err, sql.ErrNoRows)`.
- **Convergence:** 1/3 agents (fd-quality only)

#### Q2. HIGH: `--days` flag silently accepts invalid/zero/negative values
- **Severity:** P1 (data corruption, silent fail)
- **Agent:** fd-quality
- **Files:** `cmd/ic/lane.go:415-419`
- **Impact:** `fmt.Sscanf(..., "%d", &days)` discards return values. `--days=0`, `--days=-1`, `--days=abc` all silently fail or produce garbage. Zero/negative durations feed incorrect window-start to velocity calculator, making all lanes appear equally unstarved. Masquerades as working.
- **Fix:** Check `Sscanf` return count; validate `days >= 1`; return exit 3 on invalid input.
- **Convergence:** 1/3 agents (fd-quality only)

#### C-01. LOW (raises to P1 via architecture concern): Early-return in Migrate skips explicit tx.Commit
- **Severity:** P2 (fragility, not a current bug)
- **Agent:** fd-correctness
- **Files:** `infra/intercore/internal/db/db.go:132-134`
- **Impact:** The `defer tx.Rollback()` fires correctly after early `return nil` if DB is already migrated. **Semantically safe in WAL mode.** However, the pattern is fragile: if future refactoring adds work between lock acquisition and the early-return that should persist, it will be silently rolled back. Explicit `tx.Rollback()` before return makes intent visible.
- **Fix:** Add explicit `tx.Rollback()` before early return.
- **Convergence:** 1/3 agents (fd-correctness only)

#### A2. MEDIUM: Dual membership sources with no reconciliation
- **Severity:** P2 (architectural divergence risk)
- **Agents:** fd-architecture (A2), fd-correctness (implicit via invariant 3), fd-quality (implicit via Q9)
- **Files:** Lane membership tracked in two independent systems:
  - `lane_members` table in intercore's SQLite DB (managed via `ic lane sync`)
  - `lane:<name>` labels on beads in Beads tracker (managed via `bd label add`)
- **Impact:** Discovery filter reads bd labels; `ic lane velocity` reads `lane_members`. A bead labeled but not synced (or vice versa) produces divergent behavior with no error signal. SKILL.md explicitly calls both `bd label add` and `ic lane sync` as two separate steps, confirming intentional architecture, but no reconciliation or divergence detection exists.
- **Fix:** Add `ic lane check <id>` subcommand to compare `lane_members` against beads with `lane:<name>` label. Does not need to auto-repair but must make divergence visible.
- **Evidence:** `lib-discovery.sh:216-218` (reads labels), `velocity.go:55-56` (reads lane_members), `SKILL.md Step 4` (two-step add).
- **Convergence:** 3/3 agents (all identified this risk from different angles)

#### C-03. LOW (MEDIUM by contract): `rows.Err()` after inner scan loop not checked before writes
- **Severity:** P2 (context cancellation vulnerability)
- **Agent:** fd-correctness
- **Files:** `infra/intercore/internal/lane/store.go:237-249`
- **Impact:** Go `database/sql` contract requires checking `rows.Err()` after loop. If context deadline fires mid-read, `rows.Next()` returns false but `existing` is only partially populated. Code proceeds with incomplete snapshot and may corrupt membership (partial reads treated as complete, beads past cursor deleted or re-inserted). In practice SQLite is unlikely to interrupt reads, but contract violation is a latent bug.
- **Fix:** Call `rows.Err()` after `rows.Close()`, before proceeding with add/remove logic.
- **Convergence:** 1/3 agents (fd-correctness only)

---

### Secondary Issues (P2) — Should Fix This Cycle

#### Q3. MEDIUM: All `json.Encode()` return errors discarded
- **Severity:** P2 (inconsistency, silent failure)
- **Agents:** fd-quality (Q3)
- **Files:** `cmd/ic/lane.go` (lines 89, 143, 214, 294, 354, 356, 401, 456)
- **Impact:** Eight `json.NewEncoder(os.Stdout).Encode()` call sites ignore errors. Diverges from sibling `discovery.go` which checks and prints to stderr. Silently dropping an encoding error means caller receives empty/partial JSON with no failure indication.
- **Fix:** Assign and check error at each site; print to stderr and return exit 2 on failure.
- **Convergence:** 1/3 agents (fd-quality only)

#### Q4. MEDIUM: `viewport.New()` can receive zero or negative dimensions
- **Severity:** P2 (panic risk, bounds)
- **Agents:** fd-quality (Q4)
- **Files:** `pkg/tui/lane_pane.go:38` (SetSize)
- **Impact:** If `width < 2` or `height < 1`, viewport receives non-positive size. `strings.Repeat("─", min(p.width-4, 60))` at line 75 panics with negative repeat count when `width < 4`.
- **Fix:** Add lower-bound guard at top of SetSize: `if width < 4 || height < 2 { return }`.
- **Convergence:** 1/3 agents (fd-quality only)

#### Q5. MEDIUM: `truncate` slices bytes not runes, corrupting multi-byte UTF-8 names
- **Severity:** P2 (UTF-8 corruption)
- **Agents:** fd-quality (Q5)
- **Files:** `pkg/tui/lane_pane.go:136-140`
- **Impact:** `s[:max-1]` indexes into byte slice. Lane name like `interop-队列` can be split mid-codepoint, producing invalid UTF-8 that corrupts rendering.
- **Fix:** Use `[]rune(s)[:max-1]` or `utf8.RuneCountInString` for slicing.
- **Convergence:** 1/3 agents (fd-quality only)

#### C-02. LOW (minor): v6 migration block has no upper bound
- **Severity:** P3 (minor, mitigated by idempotency guard)
- **Agents:** fd-correctness (C-02)
- **Files:** `infra/intercore/internal/db/db.go:137`
- **Impact:** `if currentVersion >= 5` with no upper bound. Fires on v13 database calling Migrate again, attempting to `ALTER TABLE` columns that already exist. Mitigated by `isDuplicateColumnError` guard, but represents a latent pattern inconsistency (all later blocks use bounded guards).
- **Fix:** Add `&& currentVersion < 6` to match pattern used in all later version blocks.
- **Convergence:** 1/3 agents (fd-correctness only)

---

### Nice-to-Have Improvements (P3/IMP)

#### A1. MEDIUM (architectural, not production-blocking): ComputeStarvation has no production caller
- **Severity:** P3 (dead API surface)
- **Agents:** fd-architecture (A1)
- **Files:** `infra/intercore/internal/lane/velocity.go:45`
- **Impact:** Exports `ComputeStarvation` (requires external `map[string]*BeadStatus`) and `ComputeStarvationFromDB` (self-contained). Only the DB variant is used in production. The external variant is exercised only in unit tests, creating dead API surface.
- **Fix:** Either document intent with `TODO(iv-xxx)` for future bead-integration, or delete `ComputeStarvation` and `BeadStatus` now to avoid misleading future contributors.
- **Convergence:** 1/3 agents (fd-architecture only)

#### A3. LOW: `sprint_create $3` lane parameter is dead code
- **Severity:** P3 (reachability)
- **Agents:** fd-architecture (A3), fd-quality (Q9)
- **Files:** `lib-sprint.sh:102-123`, `sprint.md:113,234`
- **Impact:** `sprint_create` has documented `$3` lane parameter that tags sprint with `lane:<name>`. Only caller in `sprint.md:234` uses single-argument call. The `sprint.md:113` instruction tells Claude to use `bd label add` directly instead, bypassing `sprint_create`'s tagging. The `$3` code path is never exercised from the primary call site.
- **Fix:** Either update `sprint.md` to pass `$SPRINT_LANE` as third argument, or remove `$3` parameter from `sprint_create`. Do not maintain both paths.
- **Convergence:** 2/3 agents (fd-architecture A3, fd-quality Q9 both identified)

#### A4. LOW: LanePane and FetchLanes are unconnected — TUI pane is orphaned
- **Severity:** P3 (integration seam)
- **Agents:** fd-architecture (A4)
- **Files:** `pkg/tui/lane_pane.go`, `internal/icdata/fetch.go`
- **Impact:** `LanePane` and `FetchLanes/FetchLaneVelocity` are defined but not imported or wired into any Bigend view, autarch TUI app, or aggregator. The pane is functional in isolation but has no path to be displayed. Acceptable for staged feature landing but requires tracking.
- **Fix:** If view wiring is intended for follow-on bead, record explicitly; otherwise mark as abandoned.
- **Convergence:** 1/3 agents (fd-architecture only)

#### A5. LOW: LaneScope on HunterConfig is consumed by zero hunter implementations
- **Severity:** P3 (no-op stub)
- **Agents:** fd-architecture (A5)
- **Files:** `internal/pollard/hunter.go:51-53`, `internal/pollard/scan.go:201`
- **Impact:** `HunterConfig.LaneScope` is populated by `--lane` flag but never read. `pollard scan --lane=interop` silently succeeds with no effect. Creates misleading API surface.
- **Fix:** Either wire `LaneScope` to hunter implementations, or remove the field and flag.
- **Convergence:** 1/3 agents (fd-architecture only)

#### A6. INFO: Starvation bar normalization comment is misleading
- **Severity:** P3 (documentation)
- **Agents:** fd-architecture (A6)
- **Files:** `pkg/tui/lane_pane.go:107`
- **Impact:** Comment says `// Normalize: 0-50 maps to 0-4 blocks` but starvation scores are unbounded (single P0 bead = 50, ten P0s = 500).
- **Fix:** Replace with: `// Score is unbounded; capped at width blocks for display.`
- **Convergence:** 1/3 agents (fd-architecture only)

#### A7. INFO: Migration test does not exercise upgrade path
- **Severity:** P3 (test coverage gap)
- **Agents:** fd-architecture (A7)
- **Files:** `infra/intercore/internal/db/db_test.go:202`
- **Impact:** `TestMigrate_V12ToV13_LaneTables` tests fresh install. Does not verify that a v12 database correctly gains the three lane tables without error. Safe in practice (uses `CREATE TABLE IF NOT EXISTS`), but test coverage gap.
- **Fix:** Create DB at v12 schema via old DDL, then call Migrate, verifying v13 tables are created. Pattern exists in other tests (e.g., `TestMigrate_V5ToV6`).
- **Convergence:** 1/3 agents (fd-architecture only)

#### Q6. LOW: No sentinel error types for lane-not-found
- **Severity:** P3 (error handling polish)
- **Agents:** fd-quality (Q6)
- **Files:** `infra/intercore/internal/lane/store.go`
- **Impact:** No `lane.ErrNotFound` sentinel. Discovery package has `ErrNotFound` in `errors.go`; lane should mirror for consistency. CLI callers cannot distinguish "not found" (exit 3) from "DB error" (exit 2) with `errors.Is`.
- **Fix:** Define `lane.ErrNotFound` and wrap it; update CLI to check with `errors.Is`.
- **Convergence:** 1/3 agents (fd-quality only)

#### Q7. LOW: `TestLaneVelocity_ThroughputReducesStarvation` does not test its premise
- **Severity:** P3 (test naming)
- **Agents:** fd-quality (Q7)
- **Files:** `internal/lane/velocity_test.go`
- **Impact:** Test fixture uses `ClosedAt: 0` (epoch) which is outside any realistic window, so test always sees zero closed beads. Name is misleading.
- **Fix:** Update fixture to use recent `ClosedAt` within window, or rename test to `TestLaneVelocity_PriorityWeighting`.
- **Convergence:** 1/3 agents (fd-quality only)

#### Q8. LOW: `status` subcommand displays member list labeled "Beads" (inconsistent vocabulary)
- **Severity:** P3 (API consistency)
- **Agents:** fd-quality (Q8)
- **Files:** `cmd/ic/lane.go:223-225`
- **Impact:** Header says "Beads (%d):" but rest of API uses "members". Minor but visible in CLI output.
- **Fix:** Change to "Members (%d):" for consistency.
- **Convergence:** 1/3 agents (fd-quality only)

#### C-04. LOW: `Close()` returns `tx.Commit()` error bare
- **Severity:** P3 (convention)
- **Agents:** fd-correctness (C-04)
- **Files:** `infra/intercore/internal/lane/store.go:189`
- **Impact:** Unwrapped error return breaks project convention of always wrapping with context. All other errors in file use `fmt.Errorf("lane ...: %w", err)`.
- **Fix:** Wrap with `fmt.Errorf("lane close: commit: %w", err)`.
- **Convergence:** 1/3 agents (fd-correctness only)

#### C-05, C-06, C-07, C-08. INFO-level improvements
- **Agents:** fd-correctness
- **Impact:** Foreign-key enforcement scope (C-05), priority range validation (C-06), closed-lane snapshot semantics (C-07), migration guard documentation (C-08).
- **Convergence:** 1/3 agents each

---

## Deduplication & Convergence Analysis

### Findings Appearing in Multiple Agents (Cross-Check Hits)

**A2 ↔ implicit in correctness invariant ↔ Q9 impact:** Dual membership divergence risk
- **Architecture perspective (A2):** Two independent sources with no reconciliation tool
- **Correctness perspective (C-01 invariant 3):** Schema design is sound, but dual-source introduces invariant that cannot be verified at DB layer
- **Quality perspective (Q9):** `sprint_create` $3 parameter is unreachable, forcing users to manually call both `bd label add` and `ic lane sync`, increasing risk of omitting one step
- **Verdict:** This is a structural design choice, not a bug. Mitigation (A2 fix: add `ic lane check`) is required.

**A3 ↔ Q9:** `sprint_create $3` parameter dead code
- **Architecture identifies it as unreachable** (call site doesn't use it)
- **Quality confirms it via sprint.md instruction analysis** (claude is told to use `bd label add` directly)
- **Convergence:** 2/3 agents. Unified recommendation: centralize lane tagging in `sprint_create`, or remove dead parameter.

**A1 vs. Q1 & Q2:** Quality issues are separate from architecture dead-API concern
- A1 = ComputeStarvation exported but unused
- Q1, Q2 = Error handling and input validation bugs
- No convergence; independent findings.

---

## Files Affected by Findings

### Critical Issues (Fix Before Merge)

1. **`infra/intercore/internal/lane/store.go`**
   - Q1: Lines 106, 123 — use `errors.Is` instead of `==`
   - C-03: Lines 237–249 — check `rows.Err()` after scan loop
   - C-04: Line 189 — wrap `tx.Commit()` error
   - Q6: No sentinel error type defined

2. **`cmd/ic/lane.go`**
   - Q2: Lines 415–419 — validate `--days` flag
   - Q3: Lines 89, 143, 214, 294, 354, 356, 401, 456 — check `json.Encode()` errors

3. **`infra/intercore/internal/db/db.go`**
   - C-01: Lines 132–134 — explicit `tx.Rollback()` before early return
   - C-02: Line 137 — add upper bound `< 6` to v6 guard

4. **`pkg/tui/lane_pane.go`**
   - Q4: Line 38 — add lower-bound guard to `SetSize`
   - Q5: Lines 136–140 — use rune slicing instead of byte slicing
   - A6: Line 107 — fix misleading comment

5. **`lib-sprint.sh` & `commands/sprint.md`**
   - A3/Q9: Lines 102–123 (`lib-sprint.sh`), lines 113, 234 (`sprint.md`) — unify lane parameter handling

### Integration Points

6. **`infra/intercore/internal/lane/velocity.go`**
   - A1: Line 45 — document intent or delete `ComputeStarvation`
   - C-06: Add bounds validation for priority range `[0,4]`

7. **`infra/intercore/internal/db/db_test.go`**
   - A7: Line 202 — add v12→v13 upgrade test

8. **`pkg/tui/lane_pane.go` & `internal/icdata/fetch.go`**
   - A4: Wire LanePane into TUI or remove

9. **`internal/pollard/hunter.go` & `scan.go`**
   - A5: Wire `LaneScope` to implementations or remove

10. **`infra/intercore/internal/lane/velocity_test.go`**
    - Q7: Update test fixture or rename test

---

## Protected Paths (No Findings Generated)

No findings matched PROTECTED_PATHS patterns. All affected files are in core feature area.

---

## Summary Statistics

- **Total Findings:** 26 (2 HIGH, 6 MEDIUM, 13 LOW, 5 INFO)
- **Severity Breakdown:**
  - P0/P1 Critical: 3 (Q1, Q2, C-01 via fragility)
  - P2 Important: 5 (Q3, Q4, Q5, C-03, A2)
  - P3 Nice-to-Have: 13 (A1, A3, A4, A5, A6, A7, Q6, Q7, Q8, C-02, C-04, + 2 analysis + improvements)
- **Agent Convergence:**
  - 3/3 agents: A2 (dual membership risk)
  - 2/3 agents: A3/Q9 (sprint lane parameter)
  - 1/3 agents: All others
- **Conflicts:** None. Agents agree on facts; disagree only on severity (all within 1 level).

---

## Gate Assessment

### Quality-Gates Review Verdict: NEEDS_ATTENTION

**Reasoning:**
1. Two HIGH issues (Q1, Q2) block merge: error handling regression and input validation silent fail
2. One MEDIUM architectural issue (A2) requires mitigation visible in code before merge
3. Three agents unanimously recommend **needs-changes** verdict
4. Schema migration strategy (C-01) is safe but fragility pattern should be cleaned
5. TUI bounds issues (Q4, Q5) are moderate risk in interactive code

**Recommendation:** Fix all P1/P2 issues before merge. P3 improvements can be tracked as follow-on beads if under time pressure, but A2 mitigation (add `ic lane check`) is essential for production safety.

**Estimated Fix Effort:**
- Q1, Q2: 10 min (error handling + validation)
- A2: 20 min (`ic lane check` command)
- C-03: 5 min (rows.Err check)
- Q3, Q4, Q5: 20 min (error checks, bounds guards, UTF-8 fix)
- C-01, C-02: 10 min (tx.Rollback, version guard)
- **Total:** ~65 minutes (1 hour 5 min)

---

## File Paths (Absolute)

### Agent Output Files
- `/root/projects/Interverse/.clavain/quality-gates/fd-architecture.md`
- `/root/projects/Interverse/.clavain/quality-gates/fd-correctness.md`
- `/root/projects/Interverse/.clavain/quality-gates/fd-quality.md`

### Verdict Files (to be written via lib-verdict.sh)
- `.clavain/verdicts/fd-architecture.json`
- `.clavain/verdicts/fd-correctness.json`
- `.clavain/verdicts/fd-quality.json`

### Generated Output Files
- `/root/projects/Interverse/.clavain/quality-gates/synthesis.md` (this file)
- `/root/projects/Interverse/.clavain/quality-gates/findings.json` (structured data)
