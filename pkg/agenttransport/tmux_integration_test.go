package agenttransport

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/term"
)

// A copy of this test executable named codex is the isolated pane process.
// Raw mode lets the parent verify exact multiline bytes and bracket framing.
func TestTransportPaneHelper(t *testing.T) {
	path := os.Getenv("AUTARCH_TRANSPORT_CAPTURE")
	if path == "" || filepath.Base(os.Args[0]) != "codex" {
		return
	}
	state, e := term.MakeRaw(int(os.Stdin.Fd()))
	if e != nil {
		t.Fatal(e)
	}
	defer term.Restore(int(os.Stdin.Fd()), state)
	os.Stdout.WriteString("\x1b[?2004hready")
	var out []byte
	one := make([]byte, 1)
	for {
		n, e := os.Stdin.Read(one)
		if e != nil {
			t.Fatal(e)
		}
		out = append(out, one[:n]...)
		if n == 1 && one[0] == '\r' {
			break
		}
	}
	if e := os.WriteFile(path, out, 0600); e != nil {
		t.Fatal(e)
	}
	time.Sleep(30 * time.Second)
}

func TestIsolatedTmuxLiteralDeliveryAndInactivePanes(t *testing.T) {
	if testing.Short() {
		t.Skip("isolated tmux integration")
	}
	bin, e := exec.LookPath("tmux")
	if e != nil {
		t.Skip("tmux unavailable")
	}
	dir, e := os.MkdirTemp("", "autarch-tmux-")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(dir)
	// macOS returns the canonical /private/tmp socket path from #{socket_path}.
	dir, e = filepath.EvalSymlinks(dir)
	if e != nil {
		t.Fatal(e)
	}
	socket := filepath.Join(dir, "s")
	// Check socket capability directly: sandboxed tmux can return success while
	// its background server exits before publishing a socket.
	probe, e := net.Listen("unix", filepath.Join(dir, "probe"))
	if e != nil {
		if strings.Contains(e.Error(), "operation not permitted") {
			t.Skipf("isolated tmux integration UNVERIFIABLE: %v", e)
		}
		t.Fatal(e)
	}
	probe.Close()
	run := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return exec.CommandContext(ctx, bin, append([]string{"-f", "/dev/null", "-S", socket}, args...)...).CombinedOutput()
	}
	t.Setenv("TMUX", "")
	t.Setenv("AUTARCH_TRANSPORT_CAPTURE", filepath.Join(dir, "received"))
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	src, e := os.Open(exe)
	if e != nil {
		t.Fatal(e)
	}
	defer src.Close()
	helper := filepath.Join(dir, "codex")
	dst, e := os.OpenFile(helper, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if e != nil {
		t.Fatal(e)
	}
	_, e = io.Copy(dst, src)
	closeErr := dst.Close()
	if e != nil || closeErr != nil {
		t.Fatalf("copy helper: %v %v", e, closeErr)
	}
	out, e := run("new-session", "-d", "-s", "fixture", "sleep", "30")
	if e != nil {
		if strings.Contains(string(out), "Operation not permitted") || strings.Contains(string(out), "operation not permitted") {
			t.Skipf("sandbox prevents isolated tmux socket; integration UNVERIFIABLE: %s", out)
		}
		t.Fatalf("isolated server: %v %s", e, out)
	}
	defer run("kill-server") // Only the freshly created fixture server.
	for _, args := range [][]string{{"set-option", "-w", "-t", "fixture", "remain-on-exit", "on"}, {"split-window", "-d", "-t", "fixture", helper, "-test.run=^TestTransportPaneHelper$"}, {"new-window", "-d", "-t", "fixture", "sleep", "30"}} {
		if out, e := run(args...); e != nil {
			t.Fatalf("fixture panes: %v %s", e, out)
		}
	}
	tr := NewTmux(nil, socket)
	ctx := context.Background()
	var selected Target
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ps, e := tr.List(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if len(ps) != 3 {
			t.Fatalf("splits/inactive window lost: %+v", ps)
		}
		for _, p := range ps {
			if p.Command == "codex" {
				selected = p.Target
			}
		}
		if selected.Socket != "" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if selected.Socket == "" {
		out, _ := run("capture-pane", "-p", "-t", "fixture:0.1")
		t.Fatalf("helper pane not found: %s", out)
	}
	// Wait until the fixture has entered raw mode and requested bracketed paste.
	// tmux 3.6b honors paste-buffer -p but does not expose that parser state as
	// a format variable, so readiness is observed from the pane itself and the
	// exact bracket framing is asserted below.
	for time.Now().Before(deadline) {
		out, e := run("capture-pane", "-p", "-t", selected.address())
		if e == nil && strings.Contains(string(out), "ready") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if out, e := run("rename-session", "-t", selected.SessionID, "renamed"); e != nil {
		t.Fatalf("rename: %v %s", e, out)
	}
	payload := "literal\n$(touch nope) `echo nope` ; ' \" \\ unicode: λ\nlast"
	if r := tr.Send(ctx, selected, payload); r.State != Delivered || r.Err != nil {
		t.Fatalf("send: %+v", r)
	}
	var got []byte
	for time.Now().Before(deadline) {
		got, e = os.ReadFile(filepath.Join(dir, "received"))
		if e == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	want := []byte("\x1b[200~" + payload + "\x1b[201~\r")
	if !bytes.Equal(got, want) {
		t.Fatalf("literal bytes: got %q want %q", got, want)
	}
	out, e = run("list-buffers", "-F", "#{buffer_name}")
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(out), "autarch-") {
		t.Fatal("message buffer leaked")
	}
	staleAttach, e := tr.AttachCommand(ctx, selected)
	if e != nil {
		t.Fatalf("prepare guarded attach: %v", e)
	}
	if out, e := run("respawn-pane", "-k", "-t", selected.address(), "sleep", "30"); e != nil {
		t.Fatalf("replace fixture pane: %v %s", e, out)
	}
	if out, e := staleAttach.CombinedOutput(); e == nil {
		t.Fatalf("execution-time attach guard exited successfully after replacement: %s", out)
	}
	if r := tr.Interrupt(ctx, selected); r.State != NotSent {
		t.Fatalf("input reached replacement: %+v", r)
	}
}
