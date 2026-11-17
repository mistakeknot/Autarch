package views

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/autarch/agent"
	"github.com/mistakeknot/autarch/internal/tui"
	"github.com/mistakeknot/autarch/pkg/agenttargets"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	kickoff := NewKickoffView()
	kickoff.SetCodingAgent(picked)
	if got := kickoff.resolvedScanAgent(t.TempDir()); got != "codex" {
		t.Fatalf("picked scan label = %q", got)
	}
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

func TestKickoffScanUsesResolvedAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("AUTARCH_DISPATCH_AGENT", "codex")
	t.Chdir(t.TempDir())
	for _, entry := range []string{"slash", "keyboard"} {
		t.Run(entry, func(t *testing.T) {
			v := NewKickoffView()
			v.Update(tea.WindowSizeMsg{Width: 160, Height: 50})
			v.focusInput = true
			v.SetScanCodebaseCallback(func(string) tea.Cmd { return func() tea.Msg { return nil } })
			if entry == "slash" {
				v.HandleSlashCommand("scan", nil)
			} else {
				v.Update(tea.KeyMsg{Type: tea.KeyF4})
			}
			if v.scanAgentName != "codex" {
				t.Fatalf("scan agent = %q", v.scanAgentName)
			}
			if !strings.Contains(v.View(), "codex") {
				t.Fatalf("resolved agent missing from scan screen: %q", v.View())
			}
		})
	}
}
