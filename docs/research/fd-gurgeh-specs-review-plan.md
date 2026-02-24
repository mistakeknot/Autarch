# Gurgeh Spec System Review: Acceptance Criteria Plan

**Reviewer:** Gurgeh spec system specialist
**Date:** 2026-02-06
**Plan reviewed:** `docs/plans/2026-02-05-acceptance-criteria-plan.md`
**Source files grounded against:**
- `/root/projects/Autarch/internal/gurgeh/arbiter/types.go` (Phase enum, ConfidenceScore, ConflictType, DraftStatus, SprintState)
- `/root/projects/Autarch/internal/gurgeh/arbiter/confidence/calculator.go` (Score struct, Calculate method, weights)
- `/root/projects/Autarch/internal/gurgeh/arbiter/consistency/engine.go` (Engine, Check, conflict detection)
- `/root/projects/Autarch/internal/gurgeh/arbiter/consistency/vision.go` (VisionInfo, CheckVisionAlignment)
- `/root/projects/Autarch/internal/gurgeh/arbiter/orchestrator.go` (researchQuality, updateConfidence, checkConsistency)
- `/root/projects/Autarch/internal/gurgeh/arbiter/research_phases.go` (DefaultResearchPlan, PhaseResearchConfig)
- `/root/projects/Autarch/internal/gurgeh/specs/schema.go` (AcceptanceCriterion struct, Spec struct)
- `/root/projects/Autarch/internal/gurgeh/specs/evolution.go` (SaveRevision, LoadHistory, atomic writes)

---

## Finding 1: AC-1.8 Incorrectly Lists 4 Confidence Components (Actual System Has 5)

**Severity:** HIGH - Functional mismatch between plan and implementation

**AC-1.8 states:**
> "Confidence score displays four components (Completeness, Consistency, Specificity, Research) and updates within 2 seconds of triage action"

**Actual implementation in `types.go` lines 93-108:**
```go
type ConfidenceScore struct {
    Completeness float64 // 0-1, weight: 20%
    Consistency  float64 // 0-1, weight: 25%
    Specificity  float64 // 0-1, weight: 20%
    Research     float64 // 0-1, weight: 20%
    Assumptions  float64 // 0-1, weight: 15%
}

func (c ConfidenceScore) Total() float64 {
    return c.Completeness*0.20 +
        c.Consistency*0.25 +
        c.Specificity*0.20 +
        c.Research*0.20 +
        c.Assumptions*0.15
}
```

**The `Assumptions` component (15% weight) is completely missing from AC-1.8.** The `confidence.Score` struct in `calculator.go` also has all 5 fields. The `Calculate()` method computes `Assumptions` using shape-aware logic (contrapositive thinking surfaces assumptions, boosting the score from 0.5 to 0.7).

**Impact:** Any test verifying AC-1.8 would pass while displaying only 80% of the confidence dimensions. The Assumptions component is the only one influenced by thinking shape selection (contrapositive), making it a distinctive feature that should be tested.

**Recommendation:** Rewrite AC-1.8:
> "Confidence score displays five components (Completeness 20%, Consistency 25%, Specificity 20%, Research 20%, Assumptions 15%) and updates within 2 seconds of triage action"

---

## Finding 2: AC-1.14 (GoalFeature Consistency) Is Mis-specified Against the Actual Engine

**Severity:** MEDIUM - Criterion describes behavior the engine does not implement

**AC-1.14 states:**
> "GoalFeature consistency check flags conflicts between Vision/Problem statements and proposed Features (e.g., feature not traceable to a stated goal)"

**What the actual consistency engine does (`engine.go` lines 49-66, 68-83):**

The `Engine.Check()` method performs exactly **one** check type: `checkUserFeatureAlignment`. This checks whether the **Problem** section mentions "solo" or "individual" while the **Features** section mentions "enterprise" or "100+". The conflict type emitted is `ConflictUserFeature` (TypeCode=0), NOT `ConflictGoalFeature` (TypeCode=1).

