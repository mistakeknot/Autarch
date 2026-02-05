---
title: "PRD Requirements Blank on Generation"
category: logic-errors
tags: [orchestrator, phase-generation, claude-code, exploration]
module: Gurgeh/Arbiter
symptom: "Requirements and later PRD phases show blank/placeholder content"
root_cause: "extractPhaseContent() only handled 4 early phases, returned empty for later phases"
date: 2026-02-04
---

# PRD Requirements Blank on Generation

## Problem

When users reached the Requirements phase (and later phases like Scope, Acceptance Criteria), the content was blank or showed placeholder text instead of generated content.

## Root Cause

The `extractPhaseContent()` method in `orchestrator.go` only had explicit handlers for the first 4 phases (Vision, Problem, Users, Features/Goals). Later phases fell through to return an empty string:

```go
// BEFORE: Only handled 4 phases
func (o *Orchestrator) extractPhaseContent(phase Phase, artifacts *scan.Artifacts) string {
    switch phase {
    case PhaseVision:
        return extractVision(artifacts)
    case PhaseProblem:
        return extractProblem(artifacts)
    case PhaseUsers:
        return extractUsers(artifacts)
    case PhaseFeaturesGoals:
        return extractFeatures(artifacts)
    default:
        return "" // BUG: Later phases got nothing!
    }
}
```

## Solution

Added dynamic generation for later phases using Claude Code subprocess via `exploration.GeneratePhase()`:

```go
// shouldUseDynamicGeneration returns true for phases that need Claude Code
func (o *Orchestrator) shouldUseDynamicGeneration(phase Phase) bool {
    switch phase {
    case PhaseCUJs, PhaseRequirements, PhaseScopeAssumptions, PhaseAcceptanceCriteria:
        return true
    default:
        return false
    }
}

// In generateDraft():
if o.shouldUseDynamicGeneration(state.Phase) {
    content, err := exploration.GeneratePhase(ctx, o.projectPath, state.Phase.String(), priorContext)
    if err == nil && content != "" {
        state.Sections[state.Phase] = SectionDraft{
            Content: content,
            Status:  DraftStatusProposed,
        }
    }
}
```

The `buildPriorContext()` helper collects content from all earlier phases to provide context for generation:

```go
func (o *Orchestrator) buildPriorContext(state *SprintState) map[string]string {
    ctx := make(map[string]string)
    phases := AllPhases()
    for _, p := range phases {
        if p >= state.Phase {
            break // Only include earlier phases
        }
        if draft, ok := state.Sections[p]; ok && draft.Content != "" {
            ctx[phaseKeyMap[p]] = draft.Content
        }
    }
    return ctx
}
```

## Key Insight

Phase content generation follows two patterns:
1. **Early phases (Vision → Features)**: Extract from scan artifacts (static analysis)
2. **Later phases (CUJs → Acceptance)**: Generate via Claude Code with prior context (LLM synthesis)

The split makes sense because later phases require synthesis across earlier phases rather than extraction from code.

## Files Changed

- `internal/gurgeh/arbiter/orchestrator.go`: Added `shouldUseDynamicGeneration()`, `buildPriorContext()`
- `internal/gurgeh/exploration/explore.go`: Added `GeneratePhase()` function

## Prevention

When adding new phases to multi-phase workflows, ensure:
1. Each phase has an explicit content source (extraction OR generation)
2. Add tests that verify content is non-empty for all phases
3. Document which phases use which generation strategy
