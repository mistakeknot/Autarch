package brief

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

// PATH contains only our fixtures, so a regression cannot launch a real agent.
func fakeDecomposeCLIs(t *testing.T, payload string) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("AUTARCH_DISPATCH_AGENT", "")
	t.Setenv("AUTARCH_DISPATCH_BACKEND", "")
	t.Setenv("BRIEF_PAYLOAD", payload)
	envelope, _ := json.Marshal(map[string]any{"type": "result", "result": payload})
	t.Setenv("BRIEF_ENVELOPE", string(envelope))
	scripts := map[string]string{
		"claude": "#!/bin/sh\nprintf '%s\\n' \"$BRIEF_ENVELOPE\"\n",
		"codex":  "#!/bin/sh\nwhile [ $# -gt 0 ]; do\n if [ \"$1\" = -o ]; then shift; printf '%s' \"$BRIEF_PAYLOAD\" > \"$1\"; exit 0; fi\n shift\ndone\nexit 1\n",
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestDecomposeEnvPreference(t *testing.T) {
	dir := fakeDecomposeCLIs(t, `{"briefs":[{"title":"Codex brief"}]}`)
	// A Claude launch fails deterministically rather than reaching a real CLI.
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte("#!/bin/sh\nexit 42\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("AUTARCH_DISPATCH_AGENT", "codex")
	briefs, err := Decompose(context.Background(), &specs.Spec{}, dir)
	if err != nil || len(briefs) != 1 {
		t.Fatalf("briefs=%v err=%v", briefs, err)
	}
}

func TestDecomposeClaudeEnvelope(t *testing.T) {
	dir := fakeDecomposeCLIs(t, `{"briefs":[{"title":"Claude brief"}]}`)
	briefs, err := Decompose(context.Background(), &specs.Spec{}, dir)
	if err != nil || len(briefs) != 1 {
		t.Fatalf("briefs=%v err=%v", briefs, err)
	}
}

func TestDecomposeRejectsEmptyResult(t *testing.T) {
	for _, payload := range []string{"", "nonsense", "{}", `{"briefs":[]}`} {
		t.Run(payload, func(t *testing.T) {
			dir := fakeDecomposeCLIs(t, payload)
			if briefs, err := Decompose(context.Background(), &specs.Spec{}, dir); err == nil {
				t.Fatalf("accepted %q: %v", payload, briefs)
			}
		})
	}
}

func TestDecomposePreferences(t *testing.T) {
	for _, tc := range []struct{ name, project, env, explicit, want string }{
		{"default", "", "", "", "claude"},
		{"project", "codex", "", "", "codex"},
		{"environment", "claude", "codex", "", "codex"},
		{"explicit", "codex", "codex", "claude", "claude"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fakeDecomposeCLIs(t, "```json\n{\"briefs\":[{\"title\":\"Task\"}]}\n```")
			marker := filepath.Join(dir, "chosen")
			t.Setenv("BRIEF_MARKER", marker)
			for _, name := range []string{"claude", "codex"} {
				path := filepath.Join(dir, name)
				script, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				script = []byte(strings.Replace(string(script), "#!/bin/sh\n", "#!/bin/sh\nprintf '"+name+"' > \"$BRIEF_MARKER\"\n", 1))
				if err := os.WriteFile(path, script, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Mkdir(filepath.Join(dir, ".gurgeh"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, ".gurgeh", "agents.toml"), []byte("[dispatch]\npreferred_agent = \""+tc.project+"\"\n"), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("AUTARCH_DISPATCH_AGENT", tc.env)
			briefs, err := Decompose(context.Background(), &specs.Spec{}, dir, tc.explicit)
			if err != nil || len(briefs) != 1 {
				t.Fatalf("briefs=%v err=%v", briefs, err)
			}
			got, err := os.ReadFile(marker)
			if err != nil || string(got) != tc.want {
				t.Fatalf("chosen=%s err=%v", got, err)
			}
		})
	}
}

func TestDecomposeRejectsUnsupportedDispatch(t *testing.T) {
	for _, tc := range []struct{ variable, value, want string }{
		{"AUTARCH_DISPATCH_BACKEND", "api", `unsupported dispatch backend "api": only subscription-cli is implemented`},
		{"AUTARCH_DISPATCH_BACKEND", "other", `unsupported dispatch backend "other": only subscription-cli is implemented`},
		{"AUTARCH_DISPATCH_AGENT", "other", `unsupported dispatch agent "other" from AUTARCH_DISPATCH_AGENT (supported: claude, codex)`},
	} {
		t.Run(tc.value+tc.variable, func(t *testing.T) {
			dir := fakeDecomposeCLIs(t, `{"briefs":[{"title":"Task"}]}`)
			t.Setenv(tc.variable, tc.value)
			_, err := Decompose(context.Background(), &specs.Spec{}, dir)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error=%v, want %s", err, tc.want)
			}
		})
	}
}
