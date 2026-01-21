# Inbox Pagination + CC/BCC Metadata Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Bead:** `n/a (no bead id provided)` — mandatory line tying the plan to the active bead/Task Master item.

**Goal:** Add opaque inbox pagination tokens and surface CC/BCC metadata in inbox payloads.

**Architecture:** Store recipient lists in message metadata (`to`, `cc`, `bcc`) and parse them for inbox payloads. Add token-based pagination using `(created_ts, message_id)` ordering with `next_token` returned for subsequent pages.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), Cobra CLI.

### Task 1: Add inbox pagination tokens in storage + coordination

**Files:**
- Modify: `internal/storage/coordination.go`
- Modify: `internal/coordination/compat.go`
- Test: `internal/storage/coordination_test.go`
- Test: `internal/coordination/compat_test.go`

**Step 1: Write the failing test**

Add to `internal/storage/coordination_test.go`:

```go
func TestFetchInboxPagination(t *testing.T) {
    db, err := OpenTemp()
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()
    if err := Migrate(db); err != nil {
        t.Fatal(err)
    }

    msgs := []Message{
        {ID: "m1", Sender: "a", Subject: "1", Body: "b", CreatedAt: "2026-01-01T00:00:01Z"},
        {ID: "m2", Sender: "a", Subject: "2", Body: "b", CreatedAt: "2026-01-01T00:00:02Z"},
        {ID: "m3", Sender: "a", Subject: "3", Body: "b", CreatedAt: "2026-01-01T00:00:03Z"},
    }
    for _, msg := range msgs {
        if err := SendMessage(db, msg, []string{"bob"}); err != nil {
            t.Fatalf("send message: %v", err)
        }
    }

    page1, next, err := FetchInboxPage(db, "bob", 2, "", false, "")
    if err != nil {
        t.Fatalf("fetch page1: %v", err)
    }
    if len(page1) != 2 || next == "" {
        t.Fatalf("expected page1 with next token")
    }

    page2, next2, err := FetchInboxPage(db, "bob", 2, "", false, next)
    if err != nil {
        t.Fatalf("fetch page2: %v", err)
    }
    if len(page2) != 1 || next2 != "" {
        t.Fatalf("expected page2 final")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/storage -run TestFetchInboxPagination -v`

Expected: FAIL with undefined `FetchInboxPage`.

**Step 3: Implement token pagination**

In `internal/storage/coordination.go`, add:
- `encodePageToken(ts, id string) string`
- `decodePageToken(token string) (ts, id string, err error)`
- `FetchInboxPage(db, recipient string, limit int, sinceTs string, urgentOnly bool, pageToken string) ([]MessageDelivery, string, error)`

Use ordering `ORDER BY m.created_ts DESC, m.id DESC` and filter by token:

```sql
AND (m.created_ts < ? OR (m.created_ts = ? AND m.id < ?))
```

Fetch `limit+1` rows; if more than `limit`, trim and return `next_token` built from the last returned row’s `created_ts` + `id`.

Update `FetchInboxWithFilters` to call `FetchInboxPage(..., "")` and ignore the token.

**Step 4: Wire into coordination compat**

In `internal/coordination/compat.go`:
- Add `PageToken string` to `FetchInboxRequest`.
- Add `NextToken string` to `FetchInboxResponse`.
- Use `storage.FetchInboxPage` and return `NextToken`.

**Step 5: Add coordination test**

In `internal/coordination/compat_test.go`, add:

```go
func TestFetchInboxPaginationToken(t *testing.T) {
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
            MessageID: fmt.Sprintf("msg-%d", i),
            Sender: "alice",
            Subject: "Hello",
            Body: "Body",
            To: []string{"bob"},
        }); err != nil {
            t.Fatalf("send: %v", err)
        }
    }

    inbox, err := FetchInbox(db, FetchInboxRequest{Recipient: "bob", Limit: 2})
    if err != nil {
        t.Fatalf("inbox: %v", err)
    }
    if inbox.NextToken == "" {
        t.Fatalf("expected next token")
    }
}
```

**Step 6: Run tests**

Run:
- `go test ./internal/storage -run TestFetchInboxPagination -v`
- `go test ./internal/coordination -run TestFetchInboxPaginationToken -v`

Expected: PASS.

**Step 7: Commit**

```bash
git add internal/storage/coordination.go internal/storage/coordination_test.go internal/coordination/compat.go internal/coordination/compat_test.go
git commit -m "feat: add inbox pagination tokens"
```

### Task 2: Surface CC/BCC metadata in inbox payloads

**Files:**
- Modify: `internal/storage/coordination.go`
- Modify: `internal/coordination/compat.go`
- Modify: `internal/cli/commands/mail.go`
- Test: `internal/coordination/compat_test.go`
- Test: `internal/cli/commands/mail_test.go`

**Step 1: Write the failing test**

Add to `internal/coordination/compat_test.go`:

```go
func TestInboxIncludesRecipientMetadata(t *testing.T) {
    db, err := storage.OpenTemp()
    if err != nil {
        t.Fatal(err)
    }
    defer db.Close()
    if err := storage.Migrate(db); err != nil {
        t.Fatal(err)
    }

    _, err = SendMessage(db, SendMessageRequest{
        MessageID: "msg-meta",
        Sender: "alice",
        Subject: "Meta",
        Body: "Body",
        To: []string{"bob"},
        Cc: []string{"carol"},
        Bcc: []string{"dave"},
    })
    if err != nil {
        t.Fatalf("send: %v", err)
    }

    inbox, err := FetchInbox(db, FetchInboxRequest{Recipient: "bob", Limit: 10})
    if err != nil {
        t.Fatalf("inbox: %v", err)
    }
    if len(inbox.Messages) != 1 || len(inbox.Messages[0].Cc) != 1 || len(inbox.Messages[0].Bcc) != 1 {
        t.Fatalf("expected cc/bcc metadata")
    }
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/coordination -run TestInboxIncludesRecipientMetadata -v`

Expected: FAIL with missing `Cc/Bcc` in inbox message.

**Step 3: Implement metadata merge + parse**

In `internal/storage/coordination.go`, add helpers:

```go
func MergeRecipientMetadata(existing string, to, cc, bcc []string) (string, error)
func ParseRecipientMetadata(metadata string) (to, cc, bcc []string)
```

Behavior:
- If `existing` is empty, create a new map.
- If `existing` is invalid JSON, wrap as `{ "_raw_metadata": "..." }`.
- Always set `to`, `cc`, `bcc` arrays.

**Step 4: Wire metadata into sends**

In `internal/coordination/compat.go`:
- Merge recipients into metadata before storing.
- Add `To`, `Cc`, `Bcc` slices to `InboxMessage` and populate them from parsed metadata.

In `internal/cli/commands/mail.go`:
- When sending, merge `--to/--cc/--bcc` into metadata JSON before calling `storage.SendMessage`.
- Include `to/cc/bcc` in JSON inbox output payloads.

**Step 5: Run tests**

Run:
- `go test ./internal/coordination -run TestInboxIncludesRecipientMetadata -v`
- `go test ./internal/cli/commands -run TestMailSendCcBcc -v`

Expected: PASS.

**Step 6: Commit**

```bash
git add internal/storage/coordination.go internal/coordination/compat.go internal/coordination/compat_test.go internal/cli/commands/mail.go internal/cli/commands/mail_test.go
git commit -m "feat: include cc/bcc metadata in inbox payloads"
```
