package review

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestNavigationContextStaysCurrentWithoutDurablePerKeyRecords(t *testing.T) {
	dir, root := t.TempDir(), t.TempDir()
	s, _ := Open(dir)
	for i := 0; i < 50; i++ {
		r := s.Apply(Request{Version: Version, ID: NewID(), Method: "context", Project: root, Context: &UIContext{Project: root, View: "review/feedback", Item: fmt.Sprint(i), Density: "Cozy", Build: "test-build"}})
		if r.Error != "" {
			t.Fatal(r.Error)
		}
	}
	state := s.Snapshot()
	if state.Context.Item != "49" {
		t.Fatal("latest selection lost")
	}
	if state.Revision != 0 || len(state.Receipts) != 0 {
		t.Errorf("navigation creates durable revisions/receipts: %d/%d", state.Revision, len(state.Receipts))
	}
	files, _ := filepath.Glob(filepath.Join(dir, "records", "*.json"))
	if len(files) != 0 {
		t.Errorf("navigation wrote %d full-state records", len(files))
	}
	r := s.Apply(Request{Version: Version, ID: "observation", Method: "feedback.save", Project: root, Text: "The selected moment"})
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Snapshot().Feedback[r.ID].Context; got.Item != "49" || got.Build != "test-build" || got.At.IsZero() {
		t.Fatalf("evidence did not freeze its actual context: %+v", got)
	}
}

func TestFeedbackRebaseRejectionContract(t *testing.T) {
	s, _ := Open(t.TempDir())
	root, other := t.TempDir(), t.TempDir()
	apply := func(r Request) Response { r.Version = Version; r.ID = NewID(); return s.Apply(r) }
	r := apply(Request{Method: "feedback.save", Project: root, Text: "Original"})
	n := s.Snapshot().Feedback[r.ID]
	t.Run("routing", func(t *testing.T) {
		if got := apply(Request{Method: "feedback.save", Project: other, Feedback: &n}).Error; got != "project mismatch" {
			t.Fatal(got)
		}
	})
	t.Run("revision", func(t *testing.T) {
		stale := n
		stale.Revision--
		if got := apply(Request{Method: "feedback.save", Project: root, Feedback: &stale}).Error; got != "stale feedback" {
			t.Fatal(got)
		}
	})
	t.Run("session binding is not a rebase", func(t *testing.T) {
		bad := n
		bad.SessionID = "missing-session"
		if got := apply(Request{Method: "feedback.save", Project: root, Feedback: &bad}).Error; got != "feedback session project mismatch" {
			t.Fatal(got)
		}
	})
}

func TestAcknowledgedFeedbackSurvivesReopenAndRetry(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	req := Request{Version: Version, ID: "save-1", Method: "feedback.save", Project: t.TempDir(), Text: "Keep the original observation"}
	first := s.Apply(req)
	if first.Error != "" {
		t.Fatal(first.Error)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	retry := s.Apply(req)
	if retry.Error != "" || first.ID != retry.ID || len(s.Snapshot().Feedback) != 1 {
		t.Fatalf("retry lost/duplicated feedback: %+v", retry)
	}
	req.Text = "different scope"
	if s.Apply(req).Error == "" {
		t.Fatal("accepted reused id with different payload")
	}
}

func TestContextUsesCanonicalProjectAndRejectsCrossProjectPayload(t *testing.T) {
	s, _ := Open(t.TempDir())
	root := t.TempDir()
	project := filepath.Join(root, "project")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(project, alias); err != nil {
		t.Fatal(err)
	}
	canonical, _ := filepath.EvalSymlinks(project)
	r := Request{Version: Version, ID: "context", Method: "context", Project: alias, Context: &UIContext{Project: alias, View: "door"}}
	if response := s.Apply(r); response.Error != "" {
		t.Fatal(response.Error)
	}
	if s.Snapshot().Context.Project != canonical {
		t.Fatalf("raw context root: %q", s.Snapshot().Context.Project)
	}
	r.ID = "wrong-context"
	r.Context = &UIContext{Project: t.TempDir()}
	if response := s.Apply(r); response.Error == "" {
		t.Fatal("accepted context outside request project")
	}
}

func TestMalformedModelSelectionCannotBecomePendingWork(t *testing.T) {
	s, _ := Open(t.TempDir())
	project := t.TempDir()
	for _, model := range []string{"/model", "provider/", " provider/model", "provider/model name", "provider/--flag"} {
		response := s.Apply(Request{Version: Version, ID: NewID(), Method: "runtime.switch", Project: project, Text: model})
		if response.Error == "" {
			t.Errorf("accepted malformed identity %q", model)
		}
	}
	if len(s.Snapshot().Turns) != 0 {
		t.Fatal("invalid selection was persisted")
	}
}

func TestFailedWriteIsNotAcknowledgedOrKeptInMemory(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(dir, "records"), filepath.Join(dir, "saved")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "records"), []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	got := s.Apply(Request{Version: Version, ID: "save", Method: "feedback.save", Text: "must not vanish", Project: t.TempDir()})
	if got.Error == "" || len(s.Snapshot().Feedback) != 0 {
		t.Fatalf("false acknowledgement: %+v", got)
	}
}

