package scheduler

import (
	"path/filepath"
	"testing"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

func newTestDispatcher(t *testing.T) *Dispatcher {
	t.Helper()
	db, err := mycroft.OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return NewDispatcher(db, nil, "test")
}

func TestLogShadow(t *testing.T) {
	d := newTestDispatcher(t)
	bead := mycroft.BeadView{ID: "Demarch-42", Title: "Test"}

	if err := d.LogShadow("grey-area", bead, "high priority match"); err != nil {
		t.Fatalf("LogShadow: %v", err)
	}

	entries, err := d.ShadowDigest(10)
	if err != nil {
		t.Fatalf("ShadowDigest: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 shadow entry, got %d", len(entries))
	}
	if entries[0].Agent != "grey-area" {
		t.Errorf("agent: got %q", entries[0].Agent)
	}
	if entries[0].Action != "shadow_suggest" {
		t.Errorf("action: got %q", entries[0].Action)
	}
}

func TestLogSuggestionApproveReject(t *testing.T) {
	d := newTestDispatcher(t)
	bead := mycroft.BeadView{ID: "Demarch-99"}

	d.LogSuggestion("grey-area", bead, "")
	d.LogApproval("grey-area", "Demarch-99")
	d.LogRejection("mistake-not", "Demarch-100", "too risky")

	entries, err := d.DispatchHistory(10)
	if err != nil {
		t.Fatalf("DispatchHistory: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Most recent first.
	if entries[0].Reason != "too risky" {
		t.Errorf("rejection reason: got %q", entries[0].Reason)
	}
}

func TestLogPauseResume(t *testing.T) {
	d := newTestDispatcher(t)

	d.LogPause()
	d.LogResume()

	entries, err := d.DispatchHistory(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
}

func TestOverridePatterns(t *testing.T) {
	d := newTestDispatcher(t)

	// Log some rejections with reasons.
	d.LogRejection("grey-area", "b1", "too complex")
	d.LogRejection("grey-area", "b2", "too complex")
	d.LogRejection("grey-area", "b3", "wrong agent")

	patterns, err := d.OverridePatterns()
	if err != nil {
		t.Fatal(err)
	}
	if len(patterns) != 2 {
		t.Fatalf("expected 2 patterns, got %d", len(patterns))
	}
	// Most frequent first.
	if patterns[0].Reason != "too complex" || patterns[0].Count != 2 {
		t.Errorf("top pattern: got %q x%d", patterns[0].Reason, patterns[0].Count)
	}
}

func TestContextJSON(t *testing.T) {
	json := ContextJSON(mycroft.T1, 1.5, "high priority")
	if json == "" {
		t.Error("empty context JSON")
	}
}
