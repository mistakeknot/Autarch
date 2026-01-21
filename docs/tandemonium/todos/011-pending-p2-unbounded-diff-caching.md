---
status: pending
priority: p2
issue_id: "011"
tags: [performance, memory, code-review]
dependencies: []
---

# Unbounded Diff and Log File Caching

## Problem Statement

Diff loading reads entire file contents into memory without size limits. Large PRs or long session logs could cause memory exhaustion.

## Findings

**Location 1 - Diff Caching:** `/Users/sma/Tandemonium/internal/tui/review_diff.go` lines 52-74

```go
func buildReviewDiffState(...) (ReviewDiffState, error) {
    state.Cache = map[string][]string{}  // Unbounded cache
    for _, path := range files {
        lines, err := diff(path)  // Full file diff loaded
        state.Cache[path] = lines  // All diffs cached
    }
}
```

**Location 2 - Log Reading:** `/Users/sma/Tandemonium/internal/tui/review_detail.go` line 74

```go
if raw, err := os.ReadFile(logPath); err == nil {
    testsSummary = FindTestSummary(string(raw))
}
```

**Projected Impact:**
- Large diff (50K line file): ~50KB+ per file in Cache
- 100 files changed: ~5MB+ memory allocation
- 24-hour agent session log: 10-100MB+

## Proposed Solutions

### Option 1: Add Size Limits (Recommended)
- **Pros:** Prevents OOM, simple implementation
- **Cons:** May truncate large diffs
- **Effort:** Small
- **Risk:** Low

```go
// For logs - read only last 64KB
const maxLogBytes = 64 * 1024
f, _ := os.Open(logPath)
info, _ := f.Stat()
if info.Size() > maxLogBytes {
    f.Seek(-maxLogBytes, io.SeekEnd)
}

// For diffs - lazy load only current file
func (s *ReviewDiffState) GetDiff(path string) ([]string, error) {
    if cached, ok := s.Cache[path]; ok {
        return cached, nil
    }
    // Load on demand, evict others
}
```

## Recommended Action

Implement Option 1 for both log files and diff caching.

## Technical Details

- **Affected files:**
  - `internal/tui/review_diff.go`
  - `internal/tui/review_detail.go`
- **Components:** TUI
- **Database changes:** None

## Acceptance Criteria

- [ ] Log file reads limited to last 64KB
- [ ] Diff cache loads on-demand per file
- [ ] Previous files evicted when new ones loaded
- [ ] Tests verify memory stays bounded

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-01-12 | Finding identified during performance review | Unbounded memory for I/O is risky |

## Resources

- N/A
