package tier

import (
	"path/filepath"
	"testing"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

func newTestFSM(t *testing.T) *FSM {
	t.Helper()
	db, err := mycroft.OpenDB(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return New(db, "test")
}

func TestFSMCurrentDefault(t *testing.T) {
	fsm := newTestFSM(t)
	tier, err := fsm.Current()
	if err != nil {
		t.Fatal(err)
	}
	if tier != mycroft.T0 {
		t.Errorf("default tier: got %v, want T0", tier)
	}
}

func TestFSMPromote(t *testing.T) {
	fsm := newTestFSM(t)

	// T0 → T1.
	if err := fsm.Promote(Evidence{Reason: "test promotion"}); err != nil {
		t.Fatalf("promote T0→T1: %v", err)
	}
	tier, _ := fsm.Current()
	if tier != mycroft.T1 {
		t.Errorf("after T0→T1: got %v, want T1", tier)
	}

	// T1 → T2.
	if err := fsm.Promote(Evidence{Reason: "earned T2"}); err != nil {
		t.Fatalf("promote T1→T2: %v", err)
	}
	tier, _ = fsm.Current()
	if tier != mycroft.T2 {
		t.Errorf("after T1→T2: got %v, want T2", tier)
	}

	// T2 → T3.
	if err := fsm.Promote(Evidence{Reason: "earned T3"}); err != nil {
		t.Fatalf("promote T2→T3: %v", err)
	}
	tier, _ = fsm.Current()
	if tier != mycroft.T3 {
		t.Errorf("after T2→T3: got %v, want T3", tier)
	}

	// T3 → error (already at max).
	if err := fsm.Promote(Evidence{}); err == nil {
		t.Error("expected error promoting past T3")
	}
}

func TestShouldPromote(t *testing.T) {
	criteria := DefaultPromotionCriteria()

	tests := []struct {
		name    string
		tier    mycroft.Tier
		history []DispatchRecord
		want    bool
	}{
		{
			"insufficient samples",
			mycroft.T0,
			make([]DispatchRecord, 5),
			false,
		},
		{
			"high approval and completion",
			mycroft.T1,
			buildHistory(20, 19, 15), // 95% approval, 75% completion
			true,
		},
		{
			"low approval",
			mycroft.T1,
			buildHistory(20, 10, 15), // 50% approval
			false,
		},
		{
			"low completion",
			mycroft.T1,
			buildHistory(20, 19, 5), // 95% approval, 25% completion
			false,
		},
		{
			"at max tier",
			mycroft.T3,
			buildHistory(20, 19, 15),
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, evidence := ShouldPromote(tt.tier, tt.history, criteria)
			if got != tt.want {
				t.Errorf("ShouldPromote = %v (reason: %s), want %v", got, evidence.Reason, tt.want)
			}
		})
	}
}

// buildHistory creates a dispatch history with the specified outcome counts.
// Outcomes are: "success" (counts as both accepted AND completed),
// "accepted" (accepted but not yet completed), "rejected".
func buildHistory(total, accepted, completed int) []DispatchRecord {
	records := make([]DispatchRecord, total)
	idx := 0
	// First: completed items (success = accepted + completed).
	for i := 0; i < completed && idx < total; i++ {
		records[idx] = DispatchRecord{Outcome: "success"}
		idx++
	}
	// Then: accepted but not completed.
	for i := 0; i < (accepted-completed) && idx < total; i++ {
		records[idx] = DispatchRecord{Outcome: "accepted"}
		idx++
	}
	// Rest: rejected.
	for idx < total {
		records[idx] = DispatchRecord{Outcome: "rejected"}
		idx++
	}
	return records
}

func TestFSMDemote(t *testing.T) {
	fsm := newTestFSM(t)

	// Promote to T1 first.
	fsm.Promote(Evidence{})

	err := fsm.Demote("circuit_breaker", Evidence{FailureRate: 0.2})
	if err != nil {
		t.Fatalf("demote: %v", err)
	}

	tier, err := fsm.Current()
	if err != nil {
		t.Fatal(err)
	}
	if tier != mycroft.T0 {
		t.Errorf("after demote: got %v, want T0", tier)
	}
}

func TestFSMDemoteAtT0(t *testing.T) {
	fsm := newTestFSM(t)
	err := fsm.Demote("test", Evidence{})
	if err == nil {
		t.Error("expected error demoting from T0")
	}
}

func TestFSMTransitions(t *testing.T) {
	fsm := newTestFSM(t)

	// Create two transitions.
	fsm.Promote(Evidence{Reason: "first"})
	fsm.Demote("test", Evidence{Reason: "second"})

	records, err := fsm.Transitions(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("transitions: got %d, want 2", len(records))
	}

	// Most recent first.
	if records[0].FromTier != 1 || records[0].ToTier != 0 {
		t.Errorf("latest transition: %d→%d, want 1→0", records[0].FromTier, records[0].ToTier)
	}
}

func TestShouldDemoteBelowMinSample(t *testing.T) {
	cfg := mycroft.DemotionTriggers{MinSampleSize: 20}
	history := make([]DispatchRecord, 5)
	for i := range history {
		history[i] = DispatchRecord{Outcome: "failure"}
	}

	demote, _, evidence := ShouldDemote(mycroft.T2, history, cfg)
	if demote {
		t.Error("should not demote below min sample size")
	}
	if evidence.SampleSize != 5 {
		t.Errorf("sample size: got %d, want 5", evidence.SampleSize)
	}
}

func TestShouldDemoteConsecutiveFailures(t *testing.T) {
	cfg := mycroft.DemotionTriggers{
		MinSampleSize:           5,
		ConsecutiveFailureLimit: 3,
		T2FailureRateThreshold:  0.15,
	}

	history := []DispatchRecord{
		{Outcome: "success"},
		{Outcome: "success"},
		{Outcome: "failure"},
		{Outcome: "failure"},
		{Outcome: "failure"},
	}

	demote, trigger, _ := ShouldDemote(mycroft.T2, history, cfg)
	if !demote {
		t.Error("should demote on 3 consecutive failures")
	}
	if trigger != "consecutive_failures" {
		t.Errorf("trigger: got %q, want consecutive_failures", trigger)
	}
}

func TestShouldDemoteCircuitBreaker(t *testing.T) {
	cfg := mycroft.DemotionTriggers{
		MinSampleSize:            20,
		ConsecutiveFailureLimit:  10,
		T2FailureRateThreshold:   0.15,
		T3FailureRateThreshold:   0.25,
	}

	// 4 failures in 20 = 20% > 15% threshold for T2.
	history := make([]DispatchRecord, 20)
	for i := range history {
		if i%5 == 0 { // Every 5th is a failure (4/20 = 20%).
			history[i] = DispatchRecord{Outcome: "failure"}
		} else {
			history[i] = DispatchRecord{Outcome: "success"}
		}
	}

	demote, trigger, evidence := ShouldDemote(mycroft.T2, history, cfg)
	if !demote {
		t.Errorf("should demote at T2 with %.1f%% failure rate", evidence.FailureRate*100)
	}
	if trigger != "circuit_breaker" {
		t.Errorf("trigger: got %q, want circuit_breaker", trigger)
	}

	// Same history at T3 should NOT demote (20% < 25% T3 threshold).
	demote, _, _ = ShouldDemote(mycroft.T3, history, cfg)
	if demote {
		t.Error("should NOT demote at T3 with 20% failure rate (threshold 25%)")
	}
}

func TestShouldDemoteT1NoRateBasedDemotion(t *testing.T) {
	cfg := mycroft.DemotionTriggers{
		MinSampleSize:           5,
		ConsecutiveFailureLimit: 10,
		T2FailureRateThreshold:  0.15,
	}

	// All failures but at T1 — no rate-based demotion (no threshold defined for T1).
	// Consecutive limit is 10, only 5 records, so no demotion.
	history := make([]DispatchRecord, 5)
	for i := range history {
		history[i] = DispatchRecord{Outcome: "failure"}
	}

	demote, _, _ := ShouldDemote(mycroft.T1, history, cfg)
	if demote {
		t.Error("should NOT demote T1 via rate (no threshold) and consecutive (5 < 10)")
	}
}

func TestShouldDemoteT1ConsecutiveFailures(t *testing.T) {
	cfg := mycroft.DemotionTriggers{
		MinSampleSize:           5,
		ConsecutiveFailureLimit: 3,
	}

	history := make([]DispatchRecord, 5)
	for i := range history {
		history[i] = DispatchRecord{Outcome: "failure"}
	}

	demote, trigger, _ := ShouldDemote(mycroft.T1, history, cfg)
	if !demote {
		t.Error("should demote T1 on 5 consecutive failures (limit 3)")
	}
	if trigger != "consecutive_failures" {
		t.Errorf("trigger: got %q, want consecutive_failures", trigger)
	}
}
