package coordination

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/config"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

func TestSendMessageAndFetchInbox(t *testing.T) {
	db, err := storage.OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		t.Fatal(err)
	}

	req := SendMessageRequest{
		MessageID:   "msg-1",
		ThreadID:    "thread-1",
		Sender:      "alice",
		Subject:     "Hello",
		Body:        "Body",
		To:          []string{"bob"},
		Cc:          []string{"carol"},
		Bcc:         []string{"dave"},
		Importance:  "high",
		AckRequired: true,
	}
	if _, err := SendMessage(db, req); err != nil {
		t.Fatalf("send message: %v", err)
	}

	inbox, err := FetchInbox(db, FetchInboxRequest{Recipient: "bob", Limit: 10})
	if err != nil {
		t.Fatalf("fetch inbox: %v", err)
	}
	if len(inbox.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(inbox.Messages))
	}
	if inbox.Messages[0].ID != "msg-1" {
		t.Fatalf("expected msg-1, got %s", inbox.Messages[0].ID)
	}
	if !inbox.Messages[0].AckRequired {
		t.Fatalf("expected ack required")
	}

	ccInbox, err := FetchInbox(db, FetchInboxRequest{Recipient: "carol", Limit: 10})
	if err != nil {
		t.Fatalf("fetch inbox: %v", err)
	}
	if len(ccInbox.Messages) != 1 {
		t.Fatalf("expected 1 cc message, got %d", len(ccInbox.Messages))
	}

	bccInbox, err := FetchInbox(db, FetchInboxRequest{Recipient: "dave", Limit: 10})
	if err != nil {
		t.Fatalf("fetch inbox: %v", err)
	}
	if len(bccInbox.Messages) != 1 {
		t.Fatalf("expected 1 bcc message, got %d", len(bccInbox.Messages))
	}
}

func TestAckMessage(t *testing.T) {
	db, err := storage.OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		t.Fatal(err)
	}

	if _, err := SendMessage(db, SendMessageRequest{
		MessageID: "msg-2",
		Sender:    "alice",
		Subject:   "Hello",
		Body:      "Body",
		To:        []string{"bob"},
	}); err != nil {
		t.Fatalf("send message: %v", err)
	}

	if _, err := AckMessage(db, AckMessageRequest{MessageID: "msg-2", Recipient: "bob"}); err != nil {
		t.Fatalf("ack message: %v", err)
	}
}

func TestReservePathsAndRelease(t *testing.T) {
	db, err := storage.OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		t.Fatal(err)
	}

	res, err := ReservePaths(db, ReservePathsRequest{Owner: "alice", Paths: []string{"a.go"}, Exclusive: true})
	if err != nil {
		t.Fatalf("reserve: %v", err)
	}
	if len(res.Granted) != 1 {
		t.Fatalf("expected grant")
	}

	conflict, err := ReservePaths(db, ReservePathsRequest{Owner: "bob", Paths: []string{"a.go"}, Exclusive: true})
	if err != nil {
		t.Fatalf("reserve conflict: %v", err)
	}
	if len(conflict.Conflicts) != 1 {
		t.Fatalf("expected conflict")
	}

	if _, err := ReleasePaths(db, ReleasePathsRequest{Owner: "alice", Paths: []string{"a.go"}}); err != nil {
		t.Fatalf("release: %v", err)
	}
}

func TestSearchAndSummarize(t *testing.T) {
	db, err := storage.OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		t.Fatal(err)
	}

	if _, err := SendMessage(db, SendMessageRequest{
		MessageID: "msg-3",
		ThreadID:  "thread-3",
		Sender:    "alice",
		Subject:   "Searchable",
		Body:      "Body",
		To:        []string{"bob"},
	}); err != nil {
		t.Fatalf("send message: %v", err)
	}

	search, err := SearchMessages(db, SearchMessagesRequest{Query: "Searchable", Limit: 10})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(search.Messages) != 1 {
		t.Fatalf("expected 1 search result")
	}

	summary, err := SummarizeThread(db, SummarizeThreadRequest{ThreadID: "thread-3"})
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if summary.MessageCount != 1 {
		t.Fatalf("expected 1 message")
	}
}

func TestMarkRead(t *testing.T) {
	db, err := storage.OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		t.Fatal(err)
	}

	if _, err := SendMessage(db, SendMessageRequest{
		MessageID: "msg-read",
		Sender:    "alice",
		Subject:   "Read",
		Body:      "Body",
		To:        []string{"bob"},
	}); err != nil {
		t.Fatalf("send message: %v", err)
	}

	if _, err := MarkRead(db, MarkReadRequest{MessageID: "msg-read", Recipient: "bob"}); err != nil {
		t.Fatalf("mark read: %v", err)
	}
}

