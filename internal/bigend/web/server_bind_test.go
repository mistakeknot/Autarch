package web

import (
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/bigend/config"
)

func TestListenAndServe_RejectsNonLoopbackAddr(t *testing.T) {
	t.Parallel()

	srv := NewServer(config.ServerConfig{}, &fakeAgg{})
	err := srv.ListenAndServe("0.0.0.0:8099")
	if err == nil {
		t.Fatal("expected error for non-loopback bind address")
	}
	if !strings.Contains(err.Error(), "refusing to bind non-loopback") {
		t.Fatalf("unexpected error: %v", err)
	}
}
