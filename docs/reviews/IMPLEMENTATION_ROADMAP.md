# FrankenTUI Inline Logging - Implementation Roadmap

**Estimated Total Time:** 10-15 hours
**Start Date:** TBD
**Status:** Ready for implementation

---

## Phase 1: Foundation (1-2 hours)

### Create Core Types and Interfaces

**File: `/root/projects/Autarch/pkg/tui/log.go`**

- [ ] Define `MessageSender` interface
- [ ] Define `LogMsg` struct with all fields
- [ ] Implement `TUIHandler` with:
  - [ ] Mutex-protected sender field
  - [ ] `SetSender()` atomic update
  - [ ] `Handle()` with pointer copy pattern
  - [ ] `WithAttrs()` and `WithGroup()` no-ops
  - [ ] `Enabled()` return true
- [ ] Add comprehensive godoc comments

**Checklist for Review:**
```
- TUIHandler compiles without errors
- MessageSender interface is minimal (one method)
- LogMsg carries all required fields (Level, Message, Timestamp, Attrs)
- Mutex only protects pointer (not the Send operation)
- Tests can mock MessageSender interface
```

---

## Phase 2: Components (4-6 hours)

### Create LogPane Buffer

**File: `/root/projects/Autarch/pkg/tui/log_pane.go`**

- [ ] Implement circular buffer with:
  - [ ] Pre-allocated 500-entry slice
  - [ ] Wraparound logic (modulo arithmetic)
  - [ ] Entry count tracking
  - [ ] O(1) append performance
- [ ] Integrate viewport for scrolling
- [ ] Implement `formatEntry()` for display
- [ ] Add `Entries()` snapshot method
- [ ] Add `Clear()` method

**Test Checklist:**
```go
- Append 600 items, verify 500 retained
- Verify oldest entries overwritten
- Verify View() renders all entries
- No nil pointer panics
- Entries() returns correct order
```

### Create TerminalWriter

**File: `/root/projects/Autarch/pkg/tui/terminal_writer.go`**

- [ ] Mutex-protected writer
- [ ] `Write()` method
- [ ] `WriteString()` convenience method
- [ ] Serialize all output access

**Test Checklist:**
```go
- Concurrent Write() calls don't corrupt output
- No data loss or reordering
- Mutex contention minimal (verify with profiling)
```

### Wire Into App

**File: `/root/projects/Autarch/internal/tui/app.go`**

- [ ] Add `logPane *pkgtui.LogPane` field to App
- [ ] Add `showLogPane bool` flag to App
- [ ] Route `LogMsg` in `Update()` to logPane
- [ ] Render logPane in `View()` (pane or overlay)
- [ ] Initialize logPane in `NewUnifiedApp()`

**Test Checklist:**
```go
- LogMsg routes to logPane.Update()
- logPane content appears in app.View()
- showLogPane flag controls visibility
- No panics with nil logPane
```

---

## Phase 3: Integration (2-3 hours)

### Main Setup

**File: `/root/projects/Autarch/cmd/autarch/main.go`**

- [ ] Add `--inline` flag to `tuiCmd()`
- [ ] Implement `setupInlineLogging()` function with:
  - [ ] Create TUIHandler with nil sender
  - [ ] Configure slog.SetDefault()
  - [ ] Redirect log.SetOutput()
  - [ ] Store handler reference (global or context)
- [ ] Modify `RunUnified()` to:
  - [ ] Call `handler.SetSender(program)` before Run()
  - [ ] Wrap with panic recovery defer
  - [ ] Set `app.ShowLogPane = true` if inline

- [ ] Modify app initialization to optionally enable logPane

**Test Checklist:**
```bash
./autarch tui --inline
# Verify: Log pane appears in UI
# Verify: slog.Info() calls appear in pane
# Verify: No panics during setup

./autarch tui
# Verify: Works without --inline flag
# Verify: Normal logging behavior unchanged
```

### Panic Recovery

**Location:** `internal/tui/run.go` or `RunUnified()` in app.go

```go
defer func() {
    if r := recover(); r != nil {
        fmt.Fprintf(os.Stderr, "Fatal: %v\n", r)
        os.Exit(1)
    }
}()
```

**Test Checklist:**
- [ ] Panic in Update doesn't kill process ungracefully
- [ ] Terminal is restored (Bubble Tea already does this)
- [ ] Stack trace written to stderr

---

## Phase 4: Testing (2-3 hours)

### Unit Tests

**File: `/root/projects/Autarch/pkg/tui/log_test.go`**

- [ ] TestTUIHandlerThreadSafety
  - 10 goroutines × 100 logs each
  - Verify all 1000 reach sender
  - Run with `-race` flag

- [ ] TestTUIHandlerGracefulDegradation
  - Nil sender doesn't panic
  - Logs silently dropped

- [ ] TestLogPaneCircularBuffer
  - 600 appends, 500 retained
  - Oldest entries discarded

- [ ] TestTerminalWriterMutualExclusion
  - Concurrent writes
  - Output not corrupted

- [ ] TestLogMsgFormatting
  - All fields preserved
  - Attributes included

### Integration Tests

**File: `/root/projects/Autarch/internal/tui/app_test.go`**

- [ ] TestInlineLoggingEndToEnd
  - Setup app with inline flag
  - Emit logs from multiple sources
  - Verify appear in logPane
  - Verify no race conditions

- [ ] TestLoggingWithoutInlineFlag
  - Normal operation unchanged
  - Logs don't crash app

### Load Tests

**File: `/root/projects/Autarch/pkg/tui/benchmark_test.go`**

