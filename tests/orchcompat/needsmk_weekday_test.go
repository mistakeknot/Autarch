package orchcompat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Build an isolated clock-injected binary without adding a runtime clock override
// to the production command. Both filing constructors keep their normal behavior.
var clockedAutarch = sync.OnceValues(func() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	listed, err := exec.CommandContext(ctx, "go", "list", "-f", "{{.Dir}}", "github.com/mistakeknot/autarch/cmd/autarch").Output()
	if err != nil {
		return "", err
	}
	sourcePath := filepath.Join(strings.TrimSpace(string(listed)), "needsmk.go")
	source, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(shared.root, "clocked")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	// The overlay extends the command in the test build only; CardFiler.Now is
	// the same clock injection used by the in-process policy tests.
	source = append(source, []byte(`
func init() {
 now := func() time.Time {
  at, err := time.Parse(time.RFC3339, os.Getenv("ORCHCOMPAT_CLOCK"))
  if err != nil { panic(err) }
  return at
 }
 fileFactory := newCardFiler
 newCardFiler = func() *homeask.CardFiler { f := fileFactory(); f.Now = now; return f }
 decideFactory := newFiler
 newFiler = func() *homeask.CardFiler { f := decideFactory(); f.Now = now; return f }
}
`)...)
	replacement := filepath.Join(dir, "needsmk.go")
	if err := os.WriteFile(replacement, source, 0600); err != nil {
		return "", err
	}
	overlay, err := json.Marshal(map[string]any{"Replace": map[string]string{sourcePath: replacement}})
	if err != nil {
		return "", err
	}
	manifest := filepath.Join(dir, "overlay.json")
	if err := os.WriteFile(manifest, overlay, 0600); err != nil {
		return "", err
	}
	bin := filepath.Join(dir, "autarch")
	build := exec.CommandContext(ctx, "go", "build", "-race", "-p", "4", "-overlay", manifest, "-o", bin, "github.com/mistakeknot/autarch/cmd/autarch")
	if out, err := build.CombinedOutput(); err != nil {
		return "", fmt.Errorf("build clocked command: %w\n%s", err, out)
	}
	return bin, nil
})

func TestOrdinaryFileProcessWithFixedClock(t *testing.T) {
	bin, err := clockedAutarch()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, at string
		held     bool
	}{
		{"Thursday noon", "2026-11-19T12:00:00-08:00", false},
		{"Thursday before noon", "2026-11-19T11:59:59-08:00", true},
		{"Monday midnight", "2026-11-16T00:00:00-08:00", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			e.command = bin
			e.vars["ORCHCOMPAT_CLOCK"] = tc.at
			ask := e.writeFile("ask.json", askJSON(nil))
			got := e.autarch("needs-mk", "file", "--project", "P1", "--title", "Choose", "--ask-file", ask)
			code := 0
			if tc.held {
				code = 6
			}
			wantCode(t, got, code)
			var result struct {
				Queued  bool   `json:"queued"`
				Card    string `json:"card"`
				CardKey string `json:"card_key"`
				Request string `json:"request"`
			}
			if err := json.Unmarshal([]byte(got.Stdout), &result); err != nil {
				t.Fatalf("stdout is not one JSON result: %q: %v", got.Stdout, err)
			}
			if result.Request == "" || result.Queued != tc.held {
				t.Fatalf("wrong queue result: %s", got.Stdout)
			}
			if tc.held {
				if result.Card != "" || result.CardKey != "" || len(e.st().Tasks) != 0 {
					t.Fatalf("held request claimed a filed card: %s", got.Stdout)
				}
			} else if result.Card == "" || result.CardKey == "" || len(e.st().Tasks) != 1 {
				t.Fatalf("ordinary delivery needs a card_key: %s", got.Stdout)
			}
		})
	}
}
