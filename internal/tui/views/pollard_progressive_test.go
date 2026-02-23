package views

import (
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/pollard/research"
	"github.com/mistakeknot/autarch/pkg/autarch"
)

func newTestPollardView() *PollardView {
	return NewPollardView(nil, nil)
}

func TestPollardView_RunStartedInitializesState(t *testing.T) {
	v := newTestPollardView()

	msg := research.RunStartedMsg{
		RunID:   "run-001",
		Hunters: []string{"github-scout", "hackernews-trendwatcher"},
	}

	result, _ := v.Update(msg)
	pv := result.(*PollardView)

	if !pv.runActive {
		t.Error("runActive should be true after RunStartedMsg")
	}
	if pv.currentRunID != "run-001" {
		t.Errorf("currentRunID = %q, want %q", pv.currentRunID, "run-001")
	}
	if len(pv.hunterStatuses) != 2 {
		t.Errorf("hunterStatuses count = %d, want 2", len(pv.hunterStatuses))
	}
	for _, name := range []string{"github-scout", "hackernews-trendwatcher"} {
		hs, ok := pv.hunterStatuses[name]
		if !ok {
			t.Errorf("missing hunter status for %q", name)
			continue
		}
		if hs.Status != research.StatusPending {
			t.Errorf("hunter %q status = %q, want %q", name, hs.Status, research.StatusPending)
		}
	}
}

func TestPollardView_HunterCompletedUpdatesStatus(t *testing.T) {
	v := newTestPollardView()

	// Start run
	v.Update(research.RunStartedMsg{
		RunID:   "run-001",
		Hunters: []string{"github-scout"},
	})

	// Start hunter
	v.Update(research.HunterStartedMsg{
		RunID:      "run-001",
		HunterName: "github-scout",
	})

	// Complete hunter
	result, _ := v.Update(research.HunterCompletedMsg{
		RunID:        "run-001",
		HunterName:   "github-scout",
		FindingCount: 3,
	})
	pv := result.(*PollardView)

	hs := pv.hunterStatuses["github-scout"]
	if hs.Status != research.StatusComplete {
		t.Errorf("hunter status = %q, want %q", hs.Status, research.StatusComplete)
	}
	if hs.Findings != 3 {
		t.Errorf("hunter findings = %d, want 3", hs.Findings)
	}
}

func TestPollardView_StaleRunIDIgnored(t *testing.T) {
	v := newTestPollardView()

	// Start first run
	v.Update(research.RunStartedMsg{
		RunID:   "run-001",
		Hunters: []string{"github-scout"},
	})

	// Start second run (supersedes first)
	v.Update(research.RunStartedMsg{
		RunID:   "run-002",
		Hunters: []string{"hackernews-trendwatcher"},
	})

	// Stale message from first run — should be ignored
	result, _ := v.Update(research.HunterCompletedMsg{
		RunID:        "run-001",
		HunterName:   "github-scout",
		FindingCount: 5,
	})
	pv := result.(*PollardView)

	if pv.currentRunID != "run-002" {
		t.Errorf("currentRunID = %q, want %q", pv.currentRunID, "run-002")
	}
	// "github-scout" should not exist in current run's statuses
	if _, ok := pv.hunterStatuses["github-scout"]; ok {
		t.Error("stale hunter should not appear in current run's statuses")
	}
}

func TestPollardView_AddFindingSortsByRelevance(t *testing.T) {
	v := newTestPollardView()

	// Add findings in non-sorted order
	v.addFinding(research.Finding{
		ID: "low", Title: "Low", Relevance: 0.3,
		Source: "src", SourceType: "test", CollectedAt: time.Now(),
	})
	v.addFinding(research.Finding{
		ID: "high", Title: "High", Relevance: 0.9,
		Source: "src", SourceType: "test", CollectedAt: time.Now(),
	})
	v.addFinding(research.Finding{
		ID: "mid", Title: "Mid", Relevance: 0.6,
		Source: "src", SourceType: "test", CollectedAt: time.Now(),
	})

	if len(v.insights) != 3 {
		t.Fatalf("insights count = %d, want 3", len(v.insights))
	}

	// Should be sorted descending by score
	expected := []struct {
		id    string
		score float64
	}{
		{"high", 0.9},
		{"mid", 0.6},
		{"low", 0.3},
	}

	for i, e := range expected {
		if v.insights[i].ID != e.id {
			t.Errorf("insights[%d].ID = %q, want %q", i, v.insights[i].ID, e.id)
		}
		if v.insights[i].Score != e.score {
			t.Errorf("insights[%d].Score = %f, want %f", i, v.insights[i].Score, e.score)
		}
	}
}

