package tui

import (
	"context"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/aggregator"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/mcp"
)

type fakeAgg struct {
	state         aggregator.State
	restartCalled bool
}

func (f *fakeAgg) GetState() aggregator.State { return f.state }
func (f *fakeAgg) Refresh(ctx context.Context) error { return nil }
func (f *fakeAgg) NewSession(name, projectPath, agentType string) error { return nil }
func (f *fakeAgg) RestartSession(name, projectPath, agentType string) error {
	f.restartCalled = true
	return nil
}
func (f *fakeAgg) RenameSession(oldName, newName string) error { return nil }
func (f *fakeAgg) ForkSession(name, projectPath, agentType string) error { return nil }
func (f *fakeAgg) AttachSession(name string) error { return nil }
func (f *fakeAgg) StartMCP(ctx context.Context, projectPath, component string) error { return nil }
func (f *fakeAgg) StopMCP(projectPath, component string) error { return nil }

func TestRestartKeyTriggersAction(t *testing.T) {
	agg := &fakeAgg{state: aggregator.State{Sessions: []aggregator.TmuxSession{{
		Name:        "demo",
		ProjectPath: "/root/projects/demo",
		AgentType:   "claude",
	}}}}
	m := New(agg)
	m.activeTab = TabSessions
	m.updateLists()

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if !agg.restartCalled {
		t.Fatalf("expected restart to be called")
	}
}

func TestForkKeyShowsPrompt(t *testing.T) {
	agg := &fakeAgg{state: aggregator.State{Sessions: []aggregator.TmuxSession{{
		Name:        "demo",
		ProjectPath: "/root/projects/demo",
		AgentType:   "claude",
	}}}}
	m := New(agg)
	m.activeTab = TabSessions
	m.updateLists()

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'f'}})
	updated := mm.(Model)
	if updated.promptMode != promptForkSession {
		t.Fatalf("expected fork prompt, got %v", updated.promptMode)
	}
}

func TestMCPPanelToggle(t *testing.T) {
	agg := &fakeAgg{state: aggregator.State{
		MCP: map[string][]mcp.ComponentStatus{
			"/root/projects/demo": {
				{
					ProjectPath: "/root/projects/demo",
					Component:   "server",
					Status:      mcp.StatusStopped,
				},
			},
		},
	}}
	m := New(agg)
	m.activeTab = TabDashboard
	m.projectsList.SetItems([]list.Item{ProjectItem{Path: "/root/projects/demo", Name: "demo"}})

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	updated := mm.(Model)
	if !updated.showMCP {
		t.Fatalf("expected MCP panel enabled")
	}
	if updated.mcpProject != "/root/projects/demo" {
		t.Fatalf("unexpected mcp project: %q", updated.mcpProject)
	}
}
