---
title: "Spec Phase Reordering Strategy"
category: architecture-decisions
tags: [prd, phases, cuj, requirements, ordering]
module: Gurgeh/Arbiter
symptom: "CUJs generated too late to inform requirements"
root_cause: "Phase ordering didn't reflect dependency relationships"
date: 2026-02-04
---

# Spec Phase Reordering Strategy

## Problem

The original PRD phase ordering placed CUJs (Critical User Journeys) near the end:

```
Vision → Problem → Users → Features → Requirements → Scope → CUJs → Acceptance
```

This meant CUJs were generated **after** Requirements, even though CUJs should logically inform what requirements are needed.

## Analysis

User feedback: "I feel like CUJs should be sooner"

The insight is that **CUJs define the user's actual workflow**, and **Requirements should be derived from those journeys**. The original ordering was:
- Features/Goals (what we want to build)
- Requirements (detailed specs)
- CUJs (how users actually use it)

This is backwards - you should understand user journeys before specifying technical requirements.

## Solution

Reordered phases to place CUJs before Requirements:

```go
const (
    PhaseVision Phase = iota      // 0: High-level purpose
    PhaseProblem                   // 1: Problem being solved
    PhaseUsers                     // 2: Target users
    PhaseFeaturesGoals             // 3: What we're building
    PhaseCUJs                      // 4: How users accomplish tasks (MOVED UP)
    PhaseRequirements              // 5: Detailed specs (derived from CUJs)
    PhaseScopeAssumptions          // 6: Boundaries and constraints
    PhaseAcceptanceCriteria        // 7: How we verify success
)
```

### Rationale

The new ordering follows a logical flow:

1. **Vision** - Why does this exist?
2. **Problem** - What pain are we solving?
3. **Users** - Who has this pain?
4. **Features/Goals** - What capabilities will help?
5. **CUJs** - How will users actually use these features?
6. **Requirements** - What must we build to support those journeys?
7. **Scope** - What's in/out of this release?
8. **Acceptance** - How do we know we're done?

CUJs at position 5 means:
- We know the features before defining journeys
- Requirements are informed by actual user workflows
- Scope decisions have journey context

## Files Changed

- `internal/gurgeh/arbiter/types.go`: Phase enum reordering
- `internal/gurgeh/exploration/explore.go`: Updated phase order in exploration
- `docs/FLOWS.md`: Updated phase documentation
- `docs/gurgeh/AGENTS.md`: Updated phase flow documentation
- `docs/oracle-architecture-review-2026-02-01.md`: Updated phase sequence

## Impact

When reordering phases in a stateful system:

1. **Enum values change** - Any persisted state using integer values will be wrong
2. **Switch statements** - Check all phase switches for ordering assumptions
3. **Documentation** - Update all references to phase order
4. **Existing sprints** - May need migration or warning

In this case, the system was in development so migration wasn't needed, but production systems would need careful handling.

## Key Insight

Phase ordering should reflect **information dependencies**:
- What information does phase N need?
- Which earlier phases provide that information?
- Can we derive phase N's content without phases after it?

If phase A needs information from phase B, A should come after B.
