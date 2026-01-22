package agent

import "testing"

type fakeReviewStore struct{ enqueued bool }

func (f *fakeReviewStore) UpdateSessionState(id, state string) error { return nil }
func (f *fakeReviewStore) UpdateTaskStatus(id, status string) error  { return nil }
func (f *fakeReviewStore) EnqueueReview(id string) error             { f.enqueued = true; return nil }
func (f *fakeReviewStore) ApplyDetectionAtomic(taskID, sessionID, state string) error {
	return nil
}

func TestApplyDetectionEnqueuesOnDone(t *testing.T) {
	fs := &fakeReviewStore{}
	_ = ApplyDetection(fs, "TAND-001", "tand-TAND-001", "done")
	if !fs.enqueued {
		t.Fatal("expected enqueue")
	}
}
