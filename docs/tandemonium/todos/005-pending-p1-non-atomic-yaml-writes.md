---
status: done
priority: p1
issue_id: "005"
tags: [data-integrity, code-review]
dependencies: []
---

# Non-Atomic YAML File Writes

## Problem Statement

The production Go code uses non-atomic writes that could result in data corruption or loss during crashes, power failures, or concurrent access.

## Findings

**Location:** `/Users/sma/Tandemonium/internal/specs/review.go` lines 23-27

```go
out, err := yaml.Marshal(doc)
if err != nil {
    return err
}
return os.WriteFile(path, out, 0o644)  // Non-atomic!
```

**Risk Scenarios:**
- Process crash during `os.WriteFile` results in truncated/corrupted YAML
- Concurrent TUI instances writing to same spec file causes race condition
- Power failure mid-write leaves partial data

**Evidence of Planned Solution:**
The Rust prototype at `/Users/sma/Tandemonium/prototypes/m0-yaml/src/main.rs` implements the correct atomic pattern (write-to-temp, fsync, rename), but this has **not been ported to the Go codebase**.

## Proposed Solutions

### Option 1: Atomic Write Pattern (Recommended)
- **Pros:** Industry standard, prevents corruption
- **Cons:** Slightly more complex
- **Effort:** Small
- **Risk:** Low

```go
func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
    dir := filepath.Dir(path)
    tmp, err := os.CreateTemp(dir, ".tmp-")
    if err != nil {
        return err
    }
    defer os.Remove(tmp.Name()) // Cleanup on failure

    if _, err := tmp.Write(data); err != nil {
        tmp.Close()
        return err
    }
    if err := tmp.Sync(); err != nil {  // Ensure data is on disk
        tmp.Close()
        return err
    }
    if err := tmp.Close(); err != nil {
        return err
    }
    return os.Rename(tmp.Name(), path)  // Atomic on POSIX
}
```

### Option 2: Advisory File Locking
- **Pros:** Prevents concurrent access
- **Cons:** Doesn't help with crash recovery
- **Effort:** Medium
- **Risk:** Low

## Recommended Action

Implement Option 1 (atomic writes) AND Option 2 (file locking) for complete protection.

## Technical Details

- **Affected files:**
  - `internal/specs/review.go` (UpdateUserStory, AppendReviewFeedback)
  - Create `internal/file/atomic.go` for shared utility
- **Components:** Specs, potentially all file writes
- **Database changes:** None

## Resolution

Added shared atomic write + advisory lock helper and updated specs writes to use it.

## Acceptance Criteria

- [x] Atomic write utility function created
- [x] All spec file writes use atomic pattern
- [x] Test verifies partial writes don't corrupt files
- [x] Test verifies concurrent writes are handled safely

## Work Log

| Date | Action | Learnings |
|------|--------|-----------|
| 2026-01-12 | Finding identified during data integrity review | Rust prototype has the right pattern - port it |
| 2026-01-22 | Implemented atomic writes + advisory locking with tests | Prevents corrupted YAML under crash/concurrency |

## Resources

- Atomic file writes: https://blog.gopheracademy.com/advent-2017/a-tale-of-two-rands/
