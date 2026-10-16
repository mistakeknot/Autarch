package icdata

import (
	"encoding/json"
	"os"
	"testing"
)

// The fixture mirrors the BaselineResult emitter in ic (written from source).
func TestCostBaselineDecodesLandedChanges(t *testing.T) {
	data, err := os.ReadFile("../../pkg/intercore/testdata/cost_baseline.from-source.json")
	if err != nil {
		t.Fatal(err)
	}
	var cb CostBaseline
	if err := json.Unmarshal(data, &cb); err != nil {
		t.Fatal(err)
	}
	if cb.LandedChanges != 12 || cb.Source != "landed_changes" || cb.Stats.Count != 9 {
		t.Errorf("baseline: %+v", cb)
	}
}
