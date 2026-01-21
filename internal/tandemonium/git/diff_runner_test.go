package git

import "testing"

type fakeRunner struct{ output string }

func (f *fakeRunner) Run(name string, args ...string) (string, error) {
	return f.output, nil
}

func TestDiffNameOnly(t *testing.T) {
	r := &fakeRunner{output: "a.txt\nb.txt\n"}
	files, err := DiffNameOnly(r, "HEAD")
	if err != nil || len(files) != 2 {
		t.Fatal("expected 2 files")
	}
}

func TestDiffNameOnlyRange(t *testing.T) {
	r := &fakeRunner{output: "c.txt\n"}
	files, err := DiffNameOnlyRange(r, "main", "feature")
	if err != nil || len(files) != 1 {
		t.Fatal("expected 1 file")
	}
	if files[0] != "c.txt" {
		t.Fatalf("unexpected file: %v", files)
	}
}
