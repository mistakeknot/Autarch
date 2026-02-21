package icdata

import "testing"

func TestKernelEventRoundtrip(t *testing.T) {
	tests := []struct {
		event KernelEvent
		str   string
	}{
		{EventPhaseAdvance, "phase.advance"},
		{EventPhaseRollback, "phase.rollback"},
		{EventGateCheck, "gate.check"},
		{EventGatePassed, "gate.passed"},
		{EventGateFailed, "gate.failed"},
		{EventDispatchSpawned, "dispatch.spawned"},
		{EventDispatchCompleted, "dispatch.completed"},
		{EventDispatchFailed, "dispatch.failed"},
		{EventDispatchCancelled, "dispatch.cancelled"},
		{EventArtifactAdded, "artifact.added"},
		{EventTokensRecorded, "tokens.recorded"},
		{EventBudgetExceeded, "budget.exceeded"},
	}

	for _, tt := range tests {
		// String() roundtrip
		if got := tt.event.String(); got != tt.str {
			t.Errorf("%d.String() = %q, want %q", tt.event, got, tt.str)
		}
		// Parse roundtrip
		if got := ParseKernelEvent(tt.str); got != tt.event {
			t.Errorf("ParseKernelEvent(%q) = %d, want %d", tt.str, got, tt.event)
		}
	}
}

func TestKernelEventUnknown(t *testing.T) {
	if got := ParseKernelEvent("gibberish"); got != EventUnknown {
		t.Errorf("ParseKernelEvent(gibberish) = %d, want EventUnknown", got)
	}
	if got := ParseKernelEvent(""); got != EventUnknown {
		t.Errorf("ParseKernelEvent(\"\") = %d, want EventUnknown", got)
	}
	if got := EventUnknown.String(); got != "unknown" {
		t.Errorf("EventUnknown.String() = %q, want \"unknown\"", got)
	}
}
