EXPLORATION: `chatpanel.go` was rendering all message content as plain wrapped text in `renderHistory()`, with no cached markdown renderer on `ChatPanel`. Existing markdown patterns in Gurgeh/Coldwine use glamour `TermRenderer`; `go.mod` already has `github.com/charmbracelet/glamour v0.10.0`, and Tokyo Night colors are defined in `pkg/tui/colors.go`.

CHANGES:
- `pkg/tui/chatpanel.go`: added `glamour` import.
- `pkg/tui/chatpanel.go`: added `mdRenderer *glamour.TermRenderer` and `mdWidth int` fields to `ChatPanel`.
- `pkg/tui/chatpanel.go`: added `markdownRenderer(width int) *glamour.TermRenderer` cache method.
- `pkg/tui/chatpanel.go`: replaced only the `renderHistory()` content-rendering block so `agent` messages render via glamour markdown with plain-text fallback; `user/system` remain plain wrapped text.

BUILD: PASS  
- `GOCACHE=/tmp/go-build-cache go build ./pkg/tui/...` passed.
- `GOCACHE=/tmp/go-build-cache go build ./cmd/...` passed.
- `GOCACHE=/tmp/go-build-cache go vet ./pkg/tui/...` failed with pre-existing warnings in `pkg/tui/loghandler.go:140` and `pkg/tui/loghandler.go:151` (lock-copy), unrelated to this change.

TESTS: PASS [59 passed, 0 failed]  
- `GOCACHE=/tmp/go-build-cache go test ./pkg/tui/... -v -short -count=1` passed.

VERDICT: NEEDS_ATTENTION [Code change is clean and scoped to `pkg/tui/chatpanel.go`, but repo has pre-existing `AGENTS.md` diff and `go vet` currently fails in `pkg/tui/loghandler.go` unrelated to this task.]