package homeask

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestLegacyFileReturnsHeldOutcome(t *testing.T) {
	bb := newFake(t, registryOnCreate)
	f := bb.filer()
	f.Now = func() time.Time { return pacificTestTime(t, "2026-11-16 10:00:00") }
	ask := base()
	ask.Project, ask.ProjectRoot, ask.RequestID = "autarch", "/sample", "held-request"
	id, err := f.File(context.Background(), ask)
	var held *HeldError
	if !errors.As(err, &held) || held.RequestKey != ask.RequestID || held.Reason != "weekday-window" {
		t.Fatalf("expected typed held outcome, got %v", err)
	}
	if err == nil || id != "" || !strings.Contains(err.Error(), ask.RequestID) || !strings.Contains(err.Error(), "queued for the Thursday batch") {
		t.Fatalf("held request reported as filed: id=%q err=%v", id, err)
	}
	if bb.count("tasks", "create") != 0 {
		t.Fatal("held request created a card")
	}
}
