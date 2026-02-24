---
agent: fd-performance
tier: 1
issues:
  - id: P1-1
    severity: P1
    section: "TUI Rendering"
    title: "lipgloss.NewStyle() allocated on every render frame"
  - id: P1-2
    severity: P1
    section: "Signal Broker"
    title: "Publish holds mutex during full subscriber fanout"
  - id: P2-1
    severity: P2
    section: "TUI Rendering"
    title: "Shell separator rebuilt from scratch every frame"
  - id: P2-2
    severity: P2
    section: "Insights API"
    title: "LoadAll reads and parses every YAML file from disk on each request"
  - id: P2-3
    severity: P2
    section: "Signal Broker"
    title: "Subscription.Stream blocks on unbuffered out channel"
  - id: P2-4
    severity: P2
    section: "SQLite"
    title: "GetStats issues 5 sequential single-row queries"
  - id: P3-1
    severity: P3
    section: "HTTP Clients"
    title: "Fetcher creates a new http.Client per NewFetcher call"
  - id: P3-2
    severity: P3
    section: "Rate Limiter"
    title: "Token bucket arithmetic truncates to zero for sub-period intervals"
  - id: P3-3
    severity: P3
    section: "Log Pane"
    title: "updateContent rebuilds entire string on every batch"
improvements:
  - id: IMP-1
    title: "Hoist lipgloss styles to package-level variables"
    section: "TUI Rendering"
  - id: IMP-2
    title: "Use RWMutex in Broker for read-heavy subscribe/unsubscribe"
    section: "Signal Broker"
  - id: IMP-3
    title: "Cache insights on the Pollard server with file-mod-time invalidation"
    section: "Insights API"
  - id: IMP-4
    title: "Consolidate GetStats into a single SQL query"
    section: "SQLite"
  - id: IMP-5
    title: "Share a single http.Client with connection pooling across hunters"
    section: "HTTP Clients"
verdict: needs-changes
---

## Summary

Autarch is a Go 1.24 monorepo combining a Bubble Tea TUI (interactive, targeting 60fps), local REST/WebSocket servers, SQLite state, and HTTP-based research hunters. The primary performance-sensitive surface is the TUI render loop -- every `View()` call must complete in under ~16ms to maintain smooth interaction. Secondary concerns are the signal broker fanout latency (real-time events), HTTP hunter throughput (rate-limited external APIs), and SQLite access patterns (single-writer WAL mode). Overall the codebase is structurally sound -- WAL mode is correctly configured via a shared helper, hunters use semaphore-bounded concurrency, the log handler properly batches messages, and the scan cache implements in-flight deduplication. However, there are several concrete issues on the render hot path and in the broker that warrant attention.

## Section-by-Section Review

### 1. TUI Rendering Performance

**Hot path**: `App.View()` / `UnifiedApp.View()` -> `ShellLayout.Render()` -> `SplitLayout.renderHorizontal()` -> per-line `padToWidth()` with `ansi.StringWidth()`.

The render pipeline is called on every Bubble Tea frame. The biggest concern is allocation pressure from `lipgloss.NewStyle()` calls inside render methods. Across `pkg/tui/` and `internal/tui/`, there are **84 + 135 = 219 occurrences** of `lipgloss.NewStyle()`. Many of these live inside `View()`, `renderHistory()`, `renderFooter()`, `renderPanel()`, and `renderHorizontal()` methods, meaning they allocate on every frame.

Specific hot-path allocations found:

- `/root/projects/Autarch/pkg/tui/shelllayout.go` lines 187-189: `Render()` builds the separator string from scratch every frame using `lipgloss.NewStyle()`, `strings.Repeat()`, and string concatenation.
- `/root/projects/Autarch/pkg/tui/chatpanel.go` lines 182-184: `View()` allocates separator style every frame.
- `/root/projects/Autarch/pkg/tui/chatpanel.go` lines 248-250: `renderHistory()` allocates `contentStyle` per message per frame.
- `/root/projects/Autarch/pkg/tui/chatpanel.go` lines 287-303: `roleStyle()` allocates a new style per message per frame.
- `/root/projects/Autarch/pkg/tui/splitlayout.go` lines 109-111: `renderHorizontal()` allocates separator style every frame.
- `/root/projects/Autarch/pkg/tui/logpane.go` line 96: `formatEntry()` allocates a new style per entry per `updateContent()` call.

