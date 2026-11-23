package homeask

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeSecretMetadata() map[string]any {
	return map[string]any{"thread": "thr_23456789ab", "names": []string{"FAKE_TEST_TOKEN"}, "path": "/fake-test/credentials.env", "project": "fake-test-project"}
}

func TestSecretCardRoundTripAndIdentity(t *testing.T) {
	r := testReq()
	base, err := RequestIdentity(r)
	if err != nil {
		t.Fatal(err)
	}
	r.Secret = fakeSecretMetadata()
	desc, identity, err := r.Description()
	if err != nil {
		t.Fatal(err)
	}
	c, err := ParseCard(desc)
	if err != nil || c.Secret == nil || !secretEqual(c.Secret, r.Secret) {
		t.Fatalf("metadata roundtrip failed: %v", err)
	}
	if identity == base {
		t.Fatal("secret metadata missing from request identity")
	}
	if _, err = ParseCard(desc + "\n```home-secret\n{}\n```\n"); err == nil {
		t.Fatal("duplicate block accepted")
	}
	for _, key := range []string{"value", "values", "token", "password", "extra", "Thread"} {
		f := newFake(t, registryOnCreate)
		r.Secret = fakeSecretMetadata()
		r.Secret[key] = "fake-test-value-not-a-secret"
		_, err = f.filer().FileCard(context.Background(), r)
		if err == nil || len(f.tasks) != 0 {
			t.Fatalf("invalid metadata reached tasks: %s", key)
		}
		if strings.Contains(err.Error(), "fake-test-value-not-a-secret") {
			t.Fatal("error echoed rejected content")
		}
	}
}

func TestSecretFileCardDelivery(t *testing.T) {
	for _, mode := range []string{"non-blocking", "hold", "boundary", "owner"} {
		t.Run(mode, func(t *testing.T) {
			bb := newFake(t, registryOnCreate)
			f := bb.filer()
			r := testReq()
			r.Secret = fakeSecretMetadata()
			r.Thread = "thr_23456789ab"
			dir := filepath.Join(t.TempDir(), "state")
			f.StateDir = dir
			hold := time.Date(2026, 11, 16, 8, 0, 0, 0, time.UTC)
			if mode == "non-blocking" {
				r.NonBlocking = true
			}
			if mode == "hold" {
				f.Now = func() time.Time { return hold }
			}
			if mode == "boundary" {
				n := 0
				f.Now = func() time.Time {
					n++
					if n == 1 {
						return hold.Add(-time.Nanosecond)
					}
					return hold
				}
			}
			if mode == "owner" {
				r.Thread = "thr_abcdefghij"
				r.Owner = "thr_23456789ab"
				bb.threads = map[string]string{r.Owner: "live"}
				r.Urgent = true
			}
			res, err := f.FileCard(context.Background(), r)
			if mode == "owner" {
				if err != nil || res.Queued {
					t.Fatalf("owner-bound card failed: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalid) || res.Queued || len(bb.tasks) != 0 {
				t.Fatalf("unsafe secret card was not refused: result=%+v err=%v", res, err)
			}
			if mode != "boundary" {
				if _, e := os.Stat(dir); !os.IsNotExist(e) {
					t.Fatal("refusal wrote state")
				}
			}
			// Boundary refusal may take locks, but must never reserve a held card.
			if mode == "boundary" {
				matches, _ := filepath.Glob(filepath.Join(dir, "*.json"))
				for _, path := range matches {
					raw, _ := os.ReadFile(path)
					if strings.Contains(string(raw), `"held":true`) {
						t.Fatal("secret reserved in hold")
					}
				}
			}
		})
	}
}
