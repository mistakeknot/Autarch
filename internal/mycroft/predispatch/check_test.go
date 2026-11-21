package predispatch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

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
	if err := Run(context.Background(), cfg.PreDispatchCheck, "b1", "agent"); err == nil || !strings.Contains(err.Error(), "exit status 7") {
		t.Fatalf("check must reject exit 7: %v", err)
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
			cfg := mycroft.DefaultConfig()
			dir := t.TempDir()
			if tc.script != "" || tc.missing {
				command := filepath.Join(dir, "check")
				if !tc.missing {
					if err := os.WriteFile(command, []byte("#!/bin/sh\n"+tc.script+"\n"), 0700); err != nil {
						t.Fatal(err)
					}
				}
				cfg.PreDispatchCheck = mycroft.PreDispatchCheck{Command: []string{command, "literal $HOME; two words"}, Timeout: mycroft.Duration(100 * time.Millisecond)}
			}
			if tc.name == "empty" {
				cfg.PreDispatchCheck.Command = []string{}
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
			err = Run(context.Background(), cfg.PreDispatchCheck, "b1", "agent")
			if elapsed := time.Since(started); tc.name == "timeout" && elapsed > time.Second {
				t.Errorf("timeout took %s; want <1s", elapsed)
			}
			if tc.spawn {
				if err != nil {
					t.Fatalf("check rejected assignment: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "pre_dispatch_check check:") || !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("check error=%v; want %s", err, tc.reason)
			}

		})
	}
}

func TestPreDispatchReasonBounded(t *testing.T) {
	check := mycroft.PreDispatchCheck{Command: []string{filepath.Join(t.TempDir(), strings.Repeat("x", 300))}}
	err := Run(context.Background(), check, "b1", "agent")
	if err == nil {
		t.Fatal("check succeeded")
	}
	if len([]rune(err.Error())) > 240 || !strings.Contains(err.Error(), "...") || !strings.Contains(err.Error(), "start error:") {
		t.Fatalf("reason=%s", err)
	}
}
