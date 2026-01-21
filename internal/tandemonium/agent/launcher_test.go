package agent

import "testing"

func TestSessionIDFormat(t *testing.T) {
    id := SessionID("TAND-001")
    if id != "tand-TAND-001" {
        t.Fatalf("unexpected id: %s", id)
    }
}