func TestApprovalBindsRevisionAndProject(t *testing.T) {
	s, _ := Open(t.TempDir())
	project := t.TempDir()
	note := s.Apply(Request{Version: Version, ID: "note", Method: "feedback.save", Project: project, Text: "too crowded"})
	proposal := Proposal{ID: "proposal", Revision: 1, Project: project, FeedbackIDs: []string{note.ID}, FeedbackRevisions: map[string]int{note.ID: 1}, Outcome: "Readable review", Change: "Increase spacing", Scope: []string{"internal/reviewtui"}, Rationale: "Separate review outcomes", Checklist: []string{"Try both densities"}, Priority: 2}
	result := s.Apply(Request{Version: Version, ID: "draft", Method: "proposal.save", Project: project, Proposal: &proposal})
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	if s.Apply(Request{Version: Version, ID: "wrong", Method: "proposal.accept", Project: t.TempDir(), Target: "proposal", Revision: 1}).Error == "" {
		t.Fatal("cross-project acceptance")
	}
	if s.Apply(Request{Version: Version, ID: "stale", Method: "proposal.accept", Project: project, Target: "proposal", Revision: 2}).Error == "" {
		t.Fatal("stale approval")
	}
	req := Request{Version: Version, ID: "accept", Method: "proposal.accept", Project: project, Target: "proposal", Revision: 1}
	if got := s.Apply(req); got.Error != "" {
		t.Fatal(got.Error)
	}
	if got := s.Apply(req); got.Error != "" {
		t.Fatal(got.Error)
	}
	if len(s.Snapshot().Executions) != 1 {
		t.Fatal("duplicate execution")
	}
	proposal.Change = "Unseen change"
	proposal.Revision = 2
	if s.Apply(Request{Version: Version, ID: "rewrite", Method: "proposal.save", Project: project, Proposal: &proposal}).Error == "" {
		t.Fatal("rewrote accepted scope")
	}
}

func TestSnapshotCannotMutateStore(t *testing.T) {
	s, _ := Open(t.TempDir())
	s.Apply(Request{Version: Version, ID: "note", Method: "feedback.save", Project: t.TempDir(), Text: "original"})
	snapshot := s.Snapshot()
	for id, n := range snapshot.Feedback {
		n.Text = "mutated"
		snapshot.Feedback[id] = n
	}
	for _, n := range s.Snapshot().Feedback {
		if n.Text != "original" {
			t.Fatal("shared mutable state")
		}
	}
}

func TestProposalRequestOwnershipPreservesIdempotenceAndSavedScope(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	note := s.Apply(Request{Version: Version, ID: "note", Method: "feedback.save", Project: project, Text: "Keep the accepted scope"})
	p := Proposal{ID: "proposal", Revision: 1, Project: project, FeedbackIDs: []string{note.ID}, FeedbackRevisions: map[string]int{note.ID: 1}, Outcome: "Readable review", Change: "Increase spacing", Scope: []string{"internal/reviewtui"}, Checklist: []string{"Try spacing"}}
	req := Request{Version: Version, ID: "proposal", Method: "proposal.save", Project: project, Proposal: &p}
	if got := s.Apply(req); got.Error != "" {
		t.Fatal(got.Error)
	}
	if got := s.Apply(req); got.Error != "" || !got.Replayed {
		t.Fatalf("unchanged request stopped being idempotent: %+v", got)
	}
	p.Scope[0] = "unseen-scope"
	p.FeedbackRevisions[note.ID] = 99
	stored := s.Snapshot().Proposals[p.ID]
	if stored.Scope[0] != "internal/reviewtui" || stored.FeedbackRevisions[note.ID] != 1 {
		t.Fatalf("caller changed authoritative scope after acknowledgement: %+v", stored)
	}
}

func TestProposalCannotMoveAcrossProjects(t *testing.T) {
	s, _ := Open(t.TempDir())
	a, b := t.TempDir(), t.TempDir()
	noteA := s.Apply(Request{Version: Version, ID: "a", Method: "feedback.save", Project: a, Text: "A"})
	noteB := s.Apply(Request{Version: Version, ID: "b", Method: "feedback.save", Project: b, Text: "B"})
	p := Proposal{ID: "shared", Project: a, Revision: 1, FeedbackIDs: []string{noteA.ID}, FeedbackRevisions: map[string]int{noteA.ID: 1}, Outcome: "A outcome", Change: "A change", Scope: []string{"src"}, Checklist: []string{"Try A"}}
	if got := s.Apply(Request{Version: Version, ID: "pa", Method: "proposal.save", Project: a, Proposal: &p}); got.Error != "" {
		t.Fatal(got.Error)
	}
	p.Project = b
	p.Revision = 2
	p.FeedbackIDs = []string{noteB.ID}
	p.FeedbackRevisions = map[string]int{noteB.ID: 1}
	if got := s.Apply(Request{Version: Version, ID: "pb", Method: "proposal.save", Project: b, Proposal: &p}); got.Error == "" {
		t.Fatal("proposal moved from A to B")
	}
}

func TestTypedReviewNoteQueuesSnapshotAndKeepsSession(t *testing.T) {
	s, _ := Open(t.TempDir())
	project := t.TempDir()
	s.Apply(Request{Version: Version, ID: "session", Method: "session.save", Project: project, Session: &Session{ID: "session", Status: "recording"}})
	r := s.Apply(Request{Version: Version, ID: "note", Method: "feedback.save", Project: project, Feedback: &Feedback{Text: "Review moment", SessionID: "session"}})
	st := s.Snapshot()
	if st.Feedback[r.ID].SessionID != "session" || len(st.Commands) != 1 || st.Commands[0].Target != r.ID || st.Commands[0].Method != "snapshot" {
		t.Fatalf("moment lost: %+v", st)
	}
}
