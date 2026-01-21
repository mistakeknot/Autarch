package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

func TestCoordHeaderShowsCountsAndFilters(t *testing.T) {
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
		{Message: storage.Message{Sender: "bob", Subject: "One"}},
		{Message: storage.Message{Sender: "carol", Subject: "Two"}},
	}
	m.CoordLocks = []storage.Reservation{
		{Path: "a.go", Owner: "alice"},
	}
	out := m.View()
	if !strings.Contains(out, "COORD: inbox=2 locks=1 urgent=off recipient=all") {
		t.Fatalf("expected coord header with counts and filters")
	}
}