The `padToWidth()` function at `/root/projects/Autarch/pkg/tui/splitlayout.go` line 174 calls `ansi.StringWidth()` which parses ANSI escape sequences. This runs once per line per frame (height lines for left pane + height lines for right pane). For a typical 50-line terminal, that is ~100 ANSI parse operations per frame. This is acceptable but worth noting as the dominant per-frame CPU cost.

The `ensureSize()` function at `/root/projects/Autarch/pkg/tui/splitlayout.go` line 152 splits content into lines, adjusts count, then re-joins. This runs twice per frame (left + right). The intermediate slice allocation is fine for typical content sizes.

**Overlay rendering** at `/root/projects/Autarch/internal/tui/app.go` lines 353-379: The `overlay()` method splits the full screen into lines, inserts overlay text using ANSI-aware truncation, then re-joins. This only runs when the palette is visible, so it is not a hot-path concern.

### 2. Signal Broker

**File**: `/root/projects/Autarch/pkg/signals/broker.go`

The `Publish()` method at line 44 holds `sync.Mutex` for the entire duration of subscriber iteration. With N subscribers, this means:
- Subscribe/Unsubscribe calls block for the entire fanout duration.
- If any subscriber channel is full, the eviction logic (drain + re-send at lines 56-64) extends the critical section.
- Concurrent publishers serialize completely.

The channel buffer size of 64 (line 33) is reasonable for the expected signal rates (competitor updates, assumption decay -- low frequency). However, the `Stream()` method at line 114 writes to `out chan<- Signal` without a select on context or non-blocking send. If the caller passes an unbuffered or slow channel, `Stream()` blocks while holding the subscriber's channel read, which means the subscriber's buffer fills up and eventually triggers drops in `Publish()`.

The `Dropped` counter (atomic.Int64) is good -- it provides observability into backpressure.

### 3. SQLite Patterns

**File**: `/root/projects/Autarch/pkg/db/open.go`

The shared `Open()` helper correctly configures:
- WAL journal mode (concurrent readers)
- NORMAL synchronous (good balance of durability vs speed)
- 5-second busy timeout (prevents immediate SQLITE_BUSY failures)
- `SetMaxOpenConns(1)` (correct for single-writer pattern)

This is solid. The single-connection limit means all writes serialize through one connection, which is the correct pattern for SQLite.

**File**: `/root/projects/Autarch/internal/pollard/state/db.go`

`GetStats()` at line 242 issues 5 separate `QueryRow` calls to get aggregate statistics:
```go
SELECT COUNT(*) FROM hunter_runs
SELECT COUNT(*) FROM hunter_runs WHERE status = 'success'
SELECT COUNT(*) FROM hunter_runs WHERE status = 'failed'
SELECT COALESCE(SUM(sources_collected), 0) FROM hunter_runs
SELECT MAX(started_at) FROM hunter_runs
```
Each one acquires and releases the single database connection. This is 5 round-trips to SQLite when a single query with `CASE WHEN` expressions could return all values at once. The hunter_runs table is small (dozens to hundreds of rows), so the absolute time is negligible (<1ms total), but the pattern is wasteful and would scale poorly if called frequently.

**File**: `/root/projects/Autarch/pkg/events/store.go`

The `Query()` method at line 139 builds SQL with dynamic `WHERE` clauses and string concatenation. This is correct (parameterized with `?` placeholders, no SQL injection risk). The query uses `ORDER BY id ASC` which benefits from the primary key index. The schema has appropriate indexes on `event_type`, `entity_type+entity_id`, `source_tool`, `created_at`, and `project_path`. No N+1 patterns detected. The `Replay()` method streams results through a callback, avoiding loading all events into memory.

### 4. HTTP Client Performance

**File**: `/root/projects/Autarch/internal/pollard/pipeline/fetcher.go`

