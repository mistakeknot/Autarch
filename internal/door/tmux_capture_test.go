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
	estate := filepath.Join(dir, "projects")
	projA := mkrepo(t, estate, "aaa", true)
	mkrepo(t, estate, "bbb", true)
	productFile(t, projA, "MISSION.md", "# Mission\nHelp readers preserve context.\n")
	productFile(t, projA, "docs/decisions/001-local.md", "# Local storage\nKeep reader history on device.\n")

	stubs := filepath.Join(dir, "stubs")
	if err := os.MkdirAll(stubs, 0o755); err != nil {
		t.Fatal(err)
	}
	cardStub := filepath.Join(stubs, "card-check-stub")
	writeExec(t, cardStub, "#!/bin/sh\n"+
		`printf '%s' '{"verdict":"absent","code":3,"card":"","reason":"","strength":{"score":0,"of":6,"confirmed":0,"drafted":0,"declined":0}}'`+"\nexit 3\n")
	zedArgs := filepath.Join(dir, "zed-args")
	writeExec(t, filepath.Join(stubs, "zed"), "#!/bin/sh\necho \"$@\" > "+zedArgs+"\n")
	resumeArgs := filepath.Join(dir, "resume-args")
	writeExec(t, filepath.Join(stubs, "claude"), "#!/bin/sh\nprintf '%s\\n' \"$@\" > "+resumeArgs+"\n")
	clipboardPath := filepath.Join(dir, "onboarding-clipboard")
	for _, name := range []string{"pbcopy", "xclip", "xsel", "wl-copy", "wl-paste"} {
		writeExec(t, filepath.Join(stubs, name), "#!/bin/sh\ncat > \"$FOUNDATION_CLIPBOARD\"\n")
	}
	id := "aaaa1111-bbbb-2222-cccc-333344445555"
	work := "iterm[reader - " + id
	transcript := filepath.Join(dir, ".claude", "projects", "-estate", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(transcript), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(transcript, []byte(userRequest+"\n"+assistantContext+"\n"+questionTool+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

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
	run("new-session", "-d", "-s", work, "-c", projA)
	errLog := filepath.Join(dir, "door-err.log")
	// HOME is the sandbox: the visit stamp and the transcript lookup must
	// never touch the real one from a test.
	doorCmd := fmt.Sprintf("env 'PATH=%s' HOME=%s AUTARCH_CARD_CHECK=%s FOUNDATION_CLIPBOARD=%s %s 2>%s; sleep 60",
		stubs+":"+os.Getenv("PATH"), dir, cardStub, clipboardPath, bin, errLog)
	run("new-session", "-d", "-s", "door", "-c", dir, "-x", "120", "-y", "30", doorCmd)

	dump := func(why string) {
		t.Helper()
		errOut, _ := os.ReadFile(errLog)
		pane := "door"
		if strings.Contains(run("list-sessions", "-F", "#{session_name}"), "reopened") {
			pane = "reopened"
		}
		t.Fatalf("%s\n--- capture ---\n%s\n--- stderr ---\n%s", why,
			run("capture-pane", "-p", "-t", pane), errOut)
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

	// The door opens on the briefing (autarch-01: orientation before
	// obligation); the rows are one tab away, which is the path mk's fingers
	// take every morning.
	waitFor("briefing to render", func() bool {
		return strings.Contains(run("capture-pane", "-p", "-t", "door"), "since ")
	})
	if cap := run("capture-pane", "-p", "-t", "door"); !strings.Contains(cap, "View: Cozy") {
		dump("bare invocation did not default to Cozy")
	}
	run("send-keys", "-t", "door", "v")
	waitFor("view selector", func() bool {
		return strings.Contains(run("capture-pane", "-p", "-t", "door"), "Compact — more projects")
	})
	run("send-keys", "-t", "door", "Down", "Enter")
	waitFor("Compact preference saved", func() bool {
		data, err := os.ReadFile(filepath.Join(dir, ".autarch", "display.yaml"))
		return err == nil && strings.Contains(string(data), "density: compact") &&
			strings.Contains(run("capture-pane", "-p", "-t", "door"), "View: Compact")
	})
	// SGR mouse input clicks the visible Time range control (one-based x/y).
	run("send-keys", "-t", "door", "-l", "\x1b[<0;28;3M\x1b[<0;28;3m")
	waitFor("time range opened by mouse", func() bool {
		return strings.Contains(run("capture-pane", "-p", "-t", "door"), "Last 30 days")
	})
	run("send-keys", "-t", "door", "Down", "Down", "Enter")
	waitFor("three-day range applied", func() bool {
		return strings.Contains(run("capture-pane", "-p", "-t", "door"), "Showing Last 3 days")
	})
	run("send-keys", "-t", "door", "w", "Home", "Enter")
	waitFor("opening range restored", func() bool {
		return strings.Contains(run("capture-pane", "-p", "-t", "door"), "Showing Last 24 hours")
	})
	run("send-keys", "-t", "door", "a")
	waitFor("saved question to render", func() bool {
		return strings.Contains(run("capture-pane", "-p", "-t", "door"), "saved question · agent stopped")
	})
	run("send-keys", "-t", "door", "Enter")
	waitFor("question and supporting evidence", func() bool {
		cap := run("capture-pane", "-p", "-t", "door")
		return strings.Contains(cap, "Should the reader open") && strings.Contains(cap, "The reader passes the keyboard check") && strings.Contains(cap, "Compact overview")
	})
	if _, err := os.Stat(resumeArgs); !os.IsNotExist(err) {
		t.Fatal("opening evidence started an agent")
	}
	run("send-keys", "-t", "door", "s")
	waitFor("explicit resume with original ID", func() bool { b, e := os.ReadFile(resumeArgs); return e == nil && string(b) == "--resume\n"+id+"\n" })
	run("send-keys", "-t", "door", "Escape")
	run("send-keys", "-t", "door", "Escape")
	run("send-keys", "-t", "door", "Tab")

	// The rendered door proves clauses (a) and (c) against a live server:
	// count on the row, fraction plus named unresolvable in the footer. The
	// rows' own footer key is required too: the briefing names both gardens
	// in its could-not-read line, so the names alone do not prove the Tab
	// has landed.
	waitFor("door to render both axes", func() bool {
		cap := run("capture-pane", "-p", "-t", "door")
		return strings.Contains(cap, "aaa") && strings.Contains(cap, "bbb") &&
			strings.Contains(cap, "enter switch/open") &&
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
		return strings.TrimSpace(run("list-clients", "-F", "#{client_session}")) == work
	})

	// z: the card opens in Zed regardless of sessions.
	run("send-keys", "-t", "door", "z")
	waitFor("zed stub to receive the card path", func() bool {
		got, err := os.ReadFile(zedArgs)
		return err == nil && strings.Contains(string(got), filepath.Join(projA, "docs", "why.md"))
	})
	// A structured answer in the source removes the request after refresh.
	answer := `{"type":"user","timestamp":"2026-09-04T10:03:00Z","message":{"content":[{"type":"tool_result","tool_use_id":"ask1","content":"User has answered your questions: List"}]}}`
	if err := os.WriteFile(transcript, []byte(userRequest+"\n"+assistantContext+"\n"+questionTool+"\n"+answer+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run("send-keys", "-t", "door", "r")
	run("send-keys", "-t", "door", "a")
	waitFor("answered question removed on refresh", func() bool {
		return strings.Contains(run("capture-pane", "-p", "-t", "door"), "No question without a later reply")
	})
	run("send-keys", "-t", "door", "q")
	waitFor("visit saved on quit", func() bool {
		_, err := os.Stat(filepath.Join(dir, ".autarch", "last-visit"))
		return err == nil
	})
	run("new-session", "-d", "-s", "reopened", "-c", dir, "-x", "120", "-y", "30", doorCmd)
	waitFor("Compact remembered on bare reopen", func() bool {
		cap := run("capture-pane", "-p", "-t", "reopened")
		return strings.Contains(cap, "View: Compact") && strings.Contains(cap, "Since last visit")
	})
	run("send-keys", "-t", "reopened", "d")
	waitFor("direct toggle back to Cozy", func() bool {
		return strings.Contains(run("capture-pane", "-p", "-t", "reopened"), "View: Cozy")
	})
	run("send-keys", "-t", "reopened", "3")
	waitFor("project rows for onboarding", func() bool { return strings.Contains(run("capture-pane", "-p", "-t", "reopened"), "enter switch/open") })
	run("send-keys", "-t", "reopened", "i")
	waitFor("product context loaded", func() bool { return strings.Contains(run("capture-pane", "-p", "-t", "reopened"), "CURRENT WORK") })
	run("send-keys", "-t", "reopened", "6")
	waitFor("foundation inventory", func() bool {
		cap := run("capture-pane", "-p", "-t", "reopened")
		return strings.Contains(cap, "Mission · Sources found") && strings.Contains(cap, "Vision · Not found")
	})
	run("send-keys", "-t", "reopened", "n")
	waitFor("onboarding brief", func() bool {
		return strings.Contains(run("capture-pane", "-p", "-t", "reopened"), "Onboard aaa to its product foundation")
	})
	run("send-keys", "-t", "reopened", "c")
	waitFor("portable brief copied", func() bool {
		data, err := os.ReadFile(clipboardPath)
		return err == nil && strings.Contains(string(data), "Project: "+projA) && strings.Contains(string(data), "MISSION.md (read)") && strings.Contains(string(data), "docs/decisions/001-local.md") && strings.Contains(string(data), "AskUserQuestion")
	})
	data, err := os.ReadFile(filepath.Join(projA, "MISSION.md"))
	if err != nil || string(data) != "# Mission\nHelp readers preserve context.\n" {
		t.Fatal("onboarding changed a source")
	}
	run("send-keys", "-t", "reopened", "Escape")
	waitFor("back from brief", func() bool {
		return strings.Contains(run("capture-pane", "-p", "-t", "reopened"), "PROJECT FOUNDATION")
	})
	productFile(t, projA, "VISION.md", "# Vision\nA workspace that remembers.\n")
	run("send-keys", "-t", "reopened", "r")
	waitFor("new source found after refresh", func() bool {
		return strings.Contains(run("capture-pane", "-p", "-t", "reopened"), "Vision · Sources found")
	})
	run("resize-window", "-t", "reopened", "-x", "40", "-y", "16")
	run("send-keys", "-t", "reopened", "n")
	waitFor("narrow onboarding brief", func() bool { return strings.Contains(run("capture-pane", "-p", "-t", "reopened"), "Onboard aaa") })
	run("send-keys", "-t", "reopened", "End")
	waitFor("onboarding brief tail reachable", func() bool {
		// A wrapped sentence is separated by the frame's vertical borders.
		cap := strings.ReplaceAll(run("capture-pane", "-p", "-t", "reopened"), "│", "")
		return strings.Contains(oneLine(cap), "completed onboarding.")
	})
}

func writeExec(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}
