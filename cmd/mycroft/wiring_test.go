package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mistakeknot/autarch/internal/homeask"
	"github.com/mistakeknot/autarch/internal/mycroft/escalate"
	"github.com/mistakeknot/autarch/internal/serve"

	"github.com/mistakeknot/autarch/internal/mycroft"
	"github.com/mistakeknot/autarch/internal/mycroft/spawn"
)

func TestRunPathSetsDecisionQueue(t *testing.T) {
	db, err := mycroft.OpenDB(filepath.Join(t.TempDir(), "d.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	o := buildOrchestrator(db, spawn.NewClaudeCodeSpawner("", "Demarch"), mycroft.DefaultConfig())
	if !o.HasQueue() {
		t.Fatal("production orchestrator has no decision queue")
	}
}

type recFiler struct{ asks []homeask.Ask }

func (f *recFiler) File(_ context.Context, a homeask.Ask) (string, error) {
	f.asks = append(f.asks, a)
	return "d1", nil
}

// Home accepts an ask only when project is a serve project name and project_root is
// the root serve resolves for it (estate: the Uqbar directory). This mirrors that check.
func homeAccepts(t *testing.T, a homeask.Ask, dirs []string, uqbar string) bool {
	t.Helper()
	if a.Project == "estate" {
		return uqbar != "" && a.ProjectRoot == uqbar
	}
	root, err := serve.NewResolver(dirs).Resolve(a.Project)
	return err == nil && root == a.ProjectRoot
}

func TestDefaultHomeWiringFilesWhatHomeAccepts(t *testing.T) {
	scan, uqbar := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(scan, "Demarch"), 0o755); err != nil {
		t.Fatal(err)
	}
	f := &recFiler{}
	q := escalate.NewDecisionQueue()
	q.SetHomeRoots(f, nil, homeRoots([]string{scan}, uqbar))
	// The bead label is lowercased; the project directory is not.
	if err := q.AddPending(escalate.PendingDecision{Agent: "a", BeadID: "D-1", BeadTitle: "t", Labels: []string{"project:demarch"}}); err != nil {
		t.Fatal(err)
	}
	if err := q.AddPending(escalate.PendingDecision{Agent: "a", BeadID: "E-1", BeadTitle: "t"}); err != nil {
		t.Fatal(err)
	}
	if len(f.asks) != 2 {
		t.Fatalf("filed %d, want 2", len(f.asks))
	}
	for _, a := range f.asks {
		if !homeAccepts(t, a, []string{scan}, uqbar) {
			t.Errorf("Home would reject %+v", a)
		}
	}
	// An unknown project, or estate without an Uqbar, files nothing.
	q2 := escalate.NewDecisionQueue()
	f2 := &recFiler{}
	q2.SetHomeRoots(f2, nil, homeRoots([]string{scan}, ""))
	if q2.AddPending(escalate.PendingDecision{Agent: "a", BeadID: "X-1", Labels: []string{"project:nope"}}) == nil ||
		q2.AddPending(escalate.PendingDecision{Agent: "a", BeadID: "E-2"}) == nil || len(f2.asks) != 0 {
		t.Fatalf("expected refusals, filed %+v", f2.asks)
	}
}
