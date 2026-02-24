# Performance Review: Acceptance Criteria Plan

**Reviewed:** 2026-02-05
**Plan:** `/root/projects/Autarch/docs/plans/2026-02-05-acceptance-criteria-plan.md`
**Reviewer profile:** Codebase-aware performance analysis against actual code

---

## Performance Profile

**Application type:** Interactive TUI (Bubble Tea) with background network I/O, embedded SQLite, and subprocess orchestration (Claude Code CLI). Hybrid: the TUI must stay responsive at ~60 FPS while long-running LLM calls and HTTP-based hunters execute in the background.

**Where performance matters most:**
1. TUI frame rendering -- must never block on I/O or computation
2. LLM subprocess latency -- dominates wall-clock time for PRD sprints
3. Pollard hunter throughput -- gated by free-tier API rate limits
4. SQLite write serialization -- MaxOpenConns(1) by design
5. WebSocket push latency for Bigend dashboard updates

**Known constraints:**
- GitHub Search API: 10 req/min unauthenticated, 30 req/min authenticated (confirmed in `/root/projects/Autarch/internal/pollard/hunters/github.go` lines 63-69)
- SQLite single writer connection with 5s busy timeout (`/root/projects/Autarch/pkg/db/open.go` line 22-29)
- Claude Code exploration has a 10-minute timeout (`/root/projects/Autarch/internal/gurgeh/exploration/explore.go` line 22)
- Bigend terminal streaming polls tmux at 100ms intervals (`/root/projects/Autarch/internal/bigend/web/server.go` line 466)

---

## Question 1: Are the timing thresholds realistic?

### Structural scan <5s -- REALISTIC

The plan correctly split the original <10s target into structural scan (<5s) and LLM exploration (<90s). The structural scan is a local `filepath.WalkDir` in `scan.go`. For a monorepo the size of Autarch (~144K LOC, ~1500 files), this completes in well under 1 second. The 5s budget is generous and realistic.

### LLM exploration <90s -- OPTIMISTIC but defensible

The `Explore()` function in `/root/projects/Autarch/internal/gurgeh/exploration/explore.go` shells out to `claude -p <prompt> --output-format stream-json --verbose --print` with a 10-minute timeout (line 22). The actual duration depends entirely on Claude Code's response time, which varies from 30s to 180s depending on codebase size, model load, and whether a session is being resumed.

The plan specifies "streaming partial results," which the code already supports -- `explore.go` parses `stream-json` output and logs tool usage in real-time (lines 55-86). The p95 target of <90s is tight but defensible because:
- Session reuse (`--resume` flag, lines 165-167 in `GeneratePhase`) avoids re-exploration
- Cached phase exploration (already implemented per AGENTS.md "Cached phase exploration" status)

**Issue (Low):** The plan says "<90s (p95)" but there is no mechanism to enforce or even measure p95. The only timeout is the 10-minute hard kill. Consider adding a soft timeout that returns partial results at 90s while allowing the full exploration to continue in background.

### First research finding <60s -- AT RISK for free-tier APIs

The plan says "<60s (raw), <120s (scored)." This depends heavily on which hunter fires first.

Looking at `DefaultResearchPlan()` in `/root/projects/Autarch/internal/gurgeh/arbiter/research_phases.go`:
- Vision phase triggers `github-scout` + `hackernews-trendwatcher` in "quick" mode
- GitHub Scout with unauthenticated rate limiter: 10 req/min

For a single query with `MaxResults` of, say, 10, GitHub Scout makes 1 search API call + potentially 10 fetch calls (for READMEs in "balanced" or "deep" mode). In "quick" mode, it likely skips README fetching. A single GitHub search API call returns in 1-3 seconds. HackerNews (Algolia API) is unauthed and fast (~500ms).

**Verdict:** <60s for the first raw finding is realistic in "quick" mode for Vision phase. However, for Problem phase (`arxiv-scout` + `openalex`, "balanced" mode), arXiv API responses can take 10-30s and OpenAlex 5-15s, plus synthesis time if enabled. The 60s target should be specified as phase-dependent: Vision <30s, Problem <60s, FeaturesGoals <90s (deep mode with competitor tracking).

