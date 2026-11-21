package views

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgtui "github.com/mistakeknot/autarch/pkg/tui"

	"github.com/mistakeknot/autarch/pkg/intercore"
)

// fakeIC stands in for the ic binary: it answers `health` and records every
// other invocation, so a test can prove which commands the TUI ran.
func fakeIC(t *testing.T) (*intercore.Client, string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do [ \"$a\" = health ] && exit 0; done\n" +
		"echo \"$@\" >> " + log + "\n" +
		"echo '[]'\n"
	bin := filepath.Join(dir, "ic")
	if err := os.WriteFile(bin, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	c, err := intercore.New(intercore.WithBinPath(bin))
	if err != nil {
		t.Fatalf("intercore.New: %v", err)
	}
	return c, log
}

func collectText(t *testing.T, ch <-chan pkgtui.StreamMsg) string {
	t.Helper()
	var b strings.Builder
	for m := range ch {
		if d, ok := m.(pkgtui.TextDelta); ok {
			b.WriteString(d.Text)
		}
	}
	return b.String()
}

// `ic dispatch spawn` needs --prompt-file, which /dispatch spawn never had, so
// the command could not succeed. It must say so instead of calling ic.
func TestDispatchSpawnCommandExplainsItIsUnavailableAndRunsNothing(t *testing.T) {
	ic, log := fakeIC(t)
	r := NewSprintCommandRouter(nil, ic, nil)

	ch, err := r.HandleMessage(context.Background(), "/dispatch spawn task build the thing")
	if err != nil {
		t.Fatal(err)
	}
	got := collectText(t, ch)
	if !strings.Contains(got, "ic dispatch spawn --prompt-file=") {
		t.Errorf("reply %q should point at the working ic command", got)
	}
	if b, err := os.ReadFile(log); err == nil {
		t.Errorf("ic was invoked: %s", b)
	}
}

func TestDispatchListStillWorks(t *testing.T) {
	ic, log := fakeIC(t)
	r := NewSprintCommandRouter(nil, ic, nil)

	ch, err := r.HandleMessage(context.Background(), "/dispatch list")
	if err != nil {
		t.Fatal(err)
	}
	if got := collectText(t, ch); !strings.Contains(got, "No dispatches") {
		t.Errorf("reply = %q", got)
	}
	if b, _ := os.ReadFile(log); !strings.Contains(string(b), "dispatch list") {
		t.Errorf("ic calls = %q, want a dispatch list", b)
	}
}
