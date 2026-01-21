package drift

import "testing"

func TestDetectDrift(t *testing.T) {
	spec := []string{"a.txt"}
	changed := []string{"a.txt", "b.txt"}
	drift := DetectDrift(spec, changed)
	if len(drift) != 1 || drift[0] != "b.txt" {
		t.Fatal("expected drift for b.txt")
	}
}
