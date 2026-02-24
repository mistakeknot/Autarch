package insights

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestInsightSaveLoad_Cases(t *testing.T) {
	collected := time.Date(2026, 1, 20, 10, 0, 0, 0, time.UTC)
	linked := time.Date(2026, 1, 20, 11, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		insight *Insight
	}{
		{
			name: "round trip with findings and links",
			insight: &Insight{
				ID:          "INS-001",
				Title:       "Competitor trend",
				Category:    CategoryCompetitive,
				CollectedAt: collected,
				Sources: []Source{
					{URL: "https://example.com/post", Type: "article"},
					{URL: "https://github.com/org/repo", Type: "github"},
				},
				Findings: []Finding{
					{
						Title:       "Fast iteration",
						Relevance:   RelevanceHigh,
						Description: "Weekly delivery cadence",
						Evidence:    []string{"screenshot-1.png"},
					},
				},
				Recommendations: []Recommendation{
					{
						FeatureHint: "Improve release visibility",
						Priority:    "p1",
						Rationale:   "Customer expectations are rising",
					},
				},
				LinkedFeatures: []string{"FEAT-001"},
				InitiativeRef:  "INIT-001",
				LinkedBy:       "test-agent",
				LinkedAt:       &linked,
			},
		},
		{
			name: "minimal insight",
			insight: &Insight{
				ID:       "INS-MIN",
				Sources:  []Source{},
				Findings: []Finding{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectPath := t.TempDir()

			if err := tt.insight.Save(projectPath); err != nil {
				t.Fatalf("Save() error = %v", err)
			}

			path := filepath.Join(projectPath, ".pollard", "insights", tt.insight.ID+".yaml")
			got, err := Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			if !reflect.DeepEqual(got, tt.insight) {
				t.Fatalf("Save/Load mismatch:\n got: %#v\nwant: %#v", got, tt.insight)
			}
		})
	}
}

func TestInsightLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("Load() error = nil, want missing file error")
	}
}

func TestLoadCompetitorWatchFormat(t *testing.T) {
	dir := t.TempDir()
	yaml := `competitor: Acme Corp
collected_at: 2026-01-15T10:00:00Z
changelog_url: https://acme.com/changelog
changes:
  - title: "AI-powered search"
    relevance: high
    threat_level: high
    recommendation:
      feature_hint: "Add semantic search"
      priority: p1
      rationale: "Closing gap"
  - title: "Minor UI tweak"
    relevance: low
    threat_level: low
  - title: "New dashboard"
    relevance: medium
    threat_level: medium
`
	path := filepath.Join(dir, "acme-watch.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.ID != "acme-watch" {
		t.Errorf("ID = %q, want %q", got.ID, "acme-watch")
	}
	if got.Category != CategoryCompetitive {
		t.Errorf("Category = %q, want %q", got.Category, CategoryCompetitive)
	}
	// Low relevance items are filtered out
	if len(got.Findings) != 2 {
		t.Fatalf("Findings count = %d, want 2 (low filtered out)", len(got.Findings))
	}
	if got.Findings[0].Title != "AI-powered search" {
		t.Errorf("Findings[0].Title = %q", got.Findings[0].Title)
	}
	if len(got.Recommendations) != 1 {
		t.Errorf("Recommendations count = %d, want 1", len(got.Recommendations))
	}
	if got.Sources[0].URL != "https://acme.com/changelog" {
		t.Errorf("Sources[0].URL = %q", got.Sources[0].URL)
	}
}

func TestLoadTrendsWatchFormat(t *testing.T) {
	dir := t.TempDir()
	yaml := `collected_at: 2026-01-16T08:00:00Z
trends:
  - title: "AI Agents are the future"
    source: hackernews
    url: https://news.ycombinator.com/item?id=12345
    points: 350
    comments: 120
    relevance: high
    signal: "Growing interest in autonomous agents"
    created_at: 2026-01-16T07:00:00Z
  - title: "Boring startup news"
    source: hackernews
    url: https://news.ycombinator.com/item?id=99999
    points: 10
    comments: 2
    relevance: low
    created_at: 2026-01-16T06:00:00Z
`
	path := filepath.Join(dir, "hn-2026-01-16.yaml")
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.ID != "hn-2026-01-16" {
		t.Errorf("ID = %q, want %q", got.ID, "hn-2026-01-16")
	}
	if got.Category != CategoryTrends {
		t.Errorf("Category = %q, want %q", got.Category, CategoryTrends)
	}
	// Low relevance filtered out
	if len(got.Findings) != 1 {
		t.Fatalf("Findings count = %d, want 1", len(got.Findings))
	}
	if got.Findings[0].Description != "Growing interest in autonomous agents" {
		t.Errorf("Findings[0].Description = %q", got.Findings[0].Description)
	}
	if len(got.Sources) != 1 {
		t.Errorf("Sources count = %d, want 1", len(got.Sources))
	}
}

func TestLoadAllRecursesSubdirectories(t *testing.T) {
	projectPath := t.TempDir()
	insightsDir := filepath.Join(projectPath, ".pollard", "insights")

	// Create competitive subdirectory with a watch file
	compDir := filepath.Join(insightsDir, "competitive")
	if err := os.MkdirAll(compDir, 0o755); err != nil {
		t.Fatal(err)
	}
	compYAML := `competitor: Rival Inc
collected_at: 2026-01-15T10:00:00Z
changelog_url: https://rival.com/changelog
changes:
  - title: "Big feature"
    relevance: high
    threat_level: high
`
	if err := os.WriteFile(filepath.Join(compDir, "rival.yaml"), []byte(compYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	// Create a native insight in the root insights dir
	native := &Insight{
		ID:       "INS-200",
		Title:    "Native insight",
		Category: CategoryUser,
	}
	if err := native.Save(projectPath); err != nil {
		t.Fatal(err)
	}

	got, err := LoadAll(projectPath)
	if err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("LoadAll() count = %d, want 2 (native + subdirectory)", len(got))
	}
}

func TestLoadUnrecognizedFormat(t *testing.T) {
	dir := t.TempDir()
	// Write YAML that matches neither native nor watch formats
	if err := os.WriteFile(filepath.Join(dir, "weird.yaml"), []byte("foo: bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Load(filepath.Join(dir, "weird.yaml"))
	if err == nil {
		t.Fatal("Load() error = nil, want unrecognized format error")
	}
}

func TestInsightLoadAll_Count(t *testing.T) {
	projectPath := t.TempDir()
	collected := time.Date(2026, 1, 21, 10, 0, 0, 0, time.UTC)
	items := []*Insight{
		{ID: "INS-101", Title: "One", Category: CategoryTrends, CollectedAt: collected},
		{ID: "INS-102", Title: "Two", Category: CategoryCompetitive, CollectedAt: collected},
		{ID: "INS-103", Title: "Three", Category: CategoryUser, CollectedAt: collected},
	}

	for _, item := range items {
		if err := item.Save(projectPath); err != nil {
			t.Fatalf("Save(%s) error = %v", item.ID, err)
		}
	}

	insightsDir := filepath.Join(projectPath, ".pollard", "insights")
	if err := os.WriteFile(filepath.Join(insightsDir, "README.txt"), []byte("ignore"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := LoadAll(projectPath)
	if err != nil {
		t.Fatalf("LoadAll() error = %v", err)
	}
	if len(got) != len(items) {
		t.Fatalf("LoadAll() count = %d, want %d", len(got), len(items))
	}
}
