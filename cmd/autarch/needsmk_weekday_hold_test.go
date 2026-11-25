package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/homeask"
	"github.com/spf13/cobra"
)

func TestNeedsMkFileCheckWeekdayHoldOnAndOff(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_x")
	at := "2026-11-25T10:00:00-08:00"
	s := &stubBB{}
	t.Setenv(homeask.EnvCardWeekdayHold, "on")
	out, err := runNeedsMk(t, s, "file", "--project", "P1", "--title", "Choose", "--ask-file", writeAsk(t), "--check", "--at", at)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if json.Unmarshal([]byte(out), &got) != nil {
		t.Fatalf("out=%q", out)
	}
	if got["checked"] != true || got["weekday_hold"] != true || got["weekday_hold_reason"] != "weekday-window" {
		t.Fatalf("on+wednesday: %+v", got)
	}
	if len(writeCalls(s)) != 0 {
		t.Fatalf("check wrote: %v", writeCalls(s))
	}

	t.Setenv(homeask.EnvCardWeekdayHold, "off")
	out, err = runNeedsMk(t, s, "file", "--project", "P1", "--title", "Choose", "--ask-file", writeAsk(t), "--check", "--at", at)
	if err != nil {
		t.Fatal(err)
	}
	got = map[string]any{}
	if json.Unmarshal([]byte(out), &got) != nil {
		t.Fatalf("out=%q", out)
	}
	if got["checked"] != true || got["weekday_hold"] != false || got["weekday_hold_reason"] != "" {
		t.Fatalf("off+wednesday: %+v", got)
	}
	if len(writeCalls(s)) != 0 {
		t.Fatalf("check wrote: %v", writeCalls(s))
	}
}

func TestNeedsMkFileAtRequiresCheck(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_x")
	s := &stubBB{}
	_, err := runNeedsMk(t, s, "file", "--project", "P1", "--title", "Choose", "--ask-file", writeAsk(t), "--at", "2026-11-25T10:00:00-08:00")
	if err == nil || !strings.Contains(err.Error(), "--at") {
		t.Fatalf("expected --at usage error, got %v", err)
	}
}

func TestNeedsMkWeekdayHoldCheckConfigAndNewCardFiler(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "needsmk.env")
	if err := os.WriteFile(path, []byte("AUTARCH_CARD_WEEKDAY_HOLD=off\n"), 0644); err != nil {
		t.Fatal(err)
	}
	root := &cobra.Command{Use: "autarch", SilenceErrors: true}
	root.AddCommand(needsMkCmd())
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"needs-mk", "weekday-hold", "--check-config", path})
	if err := root.Execute(); err != nil {
		t.Fatalf("check-config: %v out=%s", err, out.String())
	}
	var got map[string]any
	if json.Unmarshal([]byte(out.String()), &got) != nil || got["ok"] != true || got["value"] != "off" {
		t.Fatalf("out=%s", out.String())
	}

	bad := filepath.Join(dir, "bad.env")
	if err := os.WriteFile(bad, []byte("AUTARCH_CARD_WEEKDAY_HOLD=maybe\n"), 0644); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	root = &cobra.Command{Use: "autarch", SilenceErrors: true}
	root.AddCommand(needsMkCmd())
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"needs-mk", "weekday-hold", "--check-config", bad})
	if err := root.Execute(); err == nil {
		t.Fatal("expected invalid config to fail")
	}

	// Real newCardFiler (not the runNeedsMk stub) must load the setting.
	t.Setenv(homeask.EnvCardWeekdayHold, "off")
	f := newCardFiler()
	if err := f.LoadWeekdayHold(); err != nil {
		t.Fatal(err)
	}
	s := f.WeekdayHoldSetting()
	if !s.Loaded() || s.Value() != "off" || s.Source != "env" {
		t.Fatalf("newCardFiler setting: loaded=%v value=%q source=%q", s.Loaded(), s.Value(), s.Source)
	}
	held, reason, _, err := f.WeekdayHoldDecision(t.Context(), homeask.CardRequest{Project: "GROK", Title: "Q", Type: "other"}, time.Date(2026, 11, 25, 10, 0, 0, 0, time.FixedZone("PST", -8*3600)))
	if err != nil || held || reason != "" {
		t.Fatalf("off should not hold: held=%v reason=%q err=%v", held, reason, err)
	}
}
