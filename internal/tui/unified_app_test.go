package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/gurgeh/arbiter"
	"github.com/mistakeknot/autarch/internal/pollard/research"
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

func TestScanResultSetsInterviewBreadcrumb(t *testing.T) {
	app := NewUnifiedApp(nil)
	app.onboardingState = OnboardingKickoff

	_, _ = app.Update(CodebaseScanResultMsg{})

	if app.onboardingState != OnboardingInterview {
		t.Fatalf("expected onboarding to move to interview after scan")
	}
}

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

func TestRunUnifiedEnablesMouse(t *testing.T) {
	// No direct program option introspection available here.
	// Verify manually by running the app and confirming wheel events scroll chat.
}

// mockSprintView implements the View and SprintStarter interfaces for testing
// the Kickoff → Sprint transition without importing internal/tui/views.
type mockSprintView struct {
	startCalled bool
	userInput   string
	orch        *arbiter.Orchestrator
}

func (v *mockSprintView) Init() tea.Cmd                             { return nil }
func (v *mockSprintView) Update(msg tea.Msg) (pkgtui.View, tea.Cmd) { return v, nil }
func (v *mockSprintView) View() string                              { return "sprint view content" }
func (v *mockSprintView) Focus() tea.Cmd                            { return nil }
func (v *mockSprintView) Blur()                                     {}
func (v *mockSprintView) Name() string                              { return "Sprint" }
func (v *mockSprintView) ShortHelp() string                         { return "Sprint help" }
func (v *mockSprintView) StartSprint(userInput string) tea.Cmd {
	v.startCalled = true
	v.userInput = userInput
	return func() tea.Msg {
		return SprintDraftUpdatedMsg{Phase: "vision", Content: "test draft"}
	}
}
func (v *mockSprintView) Orchestrator() *arbiter.Orchestrator { return v.orch }

// TestProjectCreatedMsgTransitionsToSprintView verifies that sending
// ProjectCreatedMsg switches from Kickoff to SprintView and starts the sprint.
func TestProjectCreatedMsgTransitionsToSprintView(t *testing.T) {
	app := NewUnifiedApp(nil)

	// Track which mock was created
	var createdSprintView *mockSprintView

	// Set up SprintView factory (mimics main.go wiring)
	app.SetSprintViewFactory(func(projectPath string) View {
		createdSprintView = &mockSprintView{}
		return createdSprintView
	})

	// Set up Kickoff factory for Init
	app.SetViewFactories(
		func() View { return &noopDashboardView{name: "Kickoff"} },
		nil, nil, nil, nil, nil,
	)

	// Initialize the app (creates Kickoff view)
	app.Init()

	// Verify starting state
	if app.currentView == nil {
		t.Fatal("currentView is nil after Init")
	}
	if app.onboardingState != OnboardingKickoff {
		t.Fatalf("expected OnboardingKickoff state, got %v", app.onboardingState)
	}

	// Simulate user completing Kickoff form
	msg := ProjectCreatedMsg{
		ProjectID:   "test-123",
		ProjectName: "Test Project",
		Description: "A test project for verifying transitions",
	}

	// Send the message
	updated, cmd := app.Update(msg)
	app = updated.(*UnifiedApp)

	// Should return commands (Init, Focus, sendWindowSize, StartSprint)
	if cmd == nil {
		t.Error("Expected commands from transition, got nil")
	}

	// Verify the SprintView factory was called
	if createdSprintView == nil {
		t.Fatal("SprintView factory was not called")
	}

	// Verify currentView is now the SprintView
	if app.currentView != createdSprintView {
		t.Errorf("currentView not updated to SprintView, got %T", app.currentView)
	}

	// Verify onboarding state changed to Interview
	if app.onboardingState != OnboardingInterview {
		t.Errorf("Expected OnboardingInterview state, got %v", app.onboardingState)
	}

	// Verify StartSprint was called with the description
	if !createdSprintView.startCalled {
		t.Error("StartSprint was not called on SprintView")
	}
	if createdSprintView.userInput != msg.Description {
		t.Errorf("StartSprint called with wrong input: got %q, want %q",
			createdSprintView.userInput, msg.Description)
	}
}

// TestProjectCreatedMsgFallsBackWithoutSprintFactory verifies graceful handling
// when no SprintView factory is set.
func TestProjectCreatedMsgFallsBackWithoutSprintFactory(t *testing.T) {
	app := NewUnifiedApp(nil)

	// Only set Kickoff factory, NOT SprintView factory
	app.SetViewFactories(
		func() View { return &noopDashboardView{name: "Kickoff"} },
		nil, nil, nil, nil, nil,
	)

	app.Init()

	msg := ProjectCreatedMsg{
		ProjectID:   "test-456",
		ProjectName: "Test Project",
		Description: "Testing fallback behavior",
	}

	// Should not panic
	updated, cmd := app.Update(msg)
	app = updated.(*UnifiedApp)

	// Should return a command (InterviewCompleteMsg fallback)
	if cmd == nil {
		t.Error("Expected fallback command, got nil")
	}

	// State should still update to Interview
	if app.onboardingState != OnboardingInterview {
		t.Errorf("Expected OnboardingInterview state even with fallback, got %v", app.onboardingState)
	}
}

