package review

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLegacyRecordsPreserveExecutionsAndUpgradeOnlyDurableSchema(t *testing.T) {
	dir := t.TempDir()
	os.Mkdir(filepath.Join(dir, "records"), 0700)
	old := State{Version: 1, Revision: 1, Executions: map[string]Execution{"legacy": {ID: "legacy", ProposalID: "old", Status: "queued"}}}
	data, _ := json.Marshal(old)
	path := filepath.Join(dir, "records", "00000000000000000001.json")
	os.WriteFile(path, data, 0600)
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Executions["legacy"].Status != "queued" {
		t.Fatal("legacy execution lost")
	}
	result := s.Apply(Request{Version: 1, ID: "new-note", Method: "feedback.save", Project: t.TempDir(), Text: "new evidence"})
	if result.Error != "" {
		t.Fatal(result.Error)
	}
	if result.Version != 1 || s.Snapshot().Version != 2 {
		t.Fatalf("wire/durable versions conflated: %+v", result)
	}
	retained, _ := os.ReadFile(path)
	if string(retained) != string(data) {
		t.Fatal("historical record rewritten")
	}
	s, err = Open(dir)
	if err != nil || s.Snapshot().Executions["legacy"].Status != "queued" {
		t.Fatal("restart lost historical authorization", err)
	}
}

func TestVisitCorrectionInvalidatesOnlyUnacceptedSynthesisAndRetainsExactWords(t *testing.T) {
	s, _ := Open(t.TempDir())
	project, _ := filepath.EvalSymlinks(t.TempDir())
	call := func(r Request) Response {
		r.Version = 1
		r.Project = project
		if r.ID == "" {
			r.ID = NewID()
		}
		out := s.Apply(r)
		if out.Error != "" {
			t.Fatal(out.Error)
		}
		return out
	}
	visit := call(Request{Method: "visit.open", Text: "A useful plan"}).ID
	call(Request{Method: "question.save", Question: &Question{ID: "q", RuntimeSession: "runtime", Title: "What matters?", Consequential: true}})
	exact := "  Keep my exact wording.\nSecond line.  "
	call(Request{ID: "a", Method: "question.answer", Target: "q", Text: exact})
	proposal := Proposal{Kind: "guidance", VisitID: visit, AnswerIDs: []string{"a"}, Project: project, Outcome: "Plan", Change: "Retain wording", Scope: []string{"docs"}, Checklist: []string{"Inspect resulting plan"}}
	first := call(Request{Method: "proposal.save", Proposal: &proposal}).ID
	proposal.ID = "accepted"
	call(Request{Method: "proposal.save", Proposal: &proposal})
	call(Request{Method: "proposal.accept", Target: "accepted", Revision: 1, Actor: "fixture-human"})
	corrected := "No, preserve every original answer and this correction."
	call(Request{ID: "correction", Method: "visit.correct", VisitID: visit, Target: "a", Text: corrected})
	state := s.Snapshot()
	if state.Proposals[first].Status != "superseded" || state.Proposals["accepted"].Status != "accepted" || len(state.Executions) != 0 {
		t.Fatal("correction crossed authority boundary")
	}
	answers := state.Visits[visit].Answers
	if len(answers) != 2 || answers[0].Text != exact || answers[1].Text != corrected || answers[1].Supersedes != "a" {
		t.Fatal("answer history rewritten", answers)
	}
	context, err := state.VisitContext(project, 100000)
	if err != nil || !strings.Contains(context, corrected) {
		t.Fatal("correction lost in handoff", err)
	}
	if _, err = state.VisitContext(project, 10); err == nil {
		t.Fatal("mandatory guidance silently truncated")
	}
}

func TestVisitReissuesOneQuestionAndPersistsViewOnRestart(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	project, _ := filepath.EvalSymlinks(t.TempDir())
	call := func(r Request) Response {
		r.Version = 1
		r.Project = project
		r.ID = NewID()
		out := s.Apply(r)
		if out.Error != "" {
			t.Fatal(out.Error)
		}
		return out
	}
	id := call(Request{Method: "visit.open"}).ID
	call(Request{Method: "question.save", Question: &Question{ID: "old", RuntimeSession: "old-runtime", Title: "Choose one", Options: []string{"First", "Second"}, Consequential: true}})
	rejected := s.Apply(Request{Version: 1, ID: NewID(), Project: project, Method: "question.save", Question: &Question{ID: "another", Consequential: true}})
	if rejected.Error == "" {
		t.Fatal("second consequential question admitted")
	}
	call(Request{Method: "visit.runtime", VisitID: id, Text: "new-runtime"})
	v := s.Snapshot().Visits[id]
	if v.OpenQuestionID == "old" || v.OpenQuestionID == "" {
		t.Fatal("question not reissued")
	}
	v.View = "map"
	v.Layout = "theme"
	v.Pace = "slow"
	v.Density = "Compact"
	v.Selection = "accepted-ruling"
	call(Request{Method: "visit.view", VisitID: id, Visit: &v})
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	v = s.Snapshot().Visits[id]
	if v.Selection != "accepted-ruling" || v.View != "map" || v.Density != "Compact" {
		t.Fatal("view lost on restart")
	}
	questions := s.Snapshot().Questions
	if len(questions) != 2 || questions[0].Status != "expired" || questions[1].PredecessorID != "old" || questions[1].RuntimeSession != "new-runtime" {
		t.Fatal("runtime question binding lost", questions)
	}
}

func TestVisitOpenPersistsOneResumableProjectVisit(t *testing.T) {
	dir, project := t.TempDir(), t.TempDir()
	s, _ := Open(dir)
	first := s.Apply(Request{Version: 1, ID: "open", Method: "visit.open", Project: project, Text: "One reviewed plan"})
	if first.Error != "" {
		t.Fatal(first.Error)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	again := s.Apply(Request{Version: 1, ID: "reconnect", Method: "visit.open", Project: project})
	if again.Error != "" || first.ID != again.ID {
		t.Fatalf("visit duplicated on reconnect: %+v", again)
	}
}

func TestVisitContextReferencesPreparationArtifactsWithoutCopyingReceipts(t *testing.T) {
	raw := json.RawMessage(`{"private_fixture_payload":"` + strings.Repeat("huge retained receipt", 10000) + `"}`)
	st := State{Preparations: map[string]Preparation{"p": {ID: "p", Project: "fixture", Status: "planning", Request: raw, Receipt: raw}}}
	context, err := st.VisitContext("fixture", 4000)
	if err != nil || strings.Contains(context, "private_fixture_payload") || !strings.Contains(context, digestBytes(raw)) {
		t.Fatal("handoff copied preparation rather than its binding", err)
	}
}
