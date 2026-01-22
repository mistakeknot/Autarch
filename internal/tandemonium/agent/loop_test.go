package agent

import "testing"

type fakeStore struct {
	sessionUpdated bool
	taskUpdated    bool
	atomicCalled   bool
}

func (f *fakeStore) UpdateSessionState(id, state string) error {
	f.sessionUpdated = true
	return nil
}

func (f *fakeStore) UpdateTaskStatus(id, status string) error {
	f.taskUpdated = true
	return nil
}

func (f *fakeStore) EnqueueReview(id string) error { return nil }
func (f *fakeStore) ApplyDetectionAtomic(taskID, sessionID, state string) error {
	f.atomicCalled = true
	return nil
}

func TestApplyDetection(t *testing.T) {
	fs := &fakeStore{}
	if err := ApplyDetection(fs, "TAND-001", "tand-TAND-001", "done"); err != nil {
		t.Fatal(err)
	}
	if !fs.atomicCalled {
		t.Fatal("expected atomic helper call")
	}
}
