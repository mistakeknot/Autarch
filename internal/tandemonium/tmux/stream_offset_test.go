package tmux

import (
	"os"
	"testing"
)

func TestReadFromOffset(t *testing.T) {
	f, err := os.CreateTemp("", "tand-log-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	_, _ = f.WriteString("one\n")
	_ = f.Sync()

	lines, next, err := ReadFromOffset(f.Name(), 0)
	if err != nil || len(lines) != 1 || next == 0 {
		t.Fatal("expected one line and advanced offset")
	}
}
