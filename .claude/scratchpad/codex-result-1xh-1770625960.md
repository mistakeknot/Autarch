EXPLORATION: `pkg/claude/run.go` already emits `EventSessionID` from `system/init`, but all chat handlers dropped it and emitted `StreamDone` without session data. `ChatPanel` forwarded slash commands upstream with no local `/new` handling, and `GlobalCommands()` had no chat reset command.

CHANGES:
- `pkg/tui/chatstream.go`: added `SessionID string` to `StreamDone`.
- `pkg/tui/claude_handler.go`: added continuation state (`Continue`, `SessionID`), prepended `-c`/`--resume <session>` before `ExtraArgs`, captured `EventSessionID`, emitted `StreamDone{SessionID: ...}`, added `SetContinue`/`ResetSession`.
- `internal/tui/views/gurgeh_chat_handler.go`: added mutex-protected continuation state, `-c`/`--resume` arg logic, session capture -> `StreamDone.SessionID`, and `SetContinue`/`ResetSession`.
- `internal/tui/views/bigend_chat_handler.go`: same continuation/session updates as above.
- `internal/tui/views/coldwine_chat_handler.go`: same continuation/session updates as above.
- `internal/tui/views/pollard_chat_handler.go`: same continuation/session updates as above.
- `pkg/tui/chatpanel.go`: added `MultiTurnHandler` interface, added `ResetSession()`, on `StreamDone` now calls `SetContinue(true, sessionID)` when present, and `SubmitInput()` now handles `/new` and `/clear` locally.
- `pkg/tui/command_picker.go`: added global command `{Command: "new", Aliases: []string{"clear"}, Description: "New conversation", Category: "chat"}`.

BUILD: PASS

TESTS: PASS [52 passed, 0 failed]

VERDICT: NEEDS_ATTENTION [requested feature changes are implemented and validated by build/tests; `go vet` fails due pre-existing warnings in `pkg/tui/loghandler.go:140` and `pkg/tui/loghandler.go:151` (`copylocks`), outside the touched scope.]