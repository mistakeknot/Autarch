# Coding standards (reviewer-only)

Read by the review sub-agent when it reviews a Go change. The implementing agent is not pointed here: this is a pilot to see whether judgement rules work better as a review check than as always-loaded instructions. Deterministic checks (for example `rune_width_guard_test.go`) run in CI and need no reading.

Review a change against these. A violation is a defect to report, not a style note.

## Concurrency

- Never return pointers to internal mutable state from synchronized methods. `State()` returns deep-copied snapshots via `Clone()`.
- Every type that crosses a goroutine boundary needs a `Clone()` method.

## Bubble Tea message handling

- In a parent `Update()`, never swallow a message a child view needs. Default to fall-through; return early only for messages the parent exclusively owns. Error messages must always reach the view layer.

## Bubble Tea sizing and text

- Subtract chrome dimensions (header, footer, sidebar, padding) from `WindowSizeMsg` before passing it to child views. A child knows only its allocated space: available width = terminal width minus the parent's horizontal padding.
- Never slice an ANSI-styled string with `[]rune` for visual-column work. Use `ansi.Truncate` / `TruncateLeft` from `charmbracelet/x/ansi`. A `[]rune(...)` conversion beside `lipgloss.Width` in one file is always a bug; `rune_width_guard_test.go` fails on it.
