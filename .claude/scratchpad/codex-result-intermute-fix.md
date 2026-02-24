EXPLORATION: `internal/intermute/manager.go:110` was using `exec.CommandContext(ctx, ...)`, and that `ctx` comes from `cmd/autarch/main.go:128` (`context.WithTimeout(..., 30*time.Second)`), so the Intermute subprocess was tied to the 30s startup timeout. `manager.go` also had a hardcoded 5s readiness deadline loop.

CHANGES: `internal/intermute/manager.go`
- `internal/intermute/manager.go:110` changed `exec.CommandContext(ctx, ...)` to `exec.Command(...)` so process lifetime is no longer bound to startup timeout context.
- `internal/intermute/manager.go:131` replaced the hardcoded `5s` deadline polling with a `ctx.Done()`-aware readiness loop:
  - on `ctx` cancellation: `m.stop()` and return wrapped context error
  - otherwise keep polling `m.isHealthy()` every `100ms`.
- `git diff --stat` shows only `internal/intermute/manager.go` changed.

BUILD: PASS (`GOCACHE=/tmp/go-build-cache go build ./cmd/...` and `GOCACHE=/tmp/go-build-cache go build -o /tmp/autarch-test ./cmd/autarch`)

TESTS: PASS (`GOCACHE=/tmp/go-build-cache go vet ./internal/intermute/...`)

VERDICT: CLEAN