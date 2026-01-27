package initflow

import (
	"context"
	"errors"
	"testing"
)

type fakeGenerator struct {
	Err error
}

func (f *fakeGenerator) Generate(_ context.Context, _ Input) (Result, error) {
	return Result{}, f.Err
}

func TestGenerateEpicsReturnsErrorOnFailure(t *testing.T) {
	gen := &fakeGenerator{Err: errors.New("boom")}
	_, err := GenerateEpics(gen, Input{Summary: "summary"})
	if err == nil {
		t.Fatal("expected error from failed generator")
	}
}

func TestFallbackEpicsReturnsStarterBacklog(t *testing.T) {
	epics := FallbackEpics()
	if len(epics) == 0 {
		t.Fatal("expected fallback epics")
	}
	if epics[0].ID != "EPIC-001" {
		t.Errorf("expected EPIC-001, got %s", epics[0].ID)
	}
	if len(epics[0].Stories) == 0 {
		t.Error("expected at least one story in fallback")
	}
}
