# Code Conventions

## General

- Use `internal/` for tool-specific, `pkg/` for shared code
- Error handling: `fmt.Errorf("context: %w", err)`
- Logging: `log/slog` with structured fields
- No external dependencies for core functionality
- SQLite: read-only connections to external DBs
- Run tests with `-race` flag

## Testing
- TDD for behavior changes
- Small unit tests over broad integration tests
- Run targeted tests: `go test ./internal/<pkg> -v`

## Concurrency Rules

- Never return pointers to internal mutable state from synchronized methods. `State()` returns deep-copied snapshots via `Clone()`.
- All types crossing goroutine boundaries need `Clone()` methods.
- Bubble Tea threading model: `Update()` and `View()` always run on the same goroutine — pointer fields shared across Model value copies are safe without mutexes.

## TUI Design Principles

**Chat-focused TUI:** All TUIs are built around a chat composer with a 50/50 split layout. Keybindings use `Ctrl+` combinations — no single-letter shortcuts during text entry. Discoverability via slash commands.

**Chat-first editing:** No "edit mode" — users refine content by chatting with the agent. This keeps the TUI conversation-centric.

- Shell layout owns focus state — use `shell.Focus()` not `component.Focused()` for routing decisions
- Use `msg.String()` for key matching instead of `key.Matches()` for raw key events
- Avoid Ctrl+J (= LF/Enter in terminals)
- Use phase constants, not integer literals, for array indices
- Non-fatal errors log to stderr with `warning:` prefix; never silently swallow errors

## Debugging

**Before debugging, check solutions:**
```bash
ls docs/solutions/
grep -r "error message" docs/solutions/
grep -l "tags:.*bubble-tea" docs/solutions/**/*.md
```

| Category | Purpose |
|----------|---------|
| `runtime-errors/` | Concurrency, panics, crashes |
| `ui-bugs/` | TUI rendering, message routing |
| `patterns/` | Architecture, design decisions |
| `workflow-issues/` | Process, methodology learnings |
| `integration/` | Cross-tool, API issues |

**After fixing a non-trivial bug**, run `/clavain:compound` to capture the solution.
