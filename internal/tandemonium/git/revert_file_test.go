package git

import "testing"

type fakeRunnerRevert struct{ output string }

func (f *fakeRunnerRevert) Run(name string, args ...string) (string, error) {
	return f.output, nil
}

func TestRevertFile(t *testing.T) {
	r := &fakeRunnerRevert{output: ""}
	if err := RevertFile(r, "main", "file.txt"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
