package main

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/mistakeknot/autarch/internal/homeask"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestNeedsMkUpdateWithoutProjectExitsThreeWithoutWrites(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_x")
	s := &stubBB{}
	p := writeAsk(t)
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var ask map[string]any
	if err := json.Unmarshal(raw, &ask); err != nil {
		t.Fatal(err)
	}
	ask["ask_key"] = "missing-project-update"
	write := func() {
		t.Helper()
		raw, err := json.Marshal(ask)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	args := []string{"file", "--project", "P1", "--title", "Ship?", "--ask-file", p}
	if _, err := runNeedsMk(t, s, args...); err != nil {
		t.Fatal(err)
	}
	original, calls := s.desc, len(s.calls)
	ask["question"] = "Changed question?"
	write()
	old := newCardFiler
	t.Cleanup(func() { newCardFiler = old })
	newCardFiler = func() *homeask.CardFiler {
		return &homeask.CardFiler{LockDir: t.TempDir(), Run: func(ctx context.Context, env []string, args ...string) homeask.BBResult {
			if strings.Join(args[:min(len(args), 3)], " ") == "home get --card" {
				return homeask.BBResult{Stdout: []byte(`{"task_id":"T1","project_id":"","state":"open","card_key":"P1-1"}`)}
			}
			return s.run(ctx, env, args...)
		}}
	}
	root := &cobra.Command{Use: "autarch", SilenceErrors: true}
	root.AddCommand(needsMkCmd())
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(append([]string{"needs-mk"}, args...))
	err = root.Execute()
	if exitCode(err) != 3 || out.Len() != 0 {
		t.Errorf("exit=%d err=%v out=%q", exitCode(err), err, out.String())
	}
	if s.desc != original || s.creates != 1 || len(s.updated) != 0 {
		t.Errorf("description changed=%v creates=%d updates=%d", s.desc != original, s.creates, len(s.updated))
	}
	for _, call := range s.calls[calls:] {
		if len(call) > 1 && call[0] == "tasks" && (call[1] == "create" || call[1] == "update" || call[1] == "comment") {
			t.Errorf("unexpected write: %v", call)
		}
	}
}

func TestNeedsMkRefilingUpdatesInPlace(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_x")
	s := &stubBB{}
	p := writeAsk(t)
	raw, _ := os.ReadFile(p)
	var ask map[string]any
	json.Unmarshal(raw, &ask)
	ask["ask_key"] = "same-ask"
	raw, _ = json.Marshal(ask)
	os.WriteFile(p, raw, 0600)
	args := []string{"file", "--project", "P1", "--title", "Ship?", "--ask-file", p}
	first, err := runNeedsMk(t, s, args...)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first, `"replay":false`) || s.creates != 1 {
		t.Fatal(first)
	}
	original, _ := homeask.ParseCard(s.desc)
	ask["question"] = "A refreshed question?"
	raw, _ = json.Marshal(ask)
	os.WriteFile(p, raw, 0600)
	out, err := runNeedsMk(t, s, args...)
	if err != nil || !strings.Contains(out, `"updated":true`) || s.creates != 1 || len(s.updated) != 1 {
		t.Fatalf("out=%s err=%v creates=%d updates=%d", out, err, s.creates, len(s.updated))
	}
	back, _ := homeask.ParseCard(s.desc)
	if back.Question != "A refreshed question?" || back.Request != original.Request || s.thread != "thr_x" {
		t.Fatalf("card=%+v", back)
	}
}