```go
func (e *Engine) checkUserFeatureAlignment(problem, features *SectionInfo) []Conflict {
    problemLower := strings.ToLower(problem.Content)
    featuresLower := strings.ToLower(features.Content)

    if (strings.Contains(problemLower, "solo") || strings.Contains(problemLower, "individual")) &&
        (strings.Contains(featuresLower, "enterprise") || strings.Contains(featuresLower, "100+")) {
        return []Conflict{{
            TypeCode: 0, // ConflictUserFeature
            Severity: 0, // SeverityBlocker
            Message:  "Feature targets enterprise users but problem describes solo/individual users",
            Sections: []int{int(PhaseProblem), int(PhaseFeaturesGoals)},
        }}
    }
    return nil
}
```

Additionally, `CheckVisionAlignment` in `vision.go` performs two checks:
1. Problem (phase 1) vs Vision Goals - keyword overlap check (emits `ConflictVisionAlignment`, TypeCode=4)
2. Features (phase 3) vs Vision Assumptions - contradiction detection (emits `ConflictVisionAlignment`, TypeCode=4)

**Key discrepancies in AC-1.14:**

1. **The term "GoalFeature"** does not match any implemented check. `ConflictGoalFeature` (TypeCode=1) is defined as a constant in `types.go` but **no code ever emits it**. The only checks that exist are `ConflictUserFeature` and `ConflictVisionAlignment`.

2. **"feature not traceable to a stated goal"** - This traceability check does not exist. The actual check is a keyword-based heuristic looking for solo/enterprise mismatches, not goal traceability.

3. **"between Vision/Problem statements and proposed Features"** - This partially describes `CheckVisionAlignment`, but that emits `ConflictVisionAlignment`, not `GoalFeature`.

4. The plan's Scope section (line 82) says "only GoalFeature ships" but the code ships `ConflictUserFeature` (TypeCode=0) and `ConflictVisionAlignment` (TypeCode=4), not `ConflictGoalFeature` (TypeCode=1).

**Impact:** AC-1.14 would test a conflict type that doesn't exist. The verification method ("Add feature unrelated to Vision, verify consistency warning") would only trigger if Vision checks are active (requires a vision spec to be loaded), and even then would emit `ConflictVisionAlignment`, not `GoalFeature`.

**Recommendation:** Replace AC-1.14 with two criteria:
- AC-1.14a: "UserFeature consistency check flags when Features target enterprise users but Problem describes solo/individual users (ConflictUserFeature, severity=blocker)"
- AC-1.14b: "VisionAlignment check flags when Features contradict strategic bets from a loaded vision spec (ConflictVisionAlignment, severity=warning, non-blocking)"

---

## Finding 3: Research Coverage Formula Discrepancy

**Severity:** MEDIUM - Plan's "research coverage" metric is vague; actual formula is well-defined but different from plan's description

**Plan states (AC-1.13, line 106):**
> "End-to-end PRD creation completes in under 25 minutes with confidence >70% and research coverage >80%"

**Plan's timing summary (line 315):**
> "Research coverage: Coverage formula applied, >60% (4 of 8 phases have configs)"

These two targets contradict each other: AC-1.13 says >80%, the timing table says >60%.

**Actual `researchQuality()` in `orchestrator.go` lines 856-908:**
```go
func researchQuality(state *SprintState) float64 {
    // Count all findings across Intermute and legacy quick scan
    findingCount := len(state.Findings)
    if state.ResearchCtx != nil {
        findingCount += len(state.ResearchCtx.GitHubHits) + len(state.ResearchCtx.HNHits)
    }
    if findingCount == 0 {
        return 0.0
    }
    // ...
    // Weighted formula: 30% count + 30% diversity + 40% relevance
    countScore := clamp01(float64(findingCount) / 10.0)
    diversityScore := clamp01(float64(len(sources)) / 3.0)
    return 0.3*countScore + 0.3*diversityScore + 0.4*avgRelevance
}
```

The formula is: `0.3 * min(findingCount/10, 1.0) + 0.3 * min(sourceTypes/3, 1.0) + 0.4 * avgRelevance`

**Key issues:**

