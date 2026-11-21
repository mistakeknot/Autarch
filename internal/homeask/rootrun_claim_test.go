package homeask

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileVerifiesRootRunClaimBeforeHomeCalls(t *testing.T) {
	script := filepath.Join(t.TempDir(), "check.sh")
	body := []byte("#!/bin/sh\ntrue\n")
	if err := os.WriteFile(script, body, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	for _, mismatch := range []bool{true, false} {
		t.Run(fmt.Sprint(mismatch), func(t *testing.T) {
			bb := newFake(t, registryOnCreate)
			f := bb.filer()
			f.Now = func() time.Time { return pacificTestTime(t, "2026-11-16 10:00:00") }
			a := Ask{V: 1, Kind: "decide", Asker: "thread", Thread: "thr_a", Project: "autarch", ProjectRoot: "/sample", Question: "Proceed?", Options: []Option{{ID: "yes", Label: "Yes", Kind: "ruling-only"}, {ID: "no", Label: "No", Kind: "ruling-only"}}, RootRun: &RootRun{Script: script, SHA256: fmt.Sprintf("%x", sum), Timeout: 30, Set: "sample"}}
			if mismatch {
				a.RootRun.SHA256 = strings.Repeat("a", 64)
			}
			id, err := f.File(context.Background(), a)
			if mismatch {
				if !errors.Is(err, ErrInvalid) || len(bb.calls) != 0 || id != "" {
					t.Fatalf("unverified claim filed: %s %v calls=%d", id, err, len(bb.calls))
				}
			} else if err != nil || id == "" {
				t.Fatal(id, err)
			}
		})
	}
}
