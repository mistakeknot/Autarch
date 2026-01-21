package git

import "testing"

func TestParseNameOnly(t *testing.T) {
	out := "a.txt\nb.txt\n"
	files := ParseNameOnly(out)
	if len(files) != 2 {
		t.Fatal("expected 2")
	}
}
