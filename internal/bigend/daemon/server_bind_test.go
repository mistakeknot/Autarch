package daemon

import (
	"strings"
	"testing"
)

func TestStart_RejectsNonLoopbackAddr(t *testing.T) {
	t.Parallel()

	srv := NewServer(Config{
		Addr:        "0.0.0.0:8100",
		ProjectDirs: nil,
	})

	err := srv.Start()
	if err == nil {
		t.Fatal("expected error for non-loopback bind address")
	}
	if !strings.Contains(err.Error(), "refusing to bind non-loopback") {
		t.Fatalf("unexpected error: %v", err)
	}
}
