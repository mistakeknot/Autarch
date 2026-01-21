package git

import (
	"errors"
	"testing"
)

type fakeBranchRunner struct{ out string }

func (f *fakeBranchRunner) Run(name string, args ...string) (string, error) {
	return f.out, nil
}

func TestBranchForTaskPrefersExactMatch(t *testing.T) {
	r := &fakeBranchRunner{out: "feature/TAND-001\nTAND-001\n"}
	branch, err := BranchForTask(r, "TAND-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "TAND-001" {
		t.Fatalf("expected exact match, got %q", branch)
	}
}

func TestBranchForTaskFallsBackToContains(t *testing.T) {
	r := &fakeBranchRunner{out: "feature/TAND-001\nbugfix/other\n"}
	branch, err := BranchForTask(r, "TAND-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if branch != "feature/TAND-001" {
		t.Fatalf("expected contains match, got %q", branch)
	}
}

func TestBranchForTaskNotFound(t *testing.T) {
	r := &fakeBranchRunner{out: "feature/OTHER\n"}
	_, err := BranchForTask(r, "TAND-001")
	if !errors.Is(err, ErrBranchNotFound) {
		t.Fatalf("expected ErrBranchNotFound, got %v", err)
	}
}