func TestPollardView_HunterStatusIcon(t *testing.T) {
	tests := []struct {
		status research.Status
		icon   string
	}{
		{research.StatusRunning, "↻"},
		{research.StatusComplete, "✓"},
		{research.StatusError, "✗"},
		{research.StatusPending, "○"},
		{"unknown", "?"},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			got := hunterStatusIcon(tt.status)
			if got != tt.icon {
				t.Errorf("hunterStatusIcon(%q) = %q, want %q", tt.status, got, tt.icon)
			}
		})
	}
}

func TestPollardView_RunCompletedClearsRunActive(t *testing.T) {
	v := newTestPollardView()

	// Start run
	v.Update(research.RunStartedMsg{
		RunID:   "run-001",
		Hunters: []string{"github-scout"},
	})

	if !v.runActive {
		t.Fatal("runActive should be true after start")
	}

	// Complete run
	result, cmd := v.Update(research.RunCompletedMsg{
		RunID:         "run-001",
		TotalFindings: 2,
		Duration:      "1.5s",
	})
	pv := result.(*PollardView)

	if pv.runActive {
		t.Error("runActive should be false after RunCompletedMsg")
	}
	// loadInsights returns nil when client is nil (nil-safe)
	if cmd != nil {
		t.Error("cmd should be nil when client is nil")
	}
}

func TestPollardView_SidebarItemsDuringRun(t *testing.T) {
	v := newTestPollardView()
	v.shell.SetSize(120, 40)

	// Start run with multiple hunters
	v.Update(research.RunStartedMsg{
		RunID:   "run-001",
		Hunters: []string{"hackernews-trendwatcher", "github-scout", "competitor-tracker"},
	})

	items := v.SidebarItems()

	// Should have 3 hunter items (sorted alphabetically), no insight items
	if len(items) != 3 {
		t.Fatalf("sidebar items = %d, want 3", len(items))
	}

	// Verify sorted order
	expectedNames := []string{"competitor-tracker", "github-scout", "hackernews-trendwatcher"}
	for i, expected := range expectedNames {
		expectedID := "hunter:" + expected
		if items[i].ID != expectedID {
			t.Errorf("items[%d].ID = %q, want %q", i, items[i].ID, expectedID)
		}
	}
}

func TestPollardView_SidebarItemsMixedHuntersAndInsights(t *testing.T) {
	v := newTestPollardView()
	v.shell.SetSize(120, 40)

	// Start run
	v.Update(research.RunStartedMsg{
		RunID:   "run-001",
		Hunters: []string{"github-scout"},
	})

	// Add a finding (becomes an insight)
	v.addFinding(research.Finding{
		ID: "insight-001", Title: "Test Insight", Relevance: 0.8,
		Source: "test", SourceType: "technology", CollectedAt: time.Now(),
	})

	items := v.SidebarItems()

	// 1 hunter + 1 insight
	if len(items) != 2 {
		t.Fatalf("sidebar items = %d, want 2", len(items))
	}

	if items[0].ID != "hunter:github-scout" {
		t.Errorf("first item = %q, want hunter:github-scout", items[0].ID)
	}
	if items[1].ID != "insight-001" {
		t.Errorf("second item = %q, want insight-001", items[1].ID)
	}
}

func TestPollardView_ViewDuringActiveRun(t *testing.T) {
	v := newTestPollardView()
	v.shell.SetSize(120, 40)

	// Simulate WindowSizeMsg for proper rendering
	v.Update(tea.WindowSizeMsg{Width: 126, Height: 46})

	// Start run
	v.Update(research.RunStartedMsg{
		RunID:   "run-001",
		Hunters: []string{"github-scout"},
	})

	output := v.View()

	// Should NOT show "Loading insights..." during active run
	if containsSubstring(output, "Loading insights") {
		t.Error("should not show 'Loading insights' during active run")
	}
}

