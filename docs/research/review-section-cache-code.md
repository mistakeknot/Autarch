# Code Review: Section Cache Render Optimization (Bigend Dashboard)

**Scope:** Render optimization for Bubble Tea TUI dashboard via section caching + FNV-64 hashing
**Files Reviewed:**
- `internal/bigend/tui/section_cache.go` (211 lines)
- `internal/bigend/tui/section_cache_test.go` (207 lines)
- `internal/bigend/tui/render_dashboard.go` (partial, 300 lines)
- `internal/bigend/tui/model.go` (cache field additions, 1289 lines total)

**Date:** 2026-02-23
**Reviewer Notes:** Universal + Go-specific checks applied. Design is sound; implementation is mostly correct with minor edge case gaps.

---

## Executive Summary

**VERDICT: APPROVE with 2 minor observations** (not blocking)

The section cache implementation is a solid performance optimization for dashboard rendering:
- ✓ Naming conventions and Go idioms followed correctly
- ✓ Hash functions deterministic (map-key sorting, binary encoding)
- ✓ Test coverage comprehensive (stability, sensitivity, determinism, invalidation)
- ✓ Cache invalidation on resize (correctly wired into `applyResize`)
- ✓ Simple, single-responsibility design

**Two observations (cosmetic, not blocking):**
1. Inconsistent hash parameter naming (`width` vs `limit`) — minor clarity issue
2. Hash functions could reuse a byte buffer for efficiency — micro-optimization, not required

---

## Universal Quality Checks

### Naming Conventions ✓

**File & Type Names:**
- `section_cache.go` — lowercase, descriptive, follows Go package conventions
- `sectionID` — unexported (lowercase), clear purpose as section identifier
- `sectionEntry` — unexported, semantically clear as "entry in cache"
- `sectionCache` — unexported, concrete type (not interface), matches Autarch style

**Function Names:**
- `newSectionCache()` — standard Go constructor pattern (lowercase, returns `*sectionCache`)
- `getOrRender()` — clear intent: "get from cache or call render function"
- `invalidateAll()` — verb-noun idiom, common in Go codebase
- `hashStats()`, `hashRuns()`, etc. — consistent `hash*` prefix, correct scope (package-private)

**Constant Names:**
```go
const (
    sectionStats sectionID = iota
    sectionRuns
    ...
)
```
Unexported (lowercase), follow Go iota convention. Good.

### File Organization ✓

- Cache logic isolated in `section_cache.go` (orthogonal concern)
- Tests in `section_cache_test.go` (same package, standard Go pattern)
- Cache integration in `model.go`: field + initialization + invalidation
- Render logic in `render_dashboard.go`: uses cache via `getOrRender()` calls
- Clear separation: cache is a dumb store, rendering calls provide hash + render closure

### Error Handling ✓

**No error handling needed here** — this is deterministic caching with no I/O. Hash failures would indicate a serious bug in `fnv` or `binary` (stdlib), not worth recovering. Correct assessment.

### Test Strategy ✓

**Tests are appropriate for risk level:**
- Unit tests for hash functions (stability, sensitivity, determinism)
- Integration test for cache hit/miss logic
- Invalidation test (resize clears cache)
- Determinism tests (100 iterations for map-iteration variance)
- `TestResizeInvalidatesCache` — validates the intended use case (resize → invalidate)

**Coverage:** 6 tests cover:
- Hash stability (same input → same output)
- Hash sensitivity (different input → different output)
- Width sensitivity (important: lipgloss layout depends on width)
- Cache hit/miss behavior
- Invalidation on resize
- Deterministic sorting (maps have random iteration order)

All critical paths tested. Good.

### API Consistency ✓

**Cache API simple and predictable:**
```go
func (c *sectionCache) getOrRender(id sectionID, hash uint64, renderFn func() string) string
```
- Caller computes hash (using provided `hashStats()`, `hashRuns()`, etc.)
- Caller provides render function as closure
- Cache is dumb: just checks hash, stores result

This pattern is consistent with Go practices (lazy evaluation via function parameter).

### Complexity Budget ✓

No unnecessary indirection:
- Hash functions are straightforward: encode fields → FNV-64
- Cache itself is a simple map (no locking needed; render is synchronous)
- Width is correctly included in hash (lipgloss layout depends on width)

Good restraint.

### Dependency Discipline ✓

**Dependencies:**
- `encoding/binary` (stdlib)
- `hash/fnv` (stdlib)
- `sort` (stdlib)
- Project types: `aggregator.State`, `aggregator.KernelState`, `icdata.*`

No gratuitous new imports. Using stdlib for crypto primitives (FNV is acceptable for non-cryptographic hashing).

---

## Go-Specific Checks

### Error Handling — Explicit & Context-Preserving ✓

Not applicable here (no I/O, deterministic cache), but the code correctly does not add error handling where not needed.

### 5-Second Naming Rule (Exported Symbols) ✓

