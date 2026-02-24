EXPLORATION: `pkg/claude/run.go` streams channel-based wire events (`EventText`, thinking start/delta/end, tool use, result/error), and `pkg/tui/chatpanel.go` uses status strings plus `IsStreaming()`/`SpinnerTick()` for UI streaming state; `pkg/tui/colors.go` is only shared palette constants. `pkg/tui/chatstream.go` was missing.

CHANGES:
- `pkg/tui/chatstream.go`: created new TUI-layer streaming abstraction with `StreamMsg` marker interface, concrete stream message types, `ChatState` + `String()`, and `ChatHandler`.

BUILD: PASS

TESTS: PASS [59 passed, 0 failed]

VERDICT: NEEDS_ATTENTION [Required `go vet ./pkg/tui/...` fails due pre-existing lock-copy warnings in `pkg/tui/loghandler.go:140` and `pkg/tui/loghandler.go:151`; `git diff --stat` also reflects unrelated existing worktree changes outside this bead.]