func TestPollardView_ShortHelpDuringRun(t *testing.T) {
	v := newTestPollardView()

	// Before run
	help := v.ShortHelp()
	if containsSubstring(help, "research active") {
		t.Error("should not show 'research active' before run starts")
	}

	// Start run
	v.Update(research.RunStartedMsg{
		RunID:   "run-001",
		Hunters: []string{"github-scout"},
	})

	help = v.ShortHelp()
	if !containsSubstring(help, "research active") {
		t.Errorf("ShortHelp during run should mention 'research active': %q", help)
	}
}

func TestPollardView_InsightSelectedAfterFinding(t *testing.T) {
	v := newTestPollardView()

	// Add findings
	v.addFinding(research.Finding{
		ID: "f1", Title: "First Finding", Summary: "Summary one",
		Source: "src", SourceType: "technology", Relevance: 0.5, CollectedAt: time.Now(),
	})
	v.addFinding(research.Finding{
		ID: "f2", Title: "Second Finding", Summary: "Summary two",
		Source: "src", SourceType: "market", Relevance: 0.9, CollectedAt: time.Now(),
	})

	// selected=0 should point to the highest-relevance insight
	if v.selected != 0 {
		t.Errorf("selected = %d, want 0", v.selected)
	}
	if v.insights[0].Title != "Second Finding" {
		t.Errorf("insights[0].Title = %q, want %q", v.insights[0].Title, "Second Finding")
	}
}

func TestPollardView_FindingConvertsToInsight(t *testing.T) {
	v := newTestPollardView()

	finding := research.Finding{
		ID:          "find-123",
		Title:       "Test Title",
		Summary:     "Test Summary",
		Source:      "test-source",
		SourceType:  "technology",
		Relevance:   0.75,
		CollectedAt: time.Now(),
	}

	v.addFinding(finding)

	if len(v.insights) != 1 {
		t.Fatalf("expected 1 insight, got %d", len(v.insights))
	}

	insight := v.insights[0]
	if insight.ID != "find-123" {
		t.Errorf("ID = %q, want %q", insight.ID, "find-123")
	}
	if insight.Title != "Test Title" {
		t.Errorf("Title = %q, want %q", insight.Title, "Test Title")
	}
	if insight.Body != "Test Summary" {
		t.Errorf("Body = %q, want %q", insight.Body, "Test Summary")
	}
	if insight.Source != "test-source" {
		t.Errorf("Source = %q, want %q", insight.Source, "test-source")
	}
	if insight.Category != "technology" {
		t.Errorf("Category = %q, want %q", insight.Category, "technology")
	}
	if insight.Score != 0.75 {
		t.Errorf("Score = %f, want 0.75", insight.Score)
	}
}

func TestPollardView_HunterErrorUpdatesStatus(t *testing.T) {
	v := newTestPollardView()

	v.Update(research.RunStartedMsg{
		RunID:   "run-001",
		Hunters: []string{"github-scout"},
	})

	v.Update(research.HunterStartedMsg{
		RunID:      "run-001",
		HunterName: "github-scout",
	})

	result, _ := v.Update(research.HunterErrorMsg{
		RunID:      "run-001",
		HunterName: "github-scout",
		Error:      fmt.Errorf("rate limited"),
	})
	pv := result.(*PollardView)

	hs := pv.hunterStatuses["github-scout"]
	if hs.Status != research.StatusError {
		t.Errorf("status = %q, want %q", hs.Status, research.StatusError)
	}
	if hs.Error != "rate limited" {
		t.Errorf("error = %q, want %q", hs.Error, "rate limited")
	}
}

func TestPollardView_CategoryIcon(t *testing.T) {
	tests := []struct {
		category string
		icon     string
	}{
		{"competitor", "⚔"},
		{"technology", "⚙"},
		{"market", "📊"},
		{"user", "👤"},
		{"unknown", "•"},
		{"", "•"},
	}

	for _, tt := range tests {
		t.Run(tt.category, func(t *testing.T) {
			got := categoryIcon(tt.category)
			if got != tt.icon {
				t.Errorf("categoryIcon(%q) = %q, want %q", tt.category, got, tt.icon)
			}
		})
	}
}

// ensure autarch.Insight is used (suppress import warning in test file)
var _ = autarch.Insight{}
