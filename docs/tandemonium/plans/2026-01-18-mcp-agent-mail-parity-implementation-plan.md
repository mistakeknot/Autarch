# MCP Agent Mail Parity Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `n/a (no bead id provided)` — mandatory line tying the plan to the active bead/Task Master item.

**Goal:** Achieve full MCP Agent Mail parity with contact policy/request flows and persisted attachments using a content-addressed store.

**Architecture:** Store attachments under `.tandemonium/attachments/<sha256>` with hash-derived paths and metadata persisted in `attachments` rows. Enforce contact policies in the coordination layer so all entrypoints (CLI, future MCP) honor them. Add contact listing and JSON outputs for policy/contact commands. Include attachment metadata in inbox responses.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), Cobra CLI, standard library.

### Task 1: Attachment schema + content-addressed store

**Files:**
- Modify: `internal/storage/db.go`
- Modify: `internal/storage/attachments.go`
- Create: `internal/storage/attachment_store.go`
- Modify: `internal/project/paths.go`
- Modify: `internal/project/init.go`
- Test: `internal/storage/attachment_test.go`

**Step 1: Write the failing test**

Add to `internal/storage/attachment_test.go`:

```go
func TestAddAttachmentsPersistsContent(t *testing.T) {
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

    src := filepath.Join(root, "sample.txt")
    if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
        t.Fatal(err)
    }

    msg := storage.Message{ID: "msg-attach", Sender: "alice", Subject: "Attach", Body: "Body"}
    if err := storage.SendMessage(db, msg, []string{"bob"}); err != nil {
        t.Fatalf("send message: %v", err)
    }

    attachments := []storage.Attachment{{MessageID: "msg-attach", Path: src, Note: "spec"}}
    if err := storage.AddAttachmentsWithStore(db, project.AttachmentsDir(root), "msg-attach", attachments); err != nil {
        t.Fatalf("add attachments: %v", err)
    }

    got, err := storage.ListAttachments(db, "msg-attach")
    if err != nil {
        t.Fatalf("list attachments: %v", err)
    }
    if len(got) != 1 || got[0].BlobHash == "" {
        t.Fatalf("expected stored attachment with hash")
    }

    storedPath := filepath.Join(project.AttachmentsDir(root), got[0].BlobHash[:2], got[0].BlobHash)
    if _, err := os.Stat(storedPath); err != nil {
        t.Fatalf("expected stored file: %v", err)
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/storage -run TestAddAttachmentsPersistsContent -v`

Expected: FAIL with undefined `AttachmentsDir` or `AddAttachmentsWithStore`.

**Step 3: Add attachment path helpers**

In `internal/project/paths.go`, add:

```go
func AttachmentsDir(root string) string {
    return filepath.Join(root, ".tandemonium", "attachments")
}
```

In `internal/project/init.go`, add the directory to the init list:

```go
filepath.Join(projectDir, ".tandemonium", "attachments"),
```

**Step 4: Extend attachment schema + structs**

In `internal/storage/db.go`, add new columns for attachments and a migration safeguard:

```sql
ALTER TABLE attachments ADD COLUMN blob_hash TEXT;
ALTER TABLE attachments ADD COLUMN byte_size INTEGER;
ALTER TABLE attachments ADD COLUMN mime_type TEXT;
```

Then update `Attachment` in `internal/storage/attachments.go`:

```go
type Attachment struct {
    MessageID  string
    Path       string
    Note       string
    CreatedAt  string
    BlobHash   string
    ByteSize   int64
    MimeType   string
}
```

**Step 5: Implement content-addressed store**

Create `internal/storage/attachment_store.go`:

```go
func StoreAttachment(storeDir, srcPath string) (hash string, size int64, mime string, err error) {
    data, err := os.ReadFile(srcPath)
    if err != nil {
        return "", 0, "", err
    }
    sum := sha256.Sum256(data)
    hash = hex.EncodeToString(sum[:])
    size = int64(len(data))
    mime = http.DetectContentType(data)

    subdir := filepath.Join(storeDir, hash[:2])
    if err := os.MkdirAll(subdir, 0o755); err != nil {
        return "", 0, "", err
    }
    dst := filepath.Join(subdir, hash)
    if _, err := os.Stat(dst); err == nil {
        return hash, size, mime, nil
    }
    return hash, size, mime, os.WriteFile(dst, data, 0o644)
}
```

Then add in `internal/storage/attachments.go`:

