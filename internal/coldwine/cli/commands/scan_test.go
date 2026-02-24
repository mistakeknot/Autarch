package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/autarch/internal/coldwine/drift"
	"github.com/mistakeknot/autarch/internal/coldwine/project"
)

func TestScanCommandWritesSummary(t *testing.T) {
	root := t.TempDir()
	if err := project.Init(root); err != nil {
		t.Fatal(err)
	}

	cmd := ScanCmd()
	cmd.SetArgs([]string{root})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, ".coldwine", "plan", "exploration.md")); err != nil {
		t.Fatalf("expected summary file: %v", err)
	}
}

func TestSelectUntrackedByConfidence(t *testing.T) {
	items := []drift.ReconciledActionItem{
		{
			ActionItem: drift.ActionItem{ID: "1", Confidence: 90, Text: "high confidence"},
			Tracked:    false,
		},
		{
			ActionItem: drift.ActionItem{ID: "2", Confidence: 70, Text: "low confidence"},
			Tracked:    false,
		},
		{
			ActionItem: drift.ActionItem{ID: "3", Confidence: 95, Text: "tracked item"},
			Tracked:    true,
		},
	}

	got := selectUntrackedByConfidence(items, 75)
	if len(got) != 1 {
		t.Fatalf("expected 1 item, got %d", len(got))
	}
	if got[0].ID != "1" {
		t.Fatalf("expected ID=1, got %s", got[0].ID)
	}
}
