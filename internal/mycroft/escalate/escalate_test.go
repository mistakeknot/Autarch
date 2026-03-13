package escalate

import "testing"

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

func TestDecisionQueue(t *testing.T) {
	q := NewDecisionQueue()

	if q.Len() != 0 {
		t.Errorf("empty queue len: got %d", q.Len())
	}

	id1 := q.Add("grey-area", "Demarch-1", "Fix test", 0, "P0 match")
	id2 := q.Add("mistake-not", "Demarch-2", "Add feature", 2, "capability match")

	if q.Len() != 2 {
		t.Errorf("after 2 adds: got %d", q.Len())
	}

	d, ok := q.Get(id1)
	if !ok {
		t.Fatal("decision 1 not found")
	}
	if d.Agent != "grey-area" {
		t.Errorf("decision 1 agent: got %q", d.Agent)
	}
	if d.Priority != 0 {
		t.Errorf("decision 1 priority: got %d", d.Priority)
	}

	q.Remove(id1)
	if q.Len() != 1 {
		t.Errorf("after remove: got %d", q.Len())
	}

	_, ok = q.Get(id1)
	if ok {
		t.Error("removed decision should not be found")
	}

	_, ok = q.Get(id2)
	if !ok {
		t.Error("decision 2 should still exist")
	}
}

func TestDecisionQueueAll(t *testing.T) {
	q := NewDecisionQueue()
	q.Add("a", "b1", "title1", 1, "r1")
	q.Add("b", "b2", "title2", 2, "r2")

	all := q.All()
	if len(all) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(all))
	}
}

func TestHighestSeverity(t *testing.T) {
	q := NewDecisionQueue()
	if q.HighestSeverity() != SeverityLow {
		t.Error("empty queue should be low severity")
	}

	q.Add("a", "b1", "title", 3, "")
	if q.HighestSeverity() != SeverityLow {
		t.Error("P3 should be low severity")
	}

	q.Add("b", "b2", "title", 1, "")
	if q.HighestSeverity() != SeverityMedium {
		t.Error("P1 should be medium severity")
	}

	q.Add("c", "b3", "title", 0, "")
	if q.HighestSeverity() != SeverityHigh {
		t.Error("P0 should be high severity")
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
