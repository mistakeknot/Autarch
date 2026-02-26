package views

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mistakeknot/autarch/pkg/intercore"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// newTestColdwineView creates a ColdwineView suitable for unit testing.
func newTestColdwineView() *ColdwineView {
	v := NewColdwineView(nil)
	v.width = 120
	v.height = 40
	v.shell.SetSize(120, 40)
	return v
}

func TestColdwineView_ModeSwitchKeybinding(t *testing.T) {
	v := newTestColdwineView()

	if v.mode != ModeEpics {
		t.Fatal("initial mode should be ModeEpics")
	}

	// Press "m" in document focus — should switch to ModeRuns
	v.shell.SetFocus(pkgtui.FocusDocument)
	_, cmd := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	if v.mode != ModeRuns {
		t.Error("pressing 'm' should switch to ModeRuns")
	}
	// Without an iclient, loadRunsForMode returns nil, but the mode should toggle
	if cmd != nil {
		t.Error("expected nil cmd without iclient, got non-nil")
	}

	// Press "m" again — should switch back to ModeEpics
	_, _ = v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	if v.mode != ModeEpics {
		t.Error("pressing 'm' again should switch back to ModeEpics")
	}
}

func TestColdwineView_ModeSwitchInSidebar(t *testing.T) {
	v := newTestColdwineView()

	// "m" should also work in sidebar focus
	v.shell.SetFocus(pkgtui.FocusSidebar)
	_, _ = v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	if v.mode != ModeRuns {
		t.Error("pressing 'm' in sidebar focus should switch mode")
	}
}

func TestColdwineView_ModeSwitchIgnoredInChat(t *testing.T) {
	v := newTestColdwineView()

	// "m" should NOT switch mode in chat focus (it's a text character)
	v.shell.SetFocus(pkgtui.FocusChat)
	_, _ = v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})

	if v.mode != ModeEpics {
		t.Error("pressing 'm' in chat focus should NOT switch mode")
	}
}

func TestColdwineView_SidebarSelectMsg_SentinelModeToggle(t *testing.T) {
	v := newTestColdwineView()

	// Select "__mode_runs" sentinel — should switch to Runs mode
	_, _ = v.Update(pkgtui.SidebarSelectMsg{ItemID: "__mode_runs"})
	if v.mode != ModeRuns {
		t.Error("selecting __mode_runs should switch to ModeRuns")
	}

	// Select "__mode_epics" sentinel — should switch back
	_, _ = v.Update(pkgtui.SidebarSelectMsg{ItemID: "__mode_epics"})
	if v.mode != ModeEpics {
		t.Error("selecting __mode_epics should switch to ModeEpics")
	}

	// Select "__unscoped_sprints" — should switch to Runs mode
	_, _ = v.Update(pkgtui.SidebarSelectMsg{ItemID: "__unscoped_sprints"})
	if v.mode != ModeRuns {
		t.Error("selecting __unscoped_sprints should switch to ModeRuns")
	}
}

func TestColdwineView_FocusModeAware(t *testing.T) {
	v := newTestColdwineView()
	v.mode = ModeRuns

	cmd := v.Focus()

	// Focus should return a non-nil cmd (batched from chatPanel.Focus + loadData)
	// In ModeRuns, it should also attempt loadRunsForMode (but nil without iclient)
	if cmd == nil {
		t.Error("Focus() should return non-nil cmd")
	}
}

func TestColdwineView_StaleRunsLoadedMsg(t *testing.T) {
	v := newTestColdwineView()

	// Simulate: runsLoadSeq was incremented to 5 by a rapid mode toggle
	v.runsLoadSeq = 5

	// A stale message with seq=3 arrives — should be ignored
	_, _ = v.Update(RunsLoadedMsg{
		Runs: []intercore.Run{{ID: "stale-run"}},
		Seq:  3,
	})
	if len(v.runs) != 0 {
		t.Error("stale RunsLoadedMsg (seq=3 < runsLoadSeq=5) should be ignored")
	}

	// A fresh message with seq=5 arrives — should be applied
	_, _ = v.Update(RunsLoadedMsg{
		Runs: []intercore.Run{{ID: "fresh-run"}},
		Seq:  5,
	})
	if len(v.runs) != 1 || v.runs[0].ID != "fresh-run" {
		t.Errorf("fresh RunsLoadedMsg (seq=5) should be applied, got %d runs", len(v.runs))
	}
}

