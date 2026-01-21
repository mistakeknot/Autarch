package agent

import "testing"

func TestAdvanceOffset(t *testing.T) {
	next := advanceOffset(10, []string{"a", "b"})
	if next <= 10 {
		t.Fatal("expected offset to advance")
	}
}
