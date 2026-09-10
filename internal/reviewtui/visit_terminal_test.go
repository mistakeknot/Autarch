package reviewtui

import (
	"encoding/json"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/pkg/review"
	"os"
	"path/filepath"
	"testing"
)

type visitTerminal struct{ m *Model }

func (v visitTerminal) Init() tea.Cmd { return nil }
func (v visitTerminal) View() string  { return v.m.View() }
func (v visitTerminal) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(ClosedMsg); ok {
		return v, tea.Quit
	}
	return v, v.m.Update(msg)
}

func TestVisitTerminalPrototype(t *testing.T) {
	if os.Getenv("AUTARCH_VISIT_TERMINAL_FIXTURE") != "1" {
		t.Skip("explicit interactive fixture")
	}
	root, err := os.MkdirTemp("/private/tmp", "autarch-visit-terminal-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	project := filepath.Join(root, "project")
	os.Mkdir(project, 0700)
	m := visitModel()
	m.project = project
	m.state.Version = 2
	m.state.Revision = 1
	v := m.state.Visits["visit"]
	v.Project = project
	m.state.Visits["visit"] = v
	for i := range m.state.Turns {
		m.state.Turns[i].Project = project
	}
	for i := range m.state.Questions {
		m.state.Questions[i].Project = project
		m.state.Questions[i].VisitID = v.ID
	}
	m.state.Proposals = map[string]review.Proposal{"synthesis": {ID: "synthesis", Project: project, Revision: 1, Kind: "guidance", VisitID: v.ID, AnswerIDs: []string{"answer"}, Outcome: "Keep one continuous visit", Change: "Retain corrections and resume", Rationale: "One coherent plan", Scope: []string{"docs"}, Checklist: []string{"Inspect exact wording"}, Status: "proposed"}}
	records := filepath.Join(root, "records")
	os.Mkdir(records, 0700)
	data, _ := json.Marshal(m.state)
	os.WriteFile(filepath.Join(records, "00000000000000000001.json"), data, 0600)
	store, err := review.Open(root)
	if err != nil {
		t.Fatal(err)
	}
	server, err := review.Listen(filepath.Join(root, "fixture.sock"), store)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	go server.Serve()
	m.client = review.Client{Socket: filepath.Join(root, "fixture.sock")}
	m.state = store.Snapshot()
	m.status = "INTERACTION FIXTURE · no model calls or real human acceptance evidence"
	if _, err = tea.NewProgram(visitTerminal{m}, tea.WithAltScreen()).Run(); err != nil {
		t.Fatal(err)
	}
}