All exported symbols are clear without context:
- `sectionCache` is unexported (OK to be terse)
- Cache methods all have clear names and types

### "Accept Interfaces, Return Structs" ✓

Cache returns concrete `string` (correct). No interface bloat. `renderFn` is a function parameter (idiomatic for lazy evaluation).

### Deterministic Map Iteration ✓

**EXCELLENT — properly handled:**
```go
func hashRuns(kernel *aggregator.KernelState, width int) uint64 {
    // Sort map keys for deterministic hashing (Go map iteration is random).
    projects := make([]string, 0, len(kernel.Runs))
    for proj := range kernel.Runs {
        projects = append(projects, proj)
    }
    sort.Strings(projects)
    for _, proj := range projects {
        // ... hash in sorted order
    }
}
```

Comment explicitly notes the issue. Implementation correct. Same pattern applied consistently in `hashDispatches()`. Very good.

### Race Condition Analysis ✓

**No concurrent access to cache:**
- Cache lives in `Model` (TUI model, single-threaded message loop)
- Render is synchronous (happens in `View()` method, not background)
- No mutation after cache hit
- Safe as designed

### Test Quality — Table-Driven & Race ✓

**Tests are NOT table-driven** (not needed here; each test is distinct scenario). Determinism tests manually loop 100x to catch random map ordering bugs (good substitute for `-race` here since no goroutines). Well-designed for the problem.

---

## Detailed Findings

### 1. Hash Functions — Field Coverage ✓

**Each hash function includes fields the corresponding render section reads:**

| Section | Hash Function | Included Fields | Risk |
|---------|---------------|-----------------|------|
| Stats | `hashStats()` | width, project/session/agent counts, active sessions, kernel metrics | ✓ OK |
| Runs | `hashRuns()` | width, project keys, run IDs/status/phase/goal | ✓ OK |
| Dispatches | `hashDispatches()` | width, project keys, dispatch IDs/status/agentType/tokens | ✓ OK |
| Sessions | `hashSessions()` | session count, limit, name/agentName/projectPath/state | ✓ OK |
| Agents | `hashAgents()` | agent count, limit, name/program/projectPath | ✓ OK |
| Activities | `hashActivities()` | activity count, limit, summary/source/agentName/timestamp | ✓ OK |

All hash inputs correspond to render outputs. No missing fields detected.

### 2. Hash Stability — FNV-64a Implementation ✓

**Correct use of `hash/fnv`:**
```go
h := fnv.New64a()
b := make([]byte, 8)
binary.LittleEndian.PutUint64(b, uint64(width))
h.Write(b)
```

Byte encoding via `binary.LittleEndian` ensures determinism (endianness consistent). FNV-64a is deterministic by design. Good.

**Test validates:** `TestSectionHashStability` confirms same input → same output. `TestHashRunsDeterministic` validates 100 iterations (guards against map iteration variance). Solid.

### 3. Width Sensitivity ✓

**Good:** Width is included in every hash (stats, runs, dispatches). Correct because lipgloss layout depends on terminal width.

