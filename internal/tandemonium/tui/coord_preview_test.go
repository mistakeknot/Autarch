package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

func TestCoordPreviewShowsSelectedMessage(t *testing.T) {
	m := NewModel()
	m.TaskList = make([]TaskItem, 15)
	for i := range m.TaskList {
		m.TaskList[i] = TaskItem{
			ID:     fmt.Sprintf("tsk-%02d", i),
			Title:  "Task",
			Status: "todo",
		}
	}
	m.RightTab = RightTabCoord
	m.CoordRecipient = "alice"
	m.CoordInbox = []storage.MessageDelivery{
		{Message: storage.Message{Sender: "bob", Subject: "Hello", Body: "First line\nSecond line"}},
	}
	out := m.View()
	if !strings.Contains(out, "PREVIEW") {
		t.Fatalf("expected preview section")
	}
	if !strings.Contains(out, "Subject: Hello") {
		t.Fatalf("expected subject in preview")
	}
	if !strings.Contains(out, "First line") {
		t.Fatalf("expected body snippet in preview")
	}
}
