package git

import "testing"

type fakeRunnerUnified struct{ out string }

func (f *fakeRunnerUnified) Run(name string, args ...string) (string, error) { return f.out, nil }

func TestDiffUnified(t *testing.T) {
	r := &fakeRunnerUnified{out: "@@ -1 +1 @@\n-old\n+new\n"}
	lines, err := DiffUnified(r, "main", "feature", "file.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(lines) == 0 || lines[0] != "@@ -1 +1 @@" {
		t.Fatalf("unexpected diff lines: %v", lines)
	}
}
