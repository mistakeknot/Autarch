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

func TestGenerateEpicsFallsBackOnError(t *testing.T) {
	gen := &fakeGenerator{Err: errors.New("boom")}
	out, err := GenerateEpics(gen, Input{Summary: "summary"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Epics) == 0 {
		t.Fatalf("expected fallback epics")
	}
}
