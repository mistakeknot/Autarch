package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

type chatStreamView struct {
	last   string
	called bool
}

func (v *chatStreamView) Init() tea.Cmd                             { return nil }
func (v *chatStreamView) Update(msg tea.Msg) (pkgtui.View, tea.Cmd) { return v, nil }
func (v *chatStreamView) View() string                              { return "content" }
func (v *chatStreamView) Focus() tea.Cmd                            { return nil }
func (v *chatStreamView) Blur()                                     {}
func (v *chatStreamView) Name() string                              { return "chat" }
func (v *chatStreamView) ShortHelp() string                         { return "Tab focus" }
func (v *chatStreamView) AppendChatLine(line string) {
	v.called = true
	v.last = line
}

func TestUnifiedAppShiftTabCyclesBack(t *testing.T) {
	app := NewUnifiedApp(nil)
	app.mode = ModeDashboard
	app.dashViews = []View{
		&noopDashboardView{name: "A"},
		&noopDashboardView{name: "B"},
	}
	app.tabs = NewTabBar([]string{"A", "B"})
	app.tabs.SetActive(1)

	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	app = updated.(*UnifiedApp)

	if app.tabs.Active() != 0 {
		t.Fatalf("expected shift+tab to move to previous tab")
	}
}

func TestUnifiedAppCtrlCQuitsWithHelpVisible(t *testing.T) {
	app := NewUnifiedApp(nil)
	app.showHelp = true

	_, cmd := app.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatalf("expected quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected QuitMsg")
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

func TestAgentStreamMessagesRouteToChat(t *testing.T) {
	app := NewUnifiedApp(nil)
	view := &chatStreamView{}
	app.currentView = view

	_, _ = app.Update(AgentStreamMsg{Line: "hello"})

	if !view.called {
		t.Fatalf("expected AppendChatLine to be called")
	}
	if view.last != "hello" {
		t.Fatalf("expected line to be forwarded")
	}
}
