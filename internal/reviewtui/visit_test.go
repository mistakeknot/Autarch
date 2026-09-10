package reviewtui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/pkg/review"
	"strings"
	"testing"
)

func visitModel() *Model {
	m := New("/project", "Cozy", review.Client{})
	m.workbench = true
	m.width, m.height = 120, 34
	m.state.Visits = map[string]review.Visit{"visit": {ID: "visit", Project: "/project", Outcome: "Keep one continuous visit", View: "outcome", Layout: "purpose", Pace: "all", Density: "Cozy", Status: "exploring", Answers: []review.VisitAnswer{{ID: "answer", QuestionID: "question", Text: "My original answer"}}}}
	m.state.Turns = []review.Turn{{Project: "/project", Kind: "Flere", Text: "A continuous conversation"}}
	m.state.Questions = []review.Question{{ID: "question", Project: "/project", Status: "pending", Title: "Which outcome matters?", Options: []string{"Retain guidance", "Start implementation"}}}
	return m
}
func TestVisitSwitchAndResizeKeepConversationAndCorrection(t *testing.T) {
	m := visitModel()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	if m.state.Visits["visit"].View != "map" {
		t.Fatal("map switch lost")
	}
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 30})
	v := m.View()
	if !strings.Contains(v, "Which outcome matters?") || !strings.Contains(v, "focused") {
		t.Fatal("narrow layout lost question/focus", v)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Alt: true, Runes: []rune("c")})
	if m.input.Value() != "My original answer" || m.correctionID != "answer" {
		t.Fatal("correction not bound to original answer")
	}
	m.input.SetValue("  Exact correction  ")
	m.saveInput()
	if m.pendingSave.Text != "  Exact correction  " || m.pendingSave.Target != "answer" {
		t.Fatal("correction wording changed")
	}
}
