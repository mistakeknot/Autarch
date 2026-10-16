package backlogrun

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestLiveTriage sends one real card through Clavain routing and the dispatch wrapper. It spends a little subscription
// quota, so it runs only when BACKLOG_RUN_LIVE=1 and CLAVAIN_SELECTED_ROOT is set.
func TestLiveTriage(t *testing.T) {
	root := os.Getenv("CLAVAIN_SELECTED_ROOT")
	if os.Getenv("BACKLOG_RUN_LIVE") != "1" || root == "" {
		t.Skip("set BACKLOG_RUN_LIVE=1 and CLAVAIN_SELECTED_ROOT to run")
	}
	work := t.TempDir()
	r := Router{Run: ExecRunner, Policy: filepath.Join(root, "config", "routing.yaml"), Dispatch: filepath.Join(root, "scripts", "dispatch.sh"), WorkDir: work, Repo: filepath.Join(work, "empty")}
	vs, seat, err := r.Triage(context.Background(), []Card{{Key: "T-1", Board: "T", Title: "Fix typo in README heading", Description: "The heading says 'Instalation'. Rename it. Done when the heading is spelled correctly.", Status: "backlog", Priority: "low", UpdatedAt: "2026-10-16T00:00:00Z"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("seat %+v verdicts %+v", seat, vs)
}
