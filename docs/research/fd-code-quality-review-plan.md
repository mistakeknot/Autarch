# Code Quality Review: Acceptance Criteria Plan

**Reviewed:** 2026-02-06
**Plan file:** `/root/projects/Autarch/docs/plans/2026-02-05-acceptance-criteria-plan.md`
**Reviewer:** Code Quality Reviewer (codebase-aware)

---

## Conventions Check

### Project conventions established from CLAUDE.md, AGENTS.md, and source code

**Error handling:**
- `fmt.Errorf("context: %w", err)` pattern used consistently (confirmed in `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go`, `/root/projects/Autarch/internal/gurgeh/specs/evolution.go`)
- Sentinel errors via `errors.New` + `errors.Is` (e.g., `ErrBlocker` in `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go:23`)
- Warning-level errors go to `fmt.Fprintf(os.Stderr, "warning: ...")` rather than returning errors (non-fatal degradation pattern)

**Naming patterns:**
- Types: PascalCase with domain prefix (e.g., `SprintState`, `ConfidenceScore`, `DraftStatus`, `ConflictType`, `SeverityBlocker`)
- Constants: `Phase` prefix for phases (`PhaseVision`, `PhaseProblem`), `DraftAccepted`, `SeverityWarning`
- Status types use string constants with snake_case values: `TaskStatusTodo = "todo"`, `TaskStatusInProgress = "in_progress"` (see `/root/projects/Autarch/pkg/contract/types.go`)
- Interface names are role-descriptive nouns: `QuickScanner`, `ResearchProvider`
- Constructor pattern: `New{Type}()` returns pointer (e.g., `NewOrchestrator`, `NewCalculator`, `NewBroker`)

**Test patterns (from 11 test files in arbiter/):**
- Package-level `_test` suffix for external tests (`package arbiter_test`), internal tests for private functions (`package confidence`, `package specs`)
- Individual `Test{Function}_{Scenario}` naming (e.g., `TestOrchestratorStartsSprint`, `TestCalculate_ShapeAwareSpecificity`)
- Table-driven tests for enumeration/mapping (see `TestPhaseString` in `/root/projects/Autarch/internal/gurgeh/arbiter/types_test.go:25-42`)
- Concurrency tests use `sync.WaitGroup` + error channels (see `TestSaveRevisionConcurrentCallsProduceUniqueVersions` in `/root/projects/Autarch/internal/gurgeh/specs/evolution_test.go:69-133`)
- Integration tests named `TestIntegration_{Scenario}` in separate `integration_test.go` files
- Test helpers use unexported struct test doubles, NOT mock libraries (e.g., `testResearchProvider` in `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator_test.go:125`)
- `t.TempDir()` for filesystem tests, `context.Background()` for context
- `AdvanceSync` variant exists specifically for test determinism (async behavior disabled in tests)
- `-race` flag mandatory per institutional learnings

**Architecture patterns:**
- Deep-copy snapshots for concurrency safety (`Clone()` methods throughout types.go)
- Mutex-protected state with `State()` returning cloned snapshots
- YAML for persistence, SQLite for high-frequency operations
- Given/When/Then format used in CUJ descriptions and requirements

### Conventions the plan adheres to

1. **YAML for persistence** -- Plan correctly specifies `.gurgeh/specs/`, `.coldwine/`, `.pollard/` as YAML-based directories (AC-1.10, AC-2.3, AC-X.7)
2. **Race condition awareness** -- Plan explicitly mandates `go test -race` for arbiter packages (Test Categories section) and includes race condition test scenarios (AC-4.2, AC-1.15, AC-3.9, AC-3.4a)
3. **Degradation pattern** -- Plan follows the project's "warning not error" convention for optional integrations (AC-X.5, AC-X.9, degradation matrix)
4. **Deep-copy awareness** -- Plan identifies `SaveRevision` non-atomic writes and version mutation as data integrity risks, which aligns with the project's Clone() discipline
5. **Test categories separation** -- Plan correctly separates manual, integration, unit, and race tests, matching existing test file organization (`integration_test.go`, `types_test.go`, etc.)

### Conventions the plan violates or misses

See Specific Issues below.

---

## Specific Issues

### Issue 1: Confidence score components mismatch between plan and code