// TestSprintCompleteMsgTransitionsToSpecSummary verifies that sending
// SprintCompleteMsg switches from SprintView to SpecSummaryView.
func TestSprintCompleteMsgTransitionsToSpecSummary(t *testing.T) {
	app := NewUnifiedApp(nil)

	// Create orchestrator with test state
	orch := arbiter.NewOrchestrator("")
	state := arbiter.NewSprintState("")
	state.Sections[arbiter.PhaseVision].Content = "Build an amazing app"
	state.Sections[arbiter.PhaseProblem].Content = "Users struggle with X"
	state.Sections[arbiter.PhaseUsers].Content = "Developers and teams"
	state.Sections[arbiter.PhaseRequirements].Content = "- Fast\n- Reliable\n- Easy to use"
	orch.SetStateForTest(state)

	sprintView := &mockSprintView{orch: orch}
	app.currentView = sprintView
	app.onboardingState = OnboardingInterview // SprintView is shown during Interview state

	// Track SpecSummary creation
	var createdSpec *SpecSummary
	app.createSpecSummaryView = func(spec *SpecSummary, coord *research.Coordinator) View {
		createdSpec = spec
		return &noopDashboardView{name: "SpecSummary"}
	}

	// Send SprintCompleteMsg
	msg := SprintCompleteMsg{}
	updated, cmd := app.Update(msg)
	app = updated.(*UnifiedApp)

	// Should return commands (Init, Focus, sendWindowSize)
	if cmd == nil {
		t.Error("Expected commands from transition, got nil")
	}

	// Verify SpecSummary factory was called with extracted state
	if createdSpec == nil {
		t.Fatal("SpecSummary factory was not called")
	}
	if createdSpec.Vision != "Build an amazing app" {
		t.Errorf("Expected Vision 'Build an amazing app', got %q", createdSpec.Vision)
	}
	if createdSpec.Problem != "Users struggle with X" {
		t.Errorf("Expected Problem 'Users struggle with X', got %q", createdSpec.Problem)
	}
	if createdSpec.Users != "Developers and teams" {
		t.Errorf("Expected Users 'Developers and teams', got %q", createdSpec.Users)
	}
	if len(createdSpec.Requirements) != 3 {
		t.Errorf("Expected 3 requirements, got %d: %v", len(createdSpec.Requirements), createdSpec.Requirements)
	}

	// Verify state transitioned
	if app.onboardingState != OnboardingSpecSummary {
		t.Errorf("Expected OnboardingSpecSummary state, got %v", app.onboardingState)
	}

	// Verify current view changed
	if app.currentView.Name() != "SpecSummary" {
		t.Errorf("Expected SpecSummary view, got %s", app.currentView.Name())
	}
}

// TestSprintCompleteMsgFallsBackToOnboardingComplete verifies graceful handling
// when SprintView or SpecSummaryView factory is unavailable.
func TestSprintCompleteMsgFallsBackToOnboardingComplete(t *testing.T) {
	app := NewUnifiedApp(nil)

	// Set up a view that doesn't implement SprintStateProvider
	app.currentView = &noopDashboardView{name: "NotASprint"}

	// No SpecSummary factory
	app.createSpecSummaryView = nil

	msg := SprintCompleteMsg{}
	_, cmd := app.Update(msg)

	// Should return a fallback command
	if cmd == nil {
		t.Error("Expected fallback command, got nil")
	}

	// Execute the command to get OnboardingCompleteMsg
	resultMsg := cmd()
	if _, ok := resultMsg.(OnboardingCompleteMsg); !ok {
		t.Errorf("Expected OnboardingCompleteMsg fallback, got %T", resultMsg)
	}
}

func TestUnifiedAppSkipOnboardingGoesToDashboard(t *testing.T) {
	app := NewUnifiedApp(nil)
	app.SetSkipOnboarding(true)
	app.SetViewFactories(
		func() View { return &noopDashboardView{name: "Kickoff"} },
		nil, nil, nil, nil,
		func(c *autarch.Client) []View {
			return []View{
				&noopDashboardView{name: "Bigend"},
				&noopDashboardView{name: "Gurgeh"},
			}
		},
	)

	app.Init()

	if app.mode != ModeDashboard {
		t.Fatalf("expected ModeDashboard, got %v", app.mode)
	}
	if len(app.dashViews) != 2 {
		t.Fatalf("expected 2 dashboard views, got %d", len(app.dashViews))
	}
}
