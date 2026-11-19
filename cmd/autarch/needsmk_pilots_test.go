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
)

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
	if e := f.SetPilot("daycap", true); e != nil {
		t.Fatal(e)
	}
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