### Badge update <5s -- REALISTIC

This is a TUI state update triggered by a `tea.Msg`. Bubble Tea message delivery is sub-millisecond within the same process. The 5s budget accounts for the full pipeline: hunter returns data -> finding created -> badge count updated. As long as findings are pushed via channel (not polled), this should complete in <1s.

### Triage to confidence update <2s -- REALISTIC

Confidence scoring in `/root/projects/Autarch/internal/gurgeh/arbiter/confidence/` is local computation over the sprint state. No I/O involved. Sub-100ms is expected. The 2s budget is very generous.

### WebSocket state update <2s -- SEE QUESTION 3

### SQLite write p99 <100ms -- SEE QUESTION 2

### Team spawn <30s for 3 teammates -- OPTIMISTIC

Each teammate is a separate Claude Code process. Process spawn alone is ~1s, but context loading (reading project files, CLAUDE.md, etc.) takes 5-15s per teammate. With 3 teammates spawning sequentially, 15-45s is more realistic. If spawned in parallel, the 30s target is achievable but depends on system resources (CPU, memory for 3 concurrent Claude Code instances).

### Deep Dive results <3 min -- REALISTIC but needs partial result handling

The plan correctly notes "configurable" and "partial OK." Pollard's `AgentTimeout` in `PipelineOptions` (hunter.go line 73) provides per-item timeout. The 3-minute target is reasonable for a targeted 1-2 query deep dive.

---

## Question 2: Does SQLite MaxOpenConns(1) bottleneck under 3+ Agent Teams teammates?

### Analysis

The code at `/root/projects/Autarch/pkg/db/open.go` sets:
```go
db.SetMaxOpenConns(1)
db.SetConnMaxLifetime(0)
// PRAGMA busy_timeout=5000
```

This is the canonical SQLite single-writer pattern. WAL mode allows concurrent readers, but with `MaxOpenConns(1)`, the Go `database/sql` pool serializes ALL operations (reads AND writes) through a single connection.

### Impact Assessment

**3+ teammates writing to `.coldwine/state.db`:**
Each teammate's task state transitions (claim, in_progress, blocked, done) generate SQLite writes. With 3 teammates, writes could arrive within milliseconds of each other. The 5s `busy_timeout` means the second and third writers will wait up to 5s for the first to release.

**Typical write duration for Coldwine:**
- Task status update: ~1-5ms (single row UPDATE with indexed lookup)
- Session state update: ~1-5ms (single row UPSERT)
- Event logging: ~1-5ms (INSERT into events table)

At 1-5ms per write, even with 3 concurrent writers, serialized throughput is ~200-1000 writes/second. This is NOT a bottleneck for the task lifecycle use case.

**However, the plan's p99 <100ms target is at risk in specific scenarios:**

1. **Bigend Refresh + Coldwine writes:** The `Refresh()` method in `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go` (line 360) calls `enrichWithTaskStats()` which opens a read connection to `.coldwine/state.db`. With `MaxOpenConns(1)`, this read blocks while any Coldwine write is in progress. If the Coldwine and Bigend processes share the same `db.Open()` instance (unlikely for separate databases), serialization occurs.

2. **Each tool has its own database:** `.coldwine/state.db`, `~/.autarch/events.db`, `.pollard/` uses YAML not SQLite. So cross-tool contention on a single SQLite database is minimal. The real contention is within a single database (`.coldwine/state.db`) under concurrent Agent Teams writes.

3. **The actual risk is not the Go pool but the SQLite WAL checkpoint.** With `synchronous=NORMAL`, WAL checkpoints are non-blocking reads but can stall writes briefly. Under sustained write load from 3+ agents, WAL file growth can cause periodic checkpoint stalls.

### Verdict

