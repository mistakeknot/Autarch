package homeask

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingProjectLocateRefusalAdvice(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	f.StateDir = t.TempDir()
	state, path, done, err := f.openState()
	if err != nil {
		t.Fatal(err)
	}
	state["sample"] = filingEntry{Attempted: true, Held: true}
	err = saveFilingState(path, state)
	done()
	if err != nil {
		t.Fatal(err)
	}
	f.Run = func(context.Context, []string, ...string) BBResult {
		return okRes(map[string]any{"status": "legacy", "decision_id": "sample"})
	}
	_, err = f.reconcileMissingProject(context.Background(), "sample", "thr_owner", ErrProjectNotFound)
	if !errors.Is(err, ErrInvalid) || errors.Is(err, ErrHomeDown) {
		t.Fatalf("expected non-outage refusal: %v", err)
	}
	if !strings.Contains(err.Error(), "ask the vizier to check whether the card exists") || strings.Contains(err.Error(), "once bb is reachable") {
		t.Fatalf("misleading advice: %v", err)
	}
}

func TestProjectAttentionStateReadFailureAdvice(t *testing.T) {
	f := newFake(t, registryOnCreate).filer()
	f.StateDir = t.TempDir()
	if err := os.WriteFile(filepath.Join(f.StateDir, "filings.json"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	attention, err := f.recordProjectOwnerAttention("sample", "thr_owner", ErrProjectNotFound)
	if err == nil {
		t.Fatal("expected state read error")
	}
	if !strings.Contains(attention.Error(), "could not read held state") || strings.Contains(attention.remediation, "no longer exists") {
		t.Fatalf("misleading advice: %v", attention)
	}
}
