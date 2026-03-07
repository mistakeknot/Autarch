# Arbiter Spec Sprint

**Primary workflow for PRD creation:** Propose-first 8-phase flow with integrated research and confidence scoring.

## Phase Flow

```
Phase 1: Vision        → Arbiter proposes project vision
Phase 2: Problem       → Problem statement + consistency check
Phase 3: Users         → Personas (informed by Ranger scan)
Phase 4: Features      → Feature list + goals
Phase 5: CUJs          → Critical User Journeys from users + features
Phase 6: Requirements  → Given/When/Then format
Phase 7: Scope         → Boundaries + assumptions
Phase 8: Acceptance    → Criteria for each CUJ

▼ PRD Complete → Handoff: Research (R), Tasks (T), Export (E)
```

**Consistency Engine:** Validates cross-section alignment (Problem↔Users, Users↔Features, Features↔CUJs, CUJs↔AC, AC↔Scope).

**Confidence Scoring (0.0-1.0):** Clarity, Completeness, Coherence, Feasibility. Low-confidence proposals show warnings but don't block.

## Key Files

| Path | Purpose |
|------|---------|
| `internal/gurgeh/arbiter/orchestrator.go` | Sprint flow: Start → Advance → Accept → Revise → Handoff |
| `internal/gurgeh/arbiter/generator.go` | AI draft generation (propose-first) |
| `internal/gurgeh/arbiter/types.go` | Phase, SprintState, ConfidenceScore, Conflict types |
| `internal/gurgeh/arbiter/consistency/` | Cross-section validation |
| `internal/gurgeh/arbiter/confidence/` | 0.0-1.0 scoring |
| `internal/gurgeh/arbiter/research_phases.go` | Phase → hunter mapping + query extractors |
| `internal/gurgeh/specs/evolution.go` | Spec versioning, assumption decay |

## Sprint Persistence

Sprints auto-save to `.gurgeh/sprints/<sprint-id>.json` with all phase content, exploration cache, and current phase pointer. Resume via `./dev autarch tui --tool=gurgeh`.
