# Mail Search Pagination Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `n/a (no bead id provided)` — mandatory line tying the plan to the active bead/Task Master item.

**Goal:** Add opaque pagination tokens to mail search results in storage, coordination compat, and CLI JSON output.

**Architecture:** Reuse `(created_ts, message_id)` ordering and the existing `encodePageToken`/`decodePageToken` helpers from storage. Add `SearchMessagesPage` that returns `next_token` based on the last returned row. Wire through coordination compat and CLI `tand mail search` with a `--page-token` flag.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), Cobra CLI.

### Task 1: Storage pagination for search

**Files:**
- Modify: `internal/storage/coordination.go`
- Test: `internal/storage/coordination_test.go`

**Step 1: Write the failing test**

Add to `internal/storage/coordination_test.go`:

```go
func TestSearchMessagesPagination(t *testing.T) {
    db, err := OpenTemp()
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()
    if err := Migrate(db); err != nil {
        t.Fatal(err)
    }

    msgs := []Message{
        {ID: "s1", Sender: "alice", Subject: "Hello", Body: "Body", CreatedAt: "2026-01-01T00:00:01Z"},
        {ID: "s2", Sender: "alice", Subject: "Hello", Body: "Body", CreatedAt: "2026-01-01T00:00:02Z"},
        {ID: "s3", Sender: "alice", Subject: "Hello", Body: "Body", CreatedAt: "2026-01-01T00:00:03Z"},
    }
    for _, msg := range msgs {
        if err := SendMessage(db, msg, []string{"bob"}); err != nil {
            t.Fatalf("send message: %v", err)
        }
    }

    page1, next, err := SearchMessagesPage(db, "Hello", 2, "")
    if err != nil {
        t.Fatalf("search page1: %v", err)
    }
    if len(page1) != 2 || next == "" {
        t.Fatalf("expected page1 with next token")
    }

    page2, next2, err := SearchMessagesPage(db, "Hello", 2, next)
    if err != nil {
        t.Fatalf("search page2: %v", err)
    }
    if len(page2) != 1 || next2 != "" {
        t.Fatalf("expected page2 final")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/storage -run TestSearchMessagesPagination -v`

Expected: FAIL with undefined `SearchMessagesPage`.

**Step 3: Implement SearchMessagesPage**

In `internal/storage/coordination.go`, add:

```go
func SearchMessagesPage(db *sql.DB, query string, limit int, pageToken string) ([]Message, string, error) {
    if strings.TrimSpace(query) == "" {
        return []Message{}, "", nil
    }
    if limit <= 0 {
        limit = 50
    }
    term := "%" + query + "%"
    q := `SELECT id, thread_id, sender, subject, body, created_ts, importance, ack_required, metadata
          FROM messages
          WHERE subject LIKE ? OR body LIKE ? OR sender LIKE ?`
    args := []interface{}{term, term, term}
    if strings.TrimSpace(pageToken) != "" {
        ts, id, err := decodePageToken(pageToken)
        if err != nil {
            return nil, "", err
        }
        q += " AND (created_ts < ? OR (created_ts = ? AND id < ?))"
        args = append(args, ts, ts, id)
    }
    q += " ORDER BY created_ts DESC, id DESC LIMIT ?"
    args = append(args, limit+1)
    ...
}
```

If more than `limit`, return `next_token` derived from the last returned row and trim to `limit`.

Update `SearchMessages` to call `SearchMessagesPage(..., "")` and ignore the token.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/storage -run TestSearchMessagesPagination -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/storage/coordination.go internal/storage/coordination_test.go
git commit -m "feat: add pagination to search messages"
```

### Task 2: Wire pagination through coordination compat

**Files:**
- Modify: `internal/coordination/compat.go`
- Test: `internal/coordination/compat_test.go`

**Step 1: Write the failing test**

Add to `internal/coordination/compat_test.go`:

```go
func TestSearchMessagesPaginationToken(t *testing.T) {
    db, err := storage.OpenTemp()
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()
    if err := storage.Migrate(db); err != nil {
        t.Fatal(err)
    }

    for i := 0; i < 3; i++ {
        if _, err := SendMessage(db, SendMessageRequest{
            MessageID: fmt.Sprintf("s-%d", i),
            Sender: "alice",
            Subject: "Hello",
            Body: "Body",
            To: []string{"bob"},
        }); err != nil {
            t.Fatalf("send: %v", err)
        }
    }

    resp, err := SearchMessages(db, SearchMessagesRequest{Query: "Hello", Limit: 2})
    if err != nil {
        t.Fatalf("search: %v", err)
    }
    if resp.NextToken == "" {
        t.Fatalf("expected next token")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/coordination -run TestSearchMessagesPaginationToken -v`

Expected: FAIL with missing `NextToken`.

**Step 3: Implement token passthrough**

In `internal/coordination/compat.go`:
- Add `PageToken string` to `SearchMessagesRequest`.
- Add `NextToken string` to `SearchMessagesResponse`.
- Use `storage.SearchMessagesPage` and return `NextToken`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/coordination -run TestSearchMessagesPaginationToken -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/coordination/compat.go internal/coordination/compat_test.go
git commit -m "feat: add search pagination to compat"
```

### Task 3: Add CLI `--page-token` for mail search

**Files:**
- Modify: `internal/cli/commands/mail.go`
- Test: `internal/cli/commands/mail_test.go`

**Step 1: Write the failing test**

Add to `internal/cli/commands/mail_test.go`:

```go
func TestMailSearchPaginationJson(t *testing.T) {
    dir := t.TempDir()
    if err := project.Init(dir); err != nil {
        t.Fatal(err)
    }
    cwd, err := os.Getwd()
    if err != nil {
        t.Fatal(err)
    }
    defer func() { _ = os.Chdir(cwd) }()
    if err := os.Chdir(dir); err != nil {
        t.Fatal(err)
    }

    for i := 0; i < 3; i++ {
        send := MailCmd()
        send.SetOut(bytes.NewBuffer(nil))
        send.SetArgs([]string{"send", "--to", "bob", "--subject", "Hello", "--body", "Body"})
        if err := send.Execute(); err != nil {
            t.Fatalf("send failed: %v", err)
        }
    }

    search := MailCmd()
    out := bytes.NewBuffer(nil)
    search.SetOut(out)
    search.SetArgs([]string{"search", "--query", "Hello", "--limit", "2", "--json"})
    if err := search.Execute(); err != nil {
        t.Fatalf("search failed: %v", err)
    }

    var payload struct {
        Messages  []storage.Message `json:"messages"`
        NextToken string            `json:"next_token"`
    }
    if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
        t.Fatalf("decode json: %v", err)
    }
    if payload.NextToken == "" {
        t.Fatalf("expected next token")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/commands -run TestMailSearchPaginationJson -v`

Expected: FAIL with missing `next_token`.

**Step 3: Implement CLI support**

In `internal/cli/commands/mail.go`:
- Add `--page-token` flag to `mail search`.
- Use `storage.SearchMessagesPage` instead of `SearchMessages`.
- JSON output should include `next_token`.

**Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/commands -run TestMailSearchPaginationJson -v`

Expected: PASS.

**Step 5: Commit**

```bash
git add internal/cli/commands/mail.go internal/cli/commands/mail_test.go
git commit -m "feat: add search pagination to mail cli"
```
