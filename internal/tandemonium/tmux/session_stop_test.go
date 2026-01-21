package tmux

import "testing"

func TestStopSessionBuildsCommand(t *testing.T) {
	r := &fakeRunner{}
	if err := StopSession(r, "tand-TAND-001"); err != nil {
		t.Fatal(err)
	}
	if len(r.cmds) == 0 || r.cmds[0][1] != "kill-session" {
		t.Fatal("expected kill-session")
	}
}
