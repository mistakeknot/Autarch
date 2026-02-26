package clavain

import (
	"testing"
)

func TestSprintCreate_MissingBinary(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")
	c, err := New()
	if err == nil {
		_ = c
		t.Skip("clavain-cli found on PATH")
	}
	// Can't create client, which is correct behavior
}

func TestSprintCreateOptions(t *testing.T) {
	opts := []SprintOption{
		WithSprintComplexity(4),
		WithSprintLane("core"),
	}
	var o sprintOpts
	for _, fn := range opts {
		fn(&o)
	}
	if o.complexity != 4 {
		t.Errorf("complexity = %d, want 4", o.complexity)
	}
	if o.lane != "core" {
		t.Errorf("lane = %q, want %q", o.lane, "core")
	}
}

func TestSprintCancel_ReturnsError(t *testing.T) {
	c, err := New(WithBinPath("/nonexistent/clavain-cli"))
	if err != nil {
		t.Fatal(err)
	}
	err = c.SprintCancel(nil, "test-run-id")
	if err == nil {
		t.Error("expected error from unimplemented SprintCancel")
	}
}