`NewFetcher()` at line 27 creates a new `http.Client` with a 30-second timeout. Each hunter also creates its own `http.Client` (e.g., GitHubScout at `/root/projects/Autarch/internal/pollard/hunters/github.go` line 34). Multiple `http.Client` instances mean separate connection pools, which prevents HTTP/2 connection reuse to the same host across different pipeline stages.

The `FetchURL()` method at line 248 uses `io.LimitReader(resp.Body, 1024*1024)` (1MB limit), which correctly prevents unbounded memory consumption from large responses.

The `FetchBatch()` method at line 36 uses a semaphore pattern with goroutines, properly respecting context cancellation. The default parallelism of 5 is reasonable for external API access.

### 5. Rate Limiting

**File**: `/root/projects/Autarch/internal/pollard/hunters/hunter.go`

The `RateLimiter.Wait()` at line 128 has a subtle arithmetic issue:

```go
tokensToAdd := int(elapsed / r.perDuration * time.Duration(r.requests))
```

When `elapsed < perDuration` (which is the common case -- checking rate limit mid-window), `elapsed / r.perDuration` truncates to 0 in integer division because `time.Duration` division returns `time.Duration`. Then `0 * anything = 0`, so tokens never refill until a full period has elapsed. This means the rate limiter effectively becomes a "burst N requests, then wait full period" limiter rather than a smooth token bucket. For the GitHub API (10 or 30 requests/minute), this is functionally acceptable but the behavior differs from what a developer reading "rate limiter" would expect.

The GitHubScout also checks HTTP 403/429 response codes and the `X-RateLimit-Reset` header (line 280-286), providing a second layer of rate-limit protection.

### 6. Insights Loading

**File**: `/root/projects/Autarch/internal/pollard/insights/insight.go`

`LoadAll()` at line 105 reads every `.yaml` file in the insights directory, parses each one with `yaml.Unmarshal`, and returns them all in memory. The Pollard server's `handleInsights()` at `/root/projects/Autarch/internal/pollard/server/server.go` line 232 calls `LoadAll()` on every HTTP GET request, then sorts and paginates the result. Even with the scan cache in the server, the insights endpoint bypasses the cache entirely (it does not use `GetOrCompute`).

For a project with dozens of insights, this is fine. For a project with hundreds of insights accumulated over months of continuous watching, this becomes a noticeable delay (disk I/O + YAML parsing per request).

### 7. Server Cache

**File**: `/root/projects/Autarch/internal/pollard/server/cache.go`

The `ScanCache` implementation is well-designed:
- LRU eviction with configurable max entries (512 default)
- TTL per entry
- In-flight deduplication via `GetOrCompute()` -- concurrent requests for the same key block on the first computation instead of triggering redundant scans
- The `hashKey()` function uses SHA-256 of JSON-marshaled request, which is correct for cache key generation

The mutex is held during `GetOrCompute` only for the inflight registration (not the computation itself), which is correct.

### 8. Job Store

**File**: `/root/projects/Autarch/internal/pollard/server/jobs.go`

The job store is backed by a shared `pkg/jobs` package with TTL-based expiration and max entry limits (20000 jobs, 24h TTL). This prevents unbounded memory growth from accumulated job records.

### 9. Log Handler Batching

**File**: `/root/projects/Autarch/pkg/tui/loghandler.go`

The `LogHandler` batches log messages into groups of 10 or on a 100ms tick, whichever comes first. This is a good pattern that prevents individual slog calls from triggering full re-renders. The 256-entry channel buffer (line 44) with non-blocking enqueue (drop on overflow, line 73) prevents log producers from blocking the main thread.

The `LogPane.updateContent()` at `/root/projects/Autarch/pkg/tui/logpane.go` line 70 rebuilds the entire formatted string from all entries (up to 500) every time a batch arrives. With 500 entries, each batch triggers ~500 `formatEntry()` calls which each allocate a `lipgloss.NewStyle()`. This is not on the main render frame path (it only runs when a log batch message arrives, at most 10x/second), but it could be made incremental.

### 10. Watcher

**File**: `/root/projects/Autarch/internal/pollard/watch/watcher.go`