```go
func AddAttachmentsWithStore(db *sql.DB, storeDir, messageID string, attachments []Attachment) error {
    for i := range attachments {
        if strings.TrimSpace(attachments[i].Path) == "" {
            return errors.New("attachment path required")
        }
        hash, size, mime, err := StoreAttachment(storeDir, attachments[i].Path)
        if err != nil {
            return err
        }
        attachments[i].BlobHash = hash
        attachments[i].ByteSize = size
        attachments[i].MimeType = mime
    }
    return AddAttachments(db, messageID, attachments)
}
```

Update `AddAttachments` to persist the new fields:

```go
if _, err := tx.Exec(`INSERT INTO attachments (message_id, path, note, created_ts, blob_hash, byte_size, mime_type)
    VALUES (?, ?, ?, ?, ?, ?, ?)`, msgID, path, att.Note, created, att.BlobHash, att.ByteSize, att.MimeType); err != nil {
    ...
}
```

Update `ListAttachments` to scan the new columns:

```go
rows, err := db.Query(`SELECT message_id, path, note, created_ts, blob_hash, byte_size, mime_type FROM attachments WHERE message_id = ? ORDER BY id`, messageID)
```

**Step 6: Run test to verify it passes**

Run: `go test ./internal/storage -run TestAddAttachmentsPersistsContent -v`

Expected: PASS.

**Step 7: Commit**

```bash
git add internal/project/paths.go internal/project/init.go internal/storage/db.go internal/storage/attachments.go internal/storage/attachment_store.go internal/storage/attachment_test.go
git commit -m "feat: persist attachments in content-addressed store"
```

### Task 2: Wire attachments into CLI + coordination + inbox responses

**Files:**
- Modify: `internal/cli/commands/mail.go`
- Modify: `internal/coordination/compat.go`
- Modify: `internal/coordination/compat_test.go`
- Modify: `internal/cli/commands/mail_test.go`
- Modify: `internal/storage/attachments.go`

**Step 1: Write the failing test**

Update `internal/coordination/compat_test.go`:

```go
func TestFetchInboxIncludesAttachments(t *testing.T) {
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

    _, err = SendMessage(db, SendMessageRequest{
        MessageID:     "msg-attach",
        Sender:        "alice",
        Subject:       "Attach",
        Body:          "Body",
        To:            []string{"bob"},
        AttachmentDir: project.AttachmentsDir(root),
        Attachments:   []AttachmentRef{{Path: src, Note: "spec"}},
    })
    if err != nil {
        t.Fatalf("send message: %v", err)
    }

    inbox, err := FetchInbox(db, FetchInboxRequest{Recipient: "bob", Limit: 10})
    if err != nil {
        t.Fatalf("fetch inbox: %v", err)
    }
    if len(inbox.Messages) != 1 || len(inbox.Messages[0].Attachments) != 1 {
        t.Fatalf("expected attachments in inbox")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/coordination -run TestFetchInboxIncludesAttachments -v`

Expected: FAIL with missing `AttachmentDir` or `Attachments` in inbox response.

**Step 3: Add attachment list helper**

In `internal/storage/attachments.go`, add:

```go
func ListAttachmentsForMessages(db *sql.DB, messageIDs []string) (map[string][]Attachment, error) {
    if len(messageIDs) == 0 {
        return map[string][]Attachment{}, nil
    }
    placeholders := make([]string, len(messageIDs))
    args := make([]interface{}, 0, len(messageIDs))
    for i, id := range messageIDs {
        placeholders[i] = "?"
        args = append(args, id)
    }
    query := fmt.Sprintf(`SELECT message_id, path, note, created_ts, blob_hash, byte_size, mime_type FROM attachments WHERE message_id IN (%s) ORDER BY id`, strings.Join(placeholders, ","))
    ...
}
```

**Step 4: Wire attachment persistence in coordination**

In `internal/coordination/compat.go`:
- Add `AttachmentDir string` to `SendMessageRequest`.
- Add `Attachments []storage.Attachment` to `InboxMessage` (or a summary struct).
- Use `storage.AddAttachmentsWithStore(db, req.AttachmentDir, msg.ID, attachments)` instead of `AddAttachments`.
- In `FetchInbox`, load attachments for message IDs using `ListAttachmentsForMessages` and attach to each message.

**Step 5: Wire attachment persistence in CLI**

In `internal/cli/commands/mail.go`, change attachment handling:

```go
storeDir := project.AttachmentsDir(root)
if err := storage.AddAttachmentsWithStore(db, storeDir, msg.ID, attachments); err != nil { ... }
```

