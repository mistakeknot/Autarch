//go:build unix

package homerun

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

// T2.6 — the audit sequence for success, failure, timeout and crash, and its append-only mirror.

var reGrammar = regexp.MustCompile(`^AUTHORIZED( STARTED (ENDED|UNCERTAIN)| ABORTED_BEFORE_START)?$`)

func kindsOf(recs []Record, attempt string) string {
	var ks []string
	for _, r := range recs {
		if r.Attempt == attempt {
			ks = append(ks, string(r.Kind))
		}
	}
	return strings.Join(ks, " ")
}

func startInfo(sha string) StartInfo {
	return StartInfo{Unit: "run-x.service", InvocationID: strings.Repeat("e", 32), CASSHA: sha}
}

func TestAuditSequences(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	cases := []struct {
		name string
		run  func(s *Store, a *Attempt) error
		want string
	}{
		{"success", func(s *Store, a *Attempt) error {
			if err := recStarted(s, ctx, a.ID, startInfo(sha1Hex), now); err != nil {
				return err
			}
			return s.RecordEnded(ctx, a.ID, EndInfo{Result: ResultSuccess, OutputSHA256: sha2Hex, OutputBytes: 5, Duration: time.Second}, now)
		}, "AUTHORIZED STARTED ENDED"},
		{"failure", func(s *Store, a *Attempt) error {
			if err := recStarted(s, ctx, a.ID, startInfo(sha1Hex), now); err != nil {
				return err
			}
			return s.RecordEnded(ctx, a.ID, EndInfo{Result: ResultFailed, ExitStatus: 2, OutputSHA256: sha2Hex, OutputBytes: 5, Duration: time.Second}, now)
		}, "AUTHORIZED STARTED ENDED"},
		{"timeout", func(s *Store, a *Attempt) error {
			if err := recStarted(s, ctx, a.ID, startInfo(sha1Hex), now); err != nil {
				return err
			}
			return s.RecordEnded(ctx, a.ID, EndInfo{Result: ResultTimeout, ExitStatus: 143, OutputSHA256: sha2Hex, OutputBytes: 0, Duration: 10 * time.Minute}, now)
		}, "AUTHORIZED STARTED ENDED"},
		{"aborted before start", func(s *Store, a *Attempt) error { return s.RecordAborted(ctx, a.ID, "launcher refused", now) }, "AUTHORIZED ABORTED_BEFORE_START"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := newStore(t)
			mp := filepath.Join(t.TempDir(), "audit.jsonl")
			if err := s.SetMirror(ctx, mp); err != nil {
				t.Fatal(err)
			}
			a, _, err := consumeFor(t, s, now, "AUT-1", OpCheck)
			if err != nil {
				t.Fatal(err)
			}
			if err := tc.run(s, a); err != nil {
				t.Fatal(err)
			}
			recs, _ := s.Records(ctx)
			if got := kindsOf(recs, a.ID); got != tc.want || !reGrammar.MatchString(got) {
				t.Fatalf("sequence %q, want %q", got, tc.want)
			}
			if err := s.CheckAuditChain(ctx, true); err != nil {
				t.Fatal(err)
			}
			mir, err := ReadMirror(mp)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(mir, recs) {
				t.Fatalf("mirror differs from the database:\n%+v\n%+v", mir, recs)
			}
			// every line names the attempt, envelope, signer and nonce
			for _, r := range mir {
				if r.Attempt != a.ID || len(r.EnvelopeDigest) != 64 || r.Signer.Principal != principal || r.Nonce == "" || r.CardID != "AUT-1" {
					t.Errorf("record lacks identity: %+v", r)
				}
			}
		})
	}
}

func TestBadTransitions(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	s, _ := newStore(t)
	a, _, err := consumeFor(t, s, now, "AUT-1", OpCheck)
	if err != nil {
		t.Fatal(err)
	}
	end := EndInfo{Result: ResultSuccess, OutputSHA256: sha2Hex, Duration: time.Second}
	if err := s.RecordEnded(ctx, a.ID, end, now); !errors.Is(err, ErrBadTransition) {
		t.Errorf("ENDED without STARTED: %v", err)
	}
	if err := recStarted(s, ctx, a.ID, startInfo(sha2Hex), now); err == nil {
		t.Error("STARTED with a CAS sha that differs from the signed script_sha256")
	}
	if err := recStarted(s, ctx, a.ID, StartInfo{Unit: "u", InvocationID: "short", CASSHA: sha1Hex}, now); err == nil {
		t.Error("STARTED with a malformed InvocationID")
	}
	if err := recStarted(s, ctx, a.ID, startInfo(sha1Hex), now); err != nil {
		t.Fatal(err)
	}
	if err := recStarted(s, ctx, a.ID, startInfo(sha1Hex), now); !errors.Is(err, ErrBadTransition) {
		t.Errorf("STARTED twice: %v", err)
	}
	if err := s.RecordAborted(ctx, a.ID, "late", now); !errors.Is(err, ErrBadTransition) {
		t.Errorf("ABORTED after STARTED: %v", err)
	}
	if err := s.RecordEnded(ctx, a.ID, EndInfo{Result: ResultSuccess, ExitStatus: 1, OutputSHA256: sha2Hex}, now); err == nil {
		t.Error("success with a non-zero exit status accepted")
	}
	if err := s.RecordEnded(ctx, a.ID, end, now); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordEnded(ctx, a.ID, end, now); !errors.Is(err, ErrBadTransition) {
		t.Errorf("ENDED twice: %v", err)
	}
	if err := recStarted(s, ctx, "01ARZ3NDEKTSV4RRFFQ69G5FAV", startInfo(sha1Hex), now); !errors.Is(err, ErrBadTransition) {
		t.Errorf("unknown attempt: %v", err)
	}
}

