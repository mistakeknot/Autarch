package server

import (
	"strings"
	"testing"
)

func TestListenAndServe_RejectsNonLoopbackAddr(t *testing.T) {
	t.Parallel()

	srv, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	defer func() { _ = srv.Close() }()

	err = srv.ListenAndServe("0.0.0.0:8090")
	if err == nil {
		t.Fatal("expected error for non-loopback bind address")
	}
	if !strings.Contains(err.Error(), "refusing to bind non-loopback") {
		t.Fatalf("unexpected error: %v", err)
	}
}
