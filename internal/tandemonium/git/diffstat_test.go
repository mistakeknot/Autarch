package git

import "testing"

func TestParseNumstat(t *testing.T) {
	out := "10\t2\tsrc/app.go\n5\t0\tREADME.md\n"
	stats := ParseNumstat(out)
	if len(stats) != 2 {
		t.Fatalf("expected 2 entries")
	}
	if stats[0].Path != "src/app.go" || stats[0].Added != 10 || stats[0].Deleted != 2 {
		t.Fatalf("unexpected stat: %+v", stats[0])
	}
}
