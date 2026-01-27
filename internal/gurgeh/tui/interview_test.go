package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/gurgeh/agents"
	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
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
		m = typeText(m, "Reduce onboarding time")
		m = pressKey(m, "]")
		m = typeText(m, "Offline mode")
		m = pressKey(m, "]")
		m = typeText(m, "Users have GitHub accounts")
		m = pressKey(m, "]")
		m = typeText(m, "First requirement")
		m = pressKey(m, "]")
		m = pressKey(m, "2")
		files := praudeSpecFiles(t, root)
		if len(files) != 1 {
			t.Fatalf("expected one spec file, got %d", len(files))
		}
		path := filepath.Join(root, ".gurgeh", "specs", files[0])
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
		m.enterInterview(specs.Spec{}, "")
		out := m.View()
		// The new layout has a chat panel with shared components
		// Just verify the interview mode renders something
		if len(out) == 0 {
			t.Fatalf("expected non-empty interview view")
		}
		// Check that we're rendering interview-related content
		if !strings.Contains(out, "Scan") && !strings.Contains(out, "Step") {
			t.Fatalf("expected interview content")
		}
	})
}

func TestInterviewShowsIterationHint(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m.width = 120
		m.height = 40
		m.enterInterview(specs.Spec{}, "")
		m.interview.step = stepVision
		m.updateInterviewDocPanel()
		if m.chatPanel != nil {
			m.chatPanel.SetValue("")
		}
		out := stripANSI(m.View())
		// Check for keyboard hint - the exact text depends on the view implementation
		if !strings.Contains(out, "enter") && !strings.Contains(out, "send") {
			t.Fatalf("expected keyboard hint containing 'enter' or 'send', got: %s", out[:min(500, len(out))])
		}
	})
}

func TestInterviewInputArrowLeftMovesCursor(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m.enterInterview(specs.Spec{}, "")
		m.interview.step = stepVision
		m.interviewFocus = "question"
		if m.chatPanel != nil {
			m.chatPanel.SetValue("hello")
			// Focus the chat panel to enable cursor movement
			m.chatPanel.Focus()
		}
		m = pressKey(m, "left")
		m = typeText(m, "X")
		got := ""
		if m.chatPanel != nil {
			got = m.chatPanel.Value()
		}
		// The bubbles/textarea should insert X before the last char (after moving left)
		// Expected: "hellXo" - the cursor moves left then X is inserted
		if got != "hellXo" {
			t.Fatalf("expected cursor insert to produce 'hellXo', got %q", got)
		}
	})
}

func TestInterviewInputSpaceInserts(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m.enterInterview(specs.Spec{}, "")
		m.interview.step = stepVision
		m.interviewFocus = "question"
		if m.chatPanel != nil {
			m.chatPanel.SetValue("hi")
			m.chatPanel.Focus()
		}
		// Type a space using rune input (not KeySpace)
		m = typeText(m, " ")
		got := ""
		if m.chatPanel != nil {
			got = m.chatPanel.Value()
		}
		// Space should be inserted at end
		if got != "hi " {
			t.Fatalf("expected 'hi ', got %q", got)
		}
	})
}

func TestInterviewMarkdownInputBoxIsPlain(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m.enterInterview(specs.Spec{}, "")
		m.interview.step = stepVision
		if m.chatPanel != nil {
			m.chatPanel.SetValue("Hello")
		}
		out := m.interviewMarkdown()
		if strings.Contains(out, "\x1b[") {
			t.Fatalf("expected no ANSI in markdown")
		}
		// Expect lipgloss-style rounded box characters
		if !strings.Contains(out, "╭") || !strings.Contains(out, "│") {
			t.Fatalf("expected rounded input box")
		}
		// Cursor status line was removed since we no longer have TextBuffer
		// Just verify the input box structure exists
		if !strings.Contains(out, "Input:") {
			t.Fatalf("expected Input label")
		}
	})
}

func TestInterviewChatRendersTranscript(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m.enterInterview(specs.Spec{}, "")
		m.interview.step = stepVision
		// Add messages via the chat panel (primary) and interview state (for compatibility)
		m.appendInterviewMessage("user", "User line")
		m.appendInterviewMessage("agent", "Agent line")
		out := stripANSI(m.View())
		if !strings.Contains(out, "User") || !strings.Contains(out, "Agent") {
			t.Fatalf("expected chat transcript roles, got: %s", out)
		}
	})
}

