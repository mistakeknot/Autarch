package review

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepeatedSessionDeletionCannotBumpFeedbackAgain(t *testing.T) {
	s, _ := Open(t.TempDir())
	project := t.TempDir()
	r := s.Apply(Request{Version: Version, ID: "session", Method: "session.save", Project: project, Session: &Session{ID: "session", Status: "stopped"}})
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	r = s.Apply(Request{Version: Version, ID: "note", Method: "feedback.save", Project: project, Feedback: &Feedback{ID: "note", Text: "Retain this", SessionID: "session"}})
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	r = s.Apply(Request{Version: Version, ID: "delete", Method: "session.delete", Project: project, Target: "session", Revision: 1})
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := s.DeleteCaptures(); err != nil {
		t.Fatal(err)
	}
	before := s.Snapshot()
	r = s.Apply(Request{Version: Version, ID: "delete-again", Method: "session.delete", Project: project, Target: "session", Revision: before.Sessions["session"].Revision})
	if r.Error == "" {
		t.Fatal("accepted a new deletion of an already deleted session")
	}
	if err := s.DeleteCaptures(); err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Feedback["note"].Revision != before.Feedback["note"].Revision {
		t.Fatal("repeated deletion invalidated a correction")
	}
}

func TestCaptureDeletionRetainsNotesAndCannotBeResurrected(t *testing.T) {
	s, _ := Open(t.TempDir())
	project := t.TempDir()
	media := filepath.Join(s.Dir(), "media", "session")
	os.MkdirAll(media, 0700)
	shot := filepath.Join(media, "shot.png")
	os.WriteFile(shot, []byte("capture"), 0600)
	source := Source{ID: "shot", Path: shot, Kind: "screenshot", Status: "available"}
	r := s.Apply(Request{Version: Version, ID: "session", Method: "session.save", Project: project, Session: &Session{ID: "session", Status: "stopped", Media: []Source{source}}})
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	r = s.Apply(Request{Version: Version, ID: "note", Method: "feedback.save", Project: project, Feedback: &Feedback{ID: "note", Text: "Original observation", SessionID: "session", Evidence: []Source{source}}})
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	session := s.Snapshot().Sessions["session"]
	r = s.Apply(Request{Version: Version, ID: "delete", Method: "session.delete", Project: project, Target: "session", Revision: session.Revision})
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	if err := s.DeleteCaptures(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(shot); !os.IsNotExist(err) {
		t.Fatal("capture retained after explicit deletion")
	}
	note := s.Snapshot().Feedback["note"]
	if note.Text != "Original observation" || note.Evidence[0].Status != "deleted" {
		t.Fatalf("lost provenance: %+v", note)
	}
	note.Evidence = []Source{source}
	note.Text = "Corrected transcript"
	r = s.Apply(Request{Version: Version, ID: "correction", Method: "feedback.save", Project: project, Feedback: &note})
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	if s.Snapshot().Feedback["note"].Evidence[0].Status != "deleted" {
		t.Fatal("resurrected deleted evidence")
	}
	r = s.Apply(Request{Version: Version, ID: "late", Method: "session.save", Project: project, Session: &session})
	if r.Error == "" {
		t.Fatal("late companion save reopened deleted session")
	}
	// Even after the investigator observes the current corrected note revision,
	// its cached source metadata must not resurrect a deleted evidence path.
	proposal := Proposal{ID: "late-proposal", Project: project, Revision: 1, FeedbackIDs: []string{"note"}, FeedbackRevisions: map[string]int{"note": s.Snapshot().Feedback["note"].Revision}, Outcome: "Readable review", Change: "Increase spacing", Scope: []string{"internal/reviewtui"}, Checklist: []string{"Check spacing"}, Evidence: []Source{source}}
	r = s.Apply(Request{Version: Version, ID: "late-proposal", Method: "proposal.save", Project: project, Proposal: &proposal})
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	if got := s.Snapshot().Proposals[proposal.ID].Evidence[0]; got.Status != "deleted" || got.Path != "" {
		t.Fatalf("late proposal restored a deleted source: %+v", got)
	}
	r = s.Apply(Request{Version: Version, ID: "late-note", Method: "feedback.save", Project: project, Feedback: &Feedback{ID: "late-note", Text: "Late observation", Evidence: []Source{source}}})
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	if got := s.Snapshot().Feedback["late-note"].Evidence[0]; got.Status != "deleted" || got.Path != "" {
		t.Fatalf("late feedback restored a deleted source: %+v", got)
	}
	// A delayed screenshot has a new source ID but still belongs to the deleted
	// session directory. It must not become available merely by arriving late.
	lateShot := Source{ID: "late-shot", Path: filepath.Join(media, "late.png"), Kind: "screenshot", Status: "available"}
	r = s.Apply(Request{Version: Version, ID: "late-snapshot", Method: "feedback.save", Project: project, Feedback: &Feedback{ID: "late-snapshot", Text: "Delayed snapshot", SessionID: "session", Evidence: []Source{lateShot}}})
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	if got := s.Snapshot().Feedback["late-snapshot"].Evidence[0]; got.Status != "deleted" || got.Path != "" {
		t.Fatalf("late snapshot appeared available in deleted capture directory: %+v", got)
	}
	otherProject := t.TempDir()
	otherSource := Source{ID: source.ID, Path: filepath.Join(otherProject, "shot.png"), Status: "available"}
	r = s.Apply(Request{Version: Version, ID: "other-project", Method: "feedback.save", Project: otherProject, Feedback: &Feedback{ID: "other-project", Text: "Independent evidence", Evidence: []Source{otherSource}}})
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	if got := s.Snapshot().Feedback["other-project"].Evidence[0]; got.Status != "available" || got.Path != otherSource.Path {
		t.Fatalf("deletion leaked across project binding: %+v", got)
	}
}

func TestCaptureDeletionDoesNotFollowSessionSymlink(t *testing.T) {
	s, _ := Open(t.TempDir())
	project := t.TempDir()
	outside := t.TempDir()
	os.WriteFile(filepath.Join(outside, "valuable"), []byte("keep"), 0600)
	os.MkdirAll(filepath.Join(s.Dir(), "media"), 0700)
	os.Symlink(outside, filepath.Join(s.Dir(), "media", "session"))
	s.Apply(Request{Version: Version, ID: "session", Method: "session.save", Project: project, Session: &Session{ID: "session", Status: "recording"}})
	session := s.Snapshot().Sessions["session"]
	if s.Apply(Request{Version: Version, ID: "active-delete", Method: "session.delete", Project: project, Target: "session", Revision: session.Revision}).Error == "" {
		t.Fatal("deleted active recording")
	}
	session.Status = "stopped"
	s.Apply(Request{Version: Version, ID: "stop", Method: "session.save", Project: project, Session: &session})
	session = s.Snapshot().Sessions["session"]
	s.Apply(Request{Version: Version, ID: "delete", Method: "session.delete", Project: project, Target: "session", Revision: session.Revision})
	if s.DeleteCaptures() == nil {
		t.Fatal("followed symlink")
	}
	if _, err := os.Stat(filepath.Join(outside, "valuable")); err != nil {
		t.Fatal(err)
	}
}
