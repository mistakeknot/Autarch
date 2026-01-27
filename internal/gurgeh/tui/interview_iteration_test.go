package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/agents"
)

func TestNewKeyStartsInterviewForNewSpec(t *testing.T) {
	withTempRootInitialized(t, func(root string) {
		m := NewModel()
		m.startNewInterview()
		if m.mode != "interview" {
			t.Fatalf("expected interview mode")
		}
		files := praudeSpecFiles(t, root)
		if len(files) != 1 {
			t.Fatalf("expected new spec file")
		}
	})
}

func TestInterviewEnterIteratesDraft(t *testing.T) {
	withTempRootInitialized(t, func(root string) {
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
		oldRun := runAgent
		runAgent = func(p agents.Profile, briefPath string) ([]byte, error) {
			return []byte("Drafted vision"), nil
		}
		defer func() { runAgent = oldRun }()

		m := NewModel()
		m.startNewInterview()
		m = pressKey(m, "2")
		m = pressKey(m, "1")
		m = pressKey(m, "2")
		m = typeText(m, "Initial vision")
		m = pressKey(m, "enter")
		out := m.View()
		if !strings.Contains(out, "Drafted vision") {
			t.Fatalf("expected draft in view")
		}
	})
}

func TestInterviewIncludesBootstrapStep(t *testing.T) {
	withTempRootInitialized(t, func(root string) {
		m := NewModel()
		m.startNewInterview()
		if m.interview.step != stepScanPrompt {
			t.Fatalf("expected scan step")
		}
		m = pressKey(m, "2")
		if m.interview.step != stepDraftConfirm {
			t.Fatalf("expected confirm step")
		}
		m = pressKey(m, "1")
		if m.interview.step != stepBootstrapPrompt {
			t.Fatalf("expected bootstrap step")
		}
		m = pressKey(m, "2")
		if m.interview.step != stepVision {
			t.Fatalf("expected vision step")
		}
	})
}
