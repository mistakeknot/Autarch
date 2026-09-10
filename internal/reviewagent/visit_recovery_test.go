package reviewagent

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mistakeknot/autarch/pkg/review"
)

func visitEngineFixture(t *testing.T) (*Engine, string) {
	t.Helper()
	project, _ := filepath.EvalSymlinks(t.TempDir())
	store, err := review.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	response := store.Apply(review.Request{Version: review.Version, ID: review.NewID(), Method: "visit.open", Project: project})
	if response.Error != "" {
		t.Fatal(response.Error)
	}
	return New(store), project
}
func visitEvent(t *testing.T, e *Engine, c *conversation, payload string) {
	t.Helper()
	var event map[string]json.RawMessage
	if err := json.Unmarshal([]byte(payload), &event); err != nil {
		t.Fatal(err)
	}
	e.event(c, event)
}
func TestControllerRestartReissuesVisitQuestion(t *testing.T) {
	e, project := visitEngineFixture(t)
	q := review.Question{ID: "old-q", Project: project, RuntimeSession: "old", Method: "input", Title: "Which exact outcome?", Consequential: true}
	response := e.store.Apply(review.Request{Version: review.Version, ID: review.NewID(), Method: "question.save", Project: project, Question: &q})
	if response.Error != "" {
		t.Fatal(response.Error)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e.Run(ctx)
	if e.store.Snapshot().Questions[0].Status != "pending" {
		t.Fatal("startup cancelled retained question")
	}
	c := &conversation{project: project, session: "new", in: &inputBuffer{}}
	visitEvent(t, e, c, `{"type":"response","command":"get_state","success":true,"data":{"sessionId":"new","model":{"provider":"fixture","id":"model"}}}`)
	questions := e.store.Snapshot().Questions
	if len(questions) != 2 || questions[0].Status != "expired" || questions[1].PredecessorID != "old-q" || questions[1].RuntimeSession != "new" || questions[1].Title != q.Title || questions[1].Status != "pending" {
		t.Fatalf("question not reissued: %+v", questions)
	}
}
func TestRejectedSecondQuestionReleasesRuntimeToolCall(t *testing.T) {
	e, project := visitEngineFixture(t)
	input := &inputBuffer{}
	c := &conversation{project: project, session: "new", in: input}
	visitEvent(t, e, c, `{"type":"extension_ui_request","method":"input","id":"first","title":"Existing question"}`)
	visitEvent(t, e, c, `{"type":"extension_ui_request","method":"input","id":"second","title":"Rejected question"}`)
	var response struct {
		ID        string `json:"id"`
		Cancelled bool   `json:"cancelled"`
	}
	if err := json.Unmarshal(input.Bytes(), &response); err != nil || response.ID != "second" || !response.Cancelled {
		t.Fatal("runtime left waiting", err, input.String())
	}
	questions := e.store.Snapshot().Questions
	if len(questions) != 1 || questions[0].ID != "first" || questions[0].Status != "pending" {
		t.Fatal("existing question lost", questions)
	}
}
func TestVisitHandoffCarriesLargeCanonicalSourceOnce(t *testing.T) {
	e, project := visitEngineFixture(t)
	source := strings.Repeat("Canonical retained guidance.\n", 6000) + "EXACT FINAL RULING.\n"
	if err := os.WriteFile(filepath.Join(project, "PHILOSOPHY.md"), []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	handoff, err := e.context(project)
	if err != nil || strings.Count(handoff, source) != 1 {
		t.Fatal("large complete guidance not transferred once", err, len(handoff))
	}
}
func TestLiveAutarchVisitHandoffFits(t *testing.T) {
	project := os.Getenv("AUTARCH_VISIT_CONTEXT_PROJECT")
	if project == "" {
		t.Skip("explicit current source context check")
	}
	store, err := review.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := New(store).context(project)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("current project handoff: %d bytes", len(handoff))
}

func TestReissuedAnswerWaitsForRuntimeAcknowledgement(t *testing.T) {
	e, project := visitEngineFixture(t)
	q := review.Question{ID: "original", Project: project, RuntimeSession: "old", Method: "input", Title: "Exact question", Consequential: true}
	e.store.Apply(review.Request{Version: review.Version, ID: review.NewID(), Method: "question.save", Project: project, Question: &q})
	visit, _ := e.store.Snapshot().ProjectVisit(project)
	e.store.Apply(review.Request{Version: review.Version, ID: review.NewID(), Method: "visit.runtime", Project: project, VisitID: visit.ID, Text: "new"})
	questions := e.store.Snapshot().Questions
	current := questions[len(questions)-1]
	input := &inputBuffer{}
	c := &conversation{project: project, session: "new", in: input}
	e.runtimes[project] = c
	r := review.Request{Version: review.Version, ID: review.NewID(), Method: "question.answer", Project: project, Target: current.ID, Text: "Exact answer"}
	if response := e.store.Apply(r); response.Error != "" {
		t.Fatal(response.Error)
	}
	e.Handle(r)
	var sent map[string]any
	if err := json.Unmarshal(input.Bytes(), &sent); err != nil || sent["streamingBehavior"] != "followUp" {
		t.Fatal("answer not queued as followup", err)
	}
	questions = e.store.Snapshot().Questions
	if questions[len(questions)-1].Delivery != "sending" {
		t.Fatal("answer claimed delivered before acknowledgement")
	}
	payload, _ := json.Marshal(map[string]any{"type": "response", "command": "prompt", "id": current.ID, "success": false, "error": "fixture rejection"})
	visitEvent(t, e, c, string(payload))
	questions = e.store.Snapshot().Questions
	if questions[len(questions)-1].Delivery != "unavailable" || questions[len(questions)-1].Answer != "Exact answer" {
		t.Fatal("rejected answer delivery or retained wording incorrect")
	}
}