**Low risk for 3 teammates.** SQLite with WAL mode and `MaxOpenConns(1)` can handle the write throughput of 3 agent task state updates without exceeding 100ms p99. The busy_timeout of 5s provides ample headroom for brief contention spikes.

**Medium risk for 5+ teammates.** If Agent Teams scales beyond 3, or if tasks involve high-frequency state updates (e.g., progress percentage updates at 1Hz per agent), the serialized writes could start hitting the 100ms p99 target.

The plan already identifies this: "SQLite single-connection bottleneck under Agent Teams" in the Data Integrity Risks section. The <100ms p99 target with the caveat "under 3 concurrent agent sessions" is accurate.

**Recommendation:** No change needed for v1 (3 teammate limit). If scaling beyond 3, consider `MaxOpenConns(2)` with WAL mode (safe for concurrent readers) or move high-frequency operations to an in-memory queue with periodic batch flush.

---

## Question 3: Are WebSocket update latency targets (<2s) achievable?

### Architecture Analysis

The plan specifies two WebSocket paths:

**Path A: Bigend terminal streaming (existing)**
- `/root/projects/Autarch/internal/bigend/web/server.go` line 466: 100ms polling ticker
- `/root/projects/Autarch/internal/bigend/daemon/server.go` line 351: same 100ms pattern
- This captures tmux pane output and sends diffs
- Latency: 0-100ms (polling interval) + WebSocket write time (~1ms on loopback)
- **Verdict: Well under 2s. Already implemented.**

**Path B: Aggregator state updates via Intermute (planned)**
- `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go` `ConnectWebSocket()` (line 157) subscribes to Intermute events
- Event flow: Tool modifies state -> Intermute broadcasts WebSocket event -> Aggregator receives -> triggers `refreshForEvent()` -> goroutine does I/O refresh -> state updated -> pushed to dashboard
- The `refreshForEvent()` (line 267) spawns goroutines for targeted refresh. These goroutines do I/O:
  - `enrichWithGurgStats()`: reads YAML files from `.gurgeh/specs/` (1-10ms)
  - `enrichWithPollardStats()`: walks `.pollard/` directory tree (1-50ms depending on file count)
  - `loadAgents()`: HTTP call to Intermute API (10-100ms on loopback)

**Latency chain for Path B:**
1. Tool writes state change: ~1ms
2. Intermute broadcasts event: ~1ms (in-process or loopback WebSocket)
3. Aggregator receives event: ~1ms
4. Goroutine refresh I/O: 10-100ms
5. State update + push to dashboard: ~1ms
6. **Total: 15-105ms typical, well under 2s**

**But there is a gap:**
The `refreshForEvent()` goroutines update `a.state` but there is no mechanism to push the updated state to dashboard WebSocket clients. The current code updates the in-memory state, but the dashboard HTML presumably polls the REST API or uses a separate WebSocket that is not wired to the aggregator event system.

Looking at the aggregator, `dispatchEvent()` (line 313) calls registered `EventHandler` functions. If the Bigend web server registers a handler that pushes to dashboard WebSocket clients, the <2s target is achievable. If the dashboard relies on polling (e.g., JavaScript `setInterval` hitting a REST endpoint), the latency depends on the poll interval.

### Verdict

**Achievable with event-driven push.** The Intermute WebSocket -> aggregator -> dashboard push chain can deliver <2s easily. The risk is if the dashboard implementation falls back to polling. The plan should add a criterion: "Dashboard updates use WebSocket push from aggregator events, not client-side polling."

**Path A (terminal streaming) already achieves <100ms.** Not a concern.

### Issue: Missing Bigend dashboard -> client push mechanism

The aggregator has `On()` for event handlers and `dispatchEvent()` for propagation, but the bridge from aggregator state change to dashboard WebSocket client notification is not implemented in the reviewed code. The `handleTerminalWS()` endpoints stream tmux output, but there is no `handleDashboardWS()` that streams aggregator state changes.

**Impact (Medium):** Without this bridge, AC-5.3 ("WebSocket updates reflect state changes within 2 seconds") is untestable. The architecture supports it, but the wiring is missing.