1. **"Research coverage" in the plan is ambiguous.** It conflates two different concepts: (a) what percentage of the 8 phases have research configs (answer: 4/8 = 50%), and (b) the `researchQuality()` score which measures finding count + source diversity + average relevance.

2. **Achieving >80% research quality requires:** at least 10 findings (count maxed), 3+ distinct source types (diversity maxed), and average relevance of 0.5+ (findings default). This is achievable but the plan doesn't specify which metric it means.

3. **The plan mentions "review coverage" (AC-1.9) as a separate concept:** "unreviewed high-relevance findings reduce score." This behavior is NOT implemented in `researchQuality()`. The function counts all findings regardless of review status. There is no feedback/triage state feeding back into this formula.

4. **Only 4 of 8 phases have research configs** in `research_phases.go`: Vision, Problem, FeaturesGoals, Requirements. The plan's AC-1.12 lists 8 phase-hunter mappings, but only 4 exist in code.

**Impact:** Tests for AC-1.9 and AC-1.13 would test behavior that doesn't exist (review coverage reducing the Research component) and use an ambiguous target (>80% of what?).

**Recommendation:**
- Define "research coverage" explicitly as `researchQuality()` output
- Fix the contradictory targets (>80% in AC-1.13 vs >60% in timing table)
- Add AC-1.9a: "Review coverage reduces Research component" (currently unimplemented - add as implementation requirement)
- Note that AC-1.12's 8-phase hunter mapping only has 4 phases implemented in `DefaultResearchPlan()`

---

## Finding 4: AcceptanceCriterion Struct Format Compatibility

**Severity:** LOW - Plan's AC IDs are compatible with the struct

**The `AcceptanceCriterion` struct in `schema.go` lines 15-18:**
```go
type AcceptanceCriterion struct {
    ID          string `yaml:"id"`
    Description string `yaml:"description"`
}
```

The plan uses IDs like "AC-1.1", "AC-2.3", "AC-X.1". These are free-form strings that would fit into the `ID` field without issue. The validation in `validate.go` (lines 206-218) checks for:
- Non-empty ID
- No duplicate IDs

The plan's hierarchical naming (CUJ number + criterion number) is compatible. However, the plan's acceptance criteria are **meta-level criteria about the product**, not the same thing as the per-spec `AcceptanceCriterion` entries that live inside a `Spec.Acceptance` field. The plan's criteria would not be stored as `AcceptanceCriterion` entries in a spec YAML; they would be test cases or verification checklists.

**This is a non-issue** -- the plan's AC IDs are for a different purpose than the struct. The struct stores per-spec acceptance criteria within exported YAML files; the plan defines product-level acceptance criteria for the Autarch system itself.

---

## Finding 5: Phase Ordering in AC-1.12 vs Actual 8-Phase Sprint

**Severity:** MEDIUM - AC-1.12 lists correct phases but wrong hunter mappings for 4 of them

**AC-1.12 states:**
> "Phase-appropriate hunters trigger automatically: Vision->GitHub Scout+HackerNews, Problem->arXiv+OpenAlex, Users->community analysis, Features->competitor tracking, CUJs->workflow patterns, Requirements->implementation patterns, Scope->inverse research, Acceptance->test patterns"

**Actual phase enum ordering in `types.go` lines 17-25:**
```go
const (
    PhaseVision Phase = iota        // 0
    PhaseProblem                    // 1
    PhaseUsers                      // 2
    PhaseFeaturesGoals              // 3
    PhaseCUJs                       // 4
    PhaseRequirements               // 5
    PhaseScopeAssumptions           // 6
    PhaseAcceptanceCriteria         // 7
)
```

The 8-phase ordering in AC-1.12 matches the code enum. However, `DefaultResearchPlan()` in `research_phases.go` only defines configs for 4 phases:

