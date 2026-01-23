package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/agents"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
)

func TestInterviewCreatesSpecWithWarnings(t *testing.T) {
	withTempRootInitialized(t, func(root string) {
		m := NewModel()
		m = pressKey(m, "n")
		m = pressKey(m, "2")
		m = pressKey(m, "1")
		m = pressKey(m, "2")
		m = typeText(m, "Vision statement")
		m = pressKey(m, "]")
		m = typeText(m, "Primary users")
		m = pressKey(m, "]")
		m = typeText(m, "Problem to solve")
		m = pressKey(m, "]")
		m = typeText(m, "First requirement")
		m = pressKey(m, "]")
		m = pressKey(m, "2")
		files := praudeSpecFiles(t, root)
		if len(files) != 1 {
			t.Fatalf("expected one spec file, got %d", len(files))
		}
		path := filepath.Join(root, ".praude", "specs", files[0])
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "critical_user_journeys") {
			t.Fatalf("expected cuj section")
		}
		if !strings.Contains(string(raw), "validation_warnings") {
			t.Fatalf("expected validation warnings metadata")
		}
	})
}

func TestInterviewMentionsPMFocusedAgent(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m.mode = "interview"
		m.interview = startInterview(m.root, specs.Spec{}, "")
		out := m.View()
		if !strings.Contains(out, "PM-focused") {
			t.Fatalf("expected PM-focused agent hint")
		}
	})
}

func TestInterviewShowsIterationHint(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m.mode = "interview"
		m.interview = startInterview(m.root, specs.Spec{}, "")
		m.interview.step = stepVision
		m.input.SetText("")
		out := stripANSI(m.View())
		if !strings.Contains(out, "Enter: iterate") {
			t.Fatalf("expected iterate hint")
		}
	})
}

func TestInterviewInputArrowLeftMovesCursor(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m.mode = "interview"
		m.interview = startInterview(m.root, specs.Spec{}, "")
		m.interview.step = stepVision
		m.interviewFocus = "question"
		m.input.SetText("hello")
		m = pressKey(m, "left")
		m = typeText(m, "X")
		if got := m.input.Text(); got != "hellXo" {
			t.Fatalf("expected cursor insert, got %q", got)
		}
	})
}

func TestInterviewInputSpaceInserts(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m.mode = "interview"
		m.interview = startInterview(m.root, specs.Spec{}, "")
		m.interview.step = stepVision
		m.interviewFocus = "question"
		m.input.SetText("hi")
		m = pressKey(m, "left")
		m = pressKey(m, "space")
		if got := m.input.Text(); got != "h i" {
			t.Fatalf("expected space insert, got %q", got)
		}
	})
}

func TestInterviewMarkdownInputBoxIsPlain(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m.mode = "interview"
		m.interview = startInterview(m.root, specs.Spec{}, "")
		m.interview.step = stepVision
		m.input.SetText("Hello")
		out := m.interviewMarkdown()
		if strings.Contains(out, "\x1b[") {
			t.Fatalf("expected no ANSI in markdown")
		}
		if !strings.Contains(out, "+") || !strings.Contains(out, "|") {
			t.Fatalf("expected ascii input box")
		}
		if !strings.Contains(out, "Input (line") {
			t.Fatalf("expected cursor status line")
		}
	})
}

func TestInterviewChatRendersTranscript(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m.mode = "interview"
		m.interview = startInterview(m.root, specs.Spec{}, "")
		m.interview.step = stepVision
		m.interview.chat = []interviewMessage{
			{Role: "user", Text: "User line"},
			{Role: "agent", Text: "Agent line"},
		}
		out := stripANSI(m.View())
		if !strings.Contains(out, "User") || !strings.Contains(out, "Agent") {
			t.Fatalf("expected chat transcript roles")
		}
		if !strings.Contains(out, "Compose") {
			t.Fatalf("expected composer label")
		}
	})
}

func TestInterviewLayoutShowsHeaderAndPanels(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m.mode = "interview"
		m.interview = startInterview(m.root, specs.Spec{}, "")
		out := stripANSI(m.View())
		if !strings.Contains(out, "Scan") || !strings.Contains(out, "Vision") {
			t.Fatalf("expected header nav steps")
		}
		if !strings.Contains(out, "PRDs") || !strings.Contains(out, "SECTION") {
			t.Fatalf("expected top panels")
		}
		if !strings.Contains(out, "Open file: Ctrl+O") {
			t.Fatalf("expected open file hint")
		}
		if !strings.Contains(out, "Compose") {
			t.Fatalf("expected composer label")
		}
	})
}

