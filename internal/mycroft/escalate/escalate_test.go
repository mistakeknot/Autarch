package escalate

import (
	"testing"

	"github.com/mistakeknot/autarch/internal/homeask"
)

func TestBadge(t *testing.T) {
	tests := []struct {
		count    int
		severity Severity
		want     string
	}{
		{0, SeverityLow, "✓ idle"},
		{3, SeverityHigh, "⚠ 3 pending"},
		{5, SeverityMedium, "● 5 pending"},
		{1, SeverityLow, "● 1 pending"},
	}
	for _, tt := range tests {
		got := Badge(tt.count, tt.severity)
		if got != tt.want {
			t.Errorf("Badge(%d, %d) = %q, want %q", tt.count, tt.severity, got, tt.want)
		}
	}
}

func TestHighestSeverity(t *testing.T) {
	m := &MemoryFiler{}
	q := NewDecisionQueue()
	q.SetHome(m, m, "/x")
	if q.HighestSeverity() != SeverityLow {
		t.Error("empty queue should be low severity")
	}
	for _, c := range []struct {
		priority int
		want     Severity
	}{{3, SeverityLow}, {1, SeverityMedium}, {0, SeverityHigh}} {
		m.Rows = append(m.Rows, homeask.ListRow{ID: "r", Priority: c.priority})
		q.expireCache()
		if got := q.HighestSeverity(); got != c.want {
			t.Errorf("after P%d: got %v want %v", c.priority, got, c.want)
		}
	}
}

func TestPriorityToSeverity(t *testing.T) {
	tests := []struct {
		priority int
		want     Severity
	}{
		{0, SeverityHigh},
		{1, SeverityMedium},
		{2, SeverityLow},
		{3, SeverityLow},
		{4, SeverityLow},
	}
	for _, tt := range tests {
		got := priorityToSeverity(tt.priority)
		if got != tt.want {
			t.Errorf("priorityToSeverity(%d) = %d, want %d", tt.priority, got, tt.want)
		}
	}
}