- **Location:** AC-1.8 -- "Confidence score displays four components (Completeness, Consistency, Specificity, Research)"
- **Convention:** The actual `ConfidenceScore` struct in `/root/projects/Autarch/internal/gurgeh/arbiter/types.go:93-99` has FIVE components: `Completeness`, `Consistency`, `Specificity`, `Research`, AND `Assumptions`. The weighted formula in `Total()` at line 102 confirms all five are used (20% + 25% + 20% + 20% + 15%).
- **Violation:** The plan says "four components" and omits `Assumptions` entirely. This means AC-1.8 tests would miss verifying 15% of the confidence calculation.
- **Fix:** Update AC-1.8 to read "five components (Completeness, Consistency, Specificity, Research, Assumptions)" and add a test that verifies `Assumptions` updates when contrapositive thinking shapes are used (matching the existing `TestCalculate_ShapeAwareAssumptions` pattern in `/root/projects/Autarch/internal/gurgeh/arbiter/confidence/calculator_test.go:94`).

### Issue 2: Given/When/Then format used inconsistently for complex ACs

- **Location:** All acceptance criteria tables (AC-1.1 through AC-X.10)
- **Convention:** The plan's own "Best Practices Findings" section (line 268) recommends "Add Given-When-Then for complex criteria" and notes "Simple criteria (AC-X.1) are fine as-is. Multi-step criteria like AC-4.8 benefit from explicit Given/When/Then structure." The project's own PRD phases use Given/When/Then for requirements (Phase 6: Requirements).
- **Violation:** Zero ACs in the plan actually use Given/When/Then format. Even the most complex multi-step criteria (AC-4.8, AC-4.4, AC-5.5) use prose descriptions. The plan recommends the format but does not apply it.
- **Fix:** At minimum, convert these complex multi-step ACs to Given/When/Then:
  - **AC-4.4** (three independent triggers for reservation release): Split into three GWT blocks -- Given task complete / Given agent disconnect / Given TTL expiry
  - **AC-4.8** (automatic reservation on task claim): Given a teammate claims a task with file patterns / When Coldwine detects the claim / Then Intermute reservation is created before work begins AND reservation covers task's declared file patterns
  - **AC-5.5** (state detection accuracy): Given 20+ state transitions occur / When Agent Teams is active / Then >95% detected correctly; AND Given Agent Teams is inactive / When only heuristics available / Then >90% detected correctly
  - **AC-1.12** (8-phase hunter mapping): Given sprint advances to phase X / When phase has configured hunters / Then those specific hunters trigger and results appear in log pane

### Issue 3: Test double strategy not specified -- plan implies mocking infrastructure the project does not use

