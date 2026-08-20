package door

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestTmuxCaptureSwitchClientAndZed is Gate B clause (b) made executable: a
// real door binary runs in a real (isolated) tmux server, enter on a row with
// a live session switches the attached client (decision 9), and z hands the
// card path to zed. Everything is scripted -- send-keys in, capture-pane and
// list-clients out -- so the test drives exactly what mk's fingers would.
//
// The server lives on its own socket; the door inherits $TMUX from its pane,
// so every tmux call it makes stays inside the sandbox even when the test
// itself runs under mk's real tmux.
func TestTmuxCaptureSwitchClientAndZed(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary and drives a tmux server; skipped in -short")
	}
	tmuxBin, err := exec.LookPath("tmux")
	if err != nil {
		t.Skip("tmux not on PATH")
	}

	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	// The estate: two absent-card projects. Both score 0, so ruling 11's
	// name tiebreak puts aaa on top -- which is where the untouched
	// selection sits, and aaa is the project we give a live session.
	estate := filepath.Join(dir, "estate")
	projA := mkrepo(t, estate, "aaa", true)
	mkrepo(t, estate, "bbb", true)

	stubs := filepath.Join(dir, "stubs")
	if err := os.MkdirAll(stubs, 0o755); err != nil {
		t.Fatal(err)
	}
	cardStub := filepath.Join(stubs, "card-check-stub")
	writeExec(t, cardStub, "#!/bin/sh\n"+
		`printf '%s' '{"verdict":"absent","code":3,"card":"","reason":"","strength":{"score":0,"of":6,"confirmed":0,"drafted":0,"declined":0}}'`+"\nexit 3\n")
	zedArgs := filepath.Join(dir, "zed-args")
	writeExec(t, filepath.Join(stubs, "zed"), "#!/bin/sh\necho \"$@\" > "+zedArgs+"\n")

	bin := filepath.Join(dir, "autarch")
	if out, err := exec.Command("go", "build", "-o", bin, "../../cmd/autarch").CombinedOutput(); err != nil {
		t.Fatalf("building door binary: %v\n%s", err, out)
	}

	// Socket in a short-named dir: t.TempDir can brush the 104-byte sun_path
	// limit on macOS.
	sockDir, err := os.MkdirTemp("", "doorcap")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(sockDir)
	sock := filepath.Join(sockDir, "s")

	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command(tmuxBin, append([]string{"-f", "/dev/null", "-S", sock}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("tmux %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	defer exec.Command(tmuxBin, "-S", sock, "kill-server").Run()

	// A working session parked inside project aaa, then the door itself in a
	// pane whose cwd is outside the estate -- so the door's own session must
	// appear, named, in the unresolved disclosure.
	run("new-session", "-d", "-s", "work", "-c", projA)
	errLog := filepath.Join(dir, "door-err.log")
	doorCmd := fmt.Sprintf("env 'PATH=%s' AUTARCH_CARD_CHECK=%s %s door --root %s --ranking %s 2>%s; sleep 60",
		stubs+":"+os.Getenv("PATH"), cardStub, bin, estate, filepath.Join(dir, "rank.yaml"), errLog)
	run("new-session", "-d", "-s", "door", "-c", dir, "-x", "120", "-y", "30", doorCmd)

	dump := func(why string) {
		t.Helper()
		errOut, _ := os.ReadFile(errLog)
		t.Fatalf("%s\n--- capture ---\n%s\n--- stderr ---\n%s", why,
			run("capture-pane", "-p", "-t", "door"), errOut)
	}
	waitFor := func(why string, cond func() bool) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			if cond() {
				return
			}
			time.Sleep(150 * time.Millisecond)
		}
		dump("timed out waiting for " + why)
	}

	// The rendered door proves clauses (a) and (c) against a live server:
	// count on the row, fraction plus named unresolvable in the footer.
	waitFor("door to render both axes", func() bool {
		cap := run("capture-pane", "-p", "-t", "door")
		return strings.Contains(cap, "aaa") && strings.Contains(cap, "bbb") &&
			strings.Contains(cap, "sessions: 1/2 resolved") &&
			strings.Contains(cap, "unresolved: door")
	})
	if cap := run("capture-pane", "-p", "-t", "door"); !strings.Contains(cap, "◆1") {
		dump("row for aaa missing its live-session count")
	}

	// switch-client needs a client to switch. A control-mode client is a
	// real client, minus the need for a pty.
	client := exec.Command(tmuxBin, "-f", "/dev/null", "-S", sock, "-C", "attach-session", "-t", "door")
	clientIn, err := client.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	client.Stdout = io.Discard
	client.Stderr = io.Discard
	if err := client.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		clientIn.Close()
		client.Process.Kill()
		client.Wait()
	}()
	waitFor("control client to attach", func() bool {
		return strings.Contains(run("list-clients", "-F", "#{client_session}"), "door")
	})

	// Enter on aaa (top row, has a session): decision 9 says the client
	// moves to that session.
	run("send-keys", "-t", "door", "Enter")
	waitFor("client to switch to the work session", func() bool {
		return strings.TrimSpace(run("list-clients", "-F", "#{client_session}")) == "work"
	})

	// z: the card opens in Zed regardless of sessions.
	run("send-keys", "-t", "door", "z")
	waitFor("zed stub to receive the card path", func() bool {
		got, err := os.ReadFile(zedArgs)
		return err == nil && strings.Contains(string(got), filepath.Join(projA, "docs", "why.md"))
	})
}

func writeExec(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}
