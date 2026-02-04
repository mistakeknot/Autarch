package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInterviewCommandUsesArbiterByDefault(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".praude", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := `validation_mode = "soft"

[agents.codex]
command = "codex"
args = []
`
	if err := os.WriteFile(filepath.Join(root, ".praude", "config.toml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	cmd := InterviewCmd()
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--vision", "Test vision"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	output := buf.String()
	if !strings.Contains(output, "via Arbiter sprint") {
		t.Fatalf("expected arbiter sprint output, got: %s", output)
	}
	entries, err := os.ReadDir(filepath.Join(root, ".praude", "specs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected spec created via arbiter")
	}
}

func TestInterviewCommandNoSprintFlag(t *testing.T) {
	// Verify --sprint flag no longer exists
	cmd := InterviewCmd()
	flag := cmd.Flags().Lookup("sprint")
	if flag != nil {
		t.Fatalf("expected --sprint flag to be removed, but it still exists")
	}
}
