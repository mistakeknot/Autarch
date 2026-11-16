package main

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/homeask"
)

func TestNeedsMkHeldExitIsDistinct(t *testing.T) {
	for _, mode := range []string{"weekday", "non-blocking"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("BB_THREAD_ID", "thr_x")
			s := &stubBB{}
			f := &homeask.CardFiler{Run: s.run, StateDir: t.TempDir(), LockDir: t.TempDir(), Now: filingTestNow}
			if mode == "weekday" {
				f.Now = func() time.Time { return time.Date(2026, 11, 16, 18, 0, 0, 0, time.UTC) }
			}
			old, check := newCardFiler, checkFilingPreconditions
			t.Cleanup(func() { newCardFiler, checkFilingPreconditions = old, check })
			newCardFiler = func() *homeask.CardFiler { return f }
			checkFilingPreconditions = func(string, string) error { return nil }
			cmd := needsMkFileCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&bytes.Buffer{})
			args := []string{"--project", "P1", "--title", "Choose", "--ask-file", writeAsk(t), "--owner-thread", "thr_x"}
			if mode == "non-blocking" {
				args = append(args, "--non-blocking")
			}
			cmd.SetArgs(args)
			err := cmd.Execute()
			if exitCode(err) != 6 {
				t.Fatalf("queued request exited %d: %v; stdout=%s", exitCode(err), err, out.String())
			}
			var result homeask.CardResult
			if err := json.Unmarshal(out.Bytes(), &result); err != nil || !result.Queued || result.Key == "" || result.CardKey != "" || result.ID != "" || s.creates != 0 {
				t.Fatalf("invalid queued output: %s; parse=%v creates=%d", out.String(), err, s.creates)
			}
		})
	}
}
