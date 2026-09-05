package reviewagent

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mistakeknot/autarch/pkg/review"
)

type inputBuffer struct{ bytes.Buffer }

func (b *inputBuffer) Close() error { return nil }
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
	defer func() { _ = c.in.Close(); _ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGTERM) }()
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
	defer func() { _ = c.in.Close(); _ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGTERM) }()
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
