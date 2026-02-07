package views

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/tui"
)

// TestGurgehOnboardingViewInit verifies that Init creates the kickoff view
// via the factory and sets the currentView.
func TestGurgehOnboardingViewInit(t *testing.T) {
	factoryCalled := false

	mockKickoff := &stubView{name: "Kickoff", viewContent: "kickoff-content"}

	cfg := tui.GurgehConfig{
		CreateKickoffView: func() tui.View {
			factoryCalled = true
			return mockKickoff
		},
	}

	v := NewGurgehOnboardingView(cfg)
	_ = v.Init()

	if !factoryCalled {
		t.Fatal("expected CreateKickoffView factory to be called")
	}

	if v.State() != tui.OnboardingKickoff {
		t.Fatalf("expected state OnboardingKickoff, got %v", v.State())
	}

	if v.Name() != "Gurgeh" {
		t.Fatalf("expected Name() == 'Gurgeh', got %q", v.Name())
	}

	// Verify the currentView was set by checking View() output
	if v.View() != "kickoff-content" {
		t.Fatalf("expected View() to show kickoff content after Init, got %q", v.View())
	}
}

// TestGurgehOnboardingViewProjectCreated verifies that ProjectCreatedMsg
// transitions state to OnboardingInterview and creates the sprint view.
func TestGurgehOnboardingViewProjectCreated(t *testing.T) {
	sprintFactoryCalled := false

	mockSprint := &stubView{name: "Sprint"}

	cfg := tui.GurgehConfig{
		CreateKickoffView: func() tui.View {
			return &stubView{name: "Kickoff"}
		},
		CreateSprintView: func(path string) tui.View {
			sprintFactoryCalled = true
			return mockSprint
		},
	}

	v := NewGurgehOnboardingView(cfg)
	v.Init() // sets up kickoff view

	// Send ProjectCreatedMsg
	msg := tui.ProjectCreatedMsg{
		ProjectID:   "test-id",
		ProjectName: "test-project",
		Description: "A test project",
	}
	updated, cmd := v.Update(msg)
	_ = cmd

	onb := updated.(*GurgehOnboardingView)
	if onb.State() != tui.OnboardingInterview {
		t.Fatalf("expected state OnboardingInterview after ProjectCreatedMsg, got %v", onb.State())
	}

	if !sprintFactoryCalled {
		t.Fatal("expected CreateSprintView factory to be called")
	}

	if onb.projectID != "test-id" {
		t.Fatalf("expected projectID 'test-id', got %q", onb.projectID)
	}

	if onb.projectName != "test-project" {
		t.Fatalf("expected projectName 'test-project', got %q", onb.projectName)
	}
}

// TestGurgehOnboardingViewInitNoFactory verifies Init returns nil when
// no kickoff factory is set.
func TestGurgehOnboardingViewInitNoFactory(t *testing.T) {
	cfg := tui.GurgehConfig{}
	v := NewGurgehOnboardingView(cfg)
	cmd := v.Init()

	if cmd != nil {
		t.Fatal("expected nil command when no kickoff factory is set")
	}
}

// TestGurgehOnboardingViewViewDelegates verifies View() delegates to currentView.
func TestGurgehOnboardingViewViewDelegates(t *testing.T) {
	cfg := tui.GurgehConfig{
		CreateKickoffView: func() tui.View {
			return &stubView{name: "Kickoff", viewContent: "kickoff-content"}
		},
	}

	v := NewGurgehOnboardingView(cfg)
	v.Init()

	output := v.View()
	if output != "kickoff-content" {
		t.Fatalf("expected View() to delegate to currentView, got %q", output)
	}
}

// --- Test helpers ---

// stubView is a minimal View implementation for testing.
type stubView struct {
	name        string
	viewContent string
	focused     bool
}

func (s *stubView) Init() tea.Cmd                        { return nil }
func (s *stubView) Update(tea.Msg) (tui.View, tea.Cmd)   { return s, nil }
func (s *stubView) View() string                         { return s.viewContent }
func (s *stubView) Focus() tea.Cmd                       { s.focused = true; return nil }
func (s *stubView) Blur()                                { s.focused = false }
func (s *stubView) Name() string                         { return s.name }
func (s *stubView) ShortHelp() string                    { return "" }
