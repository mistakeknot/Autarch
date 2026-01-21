package agent

import "testing"

type fakeStore struct{ sessionUpdated, taskUpdated bool }

func (f *fakeStore) UpdateSessionState(id, state string) error {
	f.sessionUpdated = true
	return nil
}

func (f *fakeStore) UpdateTaskStatus(id, status string) error {
	f.taskUpdated = true
	return nil
}

func (f *fakeStore) EnqueueReview(id string) error { return nil }

func TestApplyDetection(t *testing.T) {
	fs := &fakeStore{}
	if err := ApplyDetection(fs, "TAND-001", "tand-TAND-001", "done"); err != nil {
		t.Fatal(err)
	}
	if !fs.sessionUpdated || !fs.taskUpdated {
		t.Fatal("expected both updates")
	}
}
