package commands

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/coldwine/project"
	"github.com/mistakeknot/autarch/internal/coldwine/storage"
	"github.com/mistakeknot/autarch/pkg/yamlsafe"
)

func TestImportFromBriefsPersistsTasks(t *testing.T) {
	root := t.TempDir()
	briefsDir := filepath.Join(root, ".gurgeh", "briefs", "PRD-001")
	if err := os.MkdirAll(briefsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	brief := `# Implement Authentication

## Outcome
Users can sign in.

## Acceptance Criteria
- [ ] Login works
`
	if err := os.WriteFile(filepath.Join(briefsDir, "BRIEF-001-auth.md"), []byte(brief), 0o644); err != nil {
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

	cmd := ImportCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--from-briefs", "PRD-001"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute import: %v", err)
	}

	dbPath := project.StateDBPath(root)
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected state db to exist: %v", err)
	}
	taskPath := filepath.Join(project.SpecsDir(root), "TASK-001.yaml")
	if _, err := os.Stat(taskPath); err != nil {
		t.Fatalf("expected task spec to exist: %v", err)
	}

	db, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := storage.Migrate(db); err != nil {
		t.Fatal(err)
	}
	task, err := storage.GetTask(db, "TASK-001")
	if err != nil {
		t.Fatalf("expected TASK-001 in db: %v", err)
	}
	if task.Title != "Implement Authentication" {
		t.Fatalf("unexpected task title: %s", task.Title)
	}

	var taskSpec map[string]any
	if _, err := yamlsafe.UnmarshalFile(taskPath, &taskSpec); err != nil {
		t.Fatalf("load task yaml: %v", err)
	}
	if got, _ := taskSpec["story_id"].(string); got == "" {
		t.Fatalf("expected story_id in task yaml")
	}
}

func TestImportFromBriefsDryRunDoesNotPersist(t *testing.T) {
	root := t.TempDir()
	briefsDir := filepath.Join(root, ".gurgeh", "briefs", "PRD-001")
	if err := os.MkdirAll(briefsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(briefsDir, "BRIEF-001-auth.md"), []byte("# Test\n"), 0o644); err != nil {
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

	cmd := ImportCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--from-briefs", "PRD-001", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute import dry-run: %v", err)
	}
	if !strings.Contains(out.String(), "Would import 1 tasks") {
		t.Fatalf("expected dry-run output, got:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(root, ".tandemonium")); !os.IsNotExist(err) {
		t.Fatalf("expected no persistence in dry-run, .tandemonium exists")
	}
}

func TestImportFromBriefsResolvesProjectRootFromSubdir(t *testing.T) {
	root := t.TempDir()
	subdir := filepath.Join(root, "nested", "subdir")
	briefsDir := filepath.Join(root, ".gurgeh", "briefs", "PRD-001")
	if err := os.MkdirAll(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(briefsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(briefsDir, "BRIEF-001-auth.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(cwd) }()
	if err := os.Chdir(subdir); err != nil {
		t.Fatal(err)
	}

	cmd := ImportCmd()
	cmd.SetOut(bytes.NewBuffer(nil))
	cmd.SetErr(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"--from-briefs", "PRD-001"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute import from subdir: %v", err)
	}

	if _, err := os.Stat(project.StateDBPath(root)); err != nil {
		t.Fatalf("expected db in project root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(subdir, ".tandemonium")); !os.IsNotExist(err) {
		t.Fatalf("unexpected .tandemonium in subdir")
	}
}
