package views

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/mycroft"
	"github.com/mistakeknot/autarch/internal/mycroft/escalate"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

func TestMycroftsViewName(t *testing.T) {
	v := NewMycroftsView()
	if v.Name() != "Mycroft" {
		t.Errorf("Name() = %q, want Mycroft", v.Name())
	}
}

func TestMycroftsViewInit(t *testing.T) {
	v := NewMycroftsView()
	// Without a data source, Init returns nil.
	cmd := v.Init()
	if cmd != nil {
		t.Error("Init() without source should return nil")
	}
}

func TestMycroftsViewSidebarItems(t *testing.T) {
	v := NewMycroftsView()
	v.fleet = mycroft.FleetView{
		Agents: []mycroft.AgentView{
			{Name: "grey-area", Status: "active", CurrentBead: "Demarch-42"},
			{Name: "mistake-not", Status: "idle"},
		},
		Work: []mycroft.BeadView{
			{ID: "Demarch-1", DepsResolved: true},
			{ID: "Demarch-2", DepsResolved: false},
			{ID: "Demarch-3", DepsResolved: true, ClaimedBy: "grey-area"},
		},
	}

	items := v.SidebarItems()

	// Should have: tier indicator, section-agents, 2 agents, work-queue, shadows.
	if len(items) < 6 {
		t.Fatalf("expected at least 6 sidebar items, got %d", len(items))
	}

	// First item is fleet overview with tier.
	if items[0].ID != "fleet-overview" {
		t.Errorf("first item ID = %q, want fleet-overview", items[0].ID)
	}

	// Check agent items exist.
	var foundGrey, foundMistake bool
	for _, item := range items {
		switch item.ID {
		case "agent:grey-area":
			foundGrey = true
			if item.Icon != "●" {
				t.Errorf("active agent icon = %q, want ●", item.Icon)
			}
		case "agent:mistake-not":
			foundMistake = true
			if item.Icon != "○" {
				t.Errorf("idle agent icon = %q, want ○", item.Icon)
			}
		}
	}
	if !foundGrey {
		t.Error("grey-area agent not in sidebar")
	}
	if !foundMistake {
		t.Error("mistake-not agent not in sidebar")
	}

	// Check work queue shows 1 ready (not 3 total).
	var foundWork bool
	for _, item := range items {
		if item.ID == "work-queue" {
			foundWork = true
			if item.Label != "── Work (1 ready)" {
				t.Errorf("work label = %q, want '── Work (1 ready)'", item.Label)
			}
		}
	}
	if !foundWork {
		t.Error("work-queue not in sidebar")
	}
}

func TestMycroftsViewFleetMsg(t *testing.T) {
	v := NewMycroftsView()

	// Simulate window size first.
	v2, _ := v.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	v = v2.(*MycroftsView)

	// Simulate fleet loaded.
	fleet := mycroft.FleetView{
		Agents: []mycroft.AgentView{
			{Name: "grey-area", Status: "active", Runtime: "claude-code"},
		},
		Work: []mycroft.BeadView{
			{ID: "Demarch-1", Title: "Fix test", Priority: 1, DepsResolved: true},
		},
	}

	v3, _ := v.Update(mycroftFleetMsg{view: fleet})
	v = v3.(*MycroftsView)

	if len(v.fleet.Agents) != 1 {
		t.Errorf("fleet agents = %d, want 1", len(v.fleet.Agents))
	}
	if v.lastRefresh.IsZero() {
		t.Error("lastRefresh should be set after fleet msg")
	}
}

func TestMycroftsViewModes(t *testing.T) {
	v := NewMycroftsView()
	v.fleet = mycroft.FleetView{
		Agents: []mycroft.AgentView{
			{Name: "grey-area", Status: "active"},
		},
	}

	// Simulate window size.
	v2, _ := v.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	v = v2.(*MycroftsView)

	// Default mode is fleet.
	output := v.View()
	if output == "" {
		t.Error("View() returned empty string")
	}

	// Switch to agent detail via sidebar select.
	v.handleSidebarSelect("agent:grey-area")
	if v.viewMode != mycroftViewAgent {
		t.Errorf("viewMode = %d, want agent", v.viewMode)
	}
	if v.selectedAgent != "grey-area" {
		t.Errorf("selectedAgent = %q, want grey-area", v.selectedAgent)
	}

	// Switch to work queue.
	v.handleSidebarSelect("work-queue")
	if v.viewMode != mycroftViewWork {
		t.Error("expected work mode")
	}

	// Switch back to fleet overview.
	v.handleSidebarSelect("fleet-overview")
	if v.viewMode != mycroftViewFleet {
		t.Error("expected fleet mode")
	}
}

func TestMycroftsViewDecisionBadge(t *testing.T) {
	v := NewMycroftsView()
	v.decisions.Add("grey-area", "Demarch-1", "Fix test", 0, "priority match")

	items := v.SidebarItems()

	var foundDecision bool
	for _, item := range items {
		if item.ID == "decisions" {
			foundDecision = true
		}
	}
	if !foundDecision {
		t.Error("expected decision badge in sidebar when decisions are pending")
	}
}

func TestMycroftsViewShortHelp(t *testing.T) {
	v := NewMycroftsView()
	help := v.ShortHelp()
	if help == "" {
		t.Error("ShortHelp() returned empty")
	}
	// Should contain the idle badge when no decisions.
	if !contains(help, "idle") {
		t.Errorf("ShortHelp = %q, expected 'idle' badge", help)
	}
}

func TestMycroftsViewCommands(t *testing.T) {
	v := NewMycroftsView()
	cmds := v.Commands()
	if len(cmds) != 2 {
		t.Errorf("Commands() = %d, want 2", len(cmds))
	}
}

func TestMycroftsViewErrorMsg(t *testing.T) {
	v := NewMycroftsView()

	// Simulate window size.
	v2, _ := v.Update(tea.WindowSizeMsg{Width: 200, Height: 50})
	v = v2.(*MycroftsView)

	// Fleet error.
	v3, _ := v.Update(mycroftFleetMsg{err: fmt.Errorf("connection refused")})
	v = v3.(*MycroftsView)

	if v.errMsg != "connection refused" {
		t.Errorf("errMsg = %q, want 'connection refused'", v.errMsg)
	}

	output := v.View()
	if !contains(output, "connection refused") {
		t.Error("error message not shown in view")
	}
}

// Suppress unused import warnings.
var (
	_ = time.Now
	_ = escalate.NewDecisionQueue
	_ = pkgtui.NewShellLayout
	_ = fmt.Sprintf
)
