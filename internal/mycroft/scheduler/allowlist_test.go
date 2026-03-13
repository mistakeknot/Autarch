package scheduler

import (
	"testing"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

func TestAllowlistCheckEmpty(t *testing.T) {
	bead := mycroft.BeadView{Type: "task", Priority: 3, Complexity: "simple"}
	if AllowlistCheck(bead, nil) {
		t.Error("empty allowlist should reject everything")
	}
}

func TestAllowlistCheckTypeMatch(t *testing.T) {
	allowlist := []mycroft.AllowlistEntry{
		{Type: "task", MaxPriority: 3, MaxComplexity: "medium"},
	}

	tests := []struct {
		name string
		bead mycroft.BeadView
		want bool
	}{
		{"task P3 simple — match", mycroft.BeadView{Type: "task", Priority: 3, Complexity: "simple"}, true},
		{"task P4 simple — match (lower pri)", mycroft.BeadView{Type: "task", Priority: 4, Complexity: "simple"}, true},
		{"task P2 simple — reject (too high pri)", mycroft.BeadView{Type: "task", Priority: 2, Complexity: "simple"}, false},
		{"task P3 medium — match", mycroft.BeadView{Type: "task", Priority: 3, Complexity: "medium"}, true},
		{"task P3 complex — reject (too complex)", mycroft.BeadView{Type: "task", Priority: 3, Complexity: "complex"}, false},
		{"bug P3 simple — reject (wrong type)", mycroft.BeadView{Type: "bug", Priority: 3, Complexity: "simple"}, false},
		{"task P3 unknown — reject (unknown escalates)", mycroft.BeadView{Type: "task", Priority: 3, Complexity: "unknown"}, false},
		{"task P3 empty — reject (empty escalates)", mycroft.BeadView{Type: "task", Priority: 3, Complexity: ""}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AllowlistCheck(tt.bead, allowlist); got != tt.want {
				t.Errorf("AllowlistCheck(%v) = %v, want %v", tt.bead, got, tt.want)
			}
		})
	}
}

func TestAllowlistCheckMultipleEntries(t *testing.T) {
	// Matches the brainstorm's example allowlist.
	allowlist := []mycroft.AllowlistEntry{
		{Type: "task", MaxPriority: 3, MaxComplexity: "medium"},
		{Type: "bug", MaxPriority: 3, MaxComplexity: "simple"},
		{Type: "docs", MaxPriority: 2, MaxComplexity: "any"},
	}

	tests := []struct {
		name string
		bead mycroft.BeadView
		want bool
	}{
		{"task P3 medium", mycroft.BeadView{Type: "task", Priority: 3, Complexity: "medium"}, true},
		{"bug P3 simple", mycroft.BeadView{Type: "bug", Priority: 3, Complexity: "simple"}, true},
		{"bug P3 medium — too complex for bug entry", mycroft.BeadView{Type: "bug", Priority: 3, Complexity: "medium"}, false},
		{"docs P2 complex — any complexity", mycroft.BeadView{Type: "docs", Priority: 2, Complexity: "complex"}, true},
		{"docs P1 — too high priority", mycroft.BeadView{Type: "docs", Priority: 1, Complexity: "simple"}, false},
		{"feature P3 — no entry for feature", mycroft.BeadView{Type: "feature", Priority: 3, Complexity: "simple"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AllowlistCheck(tt.bead, allowlist); got != tt.want {
				t.Errorf("AllowlistCheck(%v) = %v, want %v", tt.bead, got, tt.want)
			}
		})
	}
}

func TestComplexityWithin(t *testing.T) {
	tests := []struct {
		bead, max string
		want      bool
	}{
		{"simple", "simple", true},
		{"simple", "medium", true},
		{"simple", "complex", true},
		{"medium", "simple", false},
		{"medium", "medium", true},
		{"complex", "medium", false},
		{"unknown", "any", false}, // unknown always escalates
		{"simple", "any", true},
		{"complex", "any", true},
		{"", "medium", false}, // empty = unknown
	}

	for _, tt := range tests {
		t.Run(tt.bead+"/"+tt.max, func(t *testing.T) {
			if got := complexityWithin(tt.bead, tt.max); got != tt.want {
				t.Errorf("complexityWithin(%q, %q) = %v, want %v", tt.bead, tt.max, got, tt.want)
			}
		})
	}
}
