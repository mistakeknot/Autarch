---
title: "feat: Claude Code-style codebase scan in Gurgeh kickoff"
type: feat
date: 2026-02-04
bead: Autarch-c8d
priority: P1
status: implemented
implementation_date: 2026-02-04
---

# Claude Code-style Codebase Scan in Gurgeh Kickoff

## Status: ✅ Implemented

**Implementation approach:** Simplified spike as recommended by reviewers (DHH, Kieran, Simplicity).

## What Was Built

### 1. Deterministic Tech Stack Detection (~50 lines)

Added `detectTechStack(root string) []EvidenceItem` to `internal/autarch/agent/scan.go`:

- Detects Go, TypeScript/JavaScript, Rust, Python from manifest files
- Uses simple `bytes.Contains()` for framework detection (no heavy parsers)
- Returns `[]EvidenceItem` directly (no intermediate types)
- Detects frameworks: Bubble Tea, GORM, Cobra, Gin, Echo, Fiber, Next.js, React, Vue, Svelte, Express, Tailwind, Tokio, Actix, Axum, Tauri, Django, FastAPI, Flask, PyTorch

### 2. Wired to Scan Flow

In `ScanCodebaseWithProgress`:
1. Call `detectTechStack()` first (instant, no LLM)
2. Report progress: `"Tech stack: Go 1.22 with Bubble Tea"`
3. Run LLM analysis (existing flow)
4. Merge deterministic evidence into `PhaseArtifacts.Vision.Evidence`

### 3. Improved LLM Prompt for Verbatim Evidence

Updated `buildScanPrompt()` to explicitly request:
- **VERBATIM QUOTES** from source files (not paraphrasing)
- Confidence levels based on directness (0.9 explicit, 0.7 implied, 0.5 tangential)
- Specific guidance for Vision/Problem/Users evidence types

## Why This Approach

**Reviewer consensus:** The original 660-line plan over-architected a simple problem.

> "The type system is NOT justified. It's solving a UX problem (spinner progress) while pretending to solve a grounding problem. The grounding problem is solved by better prompts, not deterministic parsers." — DHH Review

**Key insight:** Better PRD grounding comes from:
1. Better LLM prompts (request verbatim quotes, not summaries)
2. Deterministic evidence merged into results (high confidence tech stack)

**NOT** from elaborate type hierarchies (`CodebaseProfile`, `LanguageInfo`, `FrameworkInfo`).

## Files Changed

- `internal/autarch/agent/scan.go`
  - Added `detectTechStack()` function (~50 lines)
  - Modified `ScanCodebaseWithProgress()` to call it and merge results
  - Updated `buildScanPrompt()` with verbatim evidence instructions

## Acceptance Criteria

- [x] Tech stack detection works for Go, TypeScript, Rust, Python
- [x] Progress shows tech stack before LLM runs
- [x] Deterministic evidence merged into Vision.Evidence
- [x] LLM prompt requests verbatim quotes
- [x] All tests pass
- [x] Builds successfully

## What Was NOT Built (YAGNI)

The following were proposed in the original plan but intentionally omitted:

| Proposed | Why Omitted |
|----------|-------------|
| `explorer/` subpackage | Overkill for ~50 lines |
| `CodebaseProfile` type | Intermediate type adds no value |
| `LanguageInfo`, `FrameworkInfo` types | Return `[]EvidenceItem` directly |
| `ProjectStructure` enum | No downstream consumer |
| `golang.org/x/mod/modfile` parser | `bytes.Contains()` is sufficient |
| 4-phase rollout | Single PR is appropriate |
| Performance benchmarks | No evidence of performance problem |

## Lessons Learned

1. **Over-planning detector fired correctly** - The project has a documented learning about this (`docs/solutions/workflow-issues/over-planning-before-reproduction-20260203.md`)

2. **Reviewer agents add value** - Three reviewers (DHH, Kieran, Simplicity) all converged on the same recommendation

3. **Context matters for reviews** - First review missed that goal was "PRD grounding"; re-review with context still recommended simplification

4. **Spike-first was right** - 50 lines of code, not 400 lines across 4 phases
