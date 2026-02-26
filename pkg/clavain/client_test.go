package clavain

import (
	"testing"
)

func TestNew_BinaryNotFound(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")
	_, err := New()
	if err == nil {
		t.Error("expected error when clavain-cli not on PATH")
	}
	if err != ErrUnavailable {
		t.Errorf("expected ErrUnavailable, got %v", err)
	}
}

func TestAvailable_NoError(t *testing.T) {
	// Just verify it doesn't panic.
	_ = Available()
}

func TestWithBinPath(t *testing.T) {
	c, err := New(WithBinPath("/nonexistent/clavain-cli"))
	if err != nil {
		t.Fatalf("WithBinPath should not error: %v", err)
	}
	if c.binPath != "/nonexistent/clavain-cli" {
		t.Errorf("binPath = %q, want /nonexistent/clavain-cli", c.binPath)
	}
}

func TestWithTimeout(t *testing.T) {
	c, err := New(WithBinPath("/nonexistent/clavain-cli"), WithTimeout(5000000000))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.timeout != 5000000000 {
		t.Errorf("timeout = %v, want 5s", c.timeout)
	}
}
