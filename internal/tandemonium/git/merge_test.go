package git

import "testing"

type fakeMergeRunner struct{ args [][]string }

func (f *fakeMergeRunner) Run(name string, args ...string) (string, error) {
	f.args = append(f.args, append([]string{name}, args...))
	return "", nil
}

func TestMergeBranch(t *testing.T) {
	r := &fakeMergeRunner{}
	_ = MergeBranch(r, "feature/TAND-001")
	if len(r.args) == 0 || r.args[0][1] != "merge" {
		t.Fatal("expected git merge")
	}
}
