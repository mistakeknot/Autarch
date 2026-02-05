---
title: "Spec Propagation Consistency Pattern"
category: architecture-decisions
tags: [orchestrator, propagation, claude-code, token-efficiency]
module: Gurgeh/Arbiter
symptom: "Changes in one phase don't update dependent phases"
root_cause: "Phases were generated independently without cross-phase consistency"
date: 2026-02-04
---

# Spec Propagation Consistency Pattern

## Problem

When users iterated on a phase (e.g., changing Vision), dependent phases (Problem, Users, etc.) would become inconsistent. The system generated each phase independently without considering how changes should ripple through the document.

User request: "The coding agent should generate every phase and also edit the other phases if needed with every iteration or scan (while saving tokens for the parts that don't need changes)"

## Solution

Created a `PropagateChanges()` function that sends all phases + feedback to Claude Code in a single call. Claude Code determines which phases need updates and returns a structured response.

### Token Efficiency

The key insight is that Claude Code can evaluate all phases in one pass and only return content for phases that actually changed. This is more efficient than:
1. Regenerating all phases (wasteful)
2. Making multiple Claude Code calls per phase (high latency)

### Implementation

```go
// exploration/explore.go
type PhaseUpdate struct {
    Phase   string
    Content string
    Changed bool
}

func PropagateChanges(ctx context.Context, cwd string, currentPhases map[string]string,
    changedPhase string, feedback string) (map[string]PhaseUpdate, error) {

    prompt := buildPropagationPrompt(currentPhases, changedPhase, feedback)

    // Single Claude Code call that returns JSON
    output, err := runClaudeCode(ctx, cwd, prompt)
    if err != nil {
        return nil, err
    }

    // Parse JSON response: {"updates": {"phase": {"content": "...", "changed": true}}}
    return parseUpdates(output)
}
```

### Phase Key Mapping

Bidirectional maps for phase name normalization:

```go
var phaseKeyMap = map[Phase]string{
    PhaseVision:             "vision",
    PhaseProblem:            "problem",
    PhaseUsers:              "users",
    PhaseFeaturesGoals:      "features",
    PhaseCUJs:               "cujs",
    PhaseRequirements:       "requirements",
    PhaseScopeAssumptions:   "scope",
    PhaseAcceptanceCriteria: "acceptance",
}

var keyPhaseMap = map[string]Phase{
    "vision":      PhaseVision,
    "problem":     PhaseProblem,
    // ... reverse mapping
}
```

### Applying Updates

```go
func (o *Orchestrator) applyPropagatedUpdates(state *SprintState, updates map[string]PhaseUpdate) {
    for key, update := range updates {
        if !update.Changed {
            continue // Skip unchanged phases (token savings!)
        }
        phase, ok := keyPhaseMap[key]
        if !ok {
            continue
        }
        state.Sections[phase] = SectionDraft{
            Content: update.Content,
            Status:  DraftStatusProposed,
        }
    }
}
```

## Key Insight

The propagation pattern balances three concerns:
1. **Consistency**: All phases stay aligned with each other
2. **Token efficiency**: Only changed phases are returned
3. **Latency**: Single Claude Code call instead of per-phase calls

The LLM is good at understanding semantic dependencies between sections, so it can accurately determine which phases need updates based on changes to others.

## Files Changed

- `internal/gurgeh/arbiter/orchestrator.go`: Added `applyPropagatedUpdates()`, phase maps
- `internal/gurgeh/exploration/explore.go`: Added `PropagateChanges()` function

## When to Use This Pattern

Apply propagation pattern when:
- Document has interdependent sections
- Changes to one section may invalidate others
- You want to minimize API calls while maintaining consistency
- The LLM can reason about section relationships
