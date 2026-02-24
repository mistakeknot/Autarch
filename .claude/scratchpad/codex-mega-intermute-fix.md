## Goal
Fix the Intermute server being killed after 30 seconds due to `exec.CommandContext` using a timeout context. The context's deadline fires and kills the subprocess even though the server should run for the entire TUI session.

## Phase 1: Explore
Read `internal/intermute/manager.go` — the entire file is ~196 lines. Focus on:
- The `start()` method (line 95-142) which uses `exec.CommandContext(ctx, ...)`
- The health-check polling loop (lines 130-137)
- Understand: ctx comes from `cmd/autarch/main.go` line 128 which creates a 30-second timeout

## Phase 2: Implement

### Fix in `internal/intermute/manager.go`, `start()` method:

The `ctx` parameter is a 30-second timeout context from the caller. Using it with `exec.CommandContext` means the server process gets killed when the timeout fires.

**Change the `exec.CommandContext` call to use `context.Background()` instead of `ctx`.** The context should still be used for the health-check deadline, but NOT for the process lifetime.

Specifically, change line 110 from:
```go
m.cmd = exec.CommandContext(ctx, binary, "serve",
```
to:
```go
m.cmd = exec.Command(binary, "serve",
```

That's it. `exec.Command` (without Context) means the process lives until explicitly stopped via `m.stop()`.

The `ctx` parameter is still useful for bounding the health-check wait. Keep using the context's deadline for the polling loop. Change the polling loop to respect ctx cancellation:

```go
// Wait for server to be ready
for {
    select {
    case <-ctx.Done():
        m.stop()
        return fmt.Errorf("intermute server did not become healthy: %w", ctx.Err())
    default:
    }
    if m.isHealthy() {
        return nil
    }
    time.Sleep(100 * time.Millisecond)
}
```

This replaces the current `deadline := time.Now().Add(5 * time.Second)` hardcoded deadline with the caller's context, which is more flexible. The 5-second hardcoded deadline is replaced by the caller's 30-second timeout, which is fine.

### Summary of changes:
1. `exec.CommandContext(ctx, binary, ...)` → `exec.Command(binary, ...)`
2. Replace the hardcoded 5-second deadline loop with a ctx-aware loop

**Only modify `internal/intermute/manager.go`. Do not modify any other file.**

## Phase 3: Verify
1. Build: `GOCACHE=/tmp/go-build-cache go build ./cmd/...`
2. Vet: `GOCACHE=/tmp/go-build-cache go vet ./internal/intermute/...`
3. Diff: `git diff --stat` (should only show manager.go)
4. Quick functional test: start the server, wait 2 seconds, check health, kill:
   ```bash
   GOCACHE=/tmp/go-build-cache go build -o /tmp/autarch-test ./cmd/autarch
   # Just verify it builds, don't actually run the TUI
   ```

## Final Report
```
EXPLORATION: [summary]
CHANGES: [files]
BUILD: PASS | FAIL
TESTS: PASS | FAIL
VERDICT: CLEAN | NEEDS_ATTENTION [reason]
```

## Constraints
- Only modify `internal/intermute/manager.go`
- Do NOT commit or push
- Do NOT modify any other file
- If GOCACHE permission errors occur, use: GOCACHE=/tmp/go-build-cache
- Module path is `github.com/mistakeknot/autarch`
