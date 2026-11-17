package views

import (
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/autarch/agent"
	"github.com/mistakeknot/autarch/internal/tui"
	"github.com/mistakeknot/autarch/pkg/agenttargets"
	"github.com/mistakeknot/autarch/pkg/autarch"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
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

func TestGurgehForwardsCodingAgent(t *testing.T) {
	v := NewGurgehView(nil, &tui.GurgehConfig{})
	defer v.onboarding.cancel()
	selected := &agent.Agent{Type: agent.TypeCodex, ProjectRoot: t.TempDir()}
	v.SetCodingAgent(selected)
	if v.onboarding.codingAgent != selected {
		t.Fatal("Gurgeh did not forward coding agent")
	}
	v.SetCodingAgent(nil)
	if v.onboarding.codingAgent != nil {
		t.Fatal("Gurgeh did not clear coding agent")
	}
}

func TestGenerationCommandsSnapshotAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("AUTARCH_DISPATCH_AGENT", "")
	t.Setenv("AUTARCH_DISPATCH_BACKEND", "")
	for _, kind := range []string{"suggestions", "epics", "tasks"} {
		t.Run(kind, func(t *testing.T) {
			selected := &agent.Agent{Type: "snapshot-agent", ProjectRoot: t.TempDir()}
			v := NewGurgehOnboardingView(tui.GurgehConfig{CodingAgent: selected})
			defer v.cancel()
			var cmd tea.Cmd
			switch kind {
			case "suggestions":
				cmd = v.generateSuggestions()
			case "epics":
				cmd = v.generateEpicsWithAgent(tui.SpecAcceptedMsg{})
			case "tasks":
				cmd = v.generateTasksWithAgent()
			}
			selected.Type = "changed-agent"
			selected.ProjectRoot = t.TempDir()
			v.SetCodingAgent(nil)
			var err error
			if kind == "suggestions" {
				err = cmd().(tui.SuggestionsReadyMsg).Error
			} else {
				batch := cmd().(tea.BatchMsg)
				err = batch[1]().(tui.GenerationErrorMsg).Error
			}
			if err == nil || !strings.Contains(err.Error(), "snapshot-agent execution failed") {
				t.Fatalf("command did not retain agent snapshot: %v", err)
			}
		})
	}
}

func TestScanProgressWithoutPreferenceNamesCodingAgent(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	t.Setenv("AUTARCH_DISPATCH_AGENT", "")
	v := NewGurgehOnboardingView(tui.GurgehConfig{})
	defer v.cancel()
	done := make(chan struct{})
	v.explore = func(context.Context, string, agenttargets.DispatchConfig) (map[string]any, string, error) {
		defer close(done)
		return map[string]any{}, "", nil
	}
	msg := v.scanCodebase(t.TempDir())().(scanProgressWithContinuation)
	if msg.Details != "Running coding agent..." {
		t.Fatalf("empty preference label: %q", msg.Details)
	}
	<-done
}

func TestCrossTabAgentSelection(t *testing.T) {
	if os.Getenv("CROSS_TAB_CHILD") != "1" {
		dir := t.TempDir()
		for _, name := range []string{"claude", "codex"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\necho fixture-version\n"), 0700); err != nil {
				t.Fatal(err)
			}
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestCrossTabAgentSelection$")
		cmd.Env = append(os.Environ(), "CROSS_TAB_CHILD=1", "PATH="+dir, "HOME="+t.TempDir(), "XDG_CONFIG_HOME="+t.TempDir(), "AUTARCH_DISPATCH_AGENT=", "AUTARCH_DISPATCH_BACKEND=")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		return
	}
	t.Chdir(t.TempDir())
	for _, late := range []bool{false, true} {
		t.Run(fmt.Sprint("createdAfterPick=", late), func(t *testing.T) {
			app := tui.NewUnifiedApp(nil)
			var gurgeh *GurgehView
			var kickoff *KickoffView
			factory := func(*autarch.Client) []tui.View {
				if late {
					app.Update(pkgtui.AgentSelectedMsg{Name: "codex"})
				}
				kickoff = NewKickoffView()
				gurgeh = NewGurgehView(nil, &tui.GurgehConfig{CreateKickoffView: func() tui.View { return kickoff }})
				return []tui.View{NewKickoffView(), gurgeh}
			}
			app.SetDashboardViewFactory(factory)
			app.Init()
			if !late {
				app.Update(pkgtui.AgentSelectedMsg{Name: "codex"})
			}

			defer gurgeh.onboarding.cancel()
			app.Update(tea.KeyMsg{Type: tea.KeyCtrlRight})
			if gurgeh.onboarding.selectedAgent != "codex" || kickoff.resolvedScanAgent(".") != "codex" || !strings.Contains(gurgeh.onboarding.agentSelector.View(), "codex") || !strings.Contains(gurgeh.onboarding.agentSelector.View(), "explicit agent selection") {
				t.Fatal("Gurgeh label did not receive pick from other tab")
			}
			configs := make(chan agenttargets.DispatchConfig, 1)
			gurgeh.onboarding.explore = func(_ context.Context, _ string, cfg agenttargets.DispatchConfig) (map[string]any, string, error) {
				configs <- cfg
				return map[string]any{}, "", nil
			}
			gurgeh.onboarding.scanCodebase(".")
			cfg := <-configs
			if cfg.PreferredAgent != "codex" || cfg.PreferredAgentSource != "explicit agent selection" {
				t.Fatalf("dispatch received %+v", cfg)
			}
		})
	}
}