func TestMirrorCatchesUpAndIsAppendOnly(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	s, _ := newStore(t)
	a, _, err := consumeFor(t, s, now, "AUT-1", OpCheck) // before the mirror exists
	if err != nil {
		t.Fatal(err)
	}
	mp := filepath.Join(t.TempDir(), "audit.jsonl")
	if err := s.SetMirror(ctx, mp); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordAborted(ctx, a.ID, "x", now); err != nil {
		t.Fatal(err)
	}
	mir, err := ReadMirror(mp)
	if err != nil || len(mir) != 2 {
		t.Fatalf("mirror holds %d records, err %v", len(mir), err)
	}
	// a second sync adds nothing
	if err := s.syncMirror(ctx); err != nil {
		t.Fatal(err)
	}
	if mir, _ = ReadMirror(mp); len(mir) != 2 {
		t.Fatalf("a repeated sync duplicated records: %d", len(mir))
	}
	fi, _ := os.Stat(mp)
	if fi.Mode().Perm()&0o007 != 0 {
		t.Errorf("mirror is world-accessible: %v", fi.Mode())
	}
}

func TestMirrorTornLineRepaired(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	s, _ := newStore(t)
	mp := filepath.Join(t.TempDir(), "audit.jsonl")
	if err := s.SetMirror(ctx, mp); err != nil {
		t.Fatal(err)
	}
	a, _, err := consumeFor(t, s, now, "AUT-1", OpCheck)
	if err != nil {
		t.Fatal(err)
	}
	// simulate a crash in the middle of the next line's write
	f, _ := os.OpenFile(mp, os.O_WRONLY|os.O_APPEND, 0)
	f.WriteString(`{"seq":2,"attempt":"` + a.ID + `","kind":"AB`)
	f.Close()
	if _, err := ReadMirror(mp); err == nil {
		t.Fatal("ReadMirror skipped a torn line instead of reporting it")
	}
	if err := s.RecordAborted(ctx, a.ID, "x", now); err != nil {
		t.Fatal(err)
	}
	mir, err := ReadMirror(mp)
	if err != nil {
		t.Fatalf("mirror not repaired: %v", err)
	}
	recs, _ := s.Records(ctx)
	if !reflect.DeepEqual(mir, recs) {
		t.Fatalf("mirror differs after repair:\n%+v\n%+v", mir, recs)
	}
}

func TestMirrorFailsClosed(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	s, _ := newStore(t)
	d := t.TempDir()
	real := filepath.Join(d, "real.jsonl")
	os.WriteFile(real, nil, 0o640)
	link := filepath.Join(d, "audit.jsonl")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMirror(ctx, link); !errors.Is(err, ErrMirror) {
		t.Fatalf("a symlinked mirror path was followed: %v", err)
	}
	// with a broken mirror the state change is committed but the call reports the failure
	a, _, err := consumeFor(t, s, now, "AUT-1", OpCheck)
	if !errors.Is(err, ErrMirror) {
		t.Fatalf("consume with a broken mirror: %v", err)
	}
	if a != nil {
		t.Error("an attempt was handed out although the audit mirror failed")
	}
	if b, _ := os.ReadFile(real); len(b) != 0 {
		t.Error("the symlink target was written")
	}
	// the database kept the record; once the mirror is healthy it catches up
	recs, _ := s.Records(ctx)
	if len(recs) != 1 {
		t.Fatalf("database holds %d records", len(recs))
	}
	good := filepath.Join(d, "good.jsonl")
	if err := s.SetMirror(ctx, good); err != nil {
		t.Fatal(err)
	}
	if mir, _ := ReadMirror(good); len(mir) != 1 {
		t.Fatalf("mirror did not catch up: %d", len(mir))
	}
}
