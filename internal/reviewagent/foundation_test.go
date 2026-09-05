package reviewagent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFoundationIncludesCanonicalGuidanceAndCoverage(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(root, "AGENTS.md"), []byte("Accepted ruling: preserve original observations."), 0644)
	result := FoundationContext(context.Background(), root)
	if !strings.Contains(result, "preserve original observations") || !strings.Contains(result, "History coverage") || !strings.Contains(result, "inferred") {
		t.Fatalf("incomplete foundation context: %s", result)
	}
}

func TestInvestigationLaunchRestrictsWritesAndAmbientTools(t *testing.T) {
	args, profile := investigationCommand("/usr/local/bin/flere", "/tmp/runtime", "/project", "/tmp/runtime/investigator.mjs")
	joined := strings.Join(args, " ")
	for _, want := range []string{"--no-builtin-tools", "--no-extensions", "--no-skills", "--mode rpc"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s: %s", want, joined)
		}
	}
	if !strings.Contains(profile, "(deny file-write*)") || !strings.Contains(profile, `(subpath "/tmp/runtime")`) {
		t.Fatal("investigation write boundary missing")
	}
}
