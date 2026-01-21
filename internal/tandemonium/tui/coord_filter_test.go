package tui

import (
	"strings"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

func TestCoordFilterUrgentOnly(t *testing.T) {
	m := NewModel()
	m.RightTab = RightTabCoord
	m.CoordRecipient = "alice"
	m.CoordInbox = []storage.MessageDelivery{
		{Message: storage.Message{Sender: "bob", Subject: "Normal", Importance: "normal"}},
		{Message: storage.Message{Sender: "carol", Subject: "Urgent", Importance: "urgent"}},
	}
	m.CoordUrgentOnly = true
	out := m.View()
	if strings.Contains(out, "Normal") {
		t.Fatalf("expected normal to be filtered")
	}
	if !strings.Contains(out, "Urgent") {
		t.Fatalf("expected urgent to be shown")
	}
}

func TestCoordFilterMentionsOnly(t *testing.T) {
	m := NewModel()
	m.RightTab = RightTabCoord
	m.CoordRecipient = "alice"
	m.CoordRecipientFilter = CoordRecipientFilterMentions
	m.CoordInbox = []storage.MessageDelivery{
		{Message: storage.Message{Sender: "bob", Subject: "Ping @alice", Importance: "normal"}},
		{Message: storage.Message{Sender: "carol", Subject: "FYI update", Importance: "normal"}},
	}
	out := m.View()
	if strings.Contains(out, "FYI update") {
		t.Fatalf("expected non-mention to be filtered")
	}
	if !strings.Contains(out, "Ping @alice") {
		t.Fatalf("expected mention to be shown")
	}
}

func TestCoordFiltersShownInSummary(t *testing.T) {
	m := NewModel()
	m.RightTab = RightTabCoord
	m.CoordUrgentOnly = true
	m.CoordRecipientFilter = CoordRecipientFilterMentions
	out := m.View()
	if !strings.Contains(out, "coord: urgent=on recipient=@mentions") {
		t.Fatalf("expected coord filters in summary")
	}
}
