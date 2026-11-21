package homeask

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseNotesOwnerRemediation(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	bb.threads = map[string]string{"thr_old": "archived"}
	f := bb.filer()
	r := testReq()
	r.Owner = "thr_old"
	r.NonBlocking = true
	if _, e := f.FileCard(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	report, _ := f.ReleaseDetailed(context.Background())
	if !strings.Contains(report.OwnerAttention[r.Key], "cancel-held "+r.Key) || strings.Contains(report.OwnerAttention[r.Key], "assign a live owner") {
		t.Fatal(report.OwnerAttention)
	}
}
func TestReleaseNotesRecoveredOwnerNotFailed(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	bb.threads = map[string]string{"thr_old": "live"}
	f := bb.filer()
	if e := f.SetPilot("linkcheck", true); e != nil {
		t.Fatal(e)
	}
	r := testReq()
	r.Owner = "thr_old"
	first, e := f.FileCard(context.Background(), r)
	if e != nil {
		t.Fatal(e)
	}
	s, p, done, e := f.openState()
	if e != nil {
		t.Fatal(e)
	}
	v := s[r.Key]
	v.Held = true
	v.Attempted = true
	s[r.Key] = v
	if e = saveFilingState(p, s); e != nil {
		t.Fatal(e)
	}
	done()
	bb.threads["thr_old"] = "archived"
	report, e := f.ReleaseDetailed(context.Background())
	if e != nil || len(report.Failures) != 0 || len(report.Groups["other"]) != 1 || report.Groups["other"][0].ID != first.ID || report.OwnerAttention[r.Key] == "" {
		t.Fatalf("recovered card listed as failure or lost attention: %+v %v", report, e)
	}
	d, _ := f.stateDir()
	b, e := os.ReadFile(filepath.Join(d, "pilots-metrics.jsonl"))
	if e != nil || !bytes.Contains(b, []byte(`"event":"released"`)) {
		t.Fatal("released event missing", string(b), e)
	}
}
func TestReleaseNotesDataClassification(t *testing.T) {
	for _, tc := range []struct {
		err  error
		data bool
	}{{fmt.Errorf("request: %w", ErrInvalid), true}, {errors.Join(errNoCardWritten, ErrInvalid), true}, {errors.Join(ErrInvalid, os.ErrPermission), false}, {errors.Join(ErrInvalid, ErrOutcomeUnknown), false}, {os.ErrPermission, false}, {errNoCardWritten, false}} {
		if got := (&ReleaseEntryErrors{Errors: []error{tc.err}}).DataOnly(); got != tc.data {
			t.Fatalf("%v classified %v want %v", tc.err, got, tc.data)
		}
	}
}
func TestReleaseNotesPersistedForFooter(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	bb.threads = map[string]string{"thr_old": "archived"}
	f := bb.filer()
	r := testReq()
	r.Owner = "thr_old"
	r.NonBlocking = true
	if _, e := f.FileCard(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	if _, e := f.ReleaseDetailed(context.Background()); e == nil {
		t.Fatal("missing data diagnostic")
	}
	var footer bytes.Buffer
	if e := f.PrintReleaseOwnerAttention(&footer); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(footer.String(), r.Key) || !strings.Contains(footer.String(), "thr_old") {
		t.Fatal("release notice did not reach footer", footer.String())
	}
	if e := f.CancelHeld(r.Key); e != nil {
		t.Fatal(e)
	}
	footer.Reset()
	if e := f.PrintReleaseOwnerAttention(&footer); e != nil || footer.Len() != 0 {
		t.Fatal("cancelled request remains flagged", footer.String(), e)
	}
}
func TestReleaseNotesRetiredOwnerRemediation(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	bb.threads = map[string]string{"thr_old": "archived"}
	f := bb.filer()
	r := testReq()
	r.Owner = "thr_old"
	r.NonBlocking = true
	if _, e := f.FileCard(context.Background(), r); e != nil {
		t.Fatal(e)
	}
	s, p, done, e := f.openState()
	if e != nil {
		t.Fatal(e)
	}
	v := s[r.Key]
	v.Attempted = true
	s[r.Key] = v
	if e = saveFilingState(p, s); e != nil {
		t.Fatal(e)
	}
	done()
	report, _ := f.ReleaseDetailed(context.Background())
	text := report.OwnerAttention[r.Key]
	if !strings.Contains(text, "request retired") || strings.Contains(text, "cancel-held") {
		t.Fatal("retired remediation is invalid", text)
	}
}
