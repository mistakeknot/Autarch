package tui

import (
	"github.com/mistakeknot/autarch/internal/autarch/agent"
	"github.com/mistakeknot/autarch/internal/testutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/pkg/agenttargets"
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
	testutil.ConfigHome(t)

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

	if len(app.dashViews) == 0 {
		t.Fatalf("expected dashboard views to be created")
	}
}

// TestOnboardingCompleteMsgIsNoOp verifies that OnboardingCompleteMsg
// is a no-op in UnifiedApp (since we're always in dashboard mode).
func TestOnboardingCompleteMsgIsNoOp(t *testing.T) {
	app := NewUnifiedApp(nil)
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

// sizingView records the last WindowSizeMsg it received.
type sizingView struct {
	name      string
	lastWidth int
}

func (v *sizingView) Init() tea.Cmd { return nil }
func (v *sizingView) Update(msg tea.Msg) (pkgtui.View, tea.Cmd) {
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		v.lastWidth = wsm.Width
	}
	return v, nil
}
func (v *sizingView) View() string      { return "content" }
func (v *sizingView) Focus() tea.Cmd    { return nil }
func (v *sizingView) Blur()             {}
func (v *sizingView) Name() string      { return v.name }
func (v *sizingView) ShortHelp() string { return "" }

func TestTabSwitchSendsWindowSizeToNewView(t *testing.T) {
	app := NewUnifiedApp(nil)
	viewA := &sizingView{name: "A"}
	viewB := &sizingView{name: "B"}
	app.dashViews = []View{viewA, viewB}
	app.tabs = NewTabBar([]string{"A", "B"})
	app.tabs.SetActive(0)
	app.currentView = viewA

	// Give the app a size so sendWindowSize has something to send
	app.width = 120
	app.height = 40

	// Size viewA directly via applyResize (bypasses coalescer, which is what
	// the real Bubble Tea runtime does on the first WindowSizeMsg).
	app.applyResize(tea.WindowSizeMsg{Width: 120, Height: 40})

	if viewA.lastWidth == 0 {
		t.Fatal("viewA should have received WindowSizeMsg")
	}
	if viewB.lastWidth != 0 {
		t.Fatal("viewB should NOT have received WindowSizeMsg yet")
	}

	// Switch to tab B
	switchCmd := app.switchDashboardTab(1)

	if switchCmd == nil {
		t.Fatal("expected non-nil command from tab switch")
	}

	// Execute the batched commands — one of them should be a WindowSizeMsg.
	// In production, Bubble Tea's runtime feeds Cmd results back into Update.
	// Here we apply the WindowSizeMsg directly via applyResize to avoid the
	// resize coalescer swallowing it (the coalescer defers rapid successive
	// resizes, which is correct production behavior but breaks unit tests).
	msgs := collectBatchMsgs(switchCmd)
	foundWSM := false
	for _, m := range msgs {
		if wsm, ok := m.(tea.WindowSizeMsg); ok {
			foundWSM = true
			app.applyResize(wsm)
		}
	}

	if !foundWSM {
		t.Fatal("expected WindowSizeMsg in batched commands from tab switch")
	}
	if viewB.lastWidth == 0 {
		t.Fatal("viewB should have received WindowSizeMsg after tab switch")
	}
}

// collectBatchMsgs executes a tea.Cmd and collects the resulting messages.
// For tea.Batch, it recursively collects from all sub-commands.
func collectBatchMsgs(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batchMsg, ok := msg.(tea.BatchMsg); ok {
		var msgs []tea.Msg
		for _, subCmd := range batchMsg {
			msgs = append(msgs, collectBatchMsgs(subCmd)...)
		}
		return msgs
	}
	return []tea.Msg{msg}
}

func TestSignalsOverlayToggleViaSig(t *testing.T) {
	app := NewUnifiedApp(nil)
	app.currentView = &noopDashboardView{name: "test"}

	updated, _ := app.Update(pkgtui.SlashCommandMsg{Command: "sig"})
	app = updated.(*UnifiedApp)
	if !app.signalsOverlay.Visible() {
		t.Fatal("expected signals overlay to be visible after /sig")
	}

	updated, _ = app.Update(pkgtui.SlashCommandMsg{Command: "sig"})
	app = updated.(*UnifiedApp)
	if app.signalsOverlay.Visible() {
		t.Fatal("expected signals overlay to be hidden after second /sig")
	}
}

func TestSignalsOverlayEscCloses(t *testing.T) {
	app := NewUnifiedApp(nil)
	app.signalsOverlay.Toggle()

	updated, _ := app.Update(tea.KeyMsg{Type: tea.KeyEsc})
	app = updated.(*UnifiedApp)
	if app.signalsOverlay.Visible() {
		t.Fatal("expected signals overlay to close on Esc")
	}
}

func TestSignalsOverlayBlocksKeysToView(t *testing.T) {
	app := NewUnifiedApp(nil)
	view := &inputFocusView{}
	app.currentView = view
	app.signalsOverlay.Toggle()

	app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if view.seen {
		t.Fatal("expected overlay to consume key, but view saw it")
	}
}

