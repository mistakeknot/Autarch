# Review: Section Cache Correctness Analysis

**Author:** Julik (Flux-drive Correctness Reviewer)
**Date:** 2026-02-23
**Status:** CRITICAL FINDINGS IDENTIFIED

---

## Executive Summary

The section cache in `internal/bigend/tui/` implements a **per-section render memoization pattern** using FNV-1a hashing to detect when data changes. While the hash functions are comprehensive and invalidation on resize occurs, **the implementation has three critical correctness issues**:

1. **CRITICAL: Model is passed by value, dashCache is a pointer → potential data race on cache access**
2. **HIGH: GetState() returns shallow copy → hash collision possible if slice contents mutate after hashing**
3. **MEDIUM: No invalidation on data change outside resize → stale cache possible during multi-frame interval between ticks**

---

## Invariants (Assumed / Derived)

From code inspection and Autarch CLAUDE.md:

1. **Model is a Bubble Tea value type** passed through Update/View calls
2. **dashCache is a pointer field** on Model (line 285 of model.go)
3. **GetState() acquires read lock, returns shallow copy** (aggregator.go:807-810)
4. **State.Sessions, State.Projects, State.Agents are slices** pointing to underlying arrays
5. **State is mutated under mutex** in Aggregator (addActivity, Refresh, etc.)
6. **Multiple frames per second** may call renderDashboard() between ticks (Bubble Tea redraws on every Update)
7. **Hash functions only hash slice lengths and summaries**, not full content identity
8. **Width is included in hash** (correct, since lipgloss layout depends on it)
9. **No explicit cache invalidation** exists except `invalidateAll()` on resize

---

## Issue 1: CRITICAL — Model Value Type with Pointer Cache (Race Condition)

### Location
- Model struct: `/home/mk/projects/Demarch/apps/autarch/internal/bigend/tui/model.go:254-286`
- Cache field: line 285 `dashCache *sectionCache`
- Usage: `m.dashCache.getOrRender()` in render_dashboard.go:21, 29, 37, etc.

### Root Cause
In Bubble Tea, the Model is **passed by value through Update() and View()** calls. When a value receiver method modifies a pointer field, the mutation is visible across all copies of the Model because they share the same pointer to the underlying cache struct.

### Failure Narrative: Concurrent Cache Writes

**Precondition:**
- First Update() receives Model by value, calls `applyResize(msg)`
- Second concurrent Update() (unlikely but theoretically possible in Bubble Tea message batching) accesses cache via `renderDashboard()`

