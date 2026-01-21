package tmux

import "testing"

type fakeRunner struct{ cmds [][]string }

func (f *fakeRunner) Run(name string, args ...string) error {
	f.cmds = append(f.cmds, append([]string{name}, args...))
	return nil
}

func TestStartSessionBuildsCommands(t *testing.T) {
	r := &fakeRunner{}
	s := Session{ID: "tand-TAND-001", Workdir: "/tmp/x", LogPath: "/tmp/log"}
	if err := StartSession(r, s); err != nil {
		t.Fatal(err)
	}
	if len(r.cmds) < 2 {
		t.Fatalf("expected commands, got %d", len(r.cmds))
	}
}

func TestStartSessionEscapesLogPath(t *testing.T) {
	r := &fakeRunner{}
	s := Session{ID: "tand-TAND-002", Workdir: "/tmp/x", LogPath: "/tmp/log's dir"}
	if err := StartSession(r, s); err != nil {
		t.Fatal(err)
	}
	if len(r.cmds) < 2 {
		t.Fatalf("expected commands, got %d", len(r.cmds))
	}
	pipe := r.cmds[1]
	if len(pipe) < 6 {
		t.Fatalf("expected pipe-pane args")
	}
	got := pipe[len(pipe)-1]
	if got != "cat >> '/tmp/log'\"'\"'s dir'" {
		t.Fatalf("unexpected pipe command: %q", got)
	}
}
