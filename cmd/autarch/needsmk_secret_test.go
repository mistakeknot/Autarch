package main

import (
	"bytes"
	"github.com/mistakeknot/autarch/internal/homeask"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestNeedsMkSecretDelivery(t *testing.T) {
	for _, tc := range []struct {
		name                               string
		hold, nonBlocking, urgent, refused bool
	}{
		{"non-blocking", false, true, false, true},
		{"non-blocking-even-urgent", false, true, true, true},
		{"weekly-hold", true, false, false, true},
		{"explicit-bypass", true, false, true, false},
		{"outside-hold", false, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BB_THREAD_ID", "thr_23456789ab")
			secret := filepath.Join(t.TempDir(), "metadata.json")
			if err := os.WriteFile(secret, []byte(`{"thread":"thr_23456789ab","names":["FAKE_TEST_TOKEN"],"path":"/fake-test/credentials.env","project":"fake-test-project"}`), 0600); err != nil {
				t.Fatal(err)
			}
			stateDir := filepath.Join(t.TempDir(), "not-created")
			s := &stubBB{}
			f := &homeask.CardFiler{Run: s.run, StateDir: stateDir, LockDir: t.TempDir(), Now: filingTestNow}
			if tc.hold {
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
			args := []string{"--project", "P1", "--title", "Choose", "--ask-file", writeAsk(t), "--secret", secret, "--owner-thread", "thr_23456789ab"}
			if tc.nonBlocking {
				args = append(args, "--non-blocking")
			}
			if tc.urgent {
				args = append(args, "--urgent")
			}
			cmd.SetArgs(args)
			err := cmd.Execute()
			if tc.refused {
				if exitCode(err) != 2 {
					t.Fatalf("expected refusal exit 2, got %d: %v", exitCode(err), err)
				}
				if !strings.Contains(err.Error(), "secret request expires before the batch releases") {
					t.Fatalf("missing expiry explanation: %v", err)
				}
				if tc.hold && !strings.Contains(err.Error(), "--urgent") {
					t.Fatalf("missing explicit bypass: %v", err)
				}
				if len(s.calls) != 0 {
					t.Fatalf("refusal contacted services: %v", s.calls)
				}
				if _, e := os.Stat(stateDir); !os.IsNotExist(e) {
					t.Fatalf("refusal wrote filing state: %v", e)
				}
				if out.Len() != 0 {
					t.Fatal("refusal printed success output")
				}
			} else if err != nil || s.creates != 1 || !strings.Contains(s.desc, "```home-secret") {
				t.Fatalf("immediate secret card did not file: err=%v creates=%d", err, s.creates)
			}
		})
	}
}