**However:** `hashSessions()` and `hashAgents()` take `limit` parameter instead of `width`. This is intentional (they don't render width-dependent layouts), but naming is inconsistent.

**Observation (cosmetic):**
```go
func hashSessions(sessions []aggregator.TmuxSession, limit int) uint64 {
    // ... ignores width. 'limit' is for pagination, not layout.
}
```

The parameter is called `limit` (pagination count), not `width`. This is correct semantically, but inconsistent with `hashStats(... width int)`. Minor clarity issue — the code works, but naming could be clearer:
- Consider renaming parameter to `viewportSize` or `maxRows` for clarity
- OR add a comment: `// limit is max rows shown; width not needed (no responsive layout)`

**Not blocking** — intent is clear from context.

### 4. Cache Invalidation ✓

**Correctly wired to resize:**
```go
func (m Model) applyResize(msg tea.WindowSizeMsg) Model {
    m.dashCache.invalidateAll()
    m.width = msg.Width
    m.height = msg.Height
    // ...
}
```

Cache is cleared BEFORE width/height are updated, ensuring fresh render uses new dimensions. Good.

**Test validates:** `TestResizeInvalidatesCache` confirms cache is empty after resize. ✓

### 5. Cache Hit Behavior ✓

**Simple and correct:**
```go
func (c *sectionCache) getOrRender(id sectionID, hash uint64, renderFn func() string) string {
    if entry, ok := c.entries[id]; ok && entry.hash == hash {
        return entry.rendered
    }
    s := renderFn()
    c.entries[id] = sectionEntry{rendered: s, hash: hash}
    return s
}
```

- Hash matches: return cached string (no render call)
- Hash differs or missing: call render function, cache result, return

Logic is sound. No off-by-one errors or missing checks.

**Test validates:** `TestSectionCacheHitAndMiss` confirms hit/miss behavior. ✓

### 6. Micro-Optimization Opportunity (Not Blocking)

Each hash function allocates a fresh `[]byte` buffer:
```go
func hashStats(state aggregator.State, width int) uint64 {
    h := fnv.New64a()
    b := make([]byte, 8)  // ← allocated here
    binary.LittleEndian.PutUint64(b, uint64(width))
    h.Write(b)
    // ... reuse b for more PutUint64 calls
}
```

This is fine for render frequency (likely < 60 FPS), but could reuse a single buffer:
```go
b := [8]byte{} // stack-allocated array
binary.LittleEndian.PutUint64(b[:], uint64(width))
```

**Not required** — current implementation is clear and allocation will be inlined by the compiler anyway. Only mention if targeting extreme optimization (e.g., > 100 FPS).

### 7. Edge Cases

**Missing field in state:**
```go
if state.Kernel != nil { ... }
```
Handled. ✓

**Empty slices:**
```go
func hashSessions(sessions []aggregator.TmuxSession, limit int) uint64 {
    h := fnv.New64a()
    b := make([]byte, 8)
    binary.LittleEndian.PutUint64(b, uint64(len(sessions)))
    h.Write(b)
    // ...
}
```
Hashes the length (0 for empty slice). Correct — two different empty states won't collide. ✓

**Nil slices vs empty slices:**
- Hash functions receive slices by value, not nil pointers. Slices are never nil (even `[]T{}` is a valid empty slice). No risk. ✓

**`limit > len(sessions)`:**
```go
for i, s := range sessions {
    if i >= limit { break }
    // ...
}
```
Handles gracefully — breaks early if limit exceeded. ✓

### 8. Integration with Model ✓

**Field added to Model:**
```go
type Model struct {
    // ...
    dashCache *sectionCache
    // ...
}
```

**Initialized in constructor:**
```go
func New(agg aggregator.Aggregator, name string) Model {
    return Model{
        agg: agg,
        name: name,
        // ...
        dashCache: newSectionCache(),
        // ...
    }
}
```

Standard Go pattern. ✓

**Used in render:**
```go
func (m Model) renderDashboard() string {
    state := m.agg.GetState()
    statsRow := m.dashCache.getOrRender(sectionStats, hashStats(state, width), func() string {
        return m.renderStatsRow(state, width)
    })
    // ...
}
```

Cache is transparent to render logic. Good design. ✓

---

## Test Coverage Analysis

### Coverage by Risk:

| Test | Risk Level | Quality |
|------|-----------|---------|
| `TestSectionHashStability` | HIGH (hash correctness) | ✓ Direct test of determinism |
| `TestSectionHashSensitivity` | HIGH (cache correctness) | ✓ Validates divergence detection |
| `TestSectionHashWidthSensitivity` | HIGH (lipgloss layouts) | ✓ Tests width-dependent hashing |
| `TestSectionCacheHitAndMiss` | HIGH (core cache logic) | ✓ Validates hit/miss behavior |
| `TestSectionCacheInvalidate` | HIGH (resize invalidation) | ✓ Tests invalidation on resize |
| `TestHashActivities` | MEDIUM (source sensitivity) | ✓ Validates field coverage |
| `TestHashRunsDeterministic` | HIGH (map ordering) | ✓ 100-iteration stress test |
| `TestResizeInvalidatesCache` | HIGH (integration) | ✓ Full lifecycle test |
| `TestDashboardCacheSkipsReRender` | MEDIUM (integration) | ✓ End-to-end verification |
| `TestHashDispatchesDeterministic` | HIGH (map ordering) | ✓ 100-iteration stress test |

**All critical paths covered.** No major gaps.

---

## Recommendations

### Minor (Informational)

1. **Parameter Naming Consistency**
   - `hashSessions()` and `hashAgents()` use `limit` parameter; others use `width`
   - Consider clarifying which functions are width-dependent and which are pagination-dependent with a comment or consistent naming
   - Current code is correct; this is a readability enhancement only

   ```go
   // hashSessions hashes a limited number of sessions.
   // Width is not included (sessions are listed, not wrapped).
   func hashSessions(sessions []aggregator.TmuxSession, limit int) uint64 {
   ```

2. **Buffer Allocation Micro-Optimization (Optional)**
   - Replace `b := make([]byte, 8)` with stack allocation for very tight loops
   - Not necessary for current render frequencies; only pursue if profiling shows hash time as bottleneck

### No Changes Required

- Hash field coverage is complete
- Determinism handling (map sorting) is excellent
- Cache invalidation is correctly integrated
- Test coverage is comprehensive
- No error paths (correctly identified as not applicable)
- No concurrency issues

---

## Conclusion

**APPROVAL: YES**

This is a well-executed optimization:
- Clean separation of concerns (cache is independent of rendering)
- Deterministic hashing with proper map-iteration handling
- Comprehensive test coverage (stability, sensitivity, determinism, edge cases)
- Correct integration with resize lifecycle
- No missing fields in hash functions

The two cosmetic observations (parameter naming, byte buffer micro-opt) are not blocking and can be addressed in a follow-up if desired.

**Ready to merge.**
