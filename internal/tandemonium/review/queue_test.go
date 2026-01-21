package review

import "testing"

func TestQueueAdd(t *testing.T) {
    q := NewQueue()
    q.Add("TAND-001")
    if q.Len() != 1 {
        t.Fatalf("expected len 1, got %d", q.Len())
    }
}