- **Location:** AC-3.6 through AC-3.8 (feedback-based learning), Test Categories section
- **Convention:** The project uses hand-written test doubles (unexported structs implementing interfaces). See `testResearchProvider` in `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator_test.go:125-163`. No mock library (gomock, mockery, testify/mock) is used anywhere in the codebase.
- **Violation:** The plan says "use fixture feedback histories, no LLM invocation" (good) but does not specify HOW to inject test fixtures. The feedback system reads from `.pollard/feedback.yaml` -- testing this requires either (a) writing fixture files to `t.TempDir()` or (b) abstracting the file reader behind an interface. The plan does not address this, and the current codebase has no `FeedbackReader` interface.
- **Fix:** Specify that AC-3.6-3.8 tests should:
  1. Use `t.TempDir()` with pre-written YAML fixture files (matching the `evolution_test.go` pattern)
  2. Define a `FeedbackStore` interface (matching the project's `QuickScanner` and `ResearchProvider` interface patterns) if file I/O needs to be decoupled
  3. Avoid introducing mock libraries -- use hand-written test doubles per project convention

### Issue 4: AC-1.1 timing target contradicts actual implementation

- **Location:** AC-1.1 -- "Kickoff completes codebase scan and displays results in doc pane within 10 seconds"
- **Convention:** The plan's own Research Insights section (line 217) correctly identifies this: "Codebase scan target of <10s is unrealistic... current timeout is set to 10 minutes." The Timing Thresholds Summary (line 298) then splits this into structural scan (<5s) and LLM exploration (<90s). But AC-1.1 still says "within 10 seconds."
- **Violation:** The AC criterion contradicts the plan's own research findings. This will produce a flaky or always-failing test.
- **Fix:** Replace AC-1.1 with two criteria:
  - AC-1.1a: "Structural file scan completes and renders project structure in doc pane" (correctness) with "<5s (p95)" performance target
  - AC-1.1b: "LLM exploration completes with streaming partial results" (correctness) with "<90s (p95)" performance target
  This matches the plan's own Timing Thresholds Summary table.

### Issue 5: Missing `AdvanceSync` convention for test determinism

- **Location:** Test Categories > Integration Testing (line 337-344)
- **Convention:** The project has `AdvanceSync` specifically for test determinism (`/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go:303-305`). All integration tests in the codebase use `AdvanceSync` (see `/root/projects/Autarch/internal/gurgeh/arbiter/integration_test.go:43`). The comment at line 303 says "Use this in tests where you need to verify scan results immediately."
- **Violation:** The plan's integration test list includes criteria that advance phases (AC-2.1-2.10, AC-1.14, AC-1.15) but does not mention using `AdvanceSync`. Tests written against these criteria will have non-deterministic behavior if they use `Advance` instead of `AdvanceSync`.
- **Fix:** Add a note to the Integration Testing section: "All tests that advance arbiter phases MUST use `AdvanceSync` (not `Advance`) to ensure scan results are available synchronously. See orchestrator.go:303."

### Issue 6: Race condition test for AC-4.2 is undertestable per plan's own Gap 1

- **Location:** Race Condition Testing > AC-4.2 -- "Two agents simultaneously request overlapping reservations -- 100 iterations, zero double-grants"
- **Convention:** The plan's own Critical Implementation Gaps section (line 192) says "Intermute `Reserve()` has no glob overlap detection... AC-4.2 is untestable because the underlying mechanism does not exist."
- **Violation:** The plan lists AC-4.2 as a race condition test target while simultaneously documenting that the underlying mechanism does not exist. This is a contradictory test specification.
- **Fix:** Move AC-4.2 to a "Blocked" section with a dependency on Intermute glob overlap implementation. Add a placeholder test that is `t.Skip("blocked: Intermute glob overlap detection not implemented -- see Gap 1")` following Go convention for skipped tests.

### Issue 7: Research coverage target inconsistency

- **Location:** AC-1.13 -- "research coverage >80%"; Timing Thresholds Summary (line 315) -- "Research coverage >60% (4 of 8 phases have configs)"
- **Convention:** The plan's own performance analysis (line 222) says "Full PRD sprint <25 min with >80% research coverage is contradictory" and the timing table says >60%.
- **Violation:** AC-1.13 still says >80% while the plan's research section and timing table say >60%. The plan contradicts itself.
- **Fix:** Align AC-1.13 with the timing table: change to ">60% research coverage" and define "research coverage" precisely against the `researchQuality()` weighted formula in `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go:856-908` (30% count + 30% diversity + 40% relevance).

### Issue 8: Missing negative test for spec export with empty phases

- **Location:** Test Categories > Negative/Failure Path Testing
- **Convention:** The plan's institutional learnings section (line 290) identifies "Phase generation split: Early phases extract from artifacts; later phases generate dynamically via Claude Code. AC should verify all 8 produce non-empty content." The existing `ExportToSpec` function is referenced at `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go:636`.
- **Violation:** The negative path tests cover hunter failure, zero-CUJ specs, feedback corruption, TTL expiry, and directory deletion -- but there is no test for what happens when export is attempted with some phases having empty content (a common real-world scenario where a user skips or partially fills phases).
- **Fix:** Add to CUJ-1 negative tests: "Spec export with 3 of 8 phases having empty content -- export succeeds with empty-section warnings, no crash, exported YAML has empty strings for missing phases."

### Issue 9: AC-2.3 references `.tandemonium/` alias without context

- **Location:** AC-2.3 -- "Accepted tasks persist to `.coldwine/` (or `.tandemonium/`) as YAML"
- **Convention:** CLAUDE.md mentions "Legacy tool names (Vauxhall/Praude/Tandemonium) still work via aliases" but does not specify how persistence directories are aliased. The plan introduces a parenthetical `(or .tandemonium/)` without specifying which is canonical.
- **Violation:** Test assertions need a single expected path. "or" in an acceptance criterion makes it untestable without additional logic.
- **Fix:** Change to: "Accepted tasks persist to `.coldwine/` as YAML conforming to contract schema" (canonical path only). If alias support is needed, add a separate AC: "AC-2.3a: `.tandemonium/` symlink or redirect resolves to `.coldwine/`."

### Issue 10: Signal dedup AC-3.4a specifies server-side enforcement, but broker is in-process

- **Location:** AC-3.4a -- "Signal deduplication prevents repeat alerts for same `(spec_id, type, affected_field)` tuple"
- **Convention:** The `Broker` in `/root/projects/Autarch/pkg/signals/broker.go` is a pure in-process fan-out with no persistence layer. The plan's Gap 3 (line 203) correctly identifies this: "The Signals broker is a pure fan-out with no deduplication."
- **Violation:** The Test Categories section (line 341) says "verify server-side persistence" for AC-3.4a, but the broker is not a server with persistence -- it is an in-memory pub-sub. "Server-side persistence" implies a different architecture than what exists.
- **Fix:** Clarify whether dedup should be:
  - (a) In-memory with restart loss (add a `seen` map to `Broker` keyed by `(spec_id, type, affected_field)`)
  - (b) SQLite-backed persistence (requires `events.db` integration)
  The test specification should match whichever is chosen. For (a): unit test with `NewBroker()` + `Publish()` + `Publish()` same signal, verify subscriber receives only one. For (b): integration test with SQLite, verify dedup survives process restart.

### Issue 11: Proposed `AgentTeamsClient` interface location

- **Location:** Dependencies section (line 421) -- "`AgentTeamsClient` interface defined in `pkg/`"
- **Convention:** The project puts cross-tool shared types in `pkg/contract/` (see `/root/projects/Autarch/pkg/contract/types.go`). Cross-tool coordination interfaces go in `pkg/intermute/` or similar domain packages. Interface names follow `{Role}` pattern: `QuickScanner`, `ResearchProvider`.
- **Violation:** The plan says "defined in `pkg/`" which is too vague. There are 7 packages under `pkg/`.
- **Fix:** Specify `pkg/contract/agent_teams.go` for the interface definition (consistent with where cross-tool types live). The interface should follow the project's naming: `TeamCoordinator` or `AgentTeamsBridge` (role-descriptive noun), not just `AgentTeamsClient` (which describes the implementation, not the role).

### Issue 12: No test for `Clone()` correctness of new types

- **Location:** Multiple sections that propose new state additions (feedback rolling window in AC-3.9, reservation state in AC-4.1-4.4)
- **Convention:** The project has thorough `Clone()` tests for all mutable state (`TestSprintStateClone` in `/root/projects/Autarch/internal/gurgeh/arbiter/types_test.go:68-160`). Every slice/map in a struct gets explicit mutation-isolation verification.
- **Violation:** The plan proposes new stateful types (feedback decisions, reservation records, signal dedup state) but includes no AC for clone correctness of these new types. If any of these hold slices or maps, they will be vulnerable to the same pointer-escape races that required deep-copy discipline in the first place.
- **Fix:** Add AC-X.11: "All new mutable state types that participate in concurrent access MUST have Clone() methods with mutation-isolation tests (matching the TestSprintStateClone pattern)."

---

## Summary

**Overall code quality alignment: Acceptable with targeted fixes needed.**

The plan demonstrates strong awareness of the project's architecture (YAML persistence, Intermute integration, degradation patterns, race condition discipline). The research insights section is particularly valuable -- it correctly identifies four critical implementation gaps and provides actionable recommendations. The test categorization (manual/integration/unit/race) broadly follows Go conventions and the project's existing test file organization.

**Top 3 changes for better consistency:**

1. **Fix the self-contradictions** (Issues 4, 6, 7): The plan's research section correctly identifies problems (unrealistic timing, untestable AC, coverage mismatch) but the AC table still contains the original incorrect values. The AC criteria MUST be updated to match the plan's own corrected analysis. Without this, any test suite written against the plan will contain guaranteed failures.

2. **Apply Given/When/Then to complex multi-step ACs** (Issue 2): The plan recommends this format in its own best-practices section but does not use it anywhere. At minimum, AC-4.4, AC-4.8, AC-5.5, and AC-1.12 need explicit preconditions and postconditions to be testable in Go's `testing` package. The current prose descriptions require human interpretation to translate into assertions.

3. **Specify the confidence score has 5 components, not 4** (Issue 1): This is a factual error that will cause test coverage gaps. The `Assumptions` component carries 15% weight in the existing `Total()` formula and is already covered by `TestCalculate_ShapeAwareAssumptions`. Missing it means the acceptance criteria cannot fully validate the feature they describe.
