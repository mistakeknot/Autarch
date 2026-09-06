package review

import (
	"path/filepath"
	"testing"
)

func TestDurableStoreRejectsAuthenticationPayload(t *testing.T) {
	s, _ := Open(t.TempDir())
	r := s.Apply(Request{Version: Version, ID: "auth", Method: "turn.save", Project: t.TempDir(), Auth: &AuthRequest{Value: "PRIVATE"}, Turn: &Turn{Kind: "user", Text: "PRIVATE"}})
	if r.Error == "" || s.Snapshot().Revision != 0 {
		t.Fatal("authentication was stored as conversation")
	}
}

func TestProposalCannotRelabelOrAcceptCorrectedFeedback(t *testing.T) {
	s, _ := Open(t.TempDir())
	root, _ := filepath.EvalSymlinks(t.TempDir())
	apply := func(r Request) Response { r.Version = Version; r.ID = NewID(); r.Project = root; return s.Apply(r) }
	nr := apply(Request{Method: "feedback.save", Text: "Original"})
	p := Proposal{Project: root, Outcome: "Readable", Change: "Change", Scope: []string{"file"}, FeedbackIDs: []string{nr.ID}, FeedbackRevisions: map[string]int{nr.ID: 1}, Checklist: []string{"Look"}}
	pr := apply(Request{Method: "proposal.save", Proposal: &p})
	if pr.Error != "" {
		t.Fatal(pr.Error)
	}
	uncited := p
	uncited.FeedbackRevisions = nil
	if r := apply(Request{Method: "proposal.save", Proposal: &uncited}); r.Error == "" {
		t.Fatal("proposal silently filled missing observed revisions")
	}
	n := s.Snapshot().Feedback[nr.ID]
	n.Text = "Corrected"
	if r := apply(Request{Method: "feedback.save", Feedback: &n}); r.Error != "" {
		t.Fatal(r.Error)
	}
	if r := apply(Request{Method: "proposal.save", Proposal: &p}); r.Error == "" {
		t.Fatal("late proposal relabelled source revision")
	}
	if r := apply(Request{Method: "proposal.accept", Target: pr.ID, Revision: 1}); r.Error == "" {
		t.Fatal("proposal based on stale transcript accepted")
	}
	if len(s.Snapshot().Executions) != 0 {
		t.Fatal("stale proposal queued execution")
	}
	p.FeedbackRevisions = map[string]int{nr.ID: 2}
	current := apply(Request{Method: "proposal.save", Proposal: &p})
	if current.Error != "" {
		t.Fatal(current.Error)
	}
	if r := apply(Request{Method: "proposal.accept", Target: current.ID, Revision: 1}); r.Error != "" {
		t.Fatal(r.Error)
	}
}
