# PRD: Mechanical Batch — Context Guardrails, Pollard Tests, Intermute Background Startup

## Problem
Three well-understood technical debt items are blocking progress: orphaned subprocesses after TUI exit (context.Background), zero test coverage on Pollard YAML persistence, and synchronous Intermute startup blocking TUI init.

## Solution
Dispatch 3 independent Codex agents in parallel. Each agent handles one self-contained fix-and-test task with no shared state.

## Features

### F1: Context cancellation guardrails (Autarch-i0u)
**What:** Replace `context.Background()` with `context.WithCancel` in 7 HIGH RISK TUI goroutine sites so subprocesses cancel on TUI exit.
**Acceptance criteria:**
- [ ] 7 HIGH RISK sites converted: sprint_view.go:90,112,476 + gurgeh_onboarding.go:89 + arbiter_view.go:148,156,264
- [ ] Each site stores cancel func and calls it on view cleanup
- [ ] Pattern matches existing correct implementation at sprint_view.go:436
- [ ] Tests verify goroutines exit when context is cancelled
- [ ] `go test ./internal/tui/... -run TestCancel -v -short` passes
- [ ] `go test ./internal/gurgeh/... -short` passes (no regressions)

### F2: Pollard YAML persistence tests (Autarch-6eg)
**What:** Write comprehensive tests for Pollard's YAML read/write layer, which currently has zero tests.
**Acceptance criteria:**
- [ ] Table-driven tests for YAML serialization round-trips
- [ ] Edge cases: empty fields, unicode content, missing files, malformed YAML
- [ ] Test coverage for all public YAML persistence functions
- [ ] `go test ./internal/pollard/... -v -short` passes
- [ ] Test files follow Go conventions (_test.go in same package)

### F3: Intermute background startup (Autarch-wq8)
**What:** Move `EnsureRunning` from synchronous blocking call to background `tea.Cmd` so TUI doesn't freeze during Intermute startup.
**Acceptance criteria:**
- [ ] `EnsureRunning` wrapped in `tea.Cmd` that runs asynchronously
- [ ] Result message (`IntermutStartedMsg` / `IntermuteStartFailedMsg`) sent back to model
- [ ] TUI remains responsive during startup
- [ ] Callers updated to handle the new async pattern
- [ ] `go test ./pkg/intermute/... -v -short` passes
- [ ] No regressions in TUI startup flow

## Non-goals
- Not addressing the 6 MEDIUM RISK `context.WithTimeout(context.TODO())` sites (future iteration)
- Not adding Pollard integration tests (only unit tests for YAML persistence)
- Not changing Intermute's actual server startup logic (only the TUI integration)

## Dependencies
- Existing correct pattern at sprint_view.go:436 (F1 reference)
- flux-drive audit at docs/research/flux-drive/Autarch-e2e/ (F1 file list)
- Codex CLI installed and configured (`~/.codex/config.toml`)

## Open Questions
None — all tasks are well-understood from prior analysis.
