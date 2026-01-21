package tui

import (
	"strings"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

func TestCoordTabRendersInboxAndLocks(t *testing.T) {
	m := NewModel()
	m.RightTab = RightTabCoord
	m.CoordRecipient = "alice"
	m.CoordInbox = []storage.MessageDelivery{
		{Message: storage.Message{ID: "msg-1", Sender: "bob", Subject: "Hello"}, Recipient: "alice"},
	}
	m.CoordLocks = []storage.Reservation{
		{Path: "a.go", Owner: "alice"},
	}
	out := m.View()
	if !strings.Contains(out, "COORD") {
		t.Fatalf("expected coord tab, got %q", out)
	}
	if !strings.Contains(out, "INBOX") || !strings.Contains(out, "LOCKS") {
		t.Fatalf("expected inbox and locks sections")
	}
}