---

## Question 4: Is the research pipeline timing realistic with free-tier APIs?

### Hunter-by-Hunter Analysis

From `/root/projects/Autarch/internal/pollard/hunters/`:

| Hunter | API | Rate Limit | Typical Response | First Finding Time |
|--------|-----|-----------|------------------|-------------------|
| `github-scout` | GitHub Search | 10/min unauthed, 30/min authed | 1-3s per call | 5-10s |
| `hackernews` | Algolia HN | Unauthed, generous | 0.5-2s | 3-5s |
| `arxiv` | arXiv API | Unauthed, ~3s rate limit between calls | 3-10s | 10-20s |
| `openalex` | OpenAlex API | Unauthed, generous | 2-5s | 5-15s |
| `pubmed` | PubMed E-utils | Unauthed 3/s, keyed 10/s | 2-5s | 5-10s |
| `competitor` | Varies (changelogs, docs) | Per-site | 5-30s | 15-60s |
| `agent` | Claude Code subprocess | N/A (token-based) | 30-120s | 45-120s |

### First Finding in <60s

For Vision phase (quick mode): `github-scout` + `hackernews-trendwatcher` run in parallel. GitHub returns in 5-10s, HN in 3-5s. First raw finding should arrive in **5-10s**, well within the 60s budget.

For Problem phase (balanced mode): `arxiv-scout` + `openalex` run in parallel. arXiv returns in 10-20s, OpenAlex in 5-15s. First finding in **10-20s**, within budget.

**Verdict: <60s for first raw finding is realistic for all phase configurations.**

### Deep Dive in <2 minutes

A deep dive triggers targeted research. In deep mode with synthesis enabled, the pipeline is:
1. Search: 5-10s
2. Fetch (README, full docs): 10-30s (parallelized by `pipeline.Fetcher` with concurrency 5)
3. Synthesize (Claude Code subprocess): 30-60s per item, limited by `AgentParallelism`
4. Score: <1s (local computation)

With `SynthesizeLimit` set reasonably (top 3-5 items) and `AgentParallelism` of 2-3:
- Total: 5 + 30 + 60 + 1 = ~96s for a thorough deep dive
- The <2 minute target is tight but achievable with `SynthesizeLimit <= 3`
- The plan wisely specifies <3 min configurable, which is more realistic

**Issue (Low):** The plan says "<2 minutes" in AC-3.5 but the timing table says "<3 min, configurable." These should be reconciled. The <3 min target with partial results is more realistic.

### Research Coverage >80% -- PROBLEMATIC

The plan wisely revised this from >80% to ">60% (4 of 8 phases have configs)" in the timing table. This matches the code: `DefaultResearchPlan()` in `research_phases.go` returns configs for only 4 phases (Vision, Problem, FeaturesGoals, Requirements). Users, CUJs, Scope, and Acceptance have no research configs.

**Verdict:** The ">60%" in the timing table is correct and matches the code. The ">80%" in AC-1.13 is unreachable without adding research configs for the remaining 4 phases. The plan should reconcile these two numbers.

---

## Question 5: Missing performance criteria

### 5a. Memory -- MEDIUM concern

**Missing criterion: Peak memory usage under Agent Teams.**

Each Claude Code subprocess consumes 100-300MB of memory. With 3 teammates + the Autarch TUI + Intermute, peak memory usage could reach 1-2GB. The plan has no memory budget or monitoring.

**Specific risk in aggregator:** The `addActivity()` method (aggregator.go line 244) prepends to a slice with `append([]Activity{activity}, a.state.Activities...)`. This creates a new slice on every event, copying all 100 activities each time. Under sustained event load, this causes frequent GC pressure. Not a serious problem at 100 events, but the pattern should not be copied for larger collections.

**Recommendation:** Add AC-X.11: "Peak RSS under 3-teammate Agent Teams session does not exceed 2GB on the Autarch host process" (excluding Claude Code subprocesses).

### 5b. Goroutine count -- LOW concern

