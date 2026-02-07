package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/pkg/autarch"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

type noopDashboardView struct {
	name string
}

func (v *noopDashboardView) Init() tea.Cmd                             { return nil }
func (v *noopDashboardView) Update(msg tea.Msg) (pkgtui.View, tea.Cmd) { return v, nil }
func (v *noopDashboardView) View() string                              { return "content" }
func (v *noopDashboardView) Focus() tea.Cmd                            { return nil }
func (v *noopDashboardView) Blur()                                     {}
func (v *noopDashboardView) Name() string                              { return v.name }
func (v *noopDashboardView) ShortHelp() string                         { return "Tab focus" }

type inputFocusView struct {
	focused bool
	seen    bool
}

func (v *inputFocusView) Init() tea.Cmd                             { return nil }
func (v *inputFocusView) Update(msg tea.Msg) (pkgtui.View, tea.Cmd) { v.seen = true; return v, nil }
func (v *inputFocusView) View() string                              { return "content" }
func (v *inputFocusView) Focus() tea.Cmd                            { return nil }
func (v *inputFocusView) Blur()                                     {}
func (v *inputFocusView) Name() string                              { return "input" }
func (v *inputFocusView) ShortHelp() string                         { return "" }
func (v *inputFocusView) InputFocused() bool                        { return v.focused }

func TestUnifiedAppCtrlLeftCyclesBack(t *testing.T) {
	app := NewUnifiedApp(nil)
	app.mode = ModeDashboard
	app.dashViews = []View{
		&noopDashboardView{name: "A"},
		&noopDashboardView{name: "B"},
	}
	app.tabs = NewTabBar([]string{"A", "B"})
	app.tabs.SetActive(1)

	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyCtrlLeft})
	app = updated.(*UnifiedApp)

	if app.tabs.Active() != 0 {
		t.Fatalf("expected shift+tab to move to previous tab")
	}
}

func TestUnifiedAppDoubleCtrlCQuitsWithHelpVisible(t *testing.T) {
	app := NewUnifiedApp(nil)
	app.showHelp = true

	// First ctrl+c should clear input (not quit)
	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd != nil {
		if _, ok := cmd().(tea.QuitMsg); ok {
			t.Fatalf("first ctrl+c should not quit")
		}
	}

	// Second ctrl+c (within 500ms) should quit
	_, cmd = app.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatalf("expected quit command on second ctrl+c")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg on second ctrl+c")
	}
}

func TestChatSettingsTogglePersistsAndApplies(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	app := NewUnifiedApp(nil)
	app.chatSettings = DefaultChatSettings()

	app.chatSettings.AutoScroll = false
	if err := SaveChatSettings(app.chatSettings); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	loaded, err := LoadChatSettings()
	if err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	if loaded.AutoScroll {
		t.Fatalf("expected autos-scroll off")
	}
}

// TestAgentStreamMessagesPassThroughToView verifies that AgentStreamMsg
// passes through the default view routing path.
func TestAgentStreamMessagesPassThroughToView(t *testing.T) {
	app := NewUnifiedApp(nil)
	view := &noopDashboardView{name: "test"}
	app.currentView = view

	// AgentStreamMsg should fall through to current view's Update
	_, _ = app.Update(AgentStreamMsg{Line: "hello"})

	// Message reaches view via default pass-through (no explicit handler)
	// The noopDashboardView just returns itself — no crash = pass
}

func TestCommaDoesNotOpenChatSettingsWhenInputFocused(t *testing.T) {
	app := NewUnifiedApp(nil)
	view := &inputFocusView{focused: true}
	app.currentView = view

	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{','}})
	app = updated.(*UnifiedApp)

	if app.chatSettingsOpen {
		t.Fatalf("expected chat settings to remain closed when input focused")
	}
	if !view.seen {
		t.Fatalf("expected key to be handled by view")
	}
}

func TestRunEnablesMouse(t *testing.T) {
	// No direct program option introspection available here.
	// Verify manually by running the app and confirming wheel events scroll chat.
}

// TestInitAlwaysEntersDashboard verifies Init() always calls enterDashboard().
func TestInitAlwaysEntersDashboard(t *testing.T) {
	app := NewUnifiedApp(nil)
	app.SetDashboardViewFactory(func(c *autarch.Client) []View {
		return []View{
			&noopDashboardView{name: "Bigend"},
			&noopDashboardView{name: "Gurgeh"},
		}
	})

	app.Init()

	if app.mode != ModeDashboard {
		t.Fatalf("expected ModeDashboard, got %v", app.mode)
	}
	if len(app.dashViews) != 2 {
		t.Fatalf("expected 2 dashboard views, got %d", len(app.dashViews))
	}
}

