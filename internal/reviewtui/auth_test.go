package reviewtui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/pkg/review"
)

func TestAuthSecretMaskedAndNeverSavedAsNote(t *testing.T) {
	m := New(t.TempDir(), "Cozy", review.Client{})
	m.authOpen = true
	m.setAuth(&review.AuthState{RuntimeID: "runtime", Operation: &review.AuthOperation{ID: "op", Status: "pending", ExpiresAt: time.Now().Add(time.Minute).UnixMilli(), Prompt: &review.AuthPrompt{ID: "prompt", Type: "secret", Message: "API key"}}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("PRIVATE-KEY")})
	if strings.Contains(m.View(), "PRIVATE-KEY") {
		t.Fatal("secret displayed")
	}
	if m.input.Value() != "" || m.pendingSave != nil {
		t.Fatal("secret entered durable composer")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.authInput.Value() != "" {
		t.Fatal("secret draft retained after closing")
	}
}

func TestSensitiveProviderInstructionsNeverRenderInRecordedTUI(t *testing.T) {
	m := New(t.TempDir(), "Cozy", review.Client{})
	m.authOpen = true
	m.width = 120
	m.height = 40
	m.setAuth(&review.AuthState{RuntimeID: "runtime", Operation: &review.AuthOperation{ID: "op", Status: "pending", ExpiresAt: time.Now().Add(time.Minute).UnixMilli(), Events: []review.AuthEvent{{URL: "https://example.test/PRIVATE", UserCode: "PRIVATE", Instructions: "PRIVATE"}}, Prompt: &review.AuthPrompt{ID: "p", Message: "PRIVATE", Options: []review.AuthOption{{ID: "private", Label: "PRIVATE"}}}}})
	if strings.Contains(m.View(), "PRIVATE") {
		t.Fatal("provider instructions entered capture surface")
	}
}

func TestAuthPromptReplacementErasesStaleDraft(t *testing.T) {
	m := New(t.TempDir(), "Cozy", review.Client{})
	m.setAuth(&review.AuthState{RuntimeID: "runtime", Operation: &review.AuthOperation{ID: "op", Prompt: &review.AuthPrompt{ID: "first", Type: "secret"}}})
	m.authInput.SetValue("PRIVATE")
	m.setAuth(&review.AuthState{RuntimeID: "runtime", Operation: &review.AuthOperation{ID: "op", Prompt: &review.AuthPrompt{ID: "second", Type: "secret"}}})
	if m.authInput.Value() != "" {
		t.Fatal("stale prompt draft reused")
	}
}

func TestCaptureControlsRemainAvailableDuringAuthentication(t *testing.T) {
	m := New("/project", "Cozy", review.Client{})
	m.authOpen, m.authBusy = true, true
	for _, key := range []tea.KeyType{tea.KeyCtrlR, tea.KeyCtrlP, tea.KeyCtrlO, tea.KeyCtrlV} {
		if cmd := m.Update(tea.KeyMsg{Type: key}); cmd == nil {
			t.Fatalf("capture key %v blocked by authentication", key)
		}
	}
}

func TestHelpAndTraceDoNotAuthorizeAcceptance(t *testing.T) {
	for _, trace := range []string{"HELP", "TRACE"} {
		m := New("/project", "Cozy", review.Client{})
		m.tab = 1
		m.detail = true
		m.trace = trace
		p := review.Proposal{ID: "p", Project: "/project", Revision: 1}
		m.state.Proposals = map[string]review.Proposal{"p": p}
		m.reviewed = &p
		if cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA}); cmd != nil {
			t.Fatal("accepted while proposal hidden")
		}
	}
}
