package scheduler

import (
	"github.com/mistakeknot/autarch/internal/mycroft/tier"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

type checkSpawner struct{ calls int }

func (s *checkSpawner) Spawn(string, mycroft.BeadView, string) (string, error) {
	s.calls++
	return "session", nil
}
func (s *checkSpawner) Kill(string) error { return nil }

func TestPreDispatchNonzero(t *testing.T) {
	dir := t.TempDir()
	command := filepath.Join(dir, "check")
	if err := os.WriteFile(command, []byte("#!/bin/sh\nexit 7\n"), 0700); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(config, []byte("pre_dispatch_check:\n  command: ["+command+"]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := mycroft.LoadConfig(config)
	if err != nil {
		t.Fatal(err)
	}
	o := newTestOrchestrator(t, mycroft.T3)
	o.cfg = cfg
	s := &checkSpawner{}
	o.spawner = s
	o.dispatchToAgent(mycroft.AgentView{Name: "agent"}, mycroft.BeadView{ID: "b1"})
	if s.calls != 0 {
		t.Fatalf("Spawn called %d times after check exits 7; want 0", s.calls)
	}
}

func TestPreDispatchCases(t *testing.T) {
	for _, tc := range []struct {
		name, script, reason string
		spawn                bool
		missing              bool
	}{
		{name: "absent", spawn: true},
		{name: "empty", spawn: true},
		{name: "success", script: `[ "$1" = 'literal $HOME; two words' ]`, spawn: true},
		{name: "nonzero", script: "exit 7", reason: "exit status 7"},
		{name: "timeout", script: "exec sleep 5", reason: "timeout after 100ms"},
		{name: "missing", missing: true, reason: "no such file or directory"},
		{name: "env_and_cwd", script: `[ "$MYCROFT_BEAD_ID" = "b1" ] && [ "$MYCROFT_AGENT" = "agent" ] && [ "$PWD" = "$CHECK_EXPECT_CWD" ]`, spawn: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := newTestOrchestrator(t, mycroft.T3)
			s := &checkSpawner{}
			o.spawner = s
			dir := t.TempDir()
			if tc.script != "" || tc.missing {
				command := filepath.Join(dir, "check")
				if !tc.missing {
					if err := os.WriteFile(command, []byte("#!/bin/sh\n"+tc.script+"\n"), 0700); err != nil {
						t.Fatal(err)
					}
				}
				o.cfg.PreDispatchCheck = mycroft.PreDispatchCheck{Command: []string{command, "literal $HOME; two words"}, Timeout: mycroft.Duration(100 * time.Millisecond)}
			}
			if tc.name == "empty" {
				o.cfg.PreDispatchCheck.Command = []string{}
			}
			cwd, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("CHECK_EXPECT_CWD", cwd)
			// Existing values must be replaced, rather than winning over check context.
			t.Setenv("MYCROFT_BEAD_ID", "wrong")
			t.Setenv("MYCROFT_AGENT", "wrong")
			started := time.Now()
			o.dispatchToAgent(mycroft.AgentView{Name: "agent"}, mycroft.BeadView{ID: "b1"})
			if elapsed := time.Since(started); tc.name == "timeout" && elapsed > time.Second {
				t.Errorf("timeout took %s; want <1s", elapsed)
			}
			want := 0
			if tc.spawn {
				want = 1
			}
			if s.calls != want {
				t.Fatalf("Spawn calls=%d, want %d", s.calls, want)
			}
			d := NewDispatcher(o.db, nil, "test")
			history, err := d.DispatchHistory(10)
			if err != nil {
				t.Fatal(err)
			}
			if len(history) != 1 {
				t.Fatalf("history=%v", history)
			}
			e := history[0]
			if tc.spawn {
				if e.Action != "auto_dispatch" || e.Outcome != "accepted" {
					t.Fatalf("dispatch=%+v", e)
				}
				return
			}
			if e.Action != "pre_dispatch_skip" || e.Outcome != "" || !strings.Contains(e.Reason, "pre_dispatch_check check:") || !strings.Contains(e.Reason, tc.reason) {
				t.Fatalf("skip=%+v", e)
			}
			records := []tier.DispatchRecord{{Action: e.Action, Outcome: e.Outcome}}
			triggers := mycroft.DefaultConfig().DemotionTriggers
			triggers.MinSampleSize = 1
			triggers.ConsecutiveFailureLimit = 1
			demote, why, evidence := tier.ShouldDemote(mycroft.T3, records, triggers)
			if demote || evidence.FailureRate != 0 {
				t.Fatalf("skip demotes: %v %s %+v", demote, why, evidence)
			}
			patterns, err := d.OverridePatterns()
			if err != nil || len(patterns) != 0 {
				t.Fatalf("override patterns=%v err=%v", patterns, err)
			}
			shadows, err := d.ShadowDigest(10)
			if err != nil || len(shadows) != 0 {
				t.Fatalf("shadow digest=%v err=%v", shadows, err)
			}
		})
	}
}

func TestPreDispatchReasonBounded(t *testing.T) {
	o := newTestOrchestrator(t, mycroft.T3)
	s := &checkSpawner{}
	o.spawner = s
	o.cfg.PreDispatchCheck.Command = []string{filepath.Join(t.TempDir(), strings.Repeat("x", 300))}
	o.dispatchToAgent(mycroft.AgentView{Name: "agent"}, mycroft.BeadView{ID: "b1"})
	entries, err := NewDispatcher(o.db, nil, "test").DispatchHistory(10)
	if err != nil || len(entries) != 1 {
		t.Fatalf("history=%v err=%v", entries, err)
	}
	if len([]rune(entries[0].Reason)) > 240 || !strings.Contains(entries[0].Reason, "...") || !strings.Contains(entries[0].Reason, "start error:") {
		t.Fatalf("reason length=%d", len(entries[0].Reason))
	}
	if s.calls != 0 {
		t.Fatal("spawned")
	}
}
