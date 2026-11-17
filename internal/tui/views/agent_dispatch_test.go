package views

import (
	"context"
	"github.com/mistakeknot/autarch/internal/autarch/agent"
	"github.com/mistakeknot/autarch/internal/tui"
	"github.com/mistakeknot/autarch/pkg/agenttargets"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestScanDispatchReceivesExplicitPick(t *testing.T) {
	if os.Getenv("SCAN_PICK_CHILD") != "1" {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "codex"), []byte("#!/bin/sh\necho test-version\n"), 0700); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestScanDispatchReceivesExplicitPick$")
		cmd.Env = append(os.Environ(), "SCAN_PICK_CHILD=1", "PATH="+dir, "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		return
	}
	t.Setenv("HOME", t.TempDir())

	t.Setenv("AUTARCH_DISPATCH_AGENT", "claude")
	picked, err := agent.DetectAgentByName("codex", func(string) (string, error) { return "/fixture/codex", nil })
	if err != nil {
		t.Fatal(err)
	}
	v := NewGurgehOnboardingView(tui.GurgehConfig{})
	defer v.cancel()
	v.SetCodingAgent(picked)
	configs := make(chan agenttargets.DispatchConfig, 1)
	v.explore = func(_ context.Context, _ string, cfg agenttargets.DispatchConfig) (map[string]any, string, error) {
		configs <- cfg
		return map[string]any{}, "", nil
	}
	v.scanCodebase(t.TempDir())
	select {
	case cfg := <-configs:
		if cfg.PreferredAgent != "codex" || cfg.PreferredAgentSource != "explicit agent selection" {
			t.Fatalf("dispatch received %+v", cfg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("dispatch not called")
	}
}