func TestSendMessageWithAttachments(t *testing.T) {
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
		Attachments: []AttachmentRef{
			{Path: src, Note: "spec"},
		},
	}); err != nil {
		t.Fatalf("send message: %v", err)
	}

	attachments, err := storage.ListAttachments(db, "msg-attach")
	if err != nil {
		t.Fatalf("list attachments: %v", err)
	}
	if len(attachments) != 1 {
		t.Fatalf("expected 1 attachment")
	}
}

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

	inbox, err := FetchInbox(db, FetchInboxRequest{Recipient: "bob", Limit: 10})
	if err != nil {
		t.Fatalf("fetch inbox: %v", err)
	}
	if len(inbox.Messages) != 1 || len(inbox.Messages[0].Attachments) != 1 {
		t.Fatalf("expected attachments in inbox")
	}
}

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

	inbox, err := FetchInbox(db, FetchInboxRequest{
		Recipient:          "bob",
		Limit:              10,
		IncludeAttachments: true,
		AttachmentDir:      project.AttachmentsDir(root),
	})
	if err != nil {
		t.Fatalf("fetch inbox: %v", err)
	}
	if len(inbox.Messages) != 1 || len(inbox.Messages[0].Attachments) != 1 || inbox.Messages[0].Attachments[0].Data == "" {
		t.Fatalf("expected inline attachment data")
	}
}

func TestSendMessageRespectsPolicy(t *testing.T) {
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

	if err := storage.SetContactPolicy(db, "bob", "contacts_only"); err != nil {
		t.Fatalf("set policy: %v", err)
	}

	if _, err := SendMessage(db, SendMessageRequest{
		MessageID: "msg-policy",
		Sender:    "alice",
		Subject:   "Hello",
		Body:      "Body",
		To:        []string{"bob"},
	}); err == nil {
		t.Fatalf("expected policy enforcement error")
	}

	if err := storage.RequestContact(db, "alice", "bob"); err != nil {
		t.Fatalf("request contact: %v", err)
	}
	if err := storage.RespondContact(db, "alice", "bob", true); err != nil {
		t.Fatalf("respond contact: %v", err)
	}

	if _, err := SendMessage(db, SendMessageRequest{
		MessageID: "msg-policy-2",
		Sender:    "alice",
		Subject:   "Hello",
		Body:      "Body",
		To:        []string{"bob"},
	}); err != nil {
		t.Fatalf("send after contact: %v", err)
	}
}

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
			Sender:    "alice",
			Subject:   "Hello",
			Body:      "Body",
			To:        []string{"bob"},
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
		Sender:    "alice",
		Subject:   "Meta",
		Body:      "Body",
		To:        []string{"bob"},
		Cc:        []string{"carol"},
		Bcc:       []string{"dave"},
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
			Sender:    "alice",
			Subject:   "Hello",
			Body:      "Body",
			To:        []string{"bob"},
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

func TestSummarizeThreadWithLLM(t *testing.T) {
	db, err := storage.OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		t.Fatal(err)
	}

	tmp := t.TempDir()
	cmdPath := filepath.Join(tmp, "summary.sh")
	script := "#!/bin/sh\necho '{\"summary\":{\"participants\":[\"alice\"],\"key_points\":[\"p1\"],\"action_items\":[]},\"examples\":[{\"id\":\"m1\",\"subject\":\"Hello\",\"body\":\"Body\"}]}'\n"
	if err := os.WriteFile(cmdPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := SendMessage(db, SendMessageRequest{
		MessageID: "m1",
		ThreadID:  "thread-llm",
		Sender:    "alice",
		Subject:   "Hello",
		Body:      "Body",
		To:        []string{"bob"},
	}); err != nil {
		t.Fatalf("send: %v", err)
	}

	resp, err := SummarizeThread(db, SummarizeThreadRequest{
		ThreadID:        "thread-llm",
		IncludeExamples: true,
		LLMMode:         true,
		LLMConfig:       config.LLMSummaryConfig{Command: cmdPath, TimeoutSeconds: 5},
	})
	if err != nil {
		t.Fatalf("summarize: %v", err)
	}
	if len(resp.Examples) == 0 || len(resp.KeyPoints) == 0 {
		t.Fatalf("expected examples and key points")
	}
}
