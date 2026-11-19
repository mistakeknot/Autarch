package homeask

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestLabelLookupProjectNotFoundClassification(t *testing.T) {
	for _, tc := range []struct {
		name    string
		result  BBResult
		missing bool
	}{
		{"missing", BBResult{Code: 1, Stderr: []byte("project not found: missing-project")}, true},
		{"prefixed", BBResult{Code: 1, Stderr: []byte("Error: project not found: missing-project")}, true},
		{"outage", BBResult{Code: 1, Stderr: []byte("connection refused")}, false},
		{"timeout", BBResult{Code: 1, TimedOut: true, Stderr: []byte("project not found: missing-project")}, false},
		{"exec failure", BBResult{Code: 1, Err: errors.New("unavailable"), Stderr: []byte("project not found: missing-project")}, false},
		{"different project", BBResult{Code: 1, Stderr: []byte("project not found: other-project")}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &CardFiler{Run: func(context.Context, []string, ...string) BBResult { return tc.result }}
			err := f.ensureLabel(context.Background(), "missing-project")
			if errors.Is(err, ErrProjectNotFound) != tc.missing || errors.Is(err, ErrHomeDown) == tc.missing {
				t.Fatalf("classification: %v", err)
			}
			if tc.missing && (!strings.Contains(err.Error(), "missing-project") || !strings.Contains(err.Error(), "no longer exists in bb")) {
				t.Fatal(err)
			}
			entries := &ReleaseEntryErrors{Errors: []error{err}}
			if entries.DataOnly() != tc.missing {
				t.Fatalf("request fault=%v: %v", entries.DataOnly(), err)
			}
			if (&ReleaseEntryErrors{Errors: []error{errors.Join(err, ErrHomeDown)}}).DataOnly() {
				t.Fatal("mixed outage suppressed")
			}
		})
	}
}