The continuous watch loop correctly uses `time.NewTicker` with proper cleanup via `defer ticker.Stop()`. The `RunOnce()` method loads the previous snapshot from disk, diffs, and saves the new snapshot. File I/O is minimal (one JSON read, one JSON write per cycle). Watch intervals default to 24 hours, so this is not performance-sensitive.

### 11. Synthesizer

**File**: `/root/projects/Autarch/internal/pollard/pipeline/synthesizer.go`

The synthesizer spawns external processes (Claude, etc.) via `exec.CommandContext`. Each synthesis call is bounded by a 2-minute timeout, and parallelism is capped at 3 by default. This is appropriate -- the external LLM call dominates latency (seconds to minutes), so internal overhead is irrelevant. Content is truncated to 2000 characters (line 197) before sending to the agent, preventing excessively large prompts.

## Issues Found

### P1-1: lipgloss.NewStyle() allocated on every render frame (P1)

**Location**: `pkg/tui/chatpanel.go`, `pkg/tui/shelllayout.go`, `pkg/tui/splitlayout.go`, `pkg/tui/logpane.go`, and views throughout `internal/tui/views/`
**Problem**: `lipgloss.NewStyle()` is called inside `View()`, `renderHistory()`, `roleStyle()`, `renderHorizontal()`, and other methods that execute on every Bubble Tea frame. Each call allocates a new Style struct. Across a single frame, this adds up to dozens of allocations.
**Impact**: At 60fps, the garbage collector sees thousands of short-lived Style allocations per second. On slower machines or terminals with high latency, this can cause GC pauses that manifest as micro-stuttering. The 219 NewStyle() calls across the TUI packages are a significant allocation source.
**Fix**: Hoist styles to package-level `var` declarations or struct fields that are computed once at init/resize time. For example:

```go
// Package-level in chatpanel.go
var (
    chatSeparatorStyle = lipgloss.NewStyle().Foreground(ColorMuted)
    chatContentStyle   = lipgloss.NewStyle().Foreground(ColorFg).PaddingLeft(2)
)
```

Trade-off: Slightly less readable code (styles declared away from usage). The benefit is eliminating the single largest source of per-frame allocations.

### P1-2: Publish holds mutex during full subscriber fanout (P1)

**Location**: `/root/projects/Autarch/pkg/signals/broker.go` lines 44-65
**Problem**: `Publish()` acquires `sync.Mutex` and iterates all subscribers, performing channel operations (including potential drain-and-resend for full channels). Subscribe, Unsubscribe, and other Publish calls block for the entire fanout.
**Impact**: With many subscribers or a subscriber with a full channel (triggering the eviction path), Publish latency grows linearly. Since signals flow from Pollard hunters into the TUI via the broker, a slow Publish could delay signal delivery. Currently the subscriber count is small (a few in-process consumers), so this is a latent issue rather than an active one. It becomes a problem if WebSocket clients are added as subscribers.
**Fix**: Take a snapshot of the subscriber set under the lock, then fanout without the lock held:

```go
func (b *Broker) Publish(sig Signal) {
    b.mu.Lock()
    snapshot := make([]*subscriber, 0, len(b.subs))
    for sub := range b.subs {
        snapshot = append(snapshot, sub)
    }
    b.mu.Unlock()
    for _, sub := range snapshot {
        // channel operations without holding lock
    }
}
```

Trade-off: Snapshot allocation per publish (one small slice). Subscribers removed between snapshot and send will receive one extra message (benign). This is the standard pattern for broadcast systems.

### P2-1: Shell separator rebuilt from scratch every frame (P2)

**Location**: `/root/projects/Autarch/pkg/tui/shelllayout.go` lines 187-189
**Problem**: Every `Render()` call builds the sidebar separator by repeating a string for the full terminal height, then styling it. This involves `strings.Repeat()` allocation + lipgloss rendering.
**Impact**: Minor per-frame allocation. The separator only changes on resize, but it is rebuilt every frame regardless.
**Fix**: Cache the rendered separator string in the `ShellLayout` struct, invalidated only in `SetSize()`.

### P2-2: LoadAll reads every YAML insight from disk per request (P2)

