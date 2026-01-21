# Attachment Streaming Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `n/a (no bead id provided)` — mandatory line tying the plan to the active bead/Task Master item.

**Goal:** Provide optional inline/streaming attachment payloads for non‑CLI consumers while preserving metadata-only defaults.

**Architecture:** Add a storage helper to read attachment bytes by `blob_hash` with size limits, expose an optional `include_attachments` flag in coordination fetch that returns base64 data for small files, and keep CLI unchanged. Use existing content-addressed storage in `.tandemonium/attachments/<hash>`.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), standard library.

### Task 1: Storage attachment reader

**Files:**
- Modify: `internal/storage/attachments.go`
- Test: `internal/storage/attachment_test.go`

**Step 1: Write the failing test**

Add to `internal/storage/attachment_test.go`:

```go
func TestReadAttachmentData(t *testing.T) {
    root := t.TempDir()
    if err := project.Init(root); err != nil {
        t.Fatal(err)
    }

    db, err := Open(project.StateDBPath(root))
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()
    if err := Migrate(db); err != nil {
        t.Fatal(err)
    }

    src := filepath.Join(root, "note.txt")
    if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
        t.Fatal(err)
    }

    msg := Message{ID: "msg-attach", Sender: "alice", Subject: "Attach", Body: "Body"}
    if err := SendMessage(db, msg, []string{"bob"}); err != nil {
        t.Fatalf("send message: %v", err)
    }

    attachments := []Attachment{{MessageID: "msg-attach", Path: src, Note: "spec"}}
    if err := AddAttachmentsWithStore(db, project.AttachmentsDir(root), "msg-attach", attachments); err != nil {
        t.Fatalf("add attachments: %v", err)
    }

    stored, err := ListAttachments(db, "msg-attach")
    if err != nil {
        t.Fatalf("list attachments: %v", err)
    }
    if len(stored) != 1 {
        t.Fatalf("expected 1 attachment")
    }

    data, err := ReadAttachmentData(project.AttachmentsDir(root), stored[0].BlobHash, 1024)
    if err != nil {
        t.Fatalf("read attachment: %v", err)
    }
    if string(data) != "hello" {
        t.Fatalf("expected attachment data")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/storage -run TestReadAttachmentData -v`

Expected: FAIL with undefined `ReadAttachmentData`.

**Step 3: Implement ReadAttachmentData**

In `internal/storage/attachments.go`, add:

```go
func ReadAttachmentData(storeDir, blobHash string, maxBytes int64) ([]byte, error) {
    if strings.TrimSpace(blobHash) == "" {
        return nil, errors.New("blob hash required")
    }
    path := filepath.Join(storeDir, blobHash[:2], blobHash)
    info, err := os.Stat(path)
    if err != nil {
        return nil, err
    }
    if maxBytes > 0 && info.Size() > maxBytes {
        return nil, fmt.Errorf("attachment exceeds limit")
    }
    return os.ReadFile(path)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/storage -run TestReadAttachmentData -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/storage/attachments.go internal/storage/attachment_test.go
git commit -m "feat: add attachment data reader"
```

### Task 2: Coordination fetch with inline attachments (opt-in)

**Files:**
- Modify: `internal/coordination/compat.go`
- Test: `internal/coordination/compat_test.go`

**Step 1: Write the failing test**

Add to `internal/coordination/compat_test.go`:

```go
func TestFetchInboxInlineAttachments(t *testing.T) {
    root := t.TempDir()
    if err := project.Init(root); err != nil {
        t.Fatal(err)
    }

    db, err := storage.Open(project.StateDBPath(root))
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()
    if err := storage.Migrate(db); err != nil {
        t.Fatal(err)
    }

    src := filepath.Join(root, "note.txt")
    if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
        t.Fatal(err)
    }

    if _, err := SendMessage(db, SendMessageRequest{
        MessageID:     "msg-attach",
        Sender:        "alice",
        Subject:       "Attach",
        Body:          "Body",
        To:            []string{"bob"},
        AttachmentDir: project.AttachmentsDir(root),
        Attachments:   []AttachmentRef{{Path: src, Note: "spec"}},
    }); err != nil {
        t.Fatalf("send message: %v", err)
    }

    inbox, err := FetchInbox(db, FetchInboxRequest{Recipient: "bob", Limit: 10, IncludeAttachments: true, AttachmentDir: project.AttachmentsDir(root)})
    if err != nil {
        t.Fatalf("fetch inbox: %v", err)
    }
    if len(inbox.Messages) != 1 || len(inbox.Messages[0].Attachments) != 1 || inbox.Messages[0].Attachments[0].Data == "" {
        t.Fatalf("expected inline attachment data")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/coordination -run TestFetchInboxInlineAttachments -v`

Expected: FAIL with missing fields.

**Step 3: Implement inline attachments**

In `internal/coordination/compat.go`:
- Extend `FetchInboxRequest` with `IncludeAttachments bool` and `AttachmentDir string`.
- Add `Data string` to `Attachment` payload in inbox (base64).
- When `IncludeAttachments` is true, read attachment bytes via `storage.ReadAttachmentData` and base64‑encode them (max 64KB per attachment).

**Step 4: Run test to verify it passes**

Run: `go test ./internal/coordination -run TestFetchInboxInlineAttachments -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/coordination/compat.go internal/coordination/compat_test.go
git commit -m "feat: add inline attachment fetch"
```

### Task 3: Document new capabilities

**Files:**
- Modify: `docs/plans/2026-01-16-mcp-compatibility.md`

**Step 1: Update compatibility notes**

Note that inline attachment payloads are now available via coordination fetch (opt-in), leaving only LLM summary examples as a gap.

**Step 2: Commit**

```bash
git add docs/plans/2026-01-16-mcp-compatibility.md
git commit -m "docs: note inline attachment fetch support"
```
