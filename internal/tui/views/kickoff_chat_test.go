package views

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/tui"
)

func TestKickoffSeedsChatHistory(t *testing.T) {
	v := NewKickoffView()
	msgs := v.ChatMessagesForTest()
	if len(msgs) == 0 {
		t.Fatal("expected seeded chat messages")
	}
	if msgs[0].Role != "system" {
		t.Fatalf("expected system role, got %q", msgs[0].Role)
	}
	if !strings.Contains(msgs[0].Content, "What do you want to build") {
		t.Fatalf("expected prompt message, got %q", msgs[0].Content)
	}
}

func TestKickoffScanPreparingMessageRoutesToChat(t *testing.T) {
	v := NewKickoffView()
	v.loading = true
	v.scanning = true
	v.loadingMsg = "Scanning codebase..."

	_, _ = v.Update(tui.ScanProgressMsg{Step: "Preparing", Details: "Building analysis prompt..."})

	if strings.Contains(v.docPanel.View(), "Building analysis prompt...") {
		t.Fatalf("expected preparing detail not to render in doc pane")
	}

	msgs := v.ChatMessagesForTest()
	found := false
	for _, msg := range msgs {
		if strings.Contains(msg.Content, "Building analysis prompt...") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected preparing detail in chat messages")
	}
}

func TestKickoffScanCompletionAutoCreatesProject(t *testing.T) {
	v := NewKickoffView()
	v.SetProjectStartCallback(func(project *Project) tea.Cmd {
		return nil
	})

	_, cmd := v.Update(tui.CodebaseScanResultMsg{
		ProjectName: "TestProject",
		Description: "A test project",
		Vision:      "Build great things",
	})

	// Should have returned a command to create the project
	if cmd == nil {
		t.Fatalf("expected createProject command after scan completion")
	}
	if v.scanResult == nil {
		t.Fatalf("expected scan result to be stored")
	}
}

func TestKickoffShowsScanValidationErrors(t *testing.T) {
	v := NewKickoffView()
	_, _ = v.Update(tui.CodebaseScanResultMsg{
		ValidationErrors: []tui.ValidationError{
			{Code: "missing_evidence", Message: "At least 2 evidence items required"},
		},
	})

	msgs := v.ChatMessagesForTest()
	found := false
	for _, msg := range msgs {
		if strings.Contains(msg.Content, "At least 2 evidence items required") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected validation error in chat messages")
	}
}

func TestKickoffMouseWheelScrollsChatWhenFocused(t *testing.T) {
	v := NewKickoffView()
	v.focusInput = true
	v.chatPanel.SetSize(60, 20)
	v.chatPanel.AddMessage("user", "One")
	v.chatPanel.AddMessage("user", "Two")

	before := v.chatPanel.ScrollOffsetForTest()
	_, _ = v.Update(tea.MouseMsg{Type: tea.MouseWheelUp})
	after := v.chatPanel.ScrollOffsetForTest()
	if after <= before {
		t.Fatalf("expected chat scroll offset to increase")
	}
}

func TestKickoffScanDoesNotOverrideDocPane(t *testing.T) {
	v := NewKickoffView()
	updated, _ := v.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	v = updated.(*KickoffView)
	v.loading = true
	v.scanning = true
	v.loadingMsg = "Found 5 files to analyze"
	v.scanPath = "/tmp/project"

	view := v.View()
	if strings.Contains(view, "Found 5 files") {
		t.Fatalf("expected scan status not to render in doc pane")
	}
}

func TestKickoffScanProgressRendersInChatOnly(t *testing.T) {
	v := NewKickoffView()
	updated, _ := v.Update(tea.WindowSizeMsg{Width: 140, Height: 40})
	v = updated.(*KickoffView)
	v.loading = true
	v.scanning = true
	v.loadingMsg = "Found 5 files to analyze"

	_, _ = v.Update(tui.ScanProgressMsg{Step: "Parsing", Details: "Extracting project information...", Files: []string{"README.md"}})

	docView := v.docPanel.View()
	if strings.Contains(docView, "Extracting project information") {
		t.Fatalf("expected scan progress not to render in doc pane")
	}
	msgs := v.ChatMessagesForTest()
	found := false
	for _, msg := range msgs {
		if strings.Contains(msg.Content, "Extracting project information") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected scan progress in chat messages")
	}
}

func TestKickoffScanFoundFilesMessageSuppressed(t *testing.T) {
	v := NewKickoffView()
	_, _ = v.Update(tui.ScanProgressMsg{Step: "Found files", Details: "Found 5 files to analyze", Files: []string{"README.md"}})

	msgs := v.ChatMessagesForTest()
	for _, msg := range msgs {
		if strings.Contains(msg.Content, "Found 5 files") {
			t.Fatalf("expected found files message to be suppressed")
		}
	}
}
