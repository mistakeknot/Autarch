package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/aggregator"
	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/discovery"
)

type fakeAggLayout struct {
	state aggregator.State
}

func (f *fakeAggLayout) GetState() aggregator.State { return f.state }
func (f *fakeAggLayout) Refresh(ctx context.Context) error { return nil }
func (f *fakeAggLayout) NewSession(string, string, string) error { return nil }
func (f *fakeAggLayout) RestartSession(string, string, string) error { return nil }
func (f *fakeAggLayout) RenameSession(string, string) error { return nil }
func (f *fakeAggLayout) ForkSession(string, string, string) error { return nil }
func (f *fakeAggLayout) AttachSession(string) error { return nil }
func (f *fakeAggLayout) StartMCP(context.Context, string, string) error { return nil }
func (f *fakeAggLayout) StopMCP(string, string) error { return nil }

func TestFilterSessionsByProject(t *testing.T) {
	agg := &fakeAggLayout{state: aggregator.State{
		Projects: []discovery.Project{
			{Path: "/p/one", Name: "one"},
			{Path: "/p/two", Name: "two"},
		},
		Sessions: []aggregator.TmuxSession{
			{Name: "a", ProjectPath: "/p/one"},
			{Name: "b", ProjectPath: "/p/two"},
		},
	}}

	m := New(agg, "")
	m.projectsList.SetItems([]list.Item{
		ProjectItem{Path: "", Name: "All"},
		ProjectItem{Path: "/p/one", Name: "one"},
	})
	m.projectsList.Select(1)
	m.updateLists()

	if len(m.sessionList.Items()) != 1 {
		t.Fatalf("expected 1 session, got %d", len(m.sessionList.Items()))
	}
}

func TestDefaultFocusIsProjects(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	if m.activePane != PaneProjects {
		t.Fatalf("expected default focus on projects")
	}
}

func TestFocusSwitching(t *testing.T) {
	agg := &fakeAggLayout{state: aggregator.State{}}
	m := New(agg, "")
	m.activePane = PaneMain

	mm, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	updated := mm.(Model)
	if updated.activePane != PaneProjects {
		t.Fatalf("expected projects pane")
	}
}

func TestRightPaneTabs(t *testing.T) {
	if TabDashboard != 0 || TabSessions != 1 || TabAgents != 2 {
		t.Fatalf("expected tabs: Dashboard, Sessions, Agents (Sessions=1 Agents=2)")
	}
}

func TestTwoPaneLayoutClamp(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	m.width = 40
	m.height = 10
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("renderTwoPane panicked: %v", r)
		}
	}()
	_ = m.renderTwoPane("left", "right")
}

func TestFocusedPaneBorderRendered(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	m.width = 80
	m.height = 20
	m.activePane = PaneProjects
	view := m.renderTwoPane("left", "right")
	border := lipgloss.RoundedBorder()
	if !strings.Contains(view, border.TopLeft) {
		t.Fatalf("expected border to render")
	}
}

func TestFocusedPaneChangesRendering(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	m.width = 80
	m.height = 20
	m.activePane = PaneProjects
	leftFocus := m.renderTwoPane("left", "right")
	m.activePane = PaneMain
	rightFocus := m.renderTwoPane("left", "right")
	if leftFocus == rightFocus {
		t.Fatalf("expected different rendering when focus changes")
	}
}

func TestHeaderIncludesBuildInfo(t *testing.T) {
	m := New(&fakeAggLayout{}, "rev abc123")
	header := m.renderHeader()
	if !strings.Contains(header, "rev abc123") {
		t.Fatalf("expected build info in header")
	}
}

func TestHeaderTabsExcludeProjects(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	m.activeTab = TabDashboard
	header := m.renderHeader()
	if strings.Contains(header, "Projects") {
		t.Fatalf("did not expect Projects in header tabs")
	}
}
