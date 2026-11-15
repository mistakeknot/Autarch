package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNeedsMkSecretMetadata(t *testing.T) {
	t.Setenv("BB_THREAD_ID", "thr_23456789ab")
	const good = `{"thread":"thr_23456789ab","names":["FAKE_TEST_TOKEN"],"path":"/fake-test/credentials.env","project":"fake-test-project"}`
	for _, tc := range []struct {
		name, body string
		accept     bool
	}{
		{"accept", good, true},
		{"extra", strings.TrimSuffix(good, "}") + `,"extra":"fake-test-value-not-a-secret"}`, false},
		{"values", strings.TrimSuffix(good, "}") + `,"values":{"FAKE_TEST_TOKEN":"fake-test-value-not-a-secret"}}`, false},
		{"relative", strings.Replace(good, "/fake-test/credentials.env", "relative", 1), false},
		{"name", strings.Replace(good, "FAKE_TEST_TOKEN", "TOKEN=value", 1), false},
		{"thread", strings.Replace(good, "thr_23456789ab", "invalid", 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := filepath.Join(t.TempDir(), "metadata.json")
			if err := os.WriteFile(f, []byte(tc.body), 0600); err != nil {
				t.Fatal(err)
			}
			s := &stubBB{}
			_, err := runNeedsMk(t, s, "file", "--project", "P1", "--title", "Provide credential", "--ask-file", writeAsk(t), "--secret", f)
			if tc.accept {
				if err != nil {
					t.Fatalf("metadata refused: %v", err)
				}
				if !strings.Contains(s.desc, "```home-secret\n") {
					t.Fatal("missing home-secret block")
				}
			} else {
				if exitCode(err) != 2 {
					t.Fatalf("invalid metadata accepted: %v", err)
				}
				if s.creates != 0 {
					t.Fatal("invalid metadata reached tasks")
				}
			}
			if err != nil && strings.Contains(err.Error(), "fake-test-value-not-a-secret") {
				t.Fatal("value echoed in error")
			}
		})
	}
}
