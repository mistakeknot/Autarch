package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestAgentSelectorToggleAndSelect(t *testing.T) {
	s := NewAgentSelector([]AgentOption{{Name: "codex"}, {Name: "claude"}})

	// ctrl+g opens (MacBook-friendly alternative to F2)
	_, _ = s.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
	if !s.Open {
		t.Fatal("expected selector open after ctrl+g")
	}

	// Close it and test F2 still works
	_, _ = s.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_, _ = s.Update(tea.KeyMsg{Type: tea.KeyF2})
	if !s.Open {
		t.Fatal("expected selector open after F2")
	}

	// Down + enter selects second option
	_, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown})
	msg, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if sel, ok := msg.(AgentSelectedMsg); !ok || sel.Name != "claude" {
		t.Fatalf("expected selection of claude, got %#v", msg)
	}
	if s.Open {
		t.Fatal("expected selector closed after selection")
	}
}

func TestAgentSelectorIgnoresNumericPick(t *testing.T) {
	s := NewAgentSelector([]AgentOption{{Name: "codex"}, {Name: "claude"}})
	_, _ = s.Update(tea.KeyMsg{Type: tea.KeyF2})

	msg, _ := s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'2'}})
	if msg != nil {
		t.Fatalf("expected no selection on numeric key, got %#v", msg)
	}
}

func TestAgentSelectorRendersAsDispatchControl(t *testing.T) {
	sel := NewAgentSelector([]AgentOption{{Name: "codex"}})
	view := sel.View()
	if !strings.Contains(view, "Dispatch: codex (ctrl+g)") {
		t.Fatalf("expected Dispatch label")
	}
}

func TestAgentSelectorClosedLabelFitsWidth(t *testing.T) {
	s := NewAgentSelector([]AgentOption{{Name: "\x1b[31m模型\x1b[0m", Source: "project config"}})
	for _, width := range []int{0, 1, 12, 30} {
		if got := lipgloss.Width(s.View(width)); got > width {
			t.Fatalf("width %d exceeds allocation %d", got, width)
		}
	}
	picked := NewAgentSelector([]AgentOption{{Name: "claude", Source: "explicit agent selection"}})
	narrow := picked.View(40)
	if lipgloss.Width(narrow) > 40 || !strings.Contains(narrow, "explicit agent") || !strings.Contains(narrow, "claude") {
		t.Fatalf("narrow label lost dispatch meaning: %s", narrow)
	}
	picked.Update(tea.KeyMsg{Type: tea.KeyF2})
	if !strings.Contains(picked.View(), "Dispatch: ") {
		t.Fatal("open label uses a different term")
	}

}

func TestChatPanelShowsAgentPreferenceSource(t *testing.T) {
	p := NewChatPanel()
	p.SetSize(60, 20)
	p.SetAgentSelector(NewAgentSelector([]AgentOption{{Name: "codex", Source: "project config"}}))
	if !strings.Contains(p.View(), "project config") {
		t.Fatal("closed composer lost preference source")
	}
}

func TestAgentSelectorCancelRestoresEffectiveLabel(t *testing.T) {
	for _, closeKey := range []tea.KeyType{tea.KeyEsc, tea.KeyF2, tea.KeyCtrlG} {
		t.Run(tea.KeyMsg{Type: closeKey}.String(), func(t *testing.T) {
			s := NewAgentSelector([]AgentOption{{Name: "codex", Source: "AUTARCH_DISPATCH_AGENT"}, {Name: "claude", Source: "detected"}})
			before := s.View()
			s.Update(tea.KeyMsg{Type: tea.KeyF2})
			s.Update(tea.KeyMsg{Type: tea.KeyDown})
			if s.Index != 1 {
				t.Fatal("navigation did not move cursor")
			}
			s.Update(tea.KeyMsg{Type: closeKey})
			if s.Open || s.Index != 0 || s.View() != before {
				t.Fatalf("cancel changed effective label: %s", s.View())
			}
			// A subsequent confirmed choice becomes the next opening's baseline.
			s.Update(tea.KeyMsg{Type: tea.KeyCtrlG})
			s.Update(tea.KeyMsg{Type: tea.KeyDown})
			s.Update(tea.KeyMsg{Type: tea.KeyEnter})
			s.Index = 1 // app confirms the requested choice
			s.Options[1].Source = "explicit agent selection"
			before = s.View()
			s.Update(tea.KeyMsg{Type: tea.KeyF2})
			s.Update(tea.KeyMsg{Type: tea.KeyUp})
			s.Update(tea.KeyMsg{Type: closeKey})
			if s.Index != 1 || s.View() != before {
				t.Fatal("cancel lost confirmed selection")
			}
		})
	}
}

