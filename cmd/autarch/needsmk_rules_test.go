package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"testing"
)

func TestNeedsMkVizierOwnership(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_x")
	for _, tc := range []struct {
		name    string
		vizier  bool
		flags   []string
		refused bool
	}{
		{"vizier needs owner", true, nil, true},
		{"vizier routes owner", true, []string{"--owner-thread", "thr_owner"}, false},
		{"ordinary routes owner", false, []string{"--owner-thread", "thr_owner"}, false},
		{"both empty refused", true, []string{"--vizier-owned", "--owner-thread", ""}, true},
		{"vizier own", true, []string{"--vizier-owned"}, false},
		{"ordinary unchanged", false, nil, false},
		{"ordinary cannot mark vizier", false, []string{"--vizier-owned"}, true},
		{"both refused", true, []string{"--vizier-owned", "--owner-thread", "thr_other"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &stubBB{vizier: tc.vizier}
			args := append([]string{"file", "--project", "P1", "--title", "Ship?", "--ask-file", writeAsk(t)}, tc.flags...)
			_, err := runNeedsMk(t, s, args...)
			if tc.refused {
				if err == nil || exitCode(err) != 2 || s.creates != 0 {
					t.Fatalf("err=%v creates=%d", err, s.creates)
				}
				if !strings.Contains(err.Error(), "--vizier-owned") {
					t.Fatal(err)
				}
			} else if err != nil || s.creates != 1 {
				t.Fatalf("err=%v creates=%d", err, s.creates)
			}
		})
	}
}

func TestNeedsMkRefusesNoticeOnly(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_x")
	for _, label := range []string{"read", "OK", "okay!", "A) Close.", "closed", "Got it!", "NOTED", "acknowledge", "acknowledged", "seen", "dismiss"} {
		t.Run(label, func(t *testing.T) {
			s := &stubBB{}
			p := writeAsk(t)
			raw, _ := os.ReadFile(p)
			var ask map[string]any
			json.Unmarshal(raw, &ask)
			ask["options"] = []any{map[string]any{"id": "one", "label": label, "kind": "ruling-only"}, map[string]any{"id": "two", "label": "B) Read!", "kind": "ruling-only"}}
			raw, _ = json.Marshal(ask)
			os.WriteFile(p, raw, 0600)
			_, err := runNeedsMk(t, s, "file", "--project", "P1", "--title", "Notice", "--ask-file", p)
			if exitCode(err) != 2 || s.creates != 0 || err == nil || !strings.Contains(err.Error(), "no decision") || !strings.Contains(err.Error(), "digest") {
				t.Fatalf("err=%v creates=%d", err, s.creates)
			}
		})
	}
}

func TestNeedsMkNoticeWithRealOptionIsUnchanged(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_x")
	s := &stubBB{}
	p := writeAsk(t)
	raw, _ := os.ReadFile(p)
	var ask map[string]any
	json.Unmarshal(raw, &ask)
	ask["options"] = []any{map[string]any{"id": "read", "label": "Read", "kind": "ruling-only"}, map[string]any{"id": "wait", "label": "Wait for review", "kind": "ruling-only"}}
	raw, _ = json.Marshal(ask)
	os.WriteFile(p, raw, 0600)
	_, err := runNeedsMk(t, s, "file", "--project", "P1", "--title", "Choose", "--ask-file", p)
	if err != nil || s.creates != 1 {
		t.Fatalf("err=%v creates=%d", err, s.creates)
	}
}

func TestNeedsMkNoticeMoveAndRootRunExempt(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_x")
	for _, kind := range []string{"move", "root-run"} {
		t.Run(kind, func(t *testing.T) {
			p := writeAsk(t)
			raw, _ := os.ReadFile(p)
			var ask map[string]any
			json.Unmarshal(raw, &ask)
			ask["options"] = []any{map[string]any{"id": "read", "label": "Read", "kind": "ruling-only"}, map[string]any{"id": "close", "label": "Close", "kind": "ruling-only"}}
			raw, _ = json.Marshal(ask)
			os.WriteFile(p, raw, 0600)
			args := []string{"file", "--project", "P1", "--title", "Action", "--ask-file", p}
			if kind == "move" {
				args = append(args, "--move", writeMove(t, prMoveJSON))
			} else {
				script := filepath.Join(t.TempDir(), "action.sh")
				os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0700)
				args = append(args, "--root-run", "script="+script+",timeout=30,set=deploy")
			}
			s := &stubBB{}
			_, err := runNeedsMk(t, s, args...)
			if err != nil || s.creates != 1 {
				t.Fatalf("err=%v creates=%d", err, s.creates)
			}
		})
	}
}
