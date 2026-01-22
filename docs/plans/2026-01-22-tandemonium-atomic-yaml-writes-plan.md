# Tandemonium Atomic YAML Writes Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `none (no bead in use)`

**Goal:** Make all Tandemonium YAML spec writes atomic and protected by advisory file locks to prevent corruption under crashes or concurrent access.

**Architecture:** Introduce a shared atomic write + lock utility (`internal/file/atomic.go`) that acquires an advisory lock, writes to a temp file in the same directory, fsyncs, renames, and fsyncs the directory. Update specs write paths (`internal/tandemonium/specs/review.go`, `internal/tandemonium/specs/create.go`) to use this helper. Provide tests for atomicity and lock behavior, with lock tests gated on non-Windows.

**Tech Stack:** Go, `database/sql` (unchanged), filesystem primitives (`os`, `filepath`, `sync`, `syscall` on Unix).

---

### Task 1: Add failing tests for atomic write + locking

**Files:**
- Create: `internal/file/atomic_test.go`
- Modify: `internal/tandemonium/specs/review_test.go`

**Step 1: Write failing test (atomic write creates output, no temp left)**

```go
package file

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestAtomicWriteFileCreatesFile(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "out.yaml")
    if err := AtomicWriteFile(path, []byte("id: T1\n"), 0o644); err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if _, err := os.Stat(path); err != nil {
        t.Fatalf("expected output file")
    }
    entries, err := os.ReadDir(dir)
    if err != nil {
        t.Fatal(err)
    }
    for _, entry := range entries {
        if strings.HasPrefix(entry.Name(), ".tmp-") {
            t.Fatalf("expected temp file removed, found %s", entry.Name())
        }
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/file -run TestAtomicWriteFileCreatesFile`

Expected: FAIL with compile error `AtomicWriteFile` undefined.

**Step 3: Write failing test (lock blocks concurrent writes)**

```go
//go:build !windows

package file

import (
    "path/filepath"
    "testing"
    "time"
)

func TestAtomicWriteFileBlocksWhenLocked(t *testing.T) {
    dir := t.TempDir()
    path := filepath.Join(dir, "out.yaml")

    lock, err := LockFile(path)
    if err != nil {
        t.Fatalf("unexpected lock error: %v", err)
    }

    done := make(chan struct{})
    go func() {
        _ = AtomicWriteFile(path, []byte("id: T2\n"), 0o644)
        close(done)
    }()

    select {
    case <-done:
        t.Fatal("expected write to block while locked")
    case <-time.After(50 * time.Millisecond):
    }

    if err := lock.Unlock(); err != nil {
        t.Fatalf("unexpected unlock error: %v", err)
    }

    select {
    case <-done:
    case <-time.After(1 * time.Second):
        t.Fatal("expected write to finish after unlock")
    }
}
```

**Step 4: Run test to verify it fails**

Run: `go test ./internal/file -run TestAtomicWriteFileBlocksWhenLocked`

Expected: FAIL with compile error `LockFile` undefined.

**Step 5: Update existing specs test to use new helper**

In `internal/tandemonium/specs/review_test.go`, change `TestWriteFileAtomic` to call `file.AtomicWriteFile` and drop the `.tmp` exact name check.

---

### Task 2: Implement atomic write + file locking helper

**Files:**
- Create: `internal/file/atomic.go`
- Create: `internal/file/lock_unix.go`
- Create: `internal/file/lock_windows.go`

**Step 1: Write minimal implementation**

`internal/file/atomic.go`

```go
package file

import (
    "os"
    "path/filepath"
)

func AtomicWriteFile(path string, data []byte, perm os.FileMode) error {
    lock, err := LockFile(path)
    if err != nil {
        return err
    }
    defer lock.Unlock()

    dir := filepath.Dir(path)
    tmp, err := os.CreateTemp(dir, ".tmp-")
    if err != nil {
        return err
    }
    tmpName := tmp.Name()
    if _, err := tmp.Write(data); err != nil {
        _ = tmp.Close()
        _ = os.Remove(tmpName)
        return err
    }
    if err := tmp.Sync(); err != nil {
        _ = tmp.Close()
        _ = os.Remove(tmpName)
        return err
    }
    if err := tmp.Close(); err != nil {
        _ = os.Remove(tmpName)
        return err
    }
    if err := os.Rename(tmpName, path); err != nil {
        _ = os.Remove(tmpName)
        return err
    }
    dirHandle, err := os.Open(dir)
    if err != nil {
        return err
    }
    defer dirHandle.Close()
    return dirHandle.Sync()
}
```

`internal/file/lock_unix.go`

```go
//go:build !windows

package file

import (
    "os"
    "syscall"
)

type FileLock struct{ f *os.File }

func LockFile(path string) (*FileLock, error) {
    lockPath := path + ".lock"
    f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
    if err != nil {
        return nil, err
    }
    if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
        _ = f.Close()
        return nil, err
    }
    return &FileLock{f: f}, nil
}

func (l *FileLock) Unlock() error {
    if l == nil || l.f == nil {
        return nil
    }
    if err := syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN); err != nil {
        _ = l.f.Close()
        return err
    }
    return l.f.Close()
}
```

`internal/file/lock_windows.go`

```go
//go:build windows

package file

import "os"

type FileLock struct{ f *os.File }

func LockFile(path string) (*FileLock, error) {
    lockPath := path + ".lock"
    f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
    if err != nil {
        return nil, err
    }
    return &FileLock{f: f}, nil
}

func (l *FileLock) Unlock() error {
    if l == nil || l.f == nil {
        return nil
    }
    return l.f.Close()
}
```

**Step 2: Run tests to verify they pass**

Run: `go test ./internal/file`

Expected: PASS.

---

### Task 3: Use atomic write helper in specs

**Files:**
- Modify: `internal/tandemonium/specs/review.go`
- Modify: `internal/tandemonium/specs/create.go`
- Modify: `internal/tandemonium/specs/review_test.go`

**Step 1: Update specs write paths**

- Replace `writeFileAtomic` with `file.AtomicWriteFile` and remove the local helper.
- Use `file.AtomicWriteFile` in `CreateQuickSpec` instead of `os.WriteFile`.

**Step 2: Run tests to verify they pass**

Run: `go test ./internal/tandemonium/specs -run TestWriteFileAtomic`

Expected: PASS.

---

### Task 4: Run targeted suite, update todo, commit

**Files:**
- Modify: `docs/tandemonium/todos/005-pending-p1-non-atomic-yaml-writes.md`

**Step 1: Run relevant tests**

Run: `go test ./internal/file ./internal/tandemonium/specs`

Expected: PASS.

**Step 2: Update todo**

Mark todo as done and note atomic+lock helper usage across spec writes.

**Step 3: Commit**

```bash
git add internal/file/atomic.go internal/file/lock_unix.go internal/file/lock_windows.go internal/file/atomic_test.go internal/tandemonium/specs/review.go internal/tandemonium/specs/create.go internal/tandemonium/specs/review_test.go docs/tandemonium/todos/005-pending-p1-non-atomic-yaml-writes.md docs/plans/2026-01-22-tandemonium-atomic-yaml-writes-plan.md
git commit -m "fix(tandemonium): make YAML writes atomic"
```

---

Plan complete and saved to `docs/plans/2026-01-22-tandemonium-atomic-yaml-writes-plan.md`.

Two execution options:

1. Subagent-Driven (this session) — I dispatch a fresh subagent per task, review between tasks
2. Parallel Session (separate) — Open a new session with executing-plans and batch execution

Which approach?
