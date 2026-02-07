package server

import (
	"strings"
	"testing"
)

func TestListenAndServe_RejectsNonLoopbackAddr(t *testing.T) {
	t.Parallel()

	s := New(t.TempDir())
	err := s.ListenAndServe("0.0.0.0:8091")
	if err == nil {
		t.Fatal("expected error for non-loopback bind address")
	}
	if !strings.Contains(err.Error(), "refusing to bind non-loopback") {
		t.Fatalf("unexpected error: %v", err)
	}
}
