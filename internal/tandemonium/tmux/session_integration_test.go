package tmux

import "testing"

func TestPipePaneCommand(t *testing.T) {
    r := &fakeRunner{}
    s := Session{ID: "tand-TAND-001", Workdir: "/tmp/x", LogPath: "/tmp/log"}
    _ = StartSession(r, s)
    found := false
    for _, cmd := range r.cmds {
        if len(cmd) >= 2 && cmd[1] == "pipe-pane" {
            found = true
        }
    }
    if !found {
        t.Fatal("expected pipe-pane")
    }
}
