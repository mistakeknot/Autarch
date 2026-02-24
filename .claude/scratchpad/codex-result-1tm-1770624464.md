EXPLORATION: `ChatPanel` already had spinner/status rendering but no stream lifecycle; `chatstream.go` defines `ChatHandler`, `ChatState`, and stream events; `composer.go` provides the needed `Value/SetValue/Reset`. Local Bubble Tea patterns in `internal/tui/views/sprint_view.go` and `internal/tui/views/gurgeh_onboarding.go` confirm the one-event-per-`tea.Cmd` (`waitFor...`) channel-read loop.

CHANGES:
- `pkg/tui/chatpanel.go`: added streaming state management for non-slash input:
  - new fields (`handler`, `chatState`, `streamCtx`, `cancelStream`, `events`)
  - `SetHandler`
  - `StreamChunkMsg` + internal `streamStartedMsg`
  - `SubmitInput()` now starts stream when handler exists, appends user message, creates agent placeholder, starts spinner, and begins event loop
  - `Update()` now handles stream start/chunk messages
  - added `handleStreamChunk`, `cleanupStream`, `waitForStreamEvent`, and `CancelStream`

BUILD: PASS

VET: FAIL (existing unrelated warnings)
- `pkg/tui/loghandler.go:140`
- `pkg/tui/loghandler.go:151`
- issue: copying a type containing `sync.Mutex`

TESTS: PASS [59 passed, 0 failed]

VERDICT: NEEDS_ATTENTION [Core streaming integration is implemented and validated by build/tests, but `go vet` has pre-existing failures, and full `git diff --stat` includes pre-existing changes in `internal/gurgeh/exploration/explore.go` and `pkg/tui/composer.go`. Task edits were limited to `pkg/tui/chatpanel.go`.]