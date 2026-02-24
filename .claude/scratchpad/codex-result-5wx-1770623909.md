EXPLORATION: `pkg/tui/chatpanel.go` already had the history/composer layout and a clear insertion point before scrolling in `renderHistory()`. `ColorPrimary` is defined in `pkg/tui/colors.go`, `bubbles v0.21.0` is present in `go.mod`, spinner API supports `spinner.New()`, `Model.Tick`, `Model.Update`, `Model.View`, and there were no existing spinner/loading patterns in `pkg/tui`.

CHANGES:
- `pkg/tui/chatpanel.go`: added spinner import, `spinner/status/streaming` fields, spinner initialization in `NewChatPanel`, new public methods `SetStatus`, `IsStreaming`, `SpinnerTick`, spinner tick handling in `Update`, and status indicator rendering in `renderHistory()` (`pkg/tui/chatpanel.go:6`, `pkg/tui/chatpanel.go:46`, `pkg/tui/chatpanel.go:55`, `pkg/tui/chatpanel.go:84`, `pkg/tui/chatpanel.go:90`, `pkg/tui/chatpanel.go:95`, `pkg/tui/chatpanel.go:141`, `pkg/tui/chatpanel.go:348`).

BUILD: FAIL (both build commands passed, but `go vet ./pkg/tui/...` failed on pre-existing `copylocks` warnings in `pkg/tui/loghandler.go:140` and `pkg/tui/loghandler.go:151`)

TESTS: PASS [52 passed, 0 failed]

VERDICT: NEEDS_ATTENTION [implementation is in place and tests pass, but vet is not clean due unrelated existing issues; `git diff --stat` also shows pre-existing changes in `internal/gurgeh/exploration/explore.go` and `pkg/tui/composer.go`.]