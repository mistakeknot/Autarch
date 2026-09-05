package review

import (
	"os"
	"path/filepath"
	"testing"
)

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
	proposal := Proposal{ID: "proposal", Revision: 1, Project: project, FeedbackIDs: []string{note.ID}, Outcome: "Readable review", Change: "Increase spacing", Scope: []string{"internal/reviewtui"}, Rationale: "Separate review outcomes", Checklist: []string{"Try both densities"}, Priority: 2}
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

func TestProposalCannotMoveAcrossProjects(t *testing.T) {
	s, _ := Open(t.TempDir())
	a, b := t.TempDir(), t.TempDir()
	noteA := s.Apply(Request{Version: Version, ID: "a", Method: "feedback.save", Project: a, Text: "A"})
	noteB := s.Apply(Request{Version: Version, ID: "b", Method: "feedback.save", Project: b, Text: "B"})
	p := Proposal{ID: "shared", Project: a, Revision: 1, FeedbackIDs: []string{noteA.ID}, Outcome: "A outcome", Change: "A change", Scope: []string{"src"}, Checklist: []string{"Try A"}}
	if got := s.Apply(Request{Version: Version, ID: "pa", Method: "proposal.save", Project: a, Proposal: &p}); got.Error != "" {
		t.Fatal(got.Error)
	}
	p.Project = b
	p.Revision = 2
	p.FeedbackIDs = []string{noteB.ID}
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
