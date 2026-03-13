package scheduler

import (
	"testing"
	"time"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

func TestRankBeads(t *testing.T) {
	now := time.Now()
	beads := []mycroft.BeadView{
		{ID: "low-p3", Priority: 3, DepsResolved: true, CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "critical-p0", Priority: 0, DepsResolved: true, CreatedAt: now},
		{ID: "high-p1-old", Priority: 1, DepsResolved: true, CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "high-p1-new", Priority: 1, DepsResolved: true, CreatedAt: now},
		{ID: "blocked", Priority: 0, DepsResolved: false, CreatedAt: now},
	}

	ranked := RankBeads(beads)

	// Blocked bead should be excluded.
	if len(ranked) != 4 {
		t.Fatalf("expected 4 eligible beads, got %d", len(ranked))
	}

	// P0 first.
	if ranked[0].ID != "critical-p0" {
		t.Errorf("rank 0: got %q, want critical-p0", ranked[0].ID)
	}

	// P1 older before P1 newer.
	if ranked[1].ID != "high-p1-old" {
		t.Errorf("rank 1: got %q, want high-p1-old", ranked[1].ID)
	}
	if ranked[2].ID != "high-p1-new" {
		t.Errorf("rank 2: got %q, want high-p1-new", ranked[2].ID)
	}

	// P3 last.
	if ranked[3].ID != "low-p3" {
		t.Errorf("rank 3: got %q, want low-p3", ranked[3].ID)
	}
}

func TestRankBeadsComplexityTiebreak(t *testing.T) {
	now := time.Now()
	beads := []mycroft.BeadView{
		{ID: "complex", Priority: 1, Complexity: "complex", DepsResolved: true, CreatedAt: now},
		{ID: "simple", Priority: 1, Complexity: "simple", DepsResolved: true, CreatedAt: now},
		{ID: "unknown", Priority: 1, Complexity: "", DepsResolved: true, CreatedAt: now},
		{ID: "medium", Priority: 1, Complexity: "medium", DepsResolved: true, CreatedAt: now},
	}

	ranked := RankBeads(beads)

	expected := []string{"simple", "medium", "complex", "unknown"}
	for i, want := range expected {
		if ranked[i].ID != want {
			t.Errorf("rank %d: got %q, want %q", i, ranked[i].ID, want)
		}
	}
}

func TestRankBeadsEmpty(t *testing.T) {
	ranked := RankBeads(nil)
	if len(ranked) != 0 {
		t.Errorf("expected 0, got %d", len(ranked))
	}
}

func TestRankBeadsAllBlocked(t *testing.T) {
	beads := []mycroft.BeadView{
		{ID: "a", Priority: 0, DepsResolved: false},
		{ID: "b", Priority: 1, DepsResolved: false},
	}
	ranked := RankBeads(beads)
	if len(ranked) != 0 {
		t.Errorf("expected 0 eligible, got %d", len(ranked))
	}
}

func TestRankBeadsPriorityBoosts(t *testing.T) {
	now := time.Now()
	beads := []mycroft.BeadView{
		{ID: "feature-p1", Type: "feature", Priority: 1, DepsResolved: true, CreatedAt: now},
		{ID: "bug-p2", Type: "bug", Priority: 2, DepsResolved: true, CreatedAt: now},
		{ID: "task-p2", Type: "task", Priority: 2, DepsResolved: true, CreatedAt: now},
	}

	// Without boosts: feature-p1, then bug-p2 and task-p2.
	noBoost := RankBeads(beads)
	if noBoost[0].ID != "feature-p1" {
		t.Errorf("no boost: first should be feature-p1, got %q", noBoost[0].ID)
	}

	// With bug boost of 2: bug-p2 effective priority = 0, beats feature-p1.
	boosts := []mycroft.PriorityBoost{
		{Type: "bug", Boost: 2},
	}
	boosted := RankBeads(beads, boosts...)
	if boosted[0].ID != "bug-p2" {
		t.Errorf("with bug boost: first should be bug-p2, got %q", boosted[0].ID)
	}
	if boosted[1].ID != "feature-p1" {
		t.Errorf("with bug boost: second should be feature-p1, got %q", boosted[1].ID)
	}
}

func TestRankBeadsPriorityBoostClamp(t *testing.T) {
	now := time.Now()
	beads := []mycroft.BeadView{
		{ID: "bug-p0", Type: "bug", Priority: 0, DepsResolved: true, CreatedAt: now},
		{ID: "bug-p1", Type: "bug", Priority: 1, DepsResolved: true, CreatedAt: now},
	}

	// Huge boost should clamp to 0, not go negative.
	boosts := []mycroft.PriorityBoost{{Type: "bug", Boost: 10}}
	ranked := RankBeads(beads, boosts...)

	// Both clamped to 0 — should fall back to age tiebreak (same time), then complexity.
	if len(ranked) != 2 {
		t.Fatalf("expected 2, got %d", len(ranked))
	}
}

func TestSelectForAgent(t *testing.T) {
	now := time.Now()
	ranked := []mycroft.BeadView{
		{ID: "a", Priority: 0, DepsResolved: true, CreatedAt: now},
		{ID: "b", Priority: 1, DepsResolved: true, CreatedAt: now, ClaimedBy: "other-agent"},
		{ID: "c", Priority: 2, DepsResolved: true, CreatedAt: now},
		{ID: "d", Priority: 3, DepsResolved: true, CreatedAt: now},
	}

	agent := mycroft.AgentView{Name: "grey-area"}
	selected := SelectForAgent(ranked, agent, nil, 2)

	// Should skip "b" (claimed by other-agent) and return "a" and "c".
	if len(selected) != 2 {
		t.Fatalf("expected 2 selected, got %d", len(selected))
	}
	if selected[0].ID != "a" {
		t.Errorf("selected 0: got %q, want a", selected[0].ID)
	}
	if selected[1].ID != "c" {
		t.Errorf("selected 1: got %q, want c", selected[1].ID)
	}
}

func TestSelectForAgentDefault(t *testing.T) {
	beads := make([]mycroft.BeadView, 5)
	for i := range beads {
		beads[i] = mycroft.BeadView{ID: string(rune('a' + i)), DepsResolved: true}
	}

	agent := mycroft.AgentView{Name: "grey-area"}
	selected := SelectForAgent(beads, agent, nil, 0) // 0 → default 3

	if len(selected) != 3 {
		t.Errorf("expected default 3 suggestions, got %d", len(selected))
	}
}
