package signals

import (
	"strings"
	"testing"
)

func TestListenAndServe_RejectsNonLoopbackAddr(t *testing.T) {
	t.Parallel()

	srv := NewServer(NewBroker())
	err := srv.ListenAndServe("0.0.0.0:8092")
	if err == nil {
		t.Fatal("expected error for non-loopback bind address")
	}
	if !strings.Contains(err.Error(), "refusing to bind non-loopback") {
		t.Fatalf("unexpected error: %v", err)
	}
}
