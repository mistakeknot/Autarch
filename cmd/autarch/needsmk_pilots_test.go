package main

import (
	"bytes"
	"encoding/json"
	"github.com/mistakeknot/autarch/internal/homeask"
	"github.com/spf13/cobra"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPilotCLIRetiredDaycap(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent", true: "present"}[existing], func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "pilots.json")
			raw := []byte(`{"daycap":{"enabled":true,"live_since":"2026-01-01T00:00:00Z","until":"2026-01-15T00:00:00Z"}}`)
			if existing {
				if err := os.WriteFile(path, raw, 0600); err != nil {
					t.Fatal(err)
				}
			}
			old := newCardFiler
			t.Cleanup(func() { newCardFiler = old })
			f := &homeask.CardFiler{StateDir: dir, LockDir: t.TempDir()}
			newCardFiler = func() *homeask.CardFiler { return f }
			for _, action := range []string{"start", "stop"} {
				c := needsMkPilotsCmd()
				var out bytes.Buffer
				c.SetOut(&out)
				c.SetArgs([]string{action, "daycap"})
				if err := c.Execute(); err != nil {
					t.Fatal(err)
				}
				for _, want := range []string{"retired", "inert", "card limits were removed"} {
					if !strings.Contains(out.String(), want) {
						t.Fatalf("missing %q: %s", want, &out)
					}
				}
				got, err := os.ReadFile(path)
				if existing && (err != nil || !bytes.Equal(got, raw)) {
					t.Fatalf("existing state changed: %s, %v", got, err)
				}
				if !existing && !os.IsNotExist(err) {
					t.Fatalf("created state: %s, %v", got, err)
				}
			}
			if existing {
				c := needsMkPilotsCmd()
				var out bytes.Buffer
				c.SetOut(&out)
				c.SetArgs([]string{})
				if err := c.Execute(); err != nil {
					t.Fatal(err)
				}
				var view map[string]struct {
					Retired bool `json:"retired"`
				}
				if err := json.Unmarshal(out.Bytes(), &view); err != nil || !view["daycap"].Retired {
					t.Fatalf("listing: %s, %v", &out, err)
				}
				got, err := os.ReadFile(path)
				if err != nil || !bytes.Equal(got, raw) {
					t.Fatalf("listing changed state: %s, %v", got, err)
				}
			}
		})
	}
}

func TestPilotCLILivePilots(t *testing.T) {
	for _, name := range []string{"coldread", "linkcheck"} {
		t.Run(name, func(t *testing.T) {
			f := &homeask.CardFiler{StateDir: t.TempDir(), LockDir: t.TempDir(), Now: filingTestNow}
			old := newCardFiler
			t.Cleanup(func() { newCardFiler = old })
			newCardFiler = func() *homeask.CardFiler { return f }
			for _, action := range []string{"start", "stop"} {
				c := needsMkPilotsCmd()
				var out bytes.Buffer
				c.SetOut(&out)
				c.SetArgs([]string{action, name})
				if err := c.Execute(); err != nil {
					t.Fatal(err)
				}
				if out.String() != name+" "+action+"\n" {
					t.Fatal(out.String())
				}
				p, err := f.Pilots()
				if err != nil {
					t.Fatal(err)
				}
				v := p[name]
				if v.Enabled != (action == "start") || !v.LiveSince.Equal(filingTestNow()) || !v.Until.Equal(filingTestNow().Add(14*24*time.Hour)) {
					t.Fatalf("pilot: %+v", v)
				}
				c = needsMkPilotsCmd()
				out.Reset()
				c.SetOut(&out)
				c.SetArgs([]string{})
				if err := c.Execute(); err != nil {
					t.Fatal(err)
				}
				want, _ := json.Marshal(p)
				if strings.TrimSpace(out.String()) != string(want) {
					t.Fatalf("live listing changed: %s", &out)
				}
			}
		})
	}
}

func TestPilotCLIPrintsDailyCapReason(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_x")
	t.Setenv("AUTARCH_CARD_DAILY_CAP", "1")
	s := &stubBB{}
	old := newCardFiler
	defer func() { newCardFiler = old }()
	oldCheck := checkFilingPreconditions
	defer func() { checkFilingPreconditions = oldCheck }()
	checkFilingPreconditions = func(string, string) error { return nil }
	f := &homeask.CardFiler{Now: filingTestNow, Run: s.run, StateDir: t.TempDir(), LockDir: t.TempDir()}
	newCardFiler = func() *homeask.CardFiler { return f }
	ask := writeAsk(t)
	execute := func(key string) (string, string, error) {
		c := &cobra.Command{Use: "autarch", SilenceErrors: true}
		c.AddCommand(needsMkCmd())
		var out, errout bytes.Buffer
		c.SetOut(&out)
		c.SetErr(&errout)
		c.SetArgs(append([]string{"needs-mk"}, filingOwnerArgs([]string{"file", "--project", "P1", "--title", "Choose", "--ask-file", ask, "--request", key})...))
		e := c.Execute()
		return out.String(), errout.String(), e
	}
	if _, _, e := execute("first"); e != nil {
		t.Fatal(e)
	}
	out, warn, e := execute("second")
	if e != nil || strings.Contains(out, `"queued":true`) || warn != "" {
		t.Fatalf("out=%s warning=%s err=%v", out, warn, e)
	}
	raw, _ := os.ReadFile(filepath.Join(f.StateDir, "filings.json"))
	var state map[string]any
	if json.Unmarshal(raw, &state) != nil {
		t.Fatal("bad state")
	}
}