func TestInterviewShowsStepAndInputField(t *testing.T) {
	withTempRootInitialized(t, func(root string) {
		m := NewModel()
		m = pressKey(m, "n")
		m = pressKey(m, "2")
		m = pressKey(m, "1")
		m = pressKey(m, "2")
		out := m.View()
		clean := stripANSI(out)
		if !strings.Contains(clean, "Compose:") {
			t.Fatalf("expected composer label")
		}
		if !strings.Contains(clean, "Enter: iterate") {
			t.Fatalf("expected iterate hint")
		}
	})
}

func TestInterviewShowsStepSidebar(t *testing.T) {
	withTempRootInitialized(t, func(root string) {
		m := NewModel()
		m = pressKey(m, "n")
		out := m.View()
		clean := stripANSI(out)
		if !strings.Contains(clean, "Scan repo") {
			t.Fatalf("expected scan step in header nav")
		}
		if !strings.Contains(clean, "Confirm draft") {
			t.Fatalf("expected confirm step in header nav")
		}
	})
}

func TestInterviewAutoAppliesSuggestions(t *testing.T) {
	withTempRootInitialized(t, func(root string) {
		if err := os.MkdirAll(filepath.Join(root, ".praude", "suggestions"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, ".praude", "briefs"), 0o755); err != nil {
			t.Fatal(err)
		}
		cfg := `validation_mode = "soft"

[agents.codex]
command = "codex"
args = []
`
		if err := os.WriteFile(filepath.Join(root, ".praude", "config.toml"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
		oldLaunch := launchAgent
		oldSub := launchSubagent
		launchAgent = func(p agents.Profile, briefPath string) error {
			entries, err := os.ReadDir(filepath.Join(root, ".praude", "suggestions"))
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				return nil
			}
			path := filepath.Join(root, ".praude", "suggestions", entries[0].Name())
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			updated := strings.ReplaceAll(string(raw), "status: pending", "status: ready")
			updated = strings.Replace(updated, "suggestion: \"\"", "suggestion: \"Agent summary\"", 1)
			updated = strings.Replace(updated, "\"REQ-001: Add requirement\"", "\"REQ-002: Agent requirement\"", 1)
			return os.WriteFile(path, []byte(updated), 0o644)
		}
		launchSubagent = launchAgent
		defer func() {
			launchAgent = oldLaunch
			launchSubagent = oldSub
		}()
		m := NewModel()
		m = pressKey(m, "n")
		m = pressKey(m, "2")
		m = pressKey(m, "1")
		m = pressKey(m, "2")
		m = typeText(m, "Vision statement")
		m = pressKey(m, "]")
		m = typeText(m, "Primary users")
		m = pressKey(m, "]")
		m = typeText(m, "Problem to solve")
		m = pressKey(m, "]")
		m = typeText(m, "First requirement")
		m = pressKey(m, "]")
		m = pressKey(m, "2")
		files := praudeSpecFiles(t, root)
		if len(files) != 1 {
			t.Fatalf("expected one spec file, got %d", len(files))
		}
		path := filepath.Join(root, ".praude", "specs", files[0])
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), "summary: Agent summary") {
			t.Fatalf("expected agent summary applied")
		}
		if !strings.Contains(string(raw), "REQ-002: Agent requirement") {
			t.Fatalf("expected agent requirements applied")
		}
	})
}

func pressKey(m Model, key string) Model {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	if key == "enter" {
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	}
	if key == "tab" {
		msg = tea.KeyMsg{Type: tea.KeyTab}
	}
	if key == "esc" {
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	}
	if key == "left" {
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	}
	if key == "right" {
		msg = tea.KeyMsg{Type: tea.KeyRight}
	}
	if key == "up" {
		msg = tea.KeyMsg{Type: tea.KeyUp}
	}
	if key == "down" {
		msg = tea.KeyMsg{Type: tea.KeyDown}
	}
	if key == "alt+left" {
		msg = tea.KeyMsg{Type: tea.KeyLeft, Alt: true}
	}
	if key == "alt+right" {
		msg = tea.KeyMsg{Type: tea.KeyRight, Alt: true}
	}
	if key == "alt+backspace" {
		msg = tea.KeyMsg{Type: tea.KeyBackspace, Alt: true}
	}
	if key == "space" {
		msg = tea.KeyMsg{Type: tea.KeySpace}
	}
	updated, _ := m.Update(msg)
	return updated.(Model)
}

func typeAndEnter(m Model, input string) Model {
	for _, r := range input {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		updated, _ := m.Update(msg)
		m = updated.(Model)
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	return updated.(Model)
}

func typeText(m Model, input string) Model {
	for _, r := range input {
		msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}}
		updated, _ := m.Update(msg)
		m = updated.(Model)
	}
	return m
}
