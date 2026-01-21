package tmux

import "testing"

func TestParseSessions(t *testing.T) {
    out := "tand-TAND-001: 1 windows (created Fri)\nother: 1 windows\n"
    got := ParseSessions(out, "tand-")
    if len(got) != 1 || got[0] != "tand-TAND-001" {
        t.Fatalf("unexpected sessions: %v", got)
    }
}
