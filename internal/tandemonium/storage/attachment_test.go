package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
)

func TestAddAndListAttachments(t *testing.T) {
	db, err := OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := Migrate(db); err != nil {
		t.Fatal(err)
	}

	msg := Message{
		ID:         "msg-attach",
		ThreadID:   "thread-attach",
		Sender:     "alice",
		Subject:    "Attach",
		Body:       "Body",
		CreatedAt:  "2026-01-16T00:00:00Z",
		Importance: "normal",
	}
	if err := SendMessage(db, msg, []string{"bob"}); err != nil {
		t.Fatalf("send message: %v", err)
	}

	attachments := []Attachment{
		{MessageID: "msg-attach", Path: "README.md", Note: "spec"},
		{MessageID: "msg-attach", Path: "docs/plan.md", Note: ""},
	}
	if err := AddAttachments(db, "msg-attach", attachments); err != nil {
		t.Fatalf("add attachments: %v", err)
	}

	got, err := ListAttachments(db, "msg-attach")
	if err != nil {
		t.Fatalf("list attachments: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(got))
	}
}

func TestAddAttachmentsPersistsContent(t *testing.T) {
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

	src := filepath.Join(root, "sample.txt")
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

	got, err := ListAttachments(db, "msg-attach")
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
