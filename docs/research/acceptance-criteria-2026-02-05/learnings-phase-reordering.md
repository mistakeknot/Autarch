# Learnings: phase reordering

**Source:** Agent result from session b817bfb0
**Task ID:** a6746c3
**Date:** 2026-02-05

---

Now I have all the information needed for a thorough analysis.

---

## Analysis: Does the Phase Reordering Learning Apply to the Acceptance Criteria Plan?

**Yes, it applies -- and the plan has both correct and incorrect aspects relative to the learning.**

### Key Insight from the Learning

From `/root/projects/Autarch/docs/solutions/architecture-decisions/spec-phase-reordering-strategy.md`:

> "Phase ordering should reflect **information dependencies**: What information does phase N need? Which earlier phases provide that information? If phase A needs information from phase B, A should come after B."

The canonical reordered phase sequence is:

```
0: Vision
1: Problem
2: Users
3: Features/Goals
4: CUJs          <-- MOVED UP from position 6
5: Requirements   <-- now AFTER CUJs
6: Scope/Assumptions
7: Acceptance Criteria
```

This is confirmed by the actual code in `/root/projects/Autarch/internal/gurgeh/arbiter/types.go` (lines 16-25).

### Question 1: Does AC-1.12 use the correct phase ordering?

**Partially correct, but contains an inconsistency.**

AC-1.12 lists the hunter mapping as:

> "Vision -> GitHub Scout+HackerNews, Problem -> arXiv+OpenAlex, Users -> community analysis, **Features -> competitor tracking, CUJs -> workflow patterns, Requirements -> implementation patterns**, Scope -> inverse research, Acceptance -> test patterns"

The **ordering** here is correct -- CUJs appears before Requirements, matching the reordered phases. However, the actual code in `research_phases.go` only defines 4 phase configs (Vision, Problem, Features, Requirements), not 8. CUJs has no hunter configuration in code. AC-1.12 claims "CUJs -> workflow patterns" and "Scope -> inverse research" and "Acceptance -> test patterns" as if all 8 phases have hunters, but only 4 of 8 exist in implementation.

The plan's own Research Insights section acknowledges this mismatch: "Only 4 of 8 phases have research configs in `research_phases.go`" and the timing table says "Research coverage >60% (4 of 8 phases have configs)". But AC-1.12 still lists all 8 phases as if they all trigger hunters, which makes the criterion untestable as written for phases 2 (Users), 4 (CUJs), 6 (Scope), and 7 (Acceptance).

### Question 2: Does the CUJ-1 description match the reordered phases?

**Yes, it matches correctly.** The CUJ-1 narrative in the plan lists the phase-to-hunter mapping in the reordered sequence: "Vision triggers market trend hunters... Problem triggers academic research... Users triggers community analysis... Features triggers competitor tracking... **CUJs triggers workflow pattern extraction**... **Requirements triggers implementation pattern search**... Scope triggers inverse research... Acceptance triggers test pattern discovery."

This correctly places CUJs (phase 4) before Requirements (phase 5), consistent with the learning.

### Question 3: Are there timing or dependency assumptions that conflict with the new phase order?

**One subtle conflict found.** In the phase sidebar display code at `/root/projects/Autarch/internal/tui/views/phase_sidebar.go` (lines 55-76), the `phaseLabel` function lists cases in an order that places Requirements (line 65) before Scope (line 67) before CUJs (line 69). This is cosmetic (switch statements don't depend on ordering), but it visually suggests the old ordering. The switch statement doesn't affect runtime behavior since it is a label lookup by enum value, but it could confuse developers who read it as representing the phase sequence.

The plan itself has **no timing or dependency conflicts** with the new phase order. The CUJ descriptions, requirements, and acceptance criteria all treat CUJs as preceding Requirements, which is correct.

### Question 4: Does the plan account for CUJs being phase 5 (before Requirements)?

**Yes, but with a numbering note.** The learning document says "CUJs at position 5" but the actual enum is `PhaseCUJs = 4` (0-indexed iota). The plan's CUJ-1 narrative and AC-1.12 both correctly sequence CUJs before Requirements. The plan does not make any assumption that Requirements content is available when generating CUJs, which is the critical dependency the reordering was designed to address.

### Summary of Findings

| Check | Result | Detail |
|-------|--------|--------|
| AC-1.12 phase ordering | Correct sequence, but overclaims | Lists 8 phase-hunter mappings when only 4 exist in code |
| CUJ-1 narrative ordering | Correct | CUJs before Requirements throughout |
| Timing/dependency conflicts | None in plan; minor cosmetic issue in sidebar code | `phase_sidebar.go` switch order is misleading but non-functional |
| CUJs-before-Requirements accounting | Correct | No assumption that Requirements are needed for CUJs |

### Suggested Incorporation

1. **AC-1.12 should be split** into two criteria: one for the 4 phases with implemented hunter configs (Vision, Problem, Features, Requirements) and one for the 4 aspirational phases (Users, CUJs, Scope, Acceptance) that need hunter configs added before the criterion is testable. This prevents AC-1.12 from being a guaranteed test failure.

2. **No changes needed** to the CUJ descriptions or dependency ordering -- they already correctly reflect the phase reordering learning.

3. **The phase_sidebar.go switch statement** at `/root/projects/Autarch/internal/tui/views/phase_sidebar.go` should have its cases reordered to match the enum ordering for code clarity, though this is a minor style issue, not a functional bug.