| Phase | AC-1.12 Claims | Actual `DefaultResearchPlan()` |
|-------|---------------|-------------------------------|
| Vision (0) | GitHub Scout + HackerNews | `github-scout` + `hackernews-trendwatcher` -- **MATCH** |
| Problem (1) | arXiv + OpenAlex | `arxiv-scout` + `openalex` -- **MATCH** |
| Users (2) | community analysis | **NO CONFIG** -- `ResearchConfigForPhase(PhaseUsers)` returns nil |
| FeaturesGoals (3) | competitor tracking | `competitor-tracker` + `github-scout` -- **PARTIAL MATCH** (also includes github-scout) |
| CUJs (4) | workflow patterns | **NO CONFIG** |
| Requirements (5) | implementation patterns | `github-scout` only, mode "balanced" -- **PARTIAL MATCH** (plan says "implementation patterns", code uses github-scout) |
| ScopeAssumptions (6) | inverse research | **NO CONFIG** |
| AcceptanceCriteria (7) | test patterns | **NO CONFIG** |

**Impact:** AC-1.12 asserts that all 8 phases trigger automatic research. Only 4 do. Testing AC-1.12 would fail for Users, CUJs, Scope, and Acceptance phases because `ResearchConfigForPhase()` returns nil for them and no hunters fire.

**Recommendation:** Split AC-1.12 into:
- AC-1.12a: "Phases with research configs (Vision, Problem, Features, Requirements) trigger appropriate hunters automatically" (testable now)
- AC-1.12b: "Phases without research configs (Users, CUJs, Scope, Acceptance) do not block sprint progression" (testable now)
- AC-1.12c: "Research configs for remaining 4 phases" (deferred to future implementation)

---

## Finding 6: SaveRevision Atomicity (Plan Correctly Identifies This Gap)

**Severity:** INFO - Plan's research insights are accurate but the code has been partially fixed

The plan's Research Insights section states:
> "Spec versioning (SaveRevision) has non-atomic two-file writes and mutates input spec version as side effect"

Checking the actual `SaveRevision` in `evolution.go` lines 42-105:

1. **File locking exists:** `fileutil.LockFile(lockPath)` on line 53-62 serializes concurrent writers per spec ID.
2. **Atomic writes exist:** Both files use `fileutil.AtomicWriteFile` (write-to-temp-then-rename pattern).
3. **Rollback on metadata failure:** Line 99-100 removes the snapshot file if the revision metadata write fails.
4. **Version mutation fixed:** Line 79-80 creates a copy (`snapshot := *spec`) and sets version on the copy, not the input spec.

The plan's claim is **outdated** -- the code has been fixed since the plan was written. The current implementation uses file locks, atomic writes, and rollback. The only remaining concern is that a successful snapshot write followed by a failed metadata write will leave the snapshot removed (correct behavior) but the caller gets an error and doesn't know the version was consumed.

**Recommendation:** Update the plan's Research Insights section to note that SaveRevision atomicity has been addressed. The remaining edge case (concurrent version number computation) is mitigated by file locking.

---

## Finding 7: Consistency Engine Only Implements 2 of 5 Conflict Types

**Severity:** MEDIUM - Plan implies GoalFeature is implemented; reality is UserFeature + VisionAlignment only

**Defined conflict types in `types.go` lines 265-271:**
```go
const (
    ConflictUserFeature     ConflictType = iota // 0 - Feature doesn't match target users
    ConflictGoalFeature                         // 1 - Goal not supported by features
    ConflictScopeCreep                          // 2 - Feature contradicts non-goals
    ConflictAssumption                          // 3 - Assumption conflicts with other content
    ConflictVisionAlignment                     // 4 - PRD section misaligned with vision spec
)
```

**Actually emitted by code:**
- `ConflictUserFeature` (0): Emitted by `engine.go:checkUserFeatureAlignment` -- basic keyword match
- `ConflictVisionAlignment` (4): Emitted by `vision.go:CheckVisionAlignment` -- keyword overlap + negation detection

**Never emitted:**
- `ConflictGoalFeature` (1): No code path generates this
- `ConflictScopeCreep` (2): No code path generates this
- `ConflictAssumption` (3): No code path generates this

The plan's Scope section (line 82) says "only GoalFeature ships" but what actually ships is UserFeature + VisionAlignment. This is a naming confusion in the plan.