func TestAgentSelectorEnterWaitsForConfirmation(t *testing.T) {
	s := NewAgentSelector([]AgentOption{{Name: "codex", Source: "AUTARCH_DISPATCH_AGENT"}, {Name: "claude", Source: "detected"}})
	before := s.View()
	s.Update(tea.KeyMsg{Type: tea.KeyF2})
	s.Update(tea.KeyMsg{Type: tea.KeyDown})
	msg, _ := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if picked, ok := msg.(AgentSelectedMsg); !ok || picked.Name != "claude" {
		t.Fatal("lost requested pick")
	}
	if s.Open || s.View() != before {
		t.Fatal("unconfirmed pick changed closed label:", s.View())
	}
}

func TestAgentSelectorOpenLabelFitsWidth(t *testing.T) {
	s := NewAgentSelector([]AgentOption{{Name: "claude"}, {Name: "codex"}})
	s.Open = true
	for _, width := range []int{0, 1, 12, 20} {
		if got := lipgloss.Width(s.View(width)); got > width {
			t.Errorf("open selector width %d exceeds allocation %d", got, width)
		}
	}
}

func TestAgentSelectorHighlightedOptionVisible(t *testing.T) {
	s := NewAgentSelector([]AgentOption{{Name: "claude"}, {Name: "codex"}, {Name: "\x1b[31m模型agent\x1b[0m"}})
	s.Open = true
	for index := range s.Options {
		s.Index = index
		for width := 0; width <= 80; width++ {
			got := s.View(width)
			plain := ansi.Strip(got)
			if lipgloss.Width(got) > width {
				t.Fatalf("width %d: %q overflows", width, got)
			}
			if width > 0 && !strings.Contains(plain, ">") {
				t.Fatalf("index %d width %d hides highlight: %q", index, width, got)
			}
			want := "> " + ansi.Strip(s.Options[index].Name)
			if width >= lipgloss.Width(want) && !strings.Contains(plain, want) {
				t.Fatalf("index %d width %d hides option: %q", index, width, got)
			}
			if strings.Contains(plain, "\x1b") {
				t.Fatalf("broken ANSI: %q", got)
			}
		}
	}
}

func TestAgentSelectorOptionWindow(t *testing.T) {
	s := NewAgentSelector([]AgentOption{{Name: "claude"}, {Name: "codex"}, {Name: "gemini"}, {Name: "kimi"}})
	s.Open = true
	for index := range s.Options {
		s.Index = index
		full := s.View()
		fullWidth := lipgloss.Width(full)
		if got := s.View(fullWidth); got != full {
			t.Fatalf("full width changed: %q != %q", got, full)
		}
		for width := 0; width < fullWidth; width++ {
			got := ansi.Strip(s.View(width))
			selected := "> " + s.Options[index].Name
			if lipgloss.Width(got) > width {
				t.Fatalf("width %d overflow: %q", width, got)
			}
			if width > 0 && !strings.Contains(got, ">") {
				t.Fatalf("width %d hides highlight: %q", width, got)
			}
			if width >= lipgloss.Width(selected) && !strings.Contains(got, selected) {
				t.Fatalf("width %d hides option: %q", width, got)
			}
			if width >= lipgloss.Width("Dispatch: "+selected) && !strings.HasPrefix(got, "Dispatch: ") {
				t.Fatalf("width %d loses prefix: %q", width, got)
			}
			neighbourVisible := false
			for i, opt := range s.Options {
				if i != index && strings.Contains(got, opt.Name) {
					neighbourVisible = true
				}
			}
			if width == fullWidth-1 && !neighbourVisible {
				t.Fatalf("near full width loses neighbours: %q", got)
			}
		}
	}
}

func TestAgentSelectorInvalidIndexFitsWidth(t *testing.T) {
	s := NewAgentSelector([]AgentOption{{Name: "claude"}, {Name: "codex"}})
	s.Open = true
	for _, index := range []int{-1, len(s.Options)} {
		s.Index = index
		for width := 0; width <= 30; width++ {
			if got := lipgloss.Width(s.View(width)); got > width {
				t.Fatalf("index %d width %d: rendered width %d", index, width, got)
			}
		}
	}
}
