package agent

import "testing"

type fakeWorktree struct{ called bool }

func (f *fakeWorktree) Create(repo, path, branch string) error {
	f.called = true
	return nil
}

type fakeSession struct{ called bool }

func (f *fakeSession) Start(id, workdir, logPath string) error {
	f.called = true
	return nil
}

func TestStartTaskWorkflow(t *testing.T) {
	w := &fakeWorktree{}
	s := &fakeSession{}
	if err := StartTask(w, s, "TAND-001", "/repo", "/wt", "/log"); err != nil {
		t.Fatal(err)
	}
	if !w.called || !s.called {
		t.Fatal("expected worktree + session start")
	}
}