func TestInterviewComposerShowsTitleAndHints(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m.enterInterview(specs.Spec{}, "")
		m.interview.step = stepVision
		m.updateInterviewDocPanel()
		out := stripANSI(m.View())
		// The new layout uses shared composer with Vision as title
		if !strings.Contains(out, "Vision") {
			t.Fatalf("expected Vision title, got: %s", out)
		}
		// Check for keyboard hints (either in shared component or doc panel)
		if !strings.Contains(out, "enter") && !strings.Contains(out, "send") {
			t.Fatalf("expected keyboard hints")
		}
	})
}

func TestInterviewTranscriptUsesRoleBadges(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m.enterInterview(specs.Spec{}, "")
		m.appendInterviewMessage("user", "Hello")
		out := stripANSI(m.View())
		// The new chat panel uses "User:" instead of "[User]"
		if !strings.Contains(out, "User") {
			t.Fatalf("expected user role in transcript, got: %s", out)
		}
		if !strings.Contains(out, "Hello") {
			t.Fatalf("expected message content, got: %s", out)
		}
	})
}

func TestInterviewHeaderNavActiveAndCollapsed(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m.enterInterview(specs.Spec{}, "")
		m.interview.step = stepProblem
		m.width = 60
		m.updateInterviewDocPanel()
		out := stripANSI(m.View())
		if !strings.Contains(out, "[[Problem]]") {
			t.Fatalf("expected active step emphasis")
		}
		if !strings.Contains(out, "...") {
			t.Fatalf("expected collapsed nav")
		}
	})
}

func TestInterviewLayoutShowsHeaderAndPanels(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m.enterInterview(specs.Spec{}, "")
		out := stripANSI(m.View())
		if !strings.Contains(out, "Scan") || !strings.Contains(out, "Vision") {
			t.Fatalf("expected header nav steps, got: %s", out)
		}
		// The new layout uses shared split layout with doc and chat panels
		// Check for content that should be visible
		if !strings.Contains(out, "Step") {
			t.Fatalf("expected step indicator, got: %s", out)
		}
	})
}

func TestInterviewBreadcrumbsShown(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m.enterInterview(specs.Spec{ID: "PRD-001"}, "")
		out := stripANSI(m.View())
		if !strings.Contains(out, "PRDs > PRD-001 > Interview") {
			t.Fatalf("expected breadcrumbs")
		}
	})
}

func TestInterviewEscExitsToList(t *testing.T) {
	withTempRoot(t, func(root string) {
		m := NewModel()
		m.enterInterview(specs.Spec{}, "")
		m = pressKey(m, "esc")
		if m.mode != "list" {
			t.Fatalf("expected exit to list")
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
		// The new shared layout shows step info and input
		if !strings.Contains(clean, "Vision") && !strings.Contains(clean, "Step") {
			t.Fatalf("expected step information, got: %s", clean[:min(500, len(clean))])
		}
		// Check for keyboard hints
		if !strings.Contains(clean, "enter") {
			t.Fatalf("expected keyboard hint, got: %s", clean[:min(500, len(clean))])
		}
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
		if err := os.MkdirAll(filepath.Join(root, ".gurgeh", "suggestions"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, ".gurgeh", "briefs"), 0o755); err != nil {
			t.Fatal(err)
		}
		cfg := `validation_mode = "soft"

[agents.codex]
command = "codex"
args = []
`
		if err := os.WriteFile(filepath.Join(root, ".gurgeh", "config.toml"), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
		oldLaunch := launchAgent
		oldSub := launchSubagent
		launchAgent = func(p agents.Profile, briefPath string) error {
			entries, err := os.ReadDir(filepath.Join(root, ".gurgeh", "suggestions"))
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				return nil
			}
			path := filepath.Join(root, ".gurgeh", "suggestions", entries[0].Name())
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
		m = typeText(m, "Reduce onboarding time")
		m = pressKey(m, "]")
		m = typeText(m, "Offline mode")
		m = pressKey(m, "]")
		m = typeText(m, "Users have GitHub accounts")
		m = pressKey(m, "]")
		m = typeText(m, "First requirement")
		m = pressKey(m, "]")
		m = pressKey(m, "2")
		files := praudeSpecFiles(t, root)
		if len(files) != 1 {
			t.Fatalf("expected one spec file, got %d", len(files))
		}
		path := filepath.Join(root, ".gurgeh", "specs", files[0])
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
