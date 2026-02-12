package sources

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestSourceCollectionSaveLoad_Cases(t *testing.T) {
	base := time.Date(2026, 1, 20, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 1, 19, 9, 0, 0, 0, time.UTC)
	published := time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		filename   string
		collection *SourceCollection
	}{
		{
			name:     "round trip with multiple source types",
			filename: "rich.yaml",
			collection: &SourceCollection{
				AgentName:   "hunter",
				Query:       "ai agents",
				CollectedAt: base,
				Repos: []GitHubRepo{
					{
						Owner:       "mistakeknot",
						Name:        "autarch",
						Description: "monorepo",
						URL:         "https://github.com/mistakeknot/autarch",
						Stars:       123,
						Language:    "Go",
						Topics:      []string{"ai", "tui"},
						UpdatedAt:   updated,
						CollectedAt: base,
						Synthesis: &Synthesis{
							Summary:            "Strong agent workflow focus",
							KeyFeatures:        []string{"tui", "workflow"},
							RelevanceRationale: "Matches product direction",
							Recommendations:    []string{"Track release cadence"},
							Confidence:         0.9,
						},
						QualityScore: &QualityScore{
							Value:      0.88,
							Level:      "high",
							Factors:    map[string]float64{"engagement": 0.9, "recency": 0.85},
							Confidence: 0.92,
						},
					},
				},
				Articles: []Article{
					{
						Title:       "Designing agent UX",
						URL:         "https://example.com/agent-ux",
						Author:      "A. Author",
						PublishedAt: published,
						Summary:     "Helpful survey",
						CollectedAt: base,
					},
				},
				Papers: []ResearchPaper{
					{
						ArxivID:     "2401.12345",
						Title:       "Agentic Workflows",
						Authors:     []string{"R. Researcher"},
						Abstract:    "Details workflow constraints.",
						URL:         "https://arxiv.org/abs/2401.12345",
						PDFURL:      "https://arxiv.org/pdf/2401.12345.pdf",
						Published:   published,
						Categories:  []string{"cs.AI"},
						Citations:   7,
						Relevance:   "high",
						HasCode:     true,
						CodeURL:     "https://github.com/example/paper-code",
						Signal:      "Useful for planning loops",
						CollectedAt: base,
					},
				},
			},
		},
		{
			name:       "empty collection",
			filename:   "empty.yaml",
			collection: &SourceCollection{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectPath := t.TempDir()

			if err := tt.collection.Save(projectPath, tt.filename); err != nil {
				t.Fatalf("Save() error = %v", err)
			}

			got, err := Load(filepath.Join(projectPath, ".pollard", "sources", tt.filename))
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			if !reflect.DeepEqual(got, tt.collection) {
				t.Fatalf("Save/Load mismatch:\n got: %#v\nwant: %#v", got, tt.collection)
			}
		})
	}
}

func TestSourceCollectionLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("Load() error = nil, want missing file error")
	}
}

func TestSourceCollectionSave_CreatesPollardSourcesDirectory(t *testing.T) {
	projectPath := t.TempDir()
	collection := &SourceCollection{AgentName: "agent"}

	if err := collection.Save(projectPath, "saved.yaml"); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(projectPath, ".pollard", "sources", "saved.yaml")); err != nil {
		t.Fatalf("saved file not found: %v", err)
	}
}