The `refreshForEvent()` method spawns goroutines without tracking. Under rapid event storms (e.g., bulk task creation), goroutines could accumulate. However:
- Each goroutine does bounded I/O (file walk, HTTP call) and exits
- The lock (`a.mu.Lock()`) serializes writes, so goroutines queue but don't leak
- No goroutine leak risk identified in steady state

**Recommendation:** Not critical for v1. If Agent Teams produces sustained event storms, add a goroutine-count metric in Bigend's health endpoint.

### 5c. File descriptor limits -- MEDIUM concern

**Missing criterion: File descriptor usage under concurrent hunters.**

Each hunter opens HTTP connections. With 4 phases triggering 2 hunters each, running in parallel:
- 8 hunter goroutines
- Each with HTTP client (potentially pooled connections)
- Plus tmux capture (`CapturePane` shells out, opening FDs)
- Plus Claude Code subprocesses (stdin/stdout/stderr pipes)
- Plus WebSocket connections (Intermute, Signals, Bigend clients)

Default `ulimit -n` on most Linux systems is 1024. Under Agent Teams with 3 teammates (3 Claude Code processes), plus Pollard hunters, plus WebSocket connections, FD usage could approach 200-300.

**Verdict:** Not a risk for the default ulimit of 1024, but worth documenting. No criterion needed for v1.

### 5d. Signal broker subscriber buffer -- LOW concern

The Signals broker in `/root/projects/Autarch/pkg/signals/broker.go` creates subscribers with `make(chan Signal, 64)` buffer. The `Publish()` method silently drops signals when the buffer is full (line 51-54):

```go
select {
case sub.ch <- sig:
default:
    // Drop if subscriber is slow.
}
```

This is a deliberate design choice (non-blocking publish), but the plan's AC-3.4a (signal deduplication) interacts poorly with it. If a dedup signal is dropped because the buffer is full, the dedup state thinks it was delivered but the subscriber never received it.

**Recommendation:** Add AC-3.4e: "Signal broker logs a warning when dropping signals due to buffer full, including signal type and subscriber identity."

### 5e. TUI rendering under research load -- LOW concern

The plan correctly identifies that research runs async and doesn't block the TUI. The Bubble Tea update loop processes `tea.Msg` synchronously, but research findings arrive via channels and are converted to messages. The 60 FPS rendering is not at risk because:
- `State()` in the arbiter returns deep copies (per institutional learnings)
- Badge updates are simple integer increments
- Doc pane rendering is on-demand (only when visible and content changes)

No missing criterion here. The plan's existing test categories cover TUI rendering adequately.

### 5f. Exploration subprocess cleanup -- LOW concern

`Explore()` in `explore.go` uses `context.WithTimeout(ctx, 10*time.Minute)` and `exec.CommandContext()`. If the context is cancelled, Go sends SIGKILL to the subprocess. However, Claude Code may have spawned its own child processes (e.g., Language Server, tools). These orphan processes are not tracked.

**Recommendation:** Not a v1 blocker. The 10-minute hard timeout is defensive. Consider process group kills (`cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}`) for clean subprocess cleanup in a future iteration.

---

## Specific Issues (by impact)

### Issue 1: AC-1.13 research coverage target contradicts code (Medium)

- **Location:** Timing Thresholds Summary row "Research coverage" vs AC-1.13
- **Problem:** AC-1.13 says ">80% research coverage" but `DefaultResearchPlan()` only configures 4 of 8 phases. The timing table correctly says ">60%". These contradict each other.
- **Impact:** AC-1.13 will always fail if "research coverage" means "phases with research."
- **Fix:** Change AC-1.13 to ">60% research coverage" or define "research coverage" as weighted by phase importance (Vision/Problem/Features worth more than Scope/Acceptance). Alternatively, add research configs for the remaining 4 phases. The timing table's note "(4 of 8 phases have configs)" is correct and should be reflected in AC-1.13.

### Issue 2: No Bigend dashboard push mechanism for state changes (Medium)

