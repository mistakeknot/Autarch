package diff

import (
	"fmt"
	"testing"

	"github.com/mistakeknot/autarch/internal/gurgeh/specs"
)

func makeSpec(id string, nReqs int) *specs.Spec {
	reqs := make([]string, nReqs)
	for i := range reqs {
		reqs[i] = fmt.Sprintf("The system shall %s item %d efficiently", []string{"process", "validate", "transform", "store"}[i%4], i)
	}
	acceptance := make([]specs.AcceptanceCriterion, nReqs/2)
	for i := range acceptance {
		acceptance[i] = specs.AcceptanceCriterion{
			ID:          fmt.Sprintf("AC-%d", i),
			Description: fmt.Sprintf("Acceptance criterion %d passes", i),
		}
	}
	return &specs.Spec{
		ID:           id,
		Title:        "Benchmark Spec",
		Requirements: reqs,
		Acceptance:   acceptance,
	}
}

func BenchmarkDiffSpecs10(b *testing.B) {
	old := makeSpec("v1", 10)
	new := makeSpec("v2", 12) // 2 added
	// Modify some requirements
	new.Requirements[0] = "The system shall process item 0 with improved performance"
	new.Requirements[3] = "The system shall store item 3 with encryption"

	b.ResetTimer()
	for b.Loop() {
		_ = DiffSpecs(old, new)
	}
}

func BenchmarkDiffSpecs50(b *testing.B) {
	old := makeSpec("v1", 50)
	new := makeSpec("v2", 55)
	for i := 0; i < 10; i++ {
		new.Requirements[i*5] = fmt.Sprintf("CHANGED: requirement %d updated", i*5)
	}

	b.ResetTimer()
	for b.Loop() {
		_ = DiffSpecs(old, new)
	}
}

func BenchmarkFormatDiff(b *testing.B) {
	old := makeSpec("v1", 20)
	new := makeSpec("v2", 25)
	new.Requirements[0] = "CHANGED: first requirement"
	d := DiffSpecs(old, new)

	b.ResetTimer()
	for b.Loop() {
		_ = FormatDiff(d)
	}
}