```go
func BenchmarkHandleHighVolume(b *testing.B) {
    // 10k logs/sec for 10 seconds
    // Measure: throughput, latency percentiles
}
```

**Verify:**
- [ ] <1ms p99 latency per log
- [ ] <5% CPU under sustained load
- [ ] No memory leaks (check with pprof)

---

## Phase 5: Documentation & Polish (1-2 hours)

### Update Architecture Docs

**File: `/root/projects/Autarch/docs/ARCHITECTURE.md`**

- [ ] Add logging subsystem section
- [ ] Document data flow: slog → TUIHandler → LogMsg → app.Update → logPane
- [ ] Include diagram

### Update AGENTS.md

**File: `/root/projects/Autarch/AGENTS.md`**

- [ ] Add inline logging to quick reference
- [ ] Document --inline flag
- [ ] Link to review documents

### Add Inline Help

**File: `cmd/autarch/main.go` flag help**

```go
cmd.Flags().BoolVar(&inline, "inline", false,
    "Display structured logs in a TUI pane (experimental)")
```

### Code Comments

- [ ] Add godoc to all public types
- [ ] Document thread-safety assumptions
- [ ] Mark LogPane as single-threaded

---

## Phase 6: Review & Merge (1 hour)

### Pre-Merge Checklist

- [ ] All tests pass: `go test ./...`
- [ ] No race conditions: `go test -race ./...`
- [ ] Lint passes: `golangci-lint run`
- [ ] Benchmark baseline established
- [ ] Code reviewed by 1+ team member
- [ ] Integration test passes on target branch

### Commit Strategy

**Commit 1:** Foundation
```
feat(tui): add inline logging infrastructure

- Add MessageSender interface for abstraction
- Implement TUIHandler with thread-safe mutex pattern
- Implement LogMsg with structured logging support
- Implement TerminalWriter for coordinated stdout access
```

**Commit 2:** Components
```
feat(tui): implement LogPane circular buffer and integration

- Add LogPane with 500-entry circular buffer
- Integrate LogPane into App.Update/View
- Add --inline flag to autarch tui command
- Wire TUIHandler setup in main
```

**Commit 3:** Tests & Docs
```
test(tui): add comprehensive inline logging tests

docs(tui): document inline logging subsystem
```

---

## Common Pitfalls & Mitigations

### Pitfall 1: Program Initialization Order

**Problem:** slog configured before program exists

**Solution:** TUIHandler created with nil sender, wired later via SetSender()

**Verification:**
```bash
./autarch tui --inline
# Logs before program ready should be silently dropped
# No panics or crashes
```

---

### Pitfall 2: Race Condition in Handler

**Problem:** Program pointer reassigned during lifecycle

**Solution:** Mutex protects pointer, copy before use

**Verification:**
```bash
go test -race ./... # Run all tests with race detector
# Should report no races
```

---

### Pitfall 3: Message Overflow

**Problem:** Program.Send() buffer fills up

**Solution:** Default 256-msg buffer is sufficient for logging

**Verification:** Profile under load, measure queue depth

---

### Pitfall 4: Stdout Interleaving

**Problem:** External libraries write directly to stdout

**Solution:** Centralized TerminalWriter with mutex

**Verification:**
```bash
./autarch tui --inline
# Generate background task (uses stdout)
# Verify output not interleaved with TUI
```

---

### Pitfall 5: Panic Crashes App

**Problem:** Crash in LogPane.Update() kills entire app

**Solution:** Defer recovery at entry point

**Verification:**
```bash
# Force panic in log_pane.go temporarily
# Verify: graceful exit, terminal restored
```

---

## Success Criteria

After Phase 6, all of these must be true:

- [ ] `autarch tui --inline` starts without errors
- [ ] Logs appear in TUI pane within 100ms
- [ ] TUI remains responsive under 10k logs/sec
- [ ] No race condition detector warnings
- [ ] All unit tests pass
- [ ] Load test completes without memory leaks
- [ ] Panic recovery works (tested manually)
- [ ] Flag disabled (normal mode) works unchanged
- [ ] Documentation updated
- [ ] Code review approved

---

## Rollback Plan

If issues emerge:

1. **Minor bugs:** Fix in place, re-test
2. **Architecture issues:** Revert to commit before "Foundation"
3. **Panic crashes:** Disable --inline flag, investigate separately

Rollback command:
```bash
git revert --no-edit <commit-hash>
```

---

## Performance Targets

After implementation, measure:

| Metric | Target | Tool |
|--------|--------|------|
| Log latency (p99) | <1ms | `go test -bench` |
| CPU under load | <5% | `pprof cpu` |
| Memory (fixed) | <100KB | `pprof heap` |
| TUI render time | <50ms | Wall clock timing |
| Message throughput | >10k/sec | Benchmarks |

---

## Sign-Off

Once complete, document in git commit:

```
This implementation:
✓ Follows Autarch architectural patterns
✓ Passes all SOLID principle checks
✓ No race conditions (verified with -race flag)
✓ Matches approved architecture review
✓ All critical requirements implemented
✓ 15+ hours effort expended
✓ Load tested and profiled

Ready for production.
```

---

## Related Resources

- **Architecture Review:** `/root/projects/Autarch/docs/reviews/inline-logging-architecture-review.md`
- **Quick Reference:** `/root/projects/Autarch/docs/reviews/inline-logging-quick-reference.md`
- **Code Patterns:** `/root/projects/Autarch/docs/reviews/inline-logging-code-patterns.md`
- **System Architecture:** `/root/projects/Autarch/docs/ARCHITECTURE.md`
- **Bubble Tea Docs:** https://github.com/charmbracelet/bubbletea

