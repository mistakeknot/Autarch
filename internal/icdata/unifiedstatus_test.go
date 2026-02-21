package icdata

import "testing"

func TestUnifyStatus(t *testing.T) {
	tests := []struct {
		raw  string
		want UnifiedStatus
	}{
		// Active
		{"working", StatusActive},
		{"running", StatusActive},
		{"active", StatusActive},
		{"in_progress", StatusActive},
		{"in-progress", StatusActive},
		{"executing", StatusActive},
		{"RUNNING", StatusActive},   // case insensitive
		{" active ", StatusActive},  // trimmed

		// Blocked
		{"blocked", StatusBlocked},
		{"stalled", StatusBlocked},
		{"permission-required", StatusBlocked},

		// Waiting
		{"waiting", StatusWaiting},
		{"idle", StatusWaiting},
		{"queued", StatusWaiting},
		{"pending", StatusWaiting},
		{"paused", StatusWaiting},
		{"todo", StatusWaiting},
		{"draft", StatusWaiting},

		// Done
		{"completed", StatusDone},
		{"done", StatusDone},
		{"complete", StatusDone},
		{"closed", StatusDone},
		{"shipped", StatusDone},

		// Error
		{"failed", StatusErr},
		{"error", StatusErr},
		{"cancelled", StatusErr},
		{"canceled", StatusErr},
		{"timeout", StatusErr},
		{"stopped", StatusErr},
		{"crashed", StatusErr},

		// Unknown
		{"", StatusUnknown},
		{"gibberish", StatusUnknown},
		{"unknown", StatusUnknown},
	}

	for _, tt := range tests {
		got := UnifyStatus(tt.raw)
		if got != tt.want {
			t.Errorf("UnifyStatus(%q) = %v, want %v", tt.raw, got, tt.want)
		}
	}
}

func TestUnifiedStatusString(t *testing.T) {
	tests := []struct {
		s    UnifiedStatus
		want string
	}{
		{StatusActive, "active"},
		{StatusBlocked, "blocked"},
		{StatusWaiting, "waiting"},
		{StatusDone, "done"},
		{StatusErr, "error"},
		{StatusUnknown, "unknown"},
	}
	for _, tt := range tests {
		if got := tt.s.String(); got != tt.want {
			t.Errorf("%d.String() = %q, want %q", tt.s, got, tt.want)
		}
	}
}
