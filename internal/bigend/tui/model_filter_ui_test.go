package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/mistakeknot/autarch/internal/bigend/aggregator"
	"github.com/mistakeknot/autarch/internal/icdata"
)

func TestFilterClearsOnEscape(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	m.activeTab = TabSessions
	m = m.withFilterActive("codex")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm := updated.(Model)
	if mm.filterInput.Value() != "" {
		t.Fatalf("expected empty filter")
	}
	if mm.filterActive {
		t.Fatalf("expected filter inactive")
	}
}

func TestFilterUIHiddenWhenEmpty(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	m.width = 80
	m.height = 20
	view := m.View()
	if strings.Contains(view, "Filter:") {
		t.Fatalf("did not expect filter line")
	}
}

func TestFilterUIShownWhenActive(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	m.activeTab = TabSessions
	m.width = 80
	m.height = 20
	m = m.withFilterActive("codex")
	view := m.View()
	if !strings.Contains(view, "Filter:") {
		t.Fatalf("expected filter line")
	}
}

func TestFilterExitsOnEnter(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	m.activeTab = TabSessions
	m = m.withFilterActive("codex")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := updated.(Model)
	if mm.filterActive {
		t.Fatalf("expected filter inactive")
	}
	if mm.filterInput.Value() != "codex" {
		t.Fatalf("expected filter value preserved")
	}
}

func TestFilterAllowsArrowKeys(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	m.activeTab = TabSessions
	m.activePane = PaneMain
	m.sessionList.SetItems([]list.Item{
		SessionItem{Session: aggregator.TmuxSession{Name: "a"}, Status: icdata.StatusActive},
		SessionItem{Session: aggregator.TmuxSession{Name: "b"}, Status: icdata.StatusActive},
	})
	m.sessionList.Select(0)
	m = m.withFilterActive("codex")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	mm := updated.(Model)
	if mm.sessionList.Index() != 1 {
		t.Fatalf("expected selection to move with arrow key")
	}
}

func TestFilterPersistsAcrossTabs(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	m.activeTab = TabSessions
	m = m.withFilterActive("codex")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	mm := updated.(Model)
	if mm.activeTab != TabAgents {
		t.Fatalf("expected tab to advance to agents")
	}
	if mm.filterInput.Value() != "" {
		t.Fatalf("expected empty filter on agents tab")
	}
	if mm.filterStates[TabAgents].Raw != "" {
		t.Fatalf("expected no agent filter state")
	}
	if mm.filterStates[TabSessions].Raw != "codex" {
		t.Fatalf("expected session filter state to persist")
	}
	if mm.filterActive {
		t.Fatalf("expected filter editing to stop on tab switch")
	}
}

func TestFilterRestoresPerTabState(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	m.activeTab = TabSessions
	m = m.withFilterActive("codex")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	mm := updated.(Model)
	if mm.activeTab != TabAgents {
		t.Fatalf("expected tab to advance to agents")
	}
	mm = mm.withFilterActive("rose")
	updated, _ = mm.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	mm = updated.(Model)
	if mm.activeTab != TabSessions {
		t.Fatalf("expected tab to return to sessions")
	}
	if mm.filterInput.Value() != "codex" {
		t.Fatalf("expected session filter value restored")
	}
	if mm.filterStates[TabAgents].Raw != "rose" {
		t.Fatalf("expected agent filter state saved")
	}
}

func TestFilterHiddenOnDashboard(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	m.width = 80
	m.height = 20
	m.filterStates = map[Tab]FilterState{
		TabSessions: parseFilter("codex"),
	}
	m.activeTab = TabDashboard
	view := m.View()
	if strings.Contains(view, "Filter:") {
		t.Fatalf("did not expect filter line on dashboard")
	}
}

func TestFooterShowsFilterHintWhenActive(t *testing.T) {
	m := New(&fakeAggLayout{}, "")
	m.width = 120
	m.activeTab = TabSessions
	m = m.withFilterActive("codex")
	footer := m.renderFooter()
	if !strings.Contains(footer, "esc/enter") {
		t.Fatalf("expected filter hint in footer")
	}
}