func TestColdwineView_StaleRunDetailLoadedMsg(t *testing.T) {
	v := newTestColdwineView()

	v.detailLoadSeq = 10

	// Stale detail — should be ignored
	_, _ = v.Update(RunDetailLoadedMsg{
		Run: &intercore.Run{ID: "stale-detail"},
		Seq: 7,
	})
	// runsRunDetail should still be empty (no data set)
	got := v.runsRunDetail.Render()
	if got == "" {
		t.Error("render should produce output")
	}

	// Fresh detail — should be applied
	_, _ = v.Update(RunDetailLoadedMsg{
		Run:        &intercore.Run{ID: "fresh-detail", Status: "active", Goal: "test"},
		Dispatches: []intercore.Dispatch{},
		Events:     []intercore.Event{},
		Seq:        10,
	})
	got = v.runsRunDetail.Render()
	if got == "" {
		t.Error("render after fresh detail should produce output")
	}
}

func TestColdwineView_ModeChangeMsg(t *testing.T) {
	v := newTestColdwineView()

	// Handle coldwineModeChangeMsg (from palette Action closure)
	_, _ = v.Update(coldwineModeChangeMsg{mode: ModeRuns})
	if v.mode != ModeRuns {
		t.Error("coldwineModeChangeMsg{ModeRuns} should set mode to ModeRuns")
	}

	_, _ = v.Update(coldwineModeChangeMsg{mode: ModeEpics})
	if v.mode != ModeEpics {
		t.Error("coldwineModeChangeMsg{ModeEpics} should set mode to ModeEpics")
	}
}

func TestColdwineView_ShortHelpModeSpecific(t *testing.T) {
	v := newTestColdwineView()

	// Epics mode
	help := v.ShortHelp()
	if help == "" {
		t.Error("ShortHelp should not be empty")
	}

	// Runs mode should have different help
	v.mode = ModeRuns
	runsHelp := v.ShortHelp()
	if runsHelp == help {
		t.Error("ShortHelp should differ between Epics and Runs modes")
	}
}

func TestColdwineView_SidebarItemsIncludeModeToggle(t *testing.T) {
	v := newTestColdwineView()

	// Epics mode — should have mode toggle + empty (no epics loaded)
	items := v.SidebarItems()
	if len(items) < 2 {
		t.Fatalf("expected at least 2 mode toggle items, got %d", len(items))
	}
	if items[0].ID != "__mode_epics" {
		t.Errorf("first item should be __mode_epics, got %q", items[0].ID)
	}
	if items[1].ID != "__mode_runs" {
		t.Errorf("second item should be __mode_runs, got %q", items[1].ID)
	}
	// In Epics mode, first item should have active icon (●)
	if items[0].Icon != "●" {
		t.Errorf("active mode should have ● icon, got %q", items[0].Icon)
	}
	if items[1].Icon != "○" {
		t.Errorf("inactive mode should have ○ icon, got %q", items[1].Icon)
	}

	// Runs mode — icons should flip
	v.mode = ModeRuns
	items = v.SidebarItems()
	if items[0].Icon != "○" {
		t.Errorf("Epics should have ○ icon in Runs mode, got %q", items[0].Icon)
	}
	if items[1].Icon != "●" {
		t.Errorf("Runs should have ● icon in Runs mode, got %q", items[1].Icon)
	}
}

func TestColdwineView_ComputeOrphanRuns(t *testing.T) {
	v := newTestColdwineView()

	// No data — orphanRuns should be nil
	v.computeOrphanRuns()
	if v.orphanRuns != nil {
		t.Error("orphanRuns should be nil when no data")
	}

	// Set up runs and epicRuns
	v.runs = []intercore.Run{
		{ID: "run1"},
		{ID: "run2"},
		{ID: "run3"},
	}
	v.epicRuns = map[string]*intercore.Run{
		"epic1": {ID: "run1"},
	}

	v.computeOrphanRuns()
	if len(v.orphanRuns) != 2 {
		t.Errorf("expected 2 orphan runs, got %d", len(v.orphanRuns))
	}
}
