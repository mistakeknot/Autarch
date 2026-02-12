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
