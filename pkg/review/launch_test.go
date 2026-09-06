package review

import (
	"strings"
	"testing"
	"time"
)

func preparedRetest(t *testing.T) (*Store, Execution) {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	note := s.Apply(Request{Version: Version, ID: "note", Method: "feedback.save", Project: project, Text: "Retest the accepted change"})
	if note.Error != "" {
		t.Fatal(note.Error)
	}
	p := Proposal{ID: "proposal", Revision: 1, Project: project, FeedbackIDs: []string{note.ID}, FeedbackRevisions: map[string]int{note.ID: 1}, Outcome: "Readable review", Change: "Increase spacing", Scope: []string{"internal/reviewtui"}, Checklist: []string{"Try both densities"}, Priority: 2}
	for _, r := range []Request{
		{Version: Version, ID: "draft", Method: "proposal.save", Project: project, Proposal: &p},
		{Version: Version, ID: "accept", Method: "proposal.accept", Project: project, Target: p.ID, Revision: 1},
	} {
		if got := s.Apply(r); got.Error != "" {
			t.Fatal(got.Error)
		}
	}
	e := s.Snapshot().Executions[p.ID]
	e.Status, e.Build, e.Binary = "ready_for_retest", "revision:sha256:build-one", "/prepared/autarch"
	if got := s.Apply(Request{Version: Version, ID: "ready", Method: "execution.save", Project: project, Execution: &e}); got.Error != "" {
		t.Fatal(got.Error)
	}
	return s, e
}

func TestPassingRetestRequiresRecordedMatchingInvocation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		at    *time.Time
		build string
	}{
		{name: "not launched"},
		{name: "timestamp missing", build: "revision:sha256:build-one"},
		{name: "zero timestamp", at: &time.Time{}, build: "revision:sha256:build-one"},
		{name: "different build", at: func() *time.Time { v := time.Now(); return &v }(), build: "revision:sha256:other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, e := preparedRetest(t)
			// Exercise malformed or stale persisted invocation evidence, as well as
			// the ordinary case where the human has not opened the build yet.
			e.InvokedAt, e.InvokedBuild = tc.at, tc.build
			s.state.Executions[e.ID] = e
			got := s.Apply(Request{Version: Version, ID: "pass", Method: "verdict.save", Project: e.Project, Verdict: &Verdict{ExecutionID: e.ID, Build: e.Build, Verdict: "pass"}})
			if !strings.Contains(got.Error, "recorded invocation") || len(s.Snapshot().Verdicts) != 0 {
				t.Fatalf("accepted pass without matching recorded invocation: %+v", got)
			}
		})
	}
}

func TestMatchingInvocationSurvivesReopenAndAllowsPassingRetest(t *testing.T) {
	s, e := preparedRetest(t)
	if got := s.Apply(Request{Version: Version, ID: "invoked", Method: "execution.invoked", Project: e.Project, Target: e.ID, Text: e.Build}); got.Error != "" {
		t.Fatal(got.Error)
	}
	s, err := Open(s.Dir())
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Version: Version, ID: "pass", Method: "verdict.save", Project: e.Project, Verdict: &Verdict{ExecutionID: e.ID, Build: e.Build, Verdict: "pass"}}
	for range 2 {
		if got := s.Apply(req); got.Error != "" {
			t.Fatal(got.Error)
		}
	}
	if len(s.Snapshot().Verdicts) != 1 {
		t.Fatal("retry duplicated verdict")
	}
}

func TestUnlaunchedBuildCanRecordFailedOrInconclusiveRetest(t *testing.T) {
	for _, verdict := range []string{"fail", "inconclusive"} {
		t.Run(verdict, func(t *testing.T) {
			s, e := preparedRetest(t)
			got := s.Apply(Request{Version: Version, ID: "verdict", Method: "verdict.save", Project: e.Project, Verdict: &Verdict{ExecutionID: e.ID, Build: e.Build, Verdict: verdict, Notes: "Build could not be opened"}})
			if got.Error != "" {
				t.Fatal(got.Error)
			}
		})
	}
}

func TestExecutionSaveCannotSupplyInvocationEvidence(t *testing.T) {
	s, e := preparedRetest(t)
	now := time.Now()
	e.InvokedAt, e.InvokedBuild = &now, e.Build
	if got := s.Apply(Request{Version: Version, ID: "forged", Method: "execution.save", Project: e.Project, Execution: &e}); got.Error != "" {
		t.Fatal(got.Error)
	}
	if s.Snapshot().Executions[e.ID].InvokedAt != nil {
		t.Fatal("execution adapter supplied its own launch evidence")
	}
	got := s.Apply(Request{Version: Version, ID: "pass", Method: "verdict.save", Project: e.Project, Verdict: &Verdict{ExecutionID: e.ID, Build: e.Build, Verdict: "pass"}})
	if got.Error == "" {
		t.Fatal("execution adapter authorized a passing retest without a launch")
	}
}
