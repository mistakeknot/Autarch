# Praude Archive/Delete/Undo Design

## Goal
Add archive/delete/undo for PRDs (and their artifacts) in both TUI and CLI. Archive should move items into `.praude/archived/` and mark status `archived`. Delete should move items into `.praude/trash/` and be undoable. Archived items are hidden by default in the list but can be toggled visible.

## Behavior Summary
- **Archive**: move PRD YAML + related artifacts (research, suggestions, briefs) into `.praude/archived/{specs,research,suggestions,briefs}/`. Also set `status: archived` in the PRD YAML.
- **Delete**: move PRD YAML + related artifacts into `.praude/trash/{specs,research,suggestions,briefs}/`.
- **Undo**: restores the most recent archive/delete action (persisted in `.praude/state.json`).
- **Visibility**: archived items hidden by default; toggle with `h` in TUI or `praude list --include-archived` in CLI.
- **Keybindings**: `a` archive, `d` delete (confirm), `u` undo, `h` toggle archived visibility.
- **Confirmation**: always confirm archive/delete in TUI.

## Data Model
- Extend `.praude/state.json` to include:
  - `show_archived` (bool)
  - `last_action` object with type (`archive`/`delete`), PRD id, from/to paths, timestamp

## CLI
- `praude archive <id>`
- `praude delete <id>`
- `praude undo`
- `praude list --include-archived`

## TUI
- Actions available in both list and detail focus.
- Confirmation modal overlays for archive/delete.
- Undo confirmation modal if last action exists.

## Error Handling
- Missing PRD id → show user-friendly error, no partial moves.
- Partial move failures → attempt rollback and surface error.
- Undo fails if files no longer exist at expected paths.

## Testing
- Unit tests for move/undo logic and path mapping.
- TUI tests for key handling and confirmation flow.
- CLI tests for archive/delete/undo and list include-archived flag.
