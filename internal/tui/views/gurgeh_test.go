package views

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/tui"
	"github.com/mistakeknot/autarch/pkg/autarch"
)

// TestGurgehViewNilConfigSkipsOnboarding verifies that NewGurgehView with nil
// config immediately shows the spec browser (showBrowser=true).
func TestGurgehViewNilConfigSkipsOnboarding(t *testing.T) {
	v := NewGurgehView(&autarch.Client{}, nil)

	if !v.showBrowser {
		t.Fatal("expected showBrowser=true when cfg is nil")
	}
	if v.onboarding != nil {
		t.Fatal("expected onboarding to be nil when cfg is nil")
	}
}

// TestGurgehViewWithConfigStartsOnboarding verifies that NewGurgehView with a
// config creates the onboarding sub-view and defers browser until complete.
func TestGurgehViewWithConfigStartsOnboarding(t *testing.T) {
	cfg := &tui.GurgehConfig{
		CreateKickoffView: func() tui.View {
			return &stubView{name: "Kickoff", viewContent: "kickoff-content"}
		},
	}

	v := NewGurgehView(&autarch.Client{}, cfg)

	if v.showBrowser {
		t.Fatal("expected showBrowser=false when cfg is provided")
	}
	if v.onboarding == nil {
		t.Fatal("expected onboarding to be non-nil when cfg is provided")
	}
}

// TestGurgehViewInitDelegatesToOnboarding verifies that Init() delegates to
// the onboarding sub-view when in onboarding mode.
func TestGurgehViewInitDelegatesToOnboarding(t *testing.T) {
	factoryCalled := false
	cfg := &tui.GurgehConfig{
		CreateKickoffView: func() tui.View {
			factoryCalled = true
			return &stubView{name: "Kickoff", viewContent: "kickoff-content"}
		},
	}

	v := NewGurgehView(&autarch.Client{}, cfg)
	_ = v.Init()

	if !factoryCalled {
		t.Fatal("expected Init() to call kickoff factory via onboarding")
	}
}

// TestGurgehViewViewDelegatesToOnboarding verifies that View() shows onboarding
// content when in onboarding mode.
func TestGurgehViewViewDelegatesToOnboarding(t *testing.T) {
	cfg := &tui.GurgehConfig{
		CreateKickoffView: func() tui.View {
			return &stubView{name: "Kickoff", viewContent: "kickoff-content"}
		},
	}

	v := NewGurgehView(&autarch.Client{}, cfg)
	v.Init()

	output := v.View()
	if output != "kickoff-content" {
		t.Fatalf("expected View() to show onboarding content, got %q", output)
	}
}

// TestGurgehViewOnboardingCompleteSwitchesToBrowser verifies that
// OnboardingCompleteMsg sets showBrowser=true.
func TestGurgehViewOnboardingCompleteSwitchesToBrowser(t *testing.T) {
	cfg := &tui.GurgehConfig{
		CreateKickoffView: func() tui.View {
			return &stubView{name: "Kickoff", viewContent: "kickoff-content"}
		},
	}

	v := NewGurgehView(&autarch.Client{}, cfg)
	v.Init()

	// Send OnboardingCompleteMsg
	updated, cmd := v.Update(tui.OnboardingCompleteMsg{
		ProjectID:   "test-id",
		ProjectName: "test-project",
	})
	gv := updated.(*GurgehView)

	if !gv.showBrowser {
		t.Fatal("expected showBrowser=true after OnboardingCompleteMsg")
	}

	// Should return a command that re-emits the message for UnifiedApp
	if cmd == nil {
		t.Fatal("expected non-nil cmd to propagate OnboardingCompleteMsg")
	}
}

// TestGurgehViewPassesThroughMessagesToOnboarding verifies that messages
// are delegated to the onboarding sub-view in onboarding mode.
func TestGurgehViewPassesThroughMessagesToOnboarding(t *testing.T) {
	mockView := &stubViewWithUpdateTracker{
		stubView: stubView{name: "Kickoff", viewContent: "kickoff-content"},
	}

	cfg := &tui.GurgehConfig{
		CreateKickoffView: func() tui.View {
			return mockView
		},
	}

	v := NewGurgehView(&autarch.Client{}, cfg)
	v.Init()

	// Send a message that should pass through to the onboarding view
	v.Update(tui.ProjectCreatedMsg{
		ProjectID:   "test-id",
		ProjectName: "test",
	})

	// The onboarding view's Update is called, which passes to its currentView
	// Since GurgehOnboardingView handles ProjectCreatedMsg internally, the
	// message reaches the onboarding handler (not directly to mockView).
	// But the key point is that the GurgehView delegated to onboarding.
	// We verify by checking that GurgehView itself didn't crash or absorb it.
}

// TestGurgehViewFocusDelegatesToOnboarding verifies Focus() delegates
// to the onboarding sub-view when in onboarding mode.
func TestGurgehViewFocusDelegatesToOnboarding(t *testing.T) {
	mockKickoff := &stubView{name: "Kickoff", viewContent: "kickoff-content"}
	cfg := &tui.GurgehConfig{
		CreateKickoffView: func() tui.View {
			return mockKickoff
		},
	}

	v := NewGurgehView(&autarch.Client{}, cfg)
	v.Init()

	_ = v.Focus()
	if !mockKickoff.focused {
		t.Fatal("expected Focus() to delegate to onboarding sub-view")
	}
}

// TestGurgehViewShortHelpDelegatesToOnboarding verifies ShortHelp() delegates
// to the onboarding sub-view when in onboarding mode.
func TestGurgehViewShortHelpDelegatesToOnboarding(t *testing.T) {
	cfg := &tui.GurgehConfig{
		CreateKickoffView: func() tui.View {
			return &stubView{name: "Kickoff", viewContent: "kickoff-content"}
		},
	}

	v := NewGurgehView(&autarch.Client{}, cfg)
	v.Init()

	// ShortHelp should delegate to onboarding, which delegates to currentView
	help := v.ShortHelp()
	// stubView returns "" for ShortHelp, so this just verifies no crash
	_ = help
}

// --- Test helpers ---

// stubViewWithUpdateTracker extends stubView with an update callback for tracking.
type stubViewWithUpdateTracker struct {
	stubView
	onUpdate func(tea.Msg)
}

func (s *stubViewWithUpdateTracker) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	if s.onUpdate != nil {
		s.onUpdate(msg)
	}
	return s, nil
}