---

## Finding 8: DraftStatus Values Match

**Severity:** INFO - Correct alignment

**Plan references:**
> "DraftStatus: Pending(0), Proposed(1), Accepted(2), NeedsRevision(3)"

**Actual `types.go` lines 63-70:**
```go
const (
    DraftPending       DraftStatus = iota // 0
    DraftProposed                         // 1
    DraftAccepted                         // 2
    DraftNeedsRevision                    // 3
)
```

This is a perfect match. No issues.

---

## Finding 9: Quick Scan Trigger Phase Inconsistency

**Severity:** LOW - Plan mentions "Quick scan moved to Users phase" but this is correct in code

The plan's institutional learnings (line 288) note:
> "Quick scan moved to Users phase (from oracle-review-issues): Changes when research evidence is available -- affects AC-1.12 hunter trigger sequence."

In `orchestrator.go` lines 336-348, the quick scan fires when advancing to PhaseUsers:
```go
if state.Phase == PhaseUsers {
    if sync {
        o.runQuickScanSync(ctx, state)
    } else {
        go func() { ... o.runQuickScanBackground(bgCtx) }()
    }
}
```

This is consistent with the institutional learning. However, `PhaseUsers` has NO research config in `DefaultResearchPlan()`, so the quick scan is the only research that fires for this phase. AC-1.12 claims "Users->community analysis" triggers, which is misleading -- it's actually the legacy quick scan (GitHub + HackerNews), not a "community analysis" hunter.

---

## Finding 10: Confidence Weights Not Reflected in Plan

**Severity:** LOW - Plan doesn't test weight correctness

The `ConfidenceScore.Total()` method uses specific weights: 20/25/20/20/15. The plan never tests whether these weights are correctly applied. A unit test for confidence calculation should verify that Total() returns the weighted sum, not just that individual components update.

**Recommendation:** Add AC-1.8a: "Confidence Total() applies correct weights: Completeness 20%, Consistency 25%, Specificity 20%, Research 20%, Assumptions 15%"

---

## Summary Table

| Finding | ID | Severity | Status |
|---------|-----|----------|--------|
| AC-1.8 lists 4 confidence components, actual has 5 (missing Assumptions 15%) | F1 | HIGH | Incorrect in plan |
| AC-1.14 references ConflictGoalFeature which no code emits | F2 | MEDIUM | Incorrect in plan |
| Research coverage formula mismatch (>80% vs >60%, review coverage unimplemented) | F3 | MEDIUM | Contradictory + unimplemented |
| AcceptanceCriterion struct compatibility | F4 | LOW | Compatible (non-issue) |
| AC-1.12 claims 8-phase research but only 4 phases have configs | F5 | MEDIUM | Partially incorrect |
| SaveRevision atomicity already fixed in code | F6 | INFO | Plan outdated |
| Only 2 of 5 conflict types implemented (UserFeature + VisionAlignment) | F7 | MEDIUM | Naming confusion in plan |
| DraftStatus values match | F8 | INFO | Correct |
| Quick scan correctly on Users phase, but AC-1.12 mislabels it | F9 | LOW | Minor label issue |
| Confidence weights not tested | F10 | LOW | Missing criterion |

---

## Recommended Plan Corrections (Priority Order)

1. **Fix AC-1.8:** Change "four components" to "five components (Completeness, Consistency, Specificity, Research, Assumptions)"
2. **Fix AC-1.14:** Replace with UserFeature + VisionAlignment criteria matching actual engine behavior
3. **Fix AC-1.12:** Reduce to 4 implemented phase-hunter mappings; defer the other 4
4. **Resolve research coverage target:** Pick one number (>60% or >80%) and define it against `researchQuality()` formula
5. **Add AC-1.9a:** Review coverage reducing Research score is unimplemented -- flag as implementation requirement
6. **Fix Scope section:** "only GoalFeature ships" should say "only UserFeature + VisionAlignment ship"
7. **Update SaveRevision insight:** Code already has file locks + atomic writes + rollback
8. **Add AC-1.8a:** Test confidence weight correctness
