package tui

import (
	"testing"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/git"
	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/storage"
)

type fakeGitRunner struct{ calls [][]string }

func (f *fakeGitRunner) Run(name string, args ...string) (string, error) {
	f.calls = append(f.calls, append([]string{name}, args...))
	return "", nil
}

func TestApproveAdapterUsesProvidedDB(t *testing.T) {
	db, err := storage.OpenTemp()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := storage.Migrate(db); err != nil {
		t.Fatal(err)
	}
	if err := storage.InsertTask(db, storage.Task{ID: "TAND-DB-2", Title: "Approve", Status: "review"}); err != nil {
		t.Fatal(err)
	}
	if err := storage.AddToReviewQueue(db, "TAND-DB-2"); err != nil {
		t.Fatal(err)
	}

	runner := &fakeGitRunner{}
	adapter := &ApproveAdapter{DB: db, Runner: runner}
	if err := adapter.Approve("TAND-DB-2", "feature/TAND-DB-2"); err != nil {
		t.Fatal(err)
	}

	if len(runner.calls) == 0 {
		t.Fatalf("expected git merge call")
	}

	task, err := storage.GetTask(db, "TAND-DB-2")
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != "done" {
		t.Fatalf("expected status done, got %s", task.Status)
	}

	queue, err := storage.ListReviewQueue(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(queue) != 0 {
		t.Fatalf("expected review queue cleared")
	}
}

var _ git.Runner = (*fakeGitRunner)(nil)