- **Location:** AC-5.3 ("WebSocket updates reflect state changes within 2 seconds")
- **Problem:** The aggregator receives Intermute events and refreshes state, but there is no WebSocket endpoint that pushes aggregator state changes to dashboard clients. The existing WebSocket endpoints (`handleTerminalWS`) only stream tmux output for individual sessions. Dashboard state updates likely rely on client-side polling.
- **Impact:** AC-5.3 passes only if polling interval is <2s, which burns CPU and bandwidth for all connected clients. Event-driven push would be more efficient and reliable.
- **Fix:** Add a `handleDashboardWS()` endpoint that subscribes to aggregator events via `On("*", ...)` and pushes state diffs to connected clients. The aggregator already has the infrastructure for this.

### Issue 3: Deep Dive timing inconsistency between AC and timing table (Low)

- **Location:** AC-3.5 says "<2 minutes," timing table says "<3 min, configurable"
- **Problem:** The AC is stricter than the timing table. With synthesis enabled for 3+ items, <2 minutes is tight.
- **Impact:** Flaky test if the AC uses the 2-minute target.
- **Fix:** Reconcile to "<3 min, configurable" in both places. AC-3.5 should say "within configured timeout (default 3 minutes)."

### Issue 4: RateLimiter is not thread-safe (Low)

- **Location:** `/root/projects/Autarch/internal/pollard/hunters/hunter.go` lines 107-154
- **Problem:** `RateLimiter.Wait()` reads and writes `r.tokens` and `r.lastRefill` without a mutex. If two goroutines call `Wait()` concurrently (e.g., parallel queries within a single hunter), they could both see `r.tokens > 0` and double-consume a token, exceeding the rate limit.
- **Impact:** Could cause 429 errors from APIs, especially GitHub's strict rate limiting. The `go test -race` flag would catch this.
- **Fix:** Add `sync.Mutex` to `RateLimiter`. This is a correctness issue, not strictly performance, but it causes performance problems (retry storms from 429s).

### Issue 5: Aggregator activities slice allocation pattern (Low)

- **Location:** `/root/projects/Autarch/internal/bigend/aggregator/aggregator.go` line 256
- **Problem:** `append([]Activity{activity}, a.state.Activities...)` allocates a new 100-element slice on every event, copying all elements.
- **Impact:** Negligible for 100 activities. Would matter if the cap were increased. Not worth fixing now.
- **Fix:** Use a ring buffer if scaling beyond 100. Skip for v1.

---

## Summary

**Overall performance risk: LOW**

The plan's timing thresholds are well-calibrated to the actual codebase. The research team that produced this plan clearly read the code -- they correctly identified the structural-scan vs LLM-exploration split, the 4-of-8 phase research config limitation, the SQLite single-connection pattern, and the signal broker's silent drop behavior.

### Must-fix items (before implementation):
1. **Reconcile AC-1.13 research coverage** (>80% vs >60%) -- inconsistency will cause guaranteed test failures
2. **Reconcile AC-3.5 deep dive timing** (<2 min vs <3 min) -- pick one number

### Should-fix items (before testing):
3. **Clarify Bigend dashboard push mechanism** for AC-5.3 -- the <2s target is achievable but requires WebSocket push, not polling
4. **Add mutex to RateLimiter** -- concurrent hunters will race on token consumption

### Skip (premature optimization):
- SQLite MaxOpenConns tuning -- single connection handles 3 teammates fine
- Activities slice allocation -- 100 elements, irrelevant
- Goroutine tracking in refreshForEvent -- bounded I/O, no leak risk
- Process group subprocess cleanup -- defensive timeout already exists
- File descriptor limit criterion -- well within default ulimit

The plan is thorough and performance-aware. The timing thresholds reflect measured understanding of the codebase rather than aspirational guesses. The main risk is not performance but the implementation gaps identified in the plan's own "Critical Implementation Gaps" section (glob overlap detection, Agent Teams bridge mechanism, signal transport path). Those are correctness issues, not performance issues.
