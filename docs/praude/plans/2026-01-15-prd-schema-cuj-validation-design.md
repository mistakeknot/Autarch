# PRD Schema: CUJ + Evidence Graph + Validation Modes

Date: 2026-01-15

## Goal
Extend Praude's PRD schema to add graph-linked CUJs and evidence-backed market
and competitive sections. Support both hard and soft validation modes while
preserving TUI-first UX and CLI parity. Keep schema in parity with Tandemonium
integration needs.

## Cross-Repo Coordination Note
This schema is consumed directly by Tandemonium. When changing this document,
update Tandemonium's design doc:
`/Users/sma/Tandemonium/docs/plans/2026-01-15-coordination-spec-graph-design.md`.

## Storage
- Specs: `.praude/specs/PRD-###.yaml`
- Research artifacts: `.praude/research/PRD-###-<timestamp>.md`
- Suggestions: `.praude/suggestions/PRD-###-<timestamp>.md`
- Briefs: `.praude/briefs/PRD-###-<timestamp>.md`

## Schema Extensions (Graph-Linked)
Add these top-level fields to the PRD spec schema:

- `critical_user_journeys`: array of CUJ objects:
  - `id` (CUJ-###, unique within PRD), `title`, `priority` (critical/high/med/low)
  - `steps[]` (ordered), `success_criteria[]`
  - `linked_requirements[]` (REQ-### references)

- `market_research`: array of findings:
  - `id` (MR-###, unique within PRD), `claim`, `evidence_refs[]`, `confidence`, `date`

- `competitive_landscape`: array of competitors:
  - `id` (COMP-###, unique within PRD), `name`, `positioning`
  - `strengths[]`, `weaknesses[]`, `risk`, `evidence_refs[]`

Evidence refs are structured objects:
- `{ path: "PRD-001-20260115.md", anchor: "#section", note: "optional" }`

## Validation Modes
Configurable via `.praude/config.toml` or CLI flag (default: soft):

- Hard validation:
  - Missing CUJ links or missing evidence refs is a failure
  - Spec cannot be approved until errors resolved

- Soft validation:
  - Same checks, but only warnings
  - Approval allowed; warnings recorded in metadata

Validation also ensures:
- CUJ IDs are unique and referenced requirements exist
- Evidence refs point to existing files
- Priority values are valid
- Market/competitive sections are optional (warnings only in both modes)

## Guided Interview (TUI-only)
Flow:
1) Prompt to scan repo + docs (full repo scan with `.gitignore` exclusions)
2) Draft PRD from scan
3) Confirm draft, then interview starts
4) Interview order: vision -> users -> problem -> requirements
5) Auto-generate CUJs immediately after interview (auto priority)
6) Auto-validate after interview
7) Prompt to run research (optional; can run in background)

## Research + Suggestions Pipeline
- Research outputs written to `.praude/research/`
- Every claim must include evidence refs
- Research includes OSS project scan for learnings/bootstrapping/insights
- Claude Code profiles spawn subagents to run research and suggestions
- Suggestions can be created via both TUI and CLI
- Background research produces suggestions in `.praude/suggestions/`
- Suggestions are reviewed per-section; accepted changes auto-apply + auto-commit

## Default CUJ Behavior
- Maintenance CUJ auto-generated for each PRD
- Priority: low
- Steps/success criteria auto-filled (minimal)
- Tasks require primary CUJ, with optional secondary CUJs
- CUJ IDs are scoped to the PRD (Tandemonium must scope by PRD ID)

## TUI/CLI Updates
- New sections for CUJs, Market Research, Competitive Landscape
- Completeness indicators per section
- Validation output shows errors vs warnings (mode-aware)
- CLI parity for validation: `praude validate <id> --mode=hard|soft`
- CLI parity for suggestions: `praude suggest <id> --agent=<profile>`
- CLI interview: `praude interview`
- CLI suggestions review/apply: `praude suggestions review <id>` and `praude suggestions apply <id> [--all]`
- CLI show should surface validation warnings stored in metadata
- TUI detail view should surface validation warnings
- TUI hotkeys: `g` interview, `r` research + launch, `p` suggestions + launch, `s` review/apply suggestions
- TUI detail shows last action status for launches/errors

## Drift Control
- Spec hash includes new CUJ and evidence sections
- Drift events require accept or reject decision
- Accept drift updates spec and auto-commits with standard messages

## Tandemonium Integration
- Tandemonium reads `.praude/specs/` directly
- CUJ IDs and evidence refs are used for task linkage and drift checks
- Critical CUJs imply drift blocking on the Tandemonium side
- Optional `critical_files[]` field can be added later if needed

## Testing
- Unit tests for schema validation and evidence refs
- ID generation tests for CUJ/MR/COMP prefixes
- CLI/TUI tests for new sections and validation modes
