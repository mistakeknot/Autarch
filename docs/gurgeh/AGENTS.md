# AGENTS.md

## Ground Rules
- Run the superpowers bootstrap first:
  - `~/.codex/superpowers/.codex/superpowers-codex bootstrap`
- Use relevant superpowers skills for every task (TDD, writing-plans, executing-plans).
- Do not use git worktrees for this repo unless explicitly requested.
- Follow TDD strictly: write the failing test, watch it fail, then implement.
- Keep changes small, testable, and committed frequently.
- Default to ASCII-only edits unless the file already uses Unicode.

## 2026-01-15 Direction Update (CUJs + Evidence Graph)
- Latest design: `docs/plans/2026-01-15-prd-schema-cuj-validation-design.md`.
- Latest plan: `docs/plans/2026-01-15-prd-schema-cuj-validation-implementation-plan.md`.
- CUJ/market/competitive sections are now part of the core PRD schema.
- Validation modes (hard/soft) must be supported.
- Suggestions are stored under `.praude/suggestions/`.

## Repo Overview
Praude is a TUI-first PM/specs CLI that generates, validates, and governs PRDs.
It orchestrates external agents (Codex, Claude Code, opencode, droid) and stores
specs as YAML files in `.praude/specs/`.

## Key Paths and Data
- `.praude/specs/` one YAML per PRD (source of truth).
- `.praude/research/` market/competitive research outputs (timestamped).
- `.praude/suggestions/` staged updates for review (timestamped).
- `.praude/briefs/` agent briefs (timestamped; always written).
- `.praude/config.toml` agent profiles (command + args).
- `internal/specs/` schema, ID generation, validation, load/save.
- `internal/brief/` brief composer.
- `internal/agents/` profile resolution + process launch.
- `internal/git/` auto-commit helper.
- `internal/tui/` TUI model/view (TUI-first UX).
- `internal/cli/commands/` CLI parity with the TUI.

## Commands
- `praude` launches the TUI.
- `praude init` initializes `.praude/`.
- `praude list` prints PRD summaries.
- `praude show <id>` prints a spec file.
- `praude run <brief> --agent=<profile>` spawns an agent with a brief.

## Agent Integration (Codex/Claude/opencode/droid)
- Profiles live under `[agents]` in `.praude/config.toml`.
- Defaults must be safe to run (empty args), with commented examples.
- `praude run` must always write a brief first; if agent not found, print the
  brief path and exit cleanly.
- Briefs are always timestamped and persisted for auditability.

## Spec Schema + Validation
- Use the full PRD schema (strategic_context, user_story, summary, requirements,
  acceptance_criteria, files_to_modify, metadata).
- Validation checks required fields and returns actionable errors.
- Spec IDs are incremental `PRD-###`, derived by scanning `.praude/specs/`.
- Accept both `.yaml` and `.yml`, but write `.yaml`.

## Drift Control (PM Authority)
- Specs are the source of truth; briefs are derived views.
- Drift detection compares the current spec hash to the working context.
- If drift detected, user chooses:
  - Accept drift: update spec and auto-commit.
  - Reject drift: generate corrective guidance and re-run agent.

## Git Auto-Commit Rules
- Spec writes auto-commit with messages:
  - `chore(praude): add PRD-###`
  - `chore(praude): update PRD-###`
- Research/brief artifacts are committed if they modify spec state.
- Always use `internal/git` helpers; do not shell out ad-hoc.

## CLI/TUI Parity Rules
- CLI must mirror TUI capabilities.
- TUI is primary; CLI is for scripting and automation.
- Any new TUI action should have a CLI equivalent unless explicitly deferred.

## Testing
- Use TDD for all behavior changes.
- Run targeted tests (`go test ./internal/<package> -v`) while iterating.
- Add package-level tests before wiring multi-package behavior.
- Prefer small unit tests over broad integration tests.

## Arbiter Subsystem

The Arbiter manages guided PRD creation through sprint state, consistency checking, and confidence scoring.

### Key Packages

| Package | Path | Purpose |
|---------|------|---------|
| `arbiter` | `internal/gurgeh/arbiter/` | Sprint state machine and persistence |
| `consistency` | `internal/gurgeh/consistency/` | Conflict detection between PRD sections |
| `confidence` | `internal/gurgeh/confidence/` | Quality scoring (5 weighted dimensions) |

### Sprint State Machine

```
Phase flow: Vision → Problem → Users → Features+Goals → Requirements → Scope+Assumptions → CUJs → Acceptance Criteria
```

| Type | Description |
|------|-------------|
| `SprintState` | Active sprint with phase, sections, drafts |
| `Phase` | 8-stage enum (Vision → AcceptanceCriteria) |
| `DraftStatus` | Section completion: Pending, Proposed, Accepted, NeedsRevision |
| `SectionDraft` | Content + options + status + user edits for each PRD section |

### Export

`ExportToSpec()` converts a completed `SprintState` back to a `specs.Spec`, enabling round-trip migration. `MigrateFromSpec()` converts in the other direction.

**Persistence:** YAML files in `.gurgeh/sprints/`. Auto-saves on state transitions.

### Consistency Engine

Pluggable `Checker` interface detects conflicts between PRD sections:

```go
type Checker interface {
    Check(state *SprintState) []Conflict
}
```

| ConflictType | Example |
|--------------|---------|
| `Contradiction` | Requirements conflict with constraints |
| `MissingDependency` | Section references undefined terms |
| `ScopeCreep` | Feature exceeds stated boundaries |

### Confidence Scoring

5 weighted dimensions produce a 0–100 score:

| Dimension | Weight | Measures |
|-----------|--------|----------|
| Completeness | 0.25 | All required sections filled |
| Consistency | 0.25 | No contradictions between sections |
| Specificity | 0.20 | Concrete vs vague language |
| Feasibility | 0.15 | Technical viability signals |
| Testability | 0.15 | Acceptance criteria quality |

---

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   bd sync
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
