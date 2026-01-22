package agentcmd

import (
	"testing"

	vconfig "github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/config"
)

func TestResolveCommandFallback(t *testing.T) {
	cfg := &vconfig.Config{}
	r := NewResolver(cfg)
	cmd, args := r.Resolve("claude", "/root/projects/demo")
	if cmd != "claude" {
		t.Fatalf("expected claude fallback, got %q", cmd)
	}
	if len(args) != 0 {
		t.Fatalf("expected no args, got %v", args)
	}
}
