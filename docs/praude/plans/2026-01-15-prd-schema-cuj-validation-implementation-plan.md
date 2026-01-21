# Implementation Plan: PRD Schema + CUJ + Evidence Validation

Date: 2026-01-15

## Scope
Update Praude to support graph-linked CUJs, evidence-backed market research,
competitive landscape sections, and hard/soft validation modes. Maintain TUI
and CLI parity. Keep schema in sync with Tandemonium. Add guided interview flow
and research/suggestions pipeline.

## Implementation Checklist (Owners)
- [ ] (Praude) Extend PRD schema types (CUJs, market, competitive, evidence refs)
- [ ] (Praude) Add maintenance CUJ defaults to templates
- [ ] (Praude) Add validation_mode config + CLI flag
- [ ] (Praude) Implement hard/soft validation rules and warnings metadata
- [ ] (Praude) Guided interview flow with repo scan + draft confirmation
- [ ] (Praude) CUJ auto-generation + priority assignment after interview
- [ ] (Praude) Auto-validate after interview
- [ ] (Praude) Research output with evidence refs + OSS scan
- [ ] (Praude) Suggestions pipeline in `.praude/suggestions/` (per-section review)
- [ ] (Praude) Spawn agents/subagents for research + suggestions (Claude Code profiles)
- [ ] (Praude) Add CLI `praude suggest <id> --agent=<profile>`
- [ ] (Praude) Show validation warnings in CLI show + TUI detail
- [ ] (Praude) Drift hash includes CUJ + evidence sections
- [ ] (Praude) Tests for schema, validation, and TUI/CLI updates

## Phase 1: Schema + Types
1) Extend PRD schema types
- Files: `internal/specs/` (schema + model structs)
- Add fields: `critical_user_journeys`, `market_research`, `competitive_landscape`
- Add evidence ref struct `{path, anchor, note}`
- Ensure ID uniqueness is enforced within PRD

2) Update spec template
- File: `internal/specs/templates/` or equivalent
- Add example CUJ, MR, COMP entries
- Add maintenance CUJ with low priority and minimal steps

## Phase 2: Validation Modes
3) Add validation mode config
- File: `.praude/config.toml` (default `validation_mode = "soft"`)
- CLI override: `praude validate <id> --mode=hard|soft`

4) Implement validation rules
- Validate CUJ IDs and linked requirements
- Validate evidence refs point to existing files
- Validate priority values
- Market/competitive sections optional (warnings only)
- On hard mode: return errors
- On soft mode: return warnings

5) Update validation output
- TUI and CLI should clearly separate errors vs warnings
- Store warnings in spec metadata if soft mode

## Phase 3: Guided Interview (TUI-only)
6) Repo scan
- Full repo scan with `.gitignore` exclusions
- Draft PRD from scan

7) Interview flow
- Confirm draft before questions
- Interview order: vision -> users -> problem -> requirements
- Auto-generate CUJs after interview (auto priority)
- Auto-validate after interview

## Phase 4: Research + Suggestions
8) Research outputs
- Write to `.praude/research/PRD-###-<timestamp>.md`
- Require evidence refs for all claims
- Include OSS project scan section
- If profile is Claude Code, spawn subagent for research

9) Suggestions pipeline
- Suggestions stored in `.praude/suggestions/PRD-###-<timestamp>.md`
- Review per-section with accept/reject
- Accept applies changes + auto-commit
- Add CLI `praude suggest <id> --agent=<profile>`
- Support agent-generated suggestions (Claude Code subagent)

## Phase 5: TUI + CLI Surface
10) TUI sections
- Add CUJ, Market Research, Competitive Landscape
- Completeness indicators
- Suggestion review workflow
- Surface validation warnings in detail panel

11) CLI parity
- `praude show` includes new sections
- `praude validate` supports mode switch
- `praude suggest` command
- `praude show` should include validation warnings (metadata)

## Phase 6: Drift + Hashing
12) Extend drift hash inputs
- Include CUJ + evidence sections in hash
- Drift detection treats these sections as spec changes

## Phase 7: Tests
13) Unit tests
- Spec validation (hard vs soft)
- Evidence ref validation
- ID uniqueness and prefix handling

14) CLI/TUI tests
- TUI snapshot for new sections
- CLI validation output formatting

## Notes
- Keep ASCII-only edits in code/comments
- Avoid git worktrees in Praude unless requested
- Update Tandemonium design doc if schema changes occur
