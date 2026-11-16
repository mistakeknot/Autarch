package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/mistakeknot/autarch/internal/homeask"
)

func TestDecideSameRequestEscalates(t *testing.T) {
	for _, field := range []string{"urgent", "keep_list"} {
		t.Run(field, func(t *testing.T) {
			s := &stubBB{}
			f := &homeask.CardFiler{StateDir: t.TempDir(), LockDir: t.TempDir(), TasksProject: "P1", Now: func() time.Time { return time.Date(2026, 11, 16, 18, 0, 0, 0, time.UTC) }, Run: s.run}
			old := newFiler
			newFiler = func() *homeask.CardFiler { return f }
			t.Cleanup(func() { newFiler = old })
			t.Setenv("BB_THREAD_ID", "")
			raw := askJSON("thr-x")
			if _, err := runDecide(t, raw, "file"); exitCode(err) != 6 {
				t.Fatal(err)
			}
			var ask map[string]any
			if err := json.Unmarshal([]byte(raw), &ask); err != nil {
				t.Fatal(err)
			}
			if field == "urgent" {
				ask[field] = true
			} else {
				ask[field] = "spend"
			}
			b, _ := json.Marshal(ask)
			out, err := runDecide(t, string(b), "file")
			if err != nil || out != "T1" || s.creates != 1 {
				t.Fatalf("same-key escalation: %s %v", out, err)
			}
			if replay, err := runDecide(t, raw, "file"); err != nil || replay != "T1" {
				t.Fatal(replay, err)
			}
			budget, err := f.BudgetStatus("autarch")
			if err != nil || budget.Ordinary != 0 || budget.Exempt != 1 {
				t.Fatal(budget, err)
			}
		})
	}
}

func TestNeedsMkSameRequestEscalates(t *testing.T) {
	for _, flag := range []string{"--urgent", "--keep-list"} {
		t.Run(flag, func(t *testing.T) {
			s := &stubBB{}
			lock := t.TempDir()
			state := t.TempDir()
			old := newCardFiler
			var filer *homeask.CardFiler
			newCardFiler = func() *homeask.CardFiler {
				filer = &homeask.CardFiler{StateDir: state, LockDir: lock, Run: s.run, Now: func() time.Time { return time.Date(2026, 11, 16, 18, 0, 0, 0, time.UTC) }}
				return filer
			}
			t.Cleanup(func() { newCardFiler = old })
			t.Setenv("BB_THREAD_ID", "thr_x")
			oldCheck := checkFilingPreconditions
			checkFilingPreconditions = func(string, string) error { return nil }
			t.Cleanup(func() { checkFilingPreconditions = oldCheck })
			run := func(args ...string) (string, error) {
				root := &cobra.Command{Use: "autarch", SilenceErrors: true}
				root.AddCommand(needsMkCmd())
				var out bytes.Buffer
				root.SetOut(&out)
				root.SetErr(&bytes.Buffer{})
				root.SetArgs(append([]string{"needs-mk"}, args...))
				err := root.Execute()
				return strings.TrimSpace(out.String()), err
			}
			p := writeAsk(t)
			base := []string{"file", "--project", "P1", "--title", "Ship?", "--ask-file", p, "--owner-thread", "thr_owner"}
			if _, err := run(base...); exitCode(err) != 6 {
				t.Fatal(err)
			}
			args := append(append([]string{}, base...), flag)
			if flag == "--keep-list" {
				args = append(args, "spend")
			}

			for _, command := range [][]string{args, base} {
				out, err := run(command...)
				if err != nil || s.creates != 1 {
					t.Fatal(out, err)
				}
				var result homeask.CardResult
				if err := json.Unmarshal([]byte(out), &result); err != nil || result.CardKey == "" {
					t.Fatalf("missing card key: %s %v", out, err)
				}
			}
			budget, err := filer.BudgetStatus("autarch")
			if err != nil || budget.Ordinary != 0 || budget.Exempt != 1 {
				t.Fatal(budget, err)
			}
		})
	}
}