Ensure attachment paths must exist (return error if missing) by checking `os.Stat` before calling `AddAttachmentsWithStore`.

**Step 6: Update CLI tests**

Update `TestMailSendWithAttachments` in `internal/cli/commands/mail_test.go` to create a temp file in the temp project dir and assert the stored blob exists under `.tandemonium/attachments`.

**Step 7: Run tests**

Run:
- `go test ./internal/coordination -run TestFetchInboxIncludesAttachments -v`
- `go test ./internal/cli/commands -run TestMailSendWithAttachments -v`

Expected: PASS.

**Step 8: Commit**

```bash
git add internal/coordination/compat.go internal/coordination/compat_test.go internal/cli/commands/mail.go internal/cli/commands/mail_test.go internal/storage/attachments.go
git commit -m "feat: include stored attachments in inbox and CLI"
```

### Task 3: Contact policy/request parity + contact listing

**Files:**
- Modify: `internal/storage/contact.go`
- Create: `internal/storage/contact_test.go`
- Modify: `internal/coordination/compat.go`
- Modify: `internal/cli/commands/mail.go`
- Modify: `internal/cli/commands/mail_test.go`

**Step 1: Write the failing test**

Create `internal/storage/contact_test.go`:

```go
func TestListContacts(t *testing.T) {
    db, err := storage.OpenTemp()
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()
    if err := storage.Migrate(db); err != nil {
        t.Fatal(err)
    }

    if err := storage.RequestContact(db, "alice", "bob"); err != nil {
        t.Fatalf("request: %v", err)
    }
    if err := storage.RespondContact(db, "alice", "bob", true); err != nil {
        t.Fatalf("respond: %v", err)
    }

    contacts, err := storage.ListContacts(db, "alice")
    if err != nil {
        t.Fatalf("list: %v", err)
    }
    if len(contacts) != 1 || contacts[0] != "bob" {
        t.Fatalf("expected bob")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/storage -run TestListContacts -v`

Expected: FAIL with undefined `ListContacts`.

**Step 3: Implement ListContacts and policy enforcement**

In `internal/storage/contact.go`, add:

```go
func ListContacts(db *sql.DB, owner string) ([]string, error) {
    if strings.TrimSpace(owner) == "" {
        return nil, errors.New("owner required")
    }
    rows, err := db.Query(`SELECT requester, recipient FROM contact_requests WHERE status = 'accepted' AND (requester = ? OR recipient = ?)`, owner, owner)
    ...
}
```

In `internal/coordination/compat.go`:
- Add `ListContactsRequest/Response` and `ListContacts` function.
- Enforce contact policies in `SendMessage` by calling `storage.GetContactPolicy`/`storage.HasAcceptedContact` for each recipient (same logic as `enforceContactPolicies` in CLI).

**Step 4: Update CLI commands with JSON outputs and list**

In `internal/cli/commands/mail.go`:
- Add `mail contact list --owner` (with `--json`) calling `storage.ListContacts`.
- Add `--json` to `mail policy set/get` and `mail contact request/respond`.

**Step 5: Update CLI tests**

Add a test in `internal/cli/commands/mail_test.go` that:
- Sets a `contacts_only` policy for `bob`.
- Attempts a send from `alice` to `bob` and expects an error.
- Then requests/accepts contact and re-sends successfully.

**Step 6: Run tests**

Run:
- `go test ./internal/storage -run TestListContacts -v`
- `go test ./internal/cli/commands -run TestMailContactPolicy -v`
- `go test ./internal/coordination -run TestSendMessageRespectsPolicy -v`

Expected: PASS.

**Step 7: Commit**

```bash
git add internal/storage/contact.go internal/storage/contact_test.go internal/coordination/compat.go internal/cli/commands/mail.go internal/cli/commands/mail_test.go
git commit -m "feat: add contact listing and policy parity"
```

### Task 4: Update compatibility notes + full test pass

**Files:**
- Modify: `docs/plans/2026-01-16-mcp-compatibility.md`
- Test: `./internal/...`

**Step 1: Update MCP compatibility notes**

In `docs/plans/2026-01-16-mcp-compatibility.md`, mark the gaps as closed and document any remaining deltas.

**Step 2: Run full coordination test suite**

Run: `go test ./internal/storage ./internal/coordination ./internal/cli/commands -v`

Expected: PASS.

**Step 3: Commit**

```bash
git add docs/plans/2026-01-16-mcp-compatibility.md
git commit -m "docs: update mcp compatibility notes"
```
