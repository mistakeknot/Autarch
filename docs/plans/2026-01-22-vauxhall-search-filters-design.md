# Vauxhall Search + Status Filters (TUI First) — Design

**Date:** 2026-01-22

## Goal
Add a lightweight search bar with status tokens to the Vauxhall TUI so users can quickly filter sessions and agents. Keep the design compatible with a later web implementation.

## Scope
- TUI only (for now)
- Applies to Sessions and Agents tabs
- Dashboard remains unfiltered

## UX Summary
- Press `/` to focus the search input
- Type text + status tokens
- `esc` clears and exits search
- Filter line appears beneath the header: `Filter: !waiting codex`

### Token Syntax
- `!running`, `!waiting`, `!idle`, `!error` (case-insensitive)
- Multiple tokens are OR across statuses
- Text terms are AND against item fields

Example: `!waiting !error codex` matches waiting/error items that include “codex”.

## Architecture
Add `FilterState` to the TUI model:
- `Raw string`
- `Terms []string`
- `Statuses map[tmux.Status]bool` (or string set)

### Data Flow
- `filterInput` (textinput) updates `filterState` on each keystroke
- `updateLists()` builds full session/agent items, then applies `filterSessions` / `filterAgents`
- Agents derive a “status hint” from linked sessions; if none, treat as unknown

### Rendering
- `renderHeader()` stays the same
- Insert a `renderFilterLine()` directly below header when `filterState.Raw != ""`
- Respect small width (no panic, hide if too narrow)

## Error Handling
- Unknown tokens are treated as text
- Empty query clears filter
- Parsing is tolerant (no hard errors)

## Testing (TDD)
- `TestFilterParsesStatusTokens`
- `TestSessionFilterAppliesStatusAndText`
- `TestFilterClearsOnEscape`
- `TestFilterUIHiddenWhenEmpty`

## Open Questions
- Should we support `!unknown` for agents without sessions?
- Should filters be sticky across tabs or per-tab?

