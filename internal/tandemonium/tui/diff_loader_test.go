package tui

import "testing"

type fakeRunner struct{ out string }

func (f *fakeRunner) Run(name string, args ...string) (string, error) { return f.out, nil }

func TestLoadDiffFiles(t *testing.T) {
	r := &fakeRunner{out: "a.txt\n"}
	files, err := LoadDiffFiles(r, "HEAD")
	if err != nil || len(files) != 1 {
		t.Fatal("expected one diff file")
	}
}