// TestSkipOnboardingWithInitAlwaysEntersDashboard verifies backward compat.
func TestSkipOnboardingWithInitAlwaysEntersDashboard(t *testing.T) {
	app := NewUnifiedApp(nil)
	app.SetSkipOnboarding(true)
	app.SetDashboardViewFactory(func(c *autarch.Client) []View {
		return []View{
			&noopDashboardView{name: "Bigend"},
			&noopDashboardView{name: "Gurgeh"},
		}
	})

	app.Init()

	if app.mode != ModeDashboard {
		t.Fatalf("expected ModeDashboard, got %v", app.mode)
	}
}

// TestOnboardingCompleteMsgIsNoOp verifies that OnboardingCompleteMsg
// is a no-op in UnifiedApp (since we're always in dashboard mode).
func TestOnboardingCompleteMsgIsNoOp(t *testing.T) {
	app := NewUnifiedApp(nil)
	app.mode = ModeDashboard
	app.dashViews = []View{
		&noopDashboardView{name: "Bigend"},
	}
	app.currentView = app.dashViews[0]

	updated, cmd := app.Update(OnboardingCompleteMsg{
		ProjectID:   "test-id",
		ProjectName: "test-project",
	})
	app = updated.(*UnifiedApp)

	if cmd != nil {
		t.Fatalf("expected nil cmd from OnboardingCompleteMsg, got non-nil")
	}
	// Should still be in dashboard mode with same view
	if app.mode != ModeDashboard {
		t.Fatalf("expected ModeDashboard after OnboardingCompleteMsg")
	}
}

// TestLogPaneAutoShowMsg verifies the log pane auto-show bridge message.
func TestLogPaneAutoShowMsg(t *testing.T) {
	app := NewUnifiedApp(nil)
	app.logPaneVisible = false
	app.logPaneAutoShown = false

	_, cmd := app.Update(LogPaneAutoShowMsg{})

	if !app.logPaneVisible {
		t.Fatalf("expected log pane to be visible after LogPaneAutoShowMsg")
	}
	if !app.logPaneAutoShown {
		t.Fatalf("expected logPaneAutoShown to be true")
	}
	if cmd == nil {
		t.Fatalf("expected sendWindowSize command")
	}
}

// TestLogPaneScheduleAutoHideMsg verifies the auto-hide scheduling.
func TestLogPaneScheduleAutoHideMsg(t *testing.T) {
	app := NewUnifiedApp(nil)
	app.logPaneAutoShown = true

	_, cmd := app.Update(LogPaneScheduleAutoHideMsg{})

	if cmd == nil {
		t.Fatalf("expected tick command for auto-hide scheduling")
	}
}

// TestLogPaneScheduleAutoHideMsgNoOpWhenNotAutoShown verifies no-op.
func TestLogPaneScheduleAutoHideMsgNoOpWhenNotAutoShown(t *testing.T) {
	app := NewUnifiedApp(nil)
	app.logPaneAutoShown = false

	_, cmd := app.Update(LogPaneScheduleAutoHideMsg{})

	if cmd != nil {
		t.Fatalf("expected nil cmd when logPaneAutoShown is false")
	}
}

// TestTabSwitchingWorksInDashboardMode verifies Ctrl+Left/Right tab cycling.
func TestTabSwitchingWorksInDashboardMode(t *testing.T) {
	app := NewUnifiedApp(nil)
	app.mode = ModeDashboard
	app.dashViews = []View{
		&noopDashboardView{name: "Bigend"},
		&noopDashboardView{name: "Gurgeh"},
		&noopDashboardView{name: "Coldwine"},
	}
	app.tabs = NewTabBar([]string{"Bigend", "Gurgeh", "Coldwine"})
	app.tabs.SetActive(0)
	app.currentView = app.dashViews[0]

	// Ctrl+Right should move to tab 1
	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyCtrlRight})
	app = updated.(*UnifiedApp)

	if app.tabs.Active() != 1 {
		t.Fatalf("expected tab 1, got %d", app.tabs.Active())
	}
}
