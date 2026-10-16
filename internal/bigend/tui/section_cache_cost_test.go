package tui

import (
	"testing"

	"github.com/mistakeknot/autarch/internal/bigend/aggregator"
	"github.com/mistakeknot/autarch/internal/icdata"
)

func TestHashCostBaselineIncludesSource(t *testing.T) {
	mk := func(src string) *aggregator.KernelState {
		return &aggregator.KernelState{CostBaseline: &icdata.CostBaseline{LandedChanges: 5, Source: src}}
	}
	if hashCostBaseline(mk("landed_changes")) == hashCostBaseline(mk("shipped_beads")) {
		t.Error("hash must change when only Source changes")
	}
}
