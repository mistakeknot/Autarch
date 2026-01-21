package tui

import (
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

func TestLoadCoordInboxUrgentOnly(t *testing.T) {
	db, err := storage.OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		t.Fatal(err)
	}

	normal := storage.Message{
		ID:         "msg-normal",
		ThreadID:   "thread-normal",
		Sender:     "alice",
		Subject:    "Normal update",
		Body:       "Body",
		CreatedAt:  "2026-01-16T00:00:00Z",
		Importance: "normal",
	}
	if err := storage.SendMessage(db, normal, []string{"bob"}); err != nil {
		t.Fatalf("send normal: %v", err)
	}

	urgent := storage.Message{
		ID:         "msg-urgent",
		ThreadID:   "thread-urgent",
		Sender:     "carol",
		Subject:    "Urgent update",
		Body:       "Body",
		CreatedAt:  "2026-01-15T00:00:00Z",
		Importance: "urgent",
	}
	if err := storage.SendMessage(db, urgent, []string{"bob"}); err != nil {
		t.Fatalf("send urgent: %v", err)
	}

	inbox, err := LoadCoordInbox(db, "bob", 1, true)
	if err != nil {
		t.Fatalf("load inbox: %v", err)
	}
	if len(inbox) != 1 {
		t.Fatalf("expected 1 message, got %d", len(inbox))
	}
	if inbox[0].Message.Importance != "urgent" {
		t.Fatalf("expected urgent message, got %s", inbox[0].Message.Importance)
	}
}
