# Tandemonium Init Epic/Story Generation Design

## Goal
Make `tandemonium init` an agent-assisted bootstrap that scans the repo, writes an exploration summary, and generates epics + stories with progress indicators.

## Decision Summary
- Use shared run-target registry; default agent is `claude`.
- `init` scans the whole repo and writes `.tandemonium/plan/exploration.md`.
- Generate epic specs in `.tandemonium/specs/EPIC-###.yaml` with story IDs `EPIC-###-S##`.
- `init` prompts for depth (1/2/3) unless `--depth` is set, then confirms before writing specs.
- Reruns prompt: skip/overwrite/prompt-per-epic. Non-interactive defaults to skip (`--existing`).
- Continuous scanning: `tandemonium scan` + TUI background loop every 15m and on new commits.

## Spec Schema (Detailed)
Epic file:
- `id`, `title`, `summary`
- `status`: `todo|in_progress|review|blocked|done`
- `priority`: `p0|p1|p2|p3`
- `acceptance_criteria`: list
- `risks`: list
- `estimates`: freeform (e.g., story points or hours)
- `stories`: list of story objects

Story object:
- `id` (format `EPIC-###-S##`)
- `title`, `summary`
- `status`, `priority`
- `acceptance_criteria`, `risks`, `estimates`

## Init Flow
1. Ensure `.tandemonium/` dirs exist.
2. Resolve agent via shared run-target registry (global + project); default `claude`.
3. Run exploration pipeline (docs, code, tests/CI, misc) and emit progress events.
4. Write exploration summary to `.tandemonium/plan/exploration.md`.
5. Build an agent prompt from exploration summary + repo metadata.
6. Request epics + stories; if agent fails, fall back to heuristic generator and note it.
7. Prompt to write specs (summary is saved to `.tandemonium/plan/exploration.md`).
8. On rerun, prompt for skip/overwrite/prompt-per-epic; non-interactive defaults to skip.

**Init Flags:**
- `--agent` (default `claude`)
- `--depth` (1-3, default 2 when prompted)
- `--existing` (`skip|overwrite|prompt`, default `skip`)
- `--tui` (optional progress UI)

## Continuous Scanning
- New command: `tandemonium scan` (on-demand).
- TUI background loop every 15m.
- Also trigger when `git rev-parse HEAD` changes (polled in the background).
- Scans update exploration summary and can propose new epics.

## Progress Indicators
- CLI: step-by-step updates (scan docs, scan code, generate prompt, write specs).
- Optional `--tui` progress view for init/scan.

## Config
Add `.tandemonium/config.toml`:

```toml
[init]
agent = "claude"
scan_interval_minutes = 15
scan_on_commit = true
existing_mode = "prompt"  # skip|overwrite|prompt
use_tui = false
```

## Risks
- Agent may not be promptable; should error with guidance.
- Large repos may require throttling or partial scanning.
- Non-interactive contexts must default safely to skip existing.

## Success Criteria
- `tandemonium init` creates exploration summary + epic specs (with user confirmation).
- Reruns behave according to selection and non-interactive defaults.
- Scan command + TUI loop update exploration summary and suggest epics.