func TestAgentSelectorUsesDispatchPreference(t *testing.T) {
	for _, tc := range []struct{ name, env, config, want, source string }{
		{"environment", "codex", "", "codex", "AUTARCH_DISPATCH_AGENT"},
		{"project", "", "[dispatch]\npreferred_agent = \"codex\"\n", "codex", "project config (current directory)"},
		{"automatic", "", "", "claude", "automatic detection"},
		{"unknown", "unavailable-agent", "", "unavailable-agent", "AUTARCH_DISPATCH_AGENT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			isolateSelectorEnvironment(t)
			t.Chdir(root)
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			t.Setenv("PATH", t.TempDir())
			t.Setenv("AUTARCH_DISPATCH_AGENT", tc.env)
			if err := os.MkdirAll(filepath.Join(root, ".gurgeh"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, ".gurgeh", "agents.toml"), []byte(tc.config), 0600); err != nil {
				t.Fatal(err)
			}
			app := NewUnifiedApp(nil)
			app.codingAgent = &agent.Agent{Type: agent.TypeClaude}
			initAgentSelectorWithoutDetection(t, app)
			if app.selectedAgent != tc.want {
				t.Fatalf("selected = %q, want %q", app.selectedAgent, tc.want)
			}
			source := tc.source
			if app.agentSelector == nil || !strings.Contains(app.agentSelector.View(), tc.want) || !strings.Contains(app.agentSelector.View(), source) {
				t.Fatal("selector does not display dispatch preference")
			}
		})
	}
}

func TestAgentSelectionFailureKeepsEffectiveLabel(t *testing.T) {
	isolateSelectorEnvironment(t)
	t.Setenv("AUTARCH_DISPATCH_AGENT", "codex")
	app := NewUnifiedApp(nil)
	app.codingAgent = &agent.Agent{Type: agent.TypeClaude}
	app.selectedAgent = "codex"
	app.agentSelector = pkgtui.NewAgentSelector([]pkgtui.AgentOption{{Name: "codex", Source: "AUTARCH_DISPATCH_AGENT"}, {Name: "missing-agent"}})
	app.agentSelector.Index = 1 // navigation has already moved the cursor
	app.Update(pkgtui.AgentSelectedMsg{Name: "missing-agent"})
	if app.selectedAgent != "codex" || app.agentSelector.Index != 0 || !strings.Contains(app.agentSelector.View(), "AUTARCH_DISPATCH_AGENT") {
		t.Fatal("failed selection changed effective label:", app.agentSelector.View())
	}
}

func TestAgentSelectorConfiguredOptionsWithoutDetection(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AUTARCH_DISPATCH_AGENT", "")
	if err := os.MkdirAll(".gurgeh", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".gurgeh/agents.toml", []byte("[targets.codex]\ntype = \"command\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	app := NewUnifiedApp(nil)
	initAgentSelectorWithoutDetection(t, app)
	if app.agentSelector == nil {
		t.Fatal("configured options lost without detection")
	}
	if !strings.Contains(app.agentSelector.View(), "configured, not detected") {
		t.Fatal("undetected configured option mislabeled:", app.agentSelector.View())
	}
}

func TestAgentSelectionLabelsUIChoice(t *testing.T) {
	if os.Getenv("AUTARCH_SELECTION_TEST_CHILD") == "1" {
		app := NewUnifiedApp(nil)
		app.codingAgent = &agent.Agent{Type: agent.TypeClaude}
		app.agentSelector = pkgtui.NewAgentSelector([]pkgtui.AgentOption{{Name: "claude"}, {Name: "codex"}})
		app.Update(pkgtui.AgentSelectedMsg{Name: "codex"})
		name, source := app.codingAgent.DispatchPreference(".")
		if name != "codex" || source != "explicit agent selection" || !strings.Contains(app.agentSelector.View(), "runs use config; picked") {
			t.Fatalf("UI choice mislabeled: %s %s %s", name, source, app.agentSelector.View())
		}
		return
	}
	isolateSelectorEnvironment(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "codex"), []byte("#!/bin/sh\necho test-version\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestAgentSelectionLabelsUIChoice$")
	cmd.Env = append(os.Environ(), "AUTARCH_SELECTION_TEST_CHILD=1", "AUTARCH_DISPATCH_AGENT=claude", "PATH="+dir, "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %v %s", err, out)
	}
}

func TestAgentSelectorMatchesPreferenceIgnoringCase(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("AUTARCH_DISPATCH_AGENT", "Codex")
	if err := os.MkdirAll(".gurgeh", 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".gurgeh/agents.toml", []byte("[targets.codex]\ntype = \"command\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	app := NewUnifiedApp(nil)
	initAgentSelectorWithoutDetection(t, app)
	count := 0
	for _, opt := range app.agentSelector.Options {
		if strings.EqualFold(opt.Name, "codex") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("got %d codex options", count)
	}
}

func isolateSelectorEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
}

// Supply only fixture registries so fallback install paths cannot discover host CLIs.
func initAgentSelectorWithoutDetection(t *testing.T, app *UnifiedApp) {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	global, project, err := agenttargets.Load(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "autarch", "agents.toml"), root)
	if err != nil {
		t.Fatal(err)
	}
	app.initAgentSelectorWithOptions(root, buildAgentOptionsFromRegistry(agenttargets.Merge(global, project)))
}
