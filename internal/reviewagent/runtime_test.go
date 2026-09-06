package reviewagent

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/pkg/review"
)

type inputBuffer struct{ bytes.Buffer }

func (b *inputBuffer) Close() error { return nil }

func installFakeFlere(t *testing.T, script string) string {
	t.Helper()
	bin := t.TempDir()
	authRoot, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	paths, _ := json.Marshal(map[string]string{"authPath": filepath.Join(authRoot, "auth.json"), "lockPath": filepath.Join(authRoot, "auth.json.lock")})
	// The preflight returns metadata only; these fixtures never initialize or
	// access a real credential file. Runtime behavior begins after this branch.
	wrapper := "#!/bin/sh\nif [ \"$1\" = --auth-storage-prepare ]; then\n printf '%s\\n' '" + string(paths) + "'\n exit 0\nfi\n" + strings.TrimPrefix(script, "#!/bin/sh\n")
	if err := os.WriteFile(filepath.Join(bin, "flere"), []byte(wrapper), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return filepath.Join(bin, "flere")
}

func fakeRuntimeEngine(t *testing.T, script string) (*Engine, string, string) {
	t.Helper()
	if runtime.GOOS != "darwin" {
		t.Skip("Mac investigation adapter")
	}
	root, _ := filepath.EvalSymlinks(t.TempDir())
	store, err := review.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fixtureBinary := installFakeFlere(t, script)
	e := New(store)
	t.Cleanup(func() {
		e.mu.Lock()
		var clients []*conversation
		for _, c := range e.runtimes {
			clients = append(clients, c)
		}
		e.mu.Unlock()
		for _, c := range clients {
			_ = c.in.Close()
			_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
			select {
			case <-c.done:
			case <-time.After(3 * time.Second):
				t.Error("fake runtime cleanup did not complete")
			}
		}
	})
	return e, root, fixtureBinary
}

func TestScanSwitchRuntimeKeepsNewHandoffAfterOldExit(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "respond.py")
	code := "import sys,json\nfor line in sys.stdin:\n r=json.loads(line)\n if r.get('type')=='prompt': print(json.dumps({'type':'response','command':'prompt','id':r['id'],'success':True}),flush=True)\n"
	if err := os.WriteFile(script, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	e, root, _ := fakeRuntimeEngine(t, "#!/bin/sh\nexec "+strconv.Quote(python)+" "+strconv.Quote(script)+"\n")
	old, err := e.start(root)
	if err != nil {
		t.Fatal(err)
	}
	var choices []string
	for _, model := range []string{"old/superseded", "chosen/current"} {
		r := e.store.Apply(review.Request{Version: review.Version, ID: review.NewID(), Method: "runtime.switch", Project: root, Text: model})
		if r.Error != "" {
			t.Fatal(r.Error)
		}
		choices = append(choices, r.ID)
	}
	e.scan()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		e.mu.Lock()
		current := e.runtimes[root]
		e.mu.Unlock()
		if current != old {
			select {
			case <-old.done:
			default:
				t.Fatal("replacement started before old cleanup")
			}
			statuses := map[string]string{}
			for _, turn := range e.store.Snapshot().Turns {
				statuses[turn.ID] = turn.Delivery
			}
			if statuses[choices[1]] == "sending" {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			if statuses[choices[0]] != "superseded" || statuses[choices[1]] != "delivered" {
				t.Fatalf("handoff lost to cleanup: %v", statuses)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("scan never completed the model handoff")
}

func TestSwitchRuntimeBoundsWaitWhenEscapedChildHoldsStdout(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mac investigation adapter")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "escaped.py")
	code := "import os,signal,time,sys,json\nif '--model' in sys.argv:\n for line in sys.stdin:\n  r=json.loads(line)\n  if r.get('type')=='prompt': print(json.dumps({'type':'response','command':'prompt','id':r['id'],'success':True}),flush=True)\n sys.exit(0)\nsignal.signal(signal.SIGTERM,signal.SIG_IGN)\npid=os.fork()\nif pid==0:\n os.setsid()\n time.sleep(30)\n os._exit(0)\nwith open(os.environ['TMPDIR']+'/escaped.pid','w') as f: f.write(str(pid))\nwhile True: time.sleep(1)\n"
	if err := os.WriteFile(script, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	e, root, _ := fakeRuntimeEngine(t, "#!/bin/sh\nexec "+strconv.Quote(python)+" "+strconv.Quote(script)+" \"$@\"\n")
	killChildren := func() {
		paths, _ := filepath.Glob(filepath.Join(e.store.Dir(), "runtime", "*", "escaped.pid"))
		for _, path := range paths {
			data, _ := os.ReadFile(path)
			pid, _ := strconv.Atoi(string(data))
			if pid > 0 {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	}
	t.Cleanup(killChildren)
	old, err := e.start(root)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		paths, _ := filepath.Glob(filepath.Join(e.store.Dir(), "runtime", "*", "escaped.pid"))
		if len(paths) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("fixture child did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	r := e.store.Apply(review.Request{Version: review.Version, ID: review.NewID(), Method: "runtime.switch", Project: root, Text: "chosen/current"})
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	e.scan()
	select {
	case <-old.done:
	case <-time.After(7 * time.Second):
		t.Error("escaped stdout wedged the old runtime and global start mutex")
		killChildren()
		select {
		case <-old.done:
		case <-time.After(2 * time.Second):
			t.Fatal("fixture failed to stop")
		}
	}
	// The handoff must also release the shared start lock for another project.
	other, _ := filepath.EvalSymlinks(t.TempDir())
	if r := e.store.Apply(review.Request{Version: review.Version, ID: review.NewID(), Method: "runtime.switch", Project: other, Text: "chosen/current"}); r.Error != "" {
		t.Fatal(r.Error)
	}
	started := make(chan error, 1)
	go func() { _, err := e.start(other); started <- err }()
	select {
	case err := <-started:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("another project's start remained blocked")
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, turn := range e.store.Snapshot().Turns {
			if turn.ID == r.ID && turn.Delivery == "delivered" {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("handoff did not finish after its old pipe was closed")
}

func TestSelectionDuringRuntimeStopSupersedesTheOldHandoff(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "stop-window.py")
	code := "import os,signal,time,sys,json\nif '--model' in sys.argv:\n for line in sys.stdin:\n  r=json.loads(line)\n  if r.get('type')=='prompt': print(json.dumps({'type':'response','command':'prompt','id':r['id'],'success':True}),flush=True)\n sys.exit(0)\nsignal.signal(signal.SIGTERM,signal.SIG_IGN)\nroot=os.environ['TMPDIR']\nopen(root+'/ready','w').close()\nfor line in sys.stdin: pass\nopen(root+'/stopping','w').close()\nwhile not os.path.exists(root+'/release'): time.sleep(.01)\n"
	if err := os.WriteFile(script, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	e, root, _ := fakeRuntimeEngine(t, "#!/bin/sh\nexec "+strconv.Quote(python)+" "+strconv.Quote(script)+" \"$@\"\n")
	old, err := e.start(root)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(e.store.Dir(), "runtime", old.session)
	waitForFile := func(name string) {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("fixture did not reach %s", name)
	}
	waitForFile("ready")
	choose := func(model string) string {
		t.Helper()
		r := e.store.Apply(review.Request{Version: review.Version, ID: review.NewID(), Method: "runtime.switch", Project: root, Text: model})
		if r.Error != "" {
			t.Fatal(r.Error)
		}
		return r.ID
	}
	first := choose("chosen/first")
	e.scan()
	waitForFile("stopping")
	latest := choose("chosen/latest")
	if err := os.WriteFile(filepath.Join(dir, "release"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	statuses := map[string]string{}
	for time.Now().Before(deadline) {
		for _, turn := range e.store.Snapshot().Turns {
			statuses[turn.ID] = turn.Delivery
		}
		if statuses[first] == "delivered" {
			t.Fatalf("superseded selection reported delivered: %v", statuses)
		}
		if statuses[first] == "superseded" && statuses[latest] == "delivered" {
			return
		}
		if statuses[first] == "superseded" {
			e.scan()
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("latest selection did not replace the stop-window handoff: %v", statuses)
}

func TestSelectionDuringRuntimePreflightCannotUseTheOlderLabel(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(t.TempDir(), "respond.py")
	code := "import sys,json\nfor line in sys.stdin:\n r=json.loads(line)\n if r.get('type')=='prompt': print(json.dumps({'type':'response','command':'prompt','id':r['id'],'success':True}),flush=True)\n"
	if err := os.WriteFile(script, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	e, root, bin := fakeRuntimeEngine(t, "#!/bin/sh\nexec "+strconv.Quote(python)+" "+strconv.Quote(script)+"\n")
	if _, err := e.start(root); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(bin)
	if err != nil {
		t.Fatal(err)
	}
	gate := t.TempDir()
	wrapper := "#!/bin/sh\nif [ \"$1\" = --auth-storage-prepare ]; then\n touch " + strconv.Quote(filepath.Join(gate, "waiting")) + "\n while [ ! -e " + strconv.Quote(filepath.Join(gate, "release")) + " ]; do sleep .01; done\nfi\n" + strings.TrimPrefix(string(original), "#!/bin/sh\n")
	if err := os.WriteFile(bin, []byte(wrapper), 0700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.WriteFile(filepath.Join(gate, "release"), nil, 0600) })
	first := e.store.Apply(review.Request{Version: review.Version, ID: review.NewID(), Method: "runtime.switch", Project: root, Text: "chosen/first"})
	if first.Error != "" {
		t.Fatal(first.Error)
	}
	e.scan()
	deadline := time.Now().Add(3 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(gate, "waiting")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("replacement never reached credential preflight")
		}
		time.Sleep(10 * time.Millisecond)
	}
	latest := e.store.Apply(review.Request{Version: review.Version, ID: review.NewID(), Method: "runtime.switch", Project: root, Text: "chosen/latest"})
	if latest.Error != "" {
		t.Fatal(latest.Error)
	}
	if err := os.WriteFile(filepath.Join(gate, "release"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		for _, turn := range e.store.Snapshot().Turns {
			if turn.ID == first.ID {
				if turn.Delivery == "delivered" {
					t.Fatal("a newer preflight selection was delivered under the previous selection's label")
				}
				if turn.Delivery == "superseded" {
					return
				}
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the preflight selection did not supersede the previous handoff")
}

func TestQuestionRoundTripAdvancesAndCancelledQuestionCannotAnswer(t *testing.T) {
	root, _ := filepath.EvalSymlinks(t.TempDir())
	store, _ := review.Open(t.TempDir())
	e := New(store)
	input := &inputBuffer{}
	c := &conversation{project: root, session: "original-session", in: input}
	e.runtimes[root] = c
	emit := func(data string) {
		t.Helper()
		var event map[string]json.RawMessage
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			t.Fatal(err)
		}
		e.event(c, event)
	}
	emit(`{"type":"extension_ui_request","method":"select","id":"first","title":"Which outcome?","options":["Readable","Dense"]}`)
	req := review.Request{Version: review.Version, ID: "answer", Method: "question.answer", Project: root, Target: "first", Text: "Readable"}
	if response := store.Apply(req); response.Error != "" {
		t.Fatal(response.Error)
	}
	e.Handle(req)
	if !strings.Contains(input.String(), `"value":"Readable"`) {
		t.Fatal("answer did not reach original runtime")
	}
	emit(`{"type":"extension_ui_request","method":"input","id":"next","title":"Which detail matters?"}`)
	if state := store.Snapshot(); len(state.Questions) != 2 || state.Questions[1].Status != "pending" {
		t.Fatal("next question missing")
	}
	e.cancelQuestions(root, "original-session")
	req.ID = "late"
	req.Target = "next"
	if response := store.Apply(req); response.Error == "" {
		t.Fatal("cancelled question accepted late answer")
	}
}
func TestJSONLPreservesUnicodeSeparators(t *testing.T) {
	store, _ := review.Open(t.TempDir())
	root := t.TempDir()
	e := New(store)
	c := &conversation{project: root, session: "test", in: &inputBuffer{}}
	e.read(c, strings.NewReader("{\"type\":\"message_end\",\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"first\u2028second\"}]}}\n"))
	if got := store.Snapshot().Turns; len(got) != 1 || got[0].Text != "first\u2028second" {
		t.Fatalf("framing changed: %+v", got)
	}
}

func TestRuntimeUsesOnlyLatestNonfailedProjectModelSelection(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Mac investigation adapter")
	}
	root, _ := filepath.EvalSymlinks(t.TempDir())
	if err := os.MkdirAll(filepath.Join(root, ".autarch"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".autarch", "review.json"), []byte(`{"version":1,"tracker":".","provider":"configured","model":"default"}`), 0600); err != nil {
		t.Fatal(err)
	}
	store, _ := review.Open(t.TempDir())
	installFakeFlere(t, "#!/bin/sh\nexec /bin/cat >/dev/null\n")
	for _, model := range []string{"first/old", "chosen/current", "bad/failed"} {
		r := store.Apply(review.Request{Version: review.Version, ID: review.NewID(), Method: "runtime.switch", Project: root, Text: model})
		if r.Error != "" {
			t.Fatal(r.Error)
		}
		status := "delivered"
		if model == "bad/failed" {
			status = "failed"
		}
		store.Apply(review.Request{Version: review.Version, ID: review.NewID(), Method: "turn.delivery", Project: root, Target: r.ID, Status: status})
	}
	e := New(store)
	c, err := e.start(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.in.Close(); <-c.done })
	var identities []string
	for i, arg := range c.cmd.Args {
		if (arg == "--provider" || arg == "--model") && i+1 < len(c.cmd.Args) {
			identities = append(identities, arg+"="+c.cmd.Args[i+1])
		}
	}
	if strings.Join(identities, " ") != "--provider=chosen --model=current" {
		t.Fatalf("historical model flags leaked: %v", identities)
	}
}

func TestLiveFlereHandshake(t *testing.T) {
	if os.Getenv("AUTARCH_LIVE_FLERE") != "1" || runtime.GOOS != "darwin" {
		t.Skip("explicit real-runtime smoke")
	}
	dir, err := os.MkdirTemp("/tmp", "autarch-flere-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	root := t.TempDir()
	store, _ := review.Open(dir)
	e := New(store)
	c, err := e.start(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = c.in.Close()
		_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGTERM)
		select {
		case <-c.done:
		case <-time.After(5 * time.Second):
			_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
			<-c.done
		}
	}()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		for _, turn := range store.Snapshot().Turns {
			if strings.Contains(turn.Text, "Original Flere session:") {
				t.Log(turn.Text)
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "runtime", "*", "runtime.log"))
	for _, file := range files {
		f, _ := os.Open(file)
		if f != nil {
			data, _ := io.ReadAll(io.LimitReader(f, 5000))
			f.Close()
			t.Log(string(data))
		}
	}
	t.Fatal("real Flere handshake never arrived")
}

func TestLiveFlereQuestions(t *testing.T) {
	if os.Getenv("AUTARCH_LIVE_FLERE_QUESTIONS") != "1" || runtime.GOOS != "darwin" {
		t.Skip("explicit provider-backed protocol smoke")
	}
	dir, err := os.MkdirTemp("/tmp", "autarch-roundtrip-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	root, _ := filepath.EvalSymlinks(t.TempDir())
	store, _ := review.Open(dir)
	e := New(store)
	c, err := e.start(root)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = c.in.Close()
		_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGTERM)
		select {
		case <-c.done:
		case <-time.After(5 * time.Second):
			_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
			<-c.done
		}
	}()
	err = c.send(map[string]any{"type": "prompt", "id": "smoke", "message": "This is a protocol test with fictional decisions, not product approval. Call ask_review_question with question 'Protocol first?' options ['One', 'Two']. After receiving the answer, immediately call ask_review_question again with question 'Protocol second?' options ['Three', 'Four']. After that answer, reply only ROUNDTRIP_OK. Do not read files or propose changes."})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(120 * time.Second)
	answered := map[string]bool{}
	for time.Now().Before(deadline) {
		state := store.Snapshot()
		for _, q := range state.Questions {
			if q.Status == "pending" && !answered[q.ID] {
				if len(q.Options) == 0 {
					t.Fatal("expected structured options")
				}
				r := review.Request{Version: review.Version, ID: review.NewID(), Method: "question.answer", Project: root, Target: q.ID, Text: q.Options[0]}
				if response := store.Apply(r); response.Error != "" {
					t.Fatal(response.Error)
				}
				e.Handle(r)
				answered[q.ID] = true
			}
		}
		for _, turn := range state.Turns {
			if strings.Contains(turn.Text, "Flere rejected prompt") {
				t.Fatal(turn.Text)
			}
			if turn.Kind == "Flere" && strings.Contains(turn.Text, "ROUNDTRIP_OK") && len(answered) == 2 {
				t.Log("Two real Flere questions answered in original session", turn.RuntimeSession, turn.Model)
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	for _, turn := range store.Snapshot().Turns {
		t.Log(turn.Kind, turn.Text)
	}
	t.Fatal("provider-backed question sequence did not finish")
}
