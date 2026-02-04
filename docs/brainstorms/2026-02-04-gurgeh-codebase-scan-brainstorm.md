# Brainstorm: Gurgeh Codebase Scan Execution

**Date:** 2026-02-04
**Bead:** Autarch-c8d (P1)
**Status:** Ready for planning

## What We're Building

Implement Claude Code-style codebase exploration in Gurgeh's Kickoff flow. When user initiates a scan (F4), the system analyzes the target codebase and extracts structured artifacts that automatically seed the sprint phases.

### Core Capabilities

1. **Tech Stack Detection**
   - Parse dependency files: `go.mod`, `package.json`, `Cargo.toml`, `requirements.txt`, `Gemfile`, etc.
   - Identify primary language(s) and frameworks
   - Extract version constraints and key dependencies

2. **README/Docs Analysis**
   - Parse README.md for project description, purpose, architecture notes
   - Look for CLAUDE.md, AGENTS.md, CONTRIBUTING.md for additional context
   - Extract stated goals, user personas, problem statements

3. **Directory Structure Analysis**
   - Identify architectural patterns (monorepo, microservices, MVC, etc.)
   - Detect entry points (`main.go`, `index.ts`, `app.py`, etc.)
   - Map key directories (cmd/, internal/, src/, tests/, docs/)

4. **Code Pattern Recognition**
   - Analyze imports to understand dependency graph
   - Detect test framework and coverage patterns
   - Identify CI/CD configuration (GitHub Actions, etc.)

## Why This Approach

**"Do what Claude Code does"** - Claude Code's exploration is effective because:
- It's comprehensive but fast (doesn't read every file)
- It prioritizes high-signal files (configs, READMEs, entry points)
- It builds a mental model before diving into specifics

This gives Gurgeh's sprint phases a head start with real evidence from the codebase rather than starting from scratch.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Scan trigger | F4 in Kickoff view | Existing UI pattern, already wired |
| Results presentation | Auto-seed silently | No extra UI friction, results flow into phases |
| Progress UX | Async with spinner | Uses existing `ScanProgressMsg` infrastructure |
| V1 scope | Full Claude Code-style | Tech stack + README + structure + patterns |

## Integration Points

### Existing Infrastructure to Use

- `ScanProgressMsg` - Progress updates during scan
- `CodebaseScanResultMsg` - Final results container
- `PhaseArtifacts` / `scan.Artifacts` - Structured per-phase data
- `orchestrator.StartWithScan()` - Injects artifacts into sprint

### New Code Needed

- `internal/gurgeh/scan/` - Actual scanning logic
  - `tech_stack.go` - Dependency file parsing
  - `readme.go` - Documentation analysis
  - `structure.go` - Directory pattern detection
  - `patterns.go` - Code pattern recognition
- Wire scanner to agent layer for async execution
- Map scan results to `PhaseArtifacts` structure

## Open Questions

1. **LLM involvement**: Should README analysis use LLM for semantic extraction, or pure heuristics?
2. **Scan depth**: How deep to recurse? Should we respect .gitignore?
3. **Caching**: Cache scan results for re-runs on same codebase?

## Success Criteria

- [ ] F4 in Kickoff triggers scan with visible progress
- [ ] Scan extracts: languages, frameworks, dependencies, project description
- [ ] Results automatically populate Vision/Problem/Users phases with evidence
- [ ] Sprint phases show `<evidence>` blocks from scan artifacts

## Next Steps

→ Proceed to `/workflows:plan` for implementation details
