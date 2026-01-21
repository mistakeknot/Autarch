package tui

import "testing"

type fakeApprover struct{ called bool }

func (f *fakeApprover) Approve(taskID, branch string) error {
	f.called = true
	return nil
}

func TestModelApprove(t *testing.T) {
	m := NewModel()
	a := &fakeApprover{}
	_ = m.ApproveTask(a, "TAND-001", "feature/TAND-001")
	if !a.called {
		t.Fatal("expected approve call")
	}
}