**Location**: `/root/projects/Autarch/internal/pollard/server/server.go` line 237, `/root/projects/Autarch/internal/pollard/insights/insight.go` line 105
**Problem**: The `/api/insights` endpoint calls `insights.LoadAll()` on every request, reading all YAML files from disk and parsing them. This bypasses the `ScanCache`.
**Impact**: With many insight files accumulated over time, each request incurs disk I/O proportional to the number of insights. For the Pollard API server (local-only, low request rate), this is unlikely to be user-visible but represents a latency floor that grows with data.
**Fix**: Either route through `ScanCache.GetOrCompute()` with a short TTL (e.g., 30 seconds), or maintain an in-memory index that invalidates on directory mtime change.

### P2-3: Subscription.Stream blocks on out channel (P2)

**Location**: `/root/projects/Autarch/pkg/signals/broker.go` lines 114-123
**Problem**: `Stream()` writes to `out <- sig` without checking if `out` has capacity or if context is cancelled during the write. If the caller provides an unbuffered channel, Stream blocks indefinitely on send. While blocked, the subscriber's internal channel (buffered at 64) fills up, and subsequent Publish calls start dropping signals.
**Impact**: A slow consumer calling `Stream()` with an unbuffered channel silently degrades the entire broker's delivery to that subscriber.
**Fix**: Use a select with context:

```go
select {
case <-ctx.Done():
    return
case out <- sig:
}
```

Wait -- this is already the read side that has the select. The issue is that `out <- sig` on line 120 is an unconditional send. Adding a select with `ctx.Done()` on the write side would prevent indefinite blocking.

### P2-4: GetStats issues 5 sequential single-row queries (P2)

**Location**: `/root/projects/Autarch/internal/pollard/state/db.go` lines 242-281
**Problem**: Five separate `QueryRow` calls for aggregates that could be a single query.
**Impact**: Negligible for current data sizes (likely <1ms total). However, the pattern is a code smell that could be copied to more performance-sensitive code paths.
**Fix**: Consolidate into one query:

```sql
SELECT
    COUNT(*) AS total_runs,
    SUM(CASE WHEN status = 'success' THEN 1 ELSE 0 END) AS successful_runs,
    SUM(CASE WHEN status = 'failed' THEN 1 ELSE 0 END) AS failed_runs,
    COALESCE(SUM(sources_collected), 0) AS total_sources,
    COALESCE(SUM(insights_generated), 0) AS total_insights,
    MAX(started_at) AS last_run_at
FROM hunter_runs
```

### P3-1: Each hunter creates its own http.Client (P3)

**Location**: `/root/projects/Autarch/internal/pollard/hunters/github.go` line 34, `/root/projects/Autarch/internal/pollard/pipeline/fetcher.go` line 28
**Problem**: GitHubScout, the Fetcher, and presumably other hunters each instantiate `&http.Client{}`. Multiple clients mean separate TCP/TLS connection pools for the same hosts.
**Impact**: Missed connection reuse for sequential requests to the same API (e.g., GitHub search then README fetch). In practice, the 30-second timeout and low request volume mean connections are reused within a single client's pool anyway. This is a minor efficiency concern.
**Fix**: Create one shared `http.Client` with appropriate timeouts and pass it through the pipeline. Or use `http.DefaultTransport` with the default client and set timeouts per-request via `context.WithTimeout`.

### P3-2: Rate limiter token refill truncates to zero (P3)

**Location**: `/root/projects/Autarch/internal/pollard/hunters/hunter.go` line 132
**Problem**: `int(elapsed / r.perDuration * time.Duration(r.requests))` -- when `elapsed < perDuration`, the integer division `elapsed / perDuration` yields 0. Tokens only refill after a full period elapses, making this a burst limiter rather than a smooth token bucket.
**Impact**: For GitHub (10 requests/minute unauthenticated), the first 10 requests proceed instantly, then all subsequent requests wait a full minute regardless of how much time has passed. This is functionally correct (does not exceed rate limits) but less efficient than smooth refilling -- it creates bursty traffic patterns.
**Fix**: Use floating-point arithmetic: `tokensToAdd := int(float64(elapsed) / float64(r.perDuration) * float64(r.requests))`. Or use `golang.org/x/time/rate` which implements a proper token bucket.

