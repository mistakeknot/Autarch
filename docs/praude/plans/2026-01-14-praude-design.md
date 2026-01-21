# Praude Design

Date: 2026-01-14

## Goal
Build a TUI-first, PM-focused CLI app that generates, validates, and governs PRD
specs. Praude orchestrates external coding agents (Codex, Claude Code,
opencode, droid) to produce competitive/market research and high-quality PRDs,
and prevents drift from approved specs.

## Storage + IDs
- Hidden project root: `.praude/`
- Specs: one YAML per file in `.praude/specs/` (write `.yaml`, read `.yaml` or `.yml`)
- Research outputs: `.praude/research/PRD-###-<timestamp>.md`
- Briefs: `.praude/briefs/PRD-###-<timestamp>.md` (always written)
- IDs: incremental `PRD-###`, derived by scanning existing spec files
- One project per repo, so IDs are global within the repo

## Spec Schema (full parity with Tandemonium)
Praude owns the full spec schema: `strategic_context`, `user_story` (with hash),
`summary`, `requirements`, `acceptance_criteria`, `files_to_modify`, and
metadata (complexity, priority, estimate). The template spec created at init is
filled with example content to demonstrate a "good" PRD.

## TUI-first UX
Default `praude` launches a two-pane TUI:
- Left: spec list (ID, title, status, completeness, last updated)
- Right: spec details with completeness indicators
Top bar shows project + view + focus; bottom bar shows hotkeys + status.
Detail pane renders Markdown for readable PRD sections. Help and tutorial
overlays are built in (`?` and `` ` ``).

Core actions:
- `g` guided interview (create PRD)
- `r` create research artifact + launch agent
- `p` create suggestions artifact + launch agent
- `s` review/apply suggestions
- `j`/`k` navigate
- `Tab` toggle focus list/detail
- `?` help
- `` ` `` tutorial
- `q` quit

## Agent Orchestration
Praude does not "think"; it orchestrates agent CLIs by:
1) Generating a PM-focused brief file
2) Spawning the chosen agent with that brief
3) Parsing and applying agent output to spec sections

Agents are configured via `.praude/config.toml` profiles. Defaults are provided
for `codex`, `claude`, `opencode`, and `droid`, with empty args and commented
examples. If an agent command is missing, Praude falls back to printing the
brief path so the user can run manually.

## Drift Control
Praude computes a story hash from `user_story.text`. When changes occur:
- If drift is detected, the user can accept (Praude updates spec + auto-commit)
  or reject (Praude generates corrective guidance and reruns the agent).

## Git Auto-Commit
All spec writes (create/update) auto-commit with messages like:
`chore(praude): add PRD-001` or `chore(praude): update PRD-001`.

## CLI Surface
TUI-first, with CLI parity for scripting:
- `praude init`
- `praude list`
- `praude show <id>`
- `praude interview`
- `praude validate <id> --mode=hard|soft`
- `praude research <id> --agent=<profile>`
- `praude suggest <id> --agent=<profile>`
- `praude suggestions review <id>`
- `praude suggestions apply <id> [--all]`
- `praude run <brief> --agent=<profile>`

## Testing
Unit tests cover ID generation, spec validation, brief composition, drift
detection, and config/profile resolution. CLI integration tests run against
temp dirs. TUI snapshots cover list/detail rendering and key actions.
