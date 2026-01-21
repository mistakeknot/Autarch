package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

func TestLoadTaskDetailReadsSpecSummary(t *testing.T) {
	root := t.TempDir()
	if err := project.Init(root); err != nil {
		t.Fatal(err)
	}
	specPath := filepath.Join(project.SpecsDir(root), "T1.yaml")
	spec := "id: T1\ntitle: Example\nsummary: |\n  Did the thing.\nstatus: todo\n"
	if err := os.WriteFile(specPath, []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := storage.OpenShared(project.StateDBPath(root))
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := storage.InsertTask(db, storage.Task{ID: "T1", Title: "Example", Status: "todo"}); err != nil {
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
	detail, err := LoadTaskDetail("T1")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Title != "Example" {
		t.Fatalf("expected title Example")
	}
	if !strings.Contains(detail.Summary, "Did the thing") {
		t.Fatalf("expected summary content")
	}
}
