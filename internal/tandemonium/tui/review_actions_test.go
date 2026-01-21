package tui

import "testing"

func TestSubmitFeedbackClearsInput(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewInputMode = ReviewInputFeedback
	m.ReviewInput = "Looks good"
	m.ReviewDetail = ReviewDetail{TaskID: "T1"}
	m.ReviewActionWriter = func(taskID, text string) error { return nil }
	m.handleReviewSubmit()
	if m.ReviewInputMode != ReviewInputNone || m.ReviewInput != "" {
		t.Fatalf("expected input cleared")
	}
}

func TestSubmitRejectRequeues(t *testing.T) {
	m := NewModel()
	m.ViewMode = ViewReview
	m.ReviewInputMode = ReviewInputFeedback
	m.ReviewPendingReject = true
	m.ReviewInput = "Needs work"
	m.ReviewDetail = ReviewDetail{TaskID: "T1"}
	m.ReviewActionWriter = func(taskID, text string) error { return nil }
	m.ReviewRejecter = func(taskID string) error { return nil }
	m.handleReviewSubmit()
	if m.ReviewPendingReject {
		t.Fatalf("expected reject cleared")
	}
}