### P3-3: LogPane.updateContent rebuilds entire string on every batch (P3)

**Location**: `/root/projects/Autarch/pkg/tui/logpane.go` lines 70-77
**Problem**: Every log batch (arriving at up to 10Hz) triggers a full rebuild of the viewport content from all 500 stored entries, each formatted with `lipgloss.NewStyle()` allocation.
**Impact**: At peak log volume (10 batches/second with 10 entries each), this causes ~5000 style allocations/second plus string building. Not on the main render frame path, but contributes to GC pressure.
**Fix**: Pre-format entries on arrival and cache the formatted strings. On batch arrival, only format the new entries and append to the cached content string.

## Improvements Suggested

### IMP-1: Hoist lipgloss styles to package-level variables

**Section**: TUI Rendering
Declare all styles used in `View()` methods as package-level variables or struct fields. This is the single highest-impact optimization for the TUI. Focus on `pkg/tui/chatpanel.go`, `pkg/tui/splitlayout.go`, `pkg/tui/shelllayout.go`, and `pkg/tui/logpane.go` first (the shared layout components), then the view-specific files. A mechanical refactor: find `lipgloss.NewStyle()` inside any method that starts with `View`, `Render`, `render`, or `format`, and hoist it.

### IMP-2: Use RWMutex in Broker or snapshot-then-fanout

**Section**: Signal Broker
Either switch to `sync.RWMutex` (Subscribe/Unsubscribe take write lock, Publish takes read lock for iteration) or use the snapshot pattern described in P1-2. The snapshot pattern is simpler and more robust for a pub/sub system.

### IMP-3: Cache insights on the Pollard server

**Section**: Insights API
Wrap the `insights.LoadAll()` call in `ScanCache.GetOrCompute()` with a key based on the directory path and a TTL of 30-60 seconds. This avoids re-reading YAML files on every API request while still picking up new insights within a reasonable window.

### IMP-4: Consolidate GetStats into a single SQL query

**Section**: SQLite
Replace the 5 separate `QueryRow` calls with one aggregate query. This is a quick win that improves code clarity and reduces connection round-trips.

### IMP-5: Share a single http.Client across the pipeline

**Section**: HTTP Clients
Create one `http.Client` with appropriate transport settings (connection pool size, TLS handshake timeout, idle connection timeout) and inject it into the Fetcher and all hunters. This enables connection reuse across pipeline stages.

## Overall Assessment

**Overall performance risk: Low-to-Medium**

The architecture is fundamentally sound. WAL mode, semaphore-bounded concurrency, log batching, scan caching with inflight dedup, and proper context cancellation are all correctly implemented. The codebase shows awareness of performance patterns (the `io.LimitReader` on HTTP responses, the 500-entry log cap, the `maxLogEntries` constant, the non-blocking log enqueue).

The main risk is death by a thousand allocations on the TUI render path. The 219 `lipgloss.NewStyle()` calls in render methods are the dominant concern -- not because any single one is slow, but because their cumulative effect at 60fps creates steady GC pressure. This is the kind of issue that manifests as "the TUI feels slightly laggy" rather than a hard performance failure.

**Must-fix items:**
- **P1-1** (style allocations): Hoist the top ~20 hottest styles in the shared layout components. This is a mechanical refactor with clear before/after measurability.
- **P1-2** (broker mutex): Adopt snapshot-then-fanout. The current code is correct for the current subscriber count but would become a bottleneck with WebSocket subscribers.

**Skip for now (premature optimization):**
- P3-1 (shared http.Client): The hunters run infrequently and serially per query. Connection reuse savings are marginal.
- P3-2 (rate limiter arithmetic): The burst behavior actually matches GitHub's rate limit model (per-minute quota, not smooth refill). The workaround (checking 403/429 headers) provides a safety net.
- P3-3 (log pane rebuild): Only 10Hz at peak, and users are unlikely to have the log pane open during performance-critical interactions.
