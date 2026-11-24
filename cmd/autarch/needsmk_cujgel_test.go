package main

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/mistakeknot/autarch/internal/homeask"
)

func cujgelFile(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "link.json")
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func cliCujgelExample(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("../../internal/homeask/testdata/cujgel/citadel-link.json")
	if err != nil {
		t.Fatal(err)
	}
	r := strings.NewReplacer("option-a", "a", "option-b", "b")
	return r.Replace(string(b))
}

func TestNeedsMkCujgel(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_fixture")
	good := cliCujgelExample(t)
	for _, tc := range []struct{ name, body, linked, field string }{
		{"accept", good, "proj_fixture", ""},
		{"mismatch", good, "proj_other", "project"},
		{"unlinked", good, "", "project"},
		{"duplicate", strings.Replace(good, `"project":`, `"project":"proj_other","project":`, 1), "proj_fixture", "project"},
		{"unknown", strings.TrimSuffix(strings.TrimSpace(good), "}") + `,"url":"fixture"}`, "proj_fixture", "url"},
		{"question", strings.Replace(good, `"question_id": "comparison_fixture"`, `"question_id":"other"`, 1), "proj_fixture", "question_id"},
		{"option", strings.Replace(good, `"option_id": "b"`, `"option_id":"c"`, 1), "proj_fixture", "option_id"},
		{"variant", strings.Replace(good, "variant_b", "variant_a", 1), "proj_fixture", "variant_id"},
		{"size", good + strings.Repeat(" ", 16384), "proj_fixture", "16 KiB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := filepath.Join(t.TempDir(), "state")
			t.Setenv(homeask.EnvCardStateDir, state)
			s := &stubBB{linkedBBProject: tc.linked}
			_, err := runNeedsMk(t, s, "file", "--project", "P1", "--title", "Compare variants", "--ask-file", writeAsk(t), "--cujgel", cujgelFile(t, tc.body))
			if tc.field == "" {
				if err != nil || s.creates != 1 || !strings.Contains(s.desc, "```cujgel/v1\n") {
					t.Fatalf("linked filing failed: %v", err)
				}
				return
			}
			if exitCode(err) != 2 || !strings.Contains(err.Error(), tc.field) || strings.Contains(err.Error(), "unknown flag") {
				t.Fatalf("want exit 2 naming %s, got %v", tc.field, err)
			}
			if s.creates != 0 || len(s.updated) != 0 {
				t.Fatal("refusal wrote a card")
			}
			if _, e := os.Stat(state); !os.IsNotExist(e) {
				t.Fatalf("refusal wrote state: %v", e)
			}
		})
	}
}

func TestNeedsMkCujgelFileKinds(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_fixture")
	for _, kind := range []string{"symlink", "directory", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "link")
			switch kind {
			case "symlink":
				if err := os.Symlink(cujgelFile(t, cliCujgelExample(t)), p); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Mkdir(p, 0700); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := syscall.Mkfifo(p, 0600); err != nil {
					t.Fatal(err)
				}
			}
			s := &stubBB{linkedBBProject: "proj_fixture"}
			_, err := runNeedsMk(t, s, "file", "--project", "P1", "--title", "Compare variants", "--ask-file", writeAsk(t), "--cujgel", p)
			if exitCode(err) != 2 || !strings.Contains(err.Error(), "regular file") || s.creates != 0 {
				t.Fatalf("%s not refused clearly: %v", kind, err)
			}
		})
	}
}

func TestNeedsMkCujgelSecretCombination(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_23456789ab")
	s := &stubBB{linkedBBProject: "proj_fixture"}
	_, err := runNeedsMk(t, s, "file", "--project", "P1", "--title", "Compare variants", "--ask-file", writeAsk(t), "--cujgel", cujgelFile(t, cliCujgelExample(t)), "--secret", cujgelFile(t, `{"thread":"thr_23456789ab","names":["FAKE_TEST_TOKEN"],"path":"/fixture/credentials.env","project":"fixture"}`))
	if exitCode(err) != 2 || !strings.Contains(err.Error(), "cannot be combined with --secret") || s.creates != 0 {
		t.Fatalf("ambiguous combination not refused: %v", err)
	}
}

func TestNeedsMkCujgelSupersedesPreservesBlock(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_fixture")
	t.Setenv(homeask.EnvCardStateDir, t.TempDir())
	s := &stubBB{linkedBBProject: "proj_fixture"}
	args := []string{"file", "--project", "P1", "--title", "Compare variants", "--ask-file", writeAsk(t), "--cujgel", cujgelFile(t, cliCujgelExample(t))}
	if _, err := runNeedsMk(t, s, args...); err != nil {
		t.Fatal(err)
	}
	start := strings.Index(s.desc, "```cujgel/v1\n")
	if start < 0 {
		t.Fatal("no stored link")
	}
	block := s.desc[start:]
	args[4] = "Compare updated variants"
	if _, err := runNeedsMk(t, s, append(args, "--supersedes", "T1")...); err != nil {
		t.Fatal(err)
	}
	if s.creates != 1 || len(s.updated) != 1 || !strings.HasSuffix(s.desc, block) {
		t.Fatal("supersedes changed the link or created a second card")
	}
}
