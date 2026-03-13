package mycroft

import (
	"encoding/json"
	"testing"
	"time"
)

func TestTierString(t *testing.T) {
	tests := []struct {
		tier Tier
		want string
	}{
		{T0, "T0:observe"},
		{T1, "T1:suggest"},
		{T2, "T2:auto-low-risk"},
		{T3, "T3:auto-full"},
		{Tier(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.tier.String(); got != tt.want {
			t.Errorf("Tier(%d).String() = %q, want %q", tt.tier, got, tt.want)
		}
	}
}

func TestFleetViewJSON(t *testing.T) {
	view := FleetView{
		Agents: []AgentView{
			{
				Name:         "grey-area",
				Runtime:      "claude-code",
				Capabilities: []string{"debugging", "test_discipline"},
				Status:       "active",
				CurrentBead:  "Demarch-42",
			},
		},
		Work: []BeadView{
			{
				ID:           "Demarch-99",
				Title:        "Fix flaky test",
				Type:         "bug",
				Priority:     1,
				Complexity:   "simple",
				DepsResolved: true,
				CreatedAt:    time.Date(2026, 3, 12, 10, 0, 0, 0, time.UTC),
			},
		},
		Freshness: map[string]time.Time{
			"intermux": time.Now(),
			"beads":    time.Now(),
		},
	}

	data, err := json.Marshal(view)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded FleetView
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(decoded.Agents) != 1 {
		t.Fatalf("agents: got %d, want 1", len(decoded.Agents))
	}
	if decoded.Agents[0].Name != "grey-area" {
		t.Errorf("agent name: got %q, want %q", decoded.Agents[0].Name, "grey-area")
	}
	if len(decoded.Work) != 1 {
		t.Fatalf("work: got %d, want 1", len(decoded.Work))
	}
	if decoded.Work[0].Priority != 1 {
		t.Errorf("priority: got %d, want 1", decoded.Work[0].Priority)
	}
}

func TestBeadViewComplexity(t *testing.T) {
	bead := BeadView{
		ID:         "Demarch-1",
		Complexity: "",
	}
	// Missing complexity should default to empty (treated as "unknown" by selector).
	if bead.Complexity != "" {
		t.Errorf("empty complexity: got %q", bead.Complexity)
	}
}
