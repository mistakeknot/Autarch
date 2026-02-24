EXPLORATION: `runClaude()` was only used inside `internal/gurgeh/exploration/explore.go` (2 call sites + definition), module path is `github.com/mistakeknot/autarch`, `ChatMessage` is a simple `{Role, Content}` struct in `pkg/tui/chatpanel.go`, and `pkg/claude/` did not exist before this change.

CHANGES:
- `pkg/claude/run.go` (new): added shared `claude` runner with:
  - `EventType` + `StreamEvent`
  - `RunStreaming(ctx, cwd, args)` with stream-json enforcement, JSONL parsing, event emission (`session_id`, text/thinking/tool_use/result/error), scanner 1MB buffer, and process wait/error handling
  - `Run(ctx, cwd, args)` wrapper returning final result/error
  - internal raw JSON types: `rawMessage`, `rawContent`, `rawBlock`
- `internal/gurgeh/exploration/explore.go`:
  - imported `github.com/mistakeknot/autarch/pkg/claude`
  - changed `runClaude()` to thin wrapper:
    - `return claude.Run(ctx, cwd, args)`
  - removed top-level local stream types (`streamMessage`, `streamContent`, `contentBlock`)
  - replaced remaining `streamMessage` usages in other functions with inline local JSON structs (no functional change to those call paths)

BUILD: PASS

TESTS: PASS [1 passed, 0 failed, 1 skipped; `pkg/claude` has no test files]

VERDICT: NEEDS_ATTENTION [Your required `git diff --stat` includes pre-existing unrelated workspace changes in `AGENTS.md` and `pkg/tui/chatpanel.go`; my scoped changes are only `internal/gurgeh/exploration/explore.go` plus new `pkg/claude/run.go`.]