**Interleaving:**
1. **T1 (Update #1):** `m.applyResize()` → calls `m.dashCache.invalidateAll()` at line 1154
2. **T1:** `invalidateAll()` iterates `for k := range c.entries` and deletes entries (line 51-53)
3. **T2 (Update #2 or Render):** Concurrent `renderDashboard()` calls `m.dashCache.getOrRender(sectionStats, ...)`
4. **T2:** `getOrRender()` reads `c.entries[id]` (line 41) — map is being modified by T1
5. **RESULT:** Map concurrent read/write → undefined behavior, potential panic or stale entry

### Evidence
- `sectionCache` struct has no synchronization (no mutex) — lines 31-33
- `getOrRender()` modifies entries map without lock — line 45
- Multiple Bubble Tea commands may execute concurrently or in rapid succession

### Impact
**HIGH if Bubble Tea batches messages; MEDIUM if sequential but possible under load**

- Panic: "fatal error: concurrent map read/write"
- Silent stale cache: hash collision between frames
- Orphaned map entries on partial invalidation

### Fix
Add `sync.Mutex` to `sectionCache`:

```go
type sectionCache struct {
    mu      sync.Mutex
    entries map[sectionID]sectionEntry
}

func (c *sectionCache) invalidateAll() {
    c.mu.Lock()
    defer c.mu.Unlock()
    for k := range c.entries {
        delete(c.entries, k)
    }
}

func (c *sectionCache) getOrRender(id sectionID, hash uint64, renderFn func() string) string {
    c.mu.Lock()
    defer c.mu.Unlock()
    if entry, ok := c.entries[id]; ok && entry.hash == hash {
        return entry.rendered
    }
    s := renderFn()
    c.entries[id] = sectionEntry{rendered: s, hash: hash}
    return s
}
```

**Effort:** 5 lines added, 3 lines modified. ~1 minute to implement.

---

## Issue 2: HIGH — Shallow Copy GetState() → Hash Collision After Mutation

### Location
- `GetState()`: `/home/mk/projects/Demarch/apps/autarch/internal/bigend/aggregator/aggregator.go:807-810`
- Hash functions: `/home/mk/projects/Demarch/apps/autarch/internal/bigend/tui/section_cache.go:60-211`
- Usage: `renderDashboard()` line 17, hash computed after GetState() at lines 21, 29, 37, etc.

### Root Cause
`GetState()` returns a **shallow copy** of the State struct. The slice headers (ptr, len, cap) are copied, but they point to the same underlying arrays. If the Aggregator mutates a session status or activity **after** GetState() returns but **before** the hash is computed, the hash reflects the new data, but the cache was keyed by the old hash.

### Failure Narrative: TOCTOU (Time-of-Check-Time-of-Use)

**Precondition:**
- Aggregator.state.Sessions[0] has UnifiedState = StatusActive
- Last cached hash computed when UnifiedState was StatusActive

**Interleaving:**
1. **T1 (refresh goroutine):** Aggregator.Refresh() completes, updates state.Sessions[0].UnifiedState = StatusDone
2. **T1:** Releases RWMutex after `a.mu.Unlock()`
3. **T2 (Bubble Tea Update):** `state := m.agg.GetState()` — acquires read lock, returns shallow copy of State
4. **T2:** `state.Sessions` points to Aggregator's underlying sessions array
5. **T3 (Bubble Tea Update, concurrent with T2):** Aggregator receives websocket event "session status changed"
6. **T3:** `a.addActivity()` → `a.state.Sessions[0].UnifiedState = StatusBlocked` (mutates the array)
7. **T2:** Continues to `hashSessions(state.Sessions, 5)` — now hashes StatusBlocked, not StatusActive
8. **T2:** `getOrRender()` misses cache (hash changed), re-renders
9. **T2:** Stores new (StatusBlocked hash, rendered output) in cache

**Result:** Cache hit on next frame would show StatusBlocked data even if underlying state reverted, OR multiple render calls for the same logical state within one frame if timing is unlucky.

### Why Hash Functions Don't Prevent This
Hash functions only hash:
- `len(sessions)` (line 163)
- Per-session: Name, AgentName, ProjectPath, UnifiedState (lines 169-173)

If a session status changes mid-frame (concurrent event handling), the slice contents change but the len() is the same, so hashes may collide on slice length alone (unlikely but possible if new session replaces old).

### Probability
**MEDIUM:** Requires concurrent event arrival during render. With Bubble Tea's 60+ Hz redraw and Intermute WebSocket events, this **will occur in production** during active development (agent status changes).

### Impact
- Stale render (old hash, new data visible)
- Wasted re-render (cache miss despite unchanged logical state)
- User sees flickering or inconsistent status

### Fix: Deep Copy in GetState()

```go
func (a *Aggregator) GetState() State {
    a.mu.RLock()
    defer a.mu.RUnlock()

    // Deep copy all slices
    projects := make([]discovery.Project, len(a.state.Projects))
    copy(projects, a.state.Projects)

    agents := make([]Agent, len(a.state.Agents))
    copy(agents, a.state.Agents)

    sessions := make([]TmuxSession, len(a.state.Sessions))
    copy(sessions, a.state.Sessions)

    activities := make([]Activity, len(a.state.Activities))
    copy(activities, a.state.Activities)

    colonies := make([]colony.Colony, len(a.state.Colonies))
    copy(colonies, a.state.Colonies)

    // Deep copy map
    mcp := make(map[string][]mcp.ComponentStatus, len(a.state.MCP))
    for k, v := range a.state.MCP {
        statuses := make([]mcp.ComponentStatus, len(v))
        copy(statuses, v)
        mcp[k] = statuses
    }

    kernel := a.state.Kernel // Already safe if accessed under lock

    return State{
        Projects:   projects,
        Agents:     agents,
        Sessions:   sessions,
        Colonies:   colonies,
        MCP:        mcp,
        Activities: activities,
        Kernel:     kernel,
        UpdatedAt:  a.state.UpdatedAt,
    }
}
```

**Effort:** ~30 lines, ~5 minutes to implement + test.

**Note:** This fix also addresses Issue #1 indirectly by ensuring the cache always sees stable snapshot.

---

## Issue 3: MEDIUM — Missing Invalidation on Data Change

### Location
- Cache invalidation: `section_cache.go:50-54` (only `invalidateAll()` exists)
- Invalidation call: `model.go:1154` (only on resize)
- Hash mismatch behavior: `section_cache.go:40-46` (re-renders on hash mismatch, but no forced invalidation on known state change)

### Root Cause
The cache **only invalidates on resize** (`applyResize()` → `invalidateAll()` at line 1154). There is **no invalidation when data itself changes**. The hash-based approach assumes that hash mismatch will naturally cause re-render, but the hash is computed **after** the render call, not before.

### Failure Narrative: Stale Cache Between Ticks

**Precondition:**
- Cache hit on renderDashboard() at frame N
- Session status changes at frame N + 0.5 (between ticks)
- Next renderDashboard() call at frame N + 1

**Interleaving:**
1. **Frame N:** `state := m.agg.GetState()` returns Sessions with status Active
2. **Frame N:** Hash computed = hash(Sessions[...].UnifiedState=Active)
3. **Frame N:** Cache hit, renderStatsRow() returns cached output "1 Active"
4. **Frame N+0.5:** WebSocket event "session status changed to Done"
5. **Frame N+0.5:** Aggregator updates Sessions[0].UnifiedState = Done (or refreshes and miss)
6. **Frame N+1:** Next `m.agg.GetState()` returns Sessions with status Done
7. **Frame N+1:** Hash computed = hash(Sessions[...].UnifiedState=Done)
8. **Frame N+1:** **Cache miss** (hash changed), re-renders, but **consumed CPU and latency**

**Result:** While correct (stale cache isn't served), it's **inefficient**. Multiple renders for the same logical state within one "view cycle" (the frame shown to user).

### Why This Happens
Bubble Tea's `Update()` returns `(Model, Cmd)`, and the Model is re-rendered immediately. If a `refreshMsg` is sent (line 838-839), it triggers another Update cycle, which calls renderDashboard() again. **No explicit signal tells the cache "data changed, invalidate section X".**

### Probability
**HIGH:** Bubble Tea redraws on every Update, and Refresh commands are frequent (every 2 seconds at line 464). During refresh, Sessions/Activities are likely to change, triggering hash mismatch and re-render.

### Impact
- **Inefficient:** Multiple renders per logical state (CPU waste)
- **Latency:** Re-rendering happens on every state change, not just when needed
- **Correctness:** Rendering is correct, but wasteful

### Fix Option 1: Invalidate on Refresh

At the start of `refresh()` or after `agg.Refresh()` completes, clear the cache:

```go
case refreshMsg:
    m.lastRefresh = time.Now()
    m.dashCache.invalidateAll()  // Clear cache on refresh
    m.updateLists()
    return m, nil
```

**Effort:** 1 line.

**Tradeoff:** More aggressive invalidation (may clear cache that could have been reused). Acceptable if refresh is rare or data typically changes.

### Fix Option 2: Signal-Based Invalidation

Have Aggregator signal which sections changed:

```go
// In Aggregator.Refresh():
func (a *Aggregator) Refresh(ctx context.Context) error {
    // ... refresh logic ...
    a.mu.Lock()
    invalidSections := []string{}
    if sessionsChanged {
        invalidSections = append(invalidSections, "sessions", "stats")
    }
    if activitiesChanged {
        invalidSections = append(invalidSections, "activity")
    }
    a.mu.Unlock()

    // Signal TUI to invalidate specific sections
    return invalidSections, nil
}

// In TUI:
case refreshMsg:
    m.lastRefresh = time.Now()
    // Invalidate only affected sections
    for _, section := range msg.InvalidSections {
        m.dashCache.invalidateSection(section)
    }
    m.updateLists()
    return m, nil
```

**Effort:** ~20 lines to implement, more complex logic.

**Tradeoff:** Fine-grained control, but requires passing change metadata from Aggregator to TUI.

---

## Issue 4: MINOR — Hash Functions Don't Cover All Renders

### Location
Hash functions: `/home/mk/projects/Demarch/apps/autarch/internal/bigend/tui/section_cache.go:105-211`

### Finding
The hash functions hash **aggregated data only**, not render state like:
- **Filter state** (Tab-local filter state in m.filterStates)
- **Expanded groups** (m.groupExpanded map, used in renderTwoPane)
- **Terminal width side effects** (Width included, good!)

**Specific case:**
- `hashStats()` includes width and kernel metrics but **does not include** session filter state
- If user applies a filter to Sessions and then switches to Dashboard, the cached stats reflect the OLD session count (before filtering)

### Evidence
- Line 65 in hashStats: `binary.LittleEndian.PutUint64(b, uint64(len(state.Projects)))`
- But `state.Projects` is the full list, not filtered by active tab or selection

### Impact
**LOW to MEDIUM:** User sees inconsistent stats (e.g., "5 Sessions" but filter shows 2 sessions active). Confusing but not data-corrupting.

### Fix
Pass filter state into hash functions:

```go
func hashStats(state aggregator.State, width int, activeTab Tab, filterState FilterState) uint64 {
    // ... existing hashing ...
    // Add filter state to hash if applicable
    if activeTab == TabSessions && len(filterState.Terms) > 0 {
        h.Write([]byte(filterState.Raw))
    }
    return h.Sum64()
}

// In renderDashboard():
statsRow := m.dashCache.getOrRender(
    sectionStats,
    hashStats(state, width, m.activeTab, m.filterStateFor(TabSessions)),
    func() string { return m.renderStatsRow(state, width) },
)
```

**Effort:** ~10 lines.

---

## Summary of Findings

| Issue | Severity | Category | Fix Effort | Risk |
|-------|----------|----------|-----------|------|
| **1. Model/pointer race** | CRITICAL | Concurrency | 5 lines | PANIC, stale cache |
| **2. Shallow GetState()** | HIGH | Data Integrity | 30 lines | Hash collision, TOCTOU |
| **3. Missing invalidation** | MEDIUM | Efficiency | 1-20 lines | CPU waste, latency |
| **4. Filter not in hash** | LOW | Correctness | 10 lines | UI inconsistency |

---

## Recommended Implementation Order

1. **First:** Fix Issue #2 (shallow copy in GetState). This is the root cause of hash unreliability.
2. **Second:** Fix Issue #1 (add mutex to cache). This prevents concurrent panics.
3. **Third:** Fix Issue #3 (invalidate on refresh). Improves efficiency.
4. **Optional:** Fix Issue #4 (filter in hash). Nice-to-have for UI consistency.

---

## Testing Recommendations

### Test Case 1: Concurrent Updates
```go
// Simulate concurrent Update() and Render() on Model
func TestModelCacheRaceCondition(t *testing.T) {
    m := New(agg, "test")
    msg := tea.WindowSizeMsg{Width: 120, Height: 40}

    // Resize in goroutine
    go func() {
        m, _ = m.Update(msg)
    }()

    // Render in parallel
    for i := 0; i < 100; i++ {
        go func() {
            _ = m.View()
        }()
    }

    time.Sleep(100 * time.Millisecond)
    // Should not panic
}
```

### Test Case 2: TOCTOU with Shallow Copy
```go
// Mutate state after GetState() returns
func TestGetStateShallowCopy(t *testing.T) {
    agg := NewAggregator(...)
    agg.state.Sessions = []TmuxSession{{Name: "s1", UnifiedState: StatusActive}}

    state := agg.GetState()
    originalStatus := state.Sessions[0].UnifiedState

    // Mutate under lock (simulating concurrent event)
    agg.mu.Lock()
    agg.state.Sessions[0].UnifiedState = StatusDone
    agg.mu.Unlock()

    // Verify state.Sessions reflects the mutation (shallow copy issue)
    if state.Sessions[0].UnifiedState != StatusDone {
        t.Fatalf("expected state to reflect mutation (deep copy works)")
    }
}
```

### Test Case 3: Cache Invalidation on Resize
```go
// Verify cache is cleared on resize
func TestCacheInvalidationOnResize(t *testing.T) {
    cache := newSectionCache()
    cache.entries[sectionStats] = sectionEntry{rendered: "cached", hash: 12345}

    cache.invalidateAll()

    if len(cache.entries) > 0 {
        t.Fatalf("expected cache to be empty after invalidateAll()")
    }
}
```

---

## References

- **Bubble Tea Concurrency:** Bubble Tea dispatches messages sequentially but Update() is called in-line, not buffered.
- **Go Map Concurrent Access:** Maps are not thread-safe; concurrent read+write causes panic.
- **Shallow vs Deep Copy:** Go's `copy()` for slices performs shallow copy (header only, not elements).
- **FNV-1a Hash Collision:** Possible but rare; risk is low if hash is complete.
