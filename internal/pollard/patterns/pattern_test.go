package patterns

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestPatternSaveLoad_Cases(t *testing.T) {
	collected := time.Date(2026, 1, 20, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		pattern *Pattern
	}{
		{
			name: "round trip with examples",
			pattern: &Pattern{
				ID:          "PAT-001",
				Title:       "Progressive disclosure panel",
				Category:    CategoryUI,
				CollectedAt: collected,
				Description: "Show essentials first and expand details on demand.",
				Examples: []Example{
					{
						Name:       "Example A",
						URL:        "https://example.com/a",
						Screenshot: "example-a.png",
						Notes:      "Works well on mobile",
					},
				},
				ImplementationHints: []string{"Use lazy loading"},
				AntiPatterns:        []string{"Rendering all sections expanded"},
				LinkedEpics:         []string{"EPIC-001"},
			},
		},
		{
			name: "minimal pattern",
			pattern: &Pattern{
				ID:       "PAT-MIN",
				Examples: []Example{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			projectPath := t.TempDir()

			if err := tt.pattern.Save(projectPath); err != nil {
				t.Fatalf("Save() error = %v", err)
			}

			path := filepath.Join(projectPath, ".pollard", "patterns", tt.pattern.ID+".yaml")
			got, err := Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}

			if !reflect.DeepEqual(got, tt.pattern) {
				t.Fatalf("Save/Load mismatch:\n got: %#v\nwant: %#v", got, tt.pattern)
			}
		})
	}
}

func TestPatternLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("Load() error = nil, want missing file error")
	}
}

func TestPatternLoadAll_Count(t *testing.T) {
	projectPath := t.TempDir()
	collected := time.Date(2026, 1, 21, 9, 0, 0, 0, time.UTC)
	items := []*Pattern{
		{ID: "PAT-101", Title: "One", Category: CategoryArch, CollectedAt: collected},
		{ID: "PAT-102", Title: "Two", Category: CategoryUI, CollectedAt: collected},
		{ID: "PAT-103", Title: "Three", Category: CategoryAnti, CollectedAt: collected},
	}

	for _, item := range items {
		if err := item.Save(projectPath); err != nil {
			t.Fatalf("Save(%s) error = %v", item.ID, err)
		}
	}

	patternsDir := filepath.Join(projectPath, ".pollard", "patterns")
	if err := os.WriteFile(filepath.Join(patternsDir, "README.txt"), []byte("ignore"), 0o644); err != nil {
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
