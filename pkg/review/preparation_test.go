package review

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A fabricated local test receipt is never used as runtime acceptance evidence.
func preparedVisitFixture(t *testing.T) (*Store, Proposal, string) {
	t.Helper()
	s, _ := Open(t.TempDir())
	project, _ := filepath.EvalSymlinks(t.TempDir())
	os.WriteFile(filepath.Join(project, "GUIDANCE.md"), []byte("Exact accepted ruling"), 0600)
	now := time.Now().UTC()
	p := Proposal{ID: "synthesis", Project: project, Revision: 1, Kind: "guidance", Status: "accepted", AcceptedActor: "fixture-human", AcceptedAt: &now, Outcome: "A reviewed plan", SourceBindings: map[string]string{"GUIDANCE.md": digestBytes([]byte("Exact accepted ruling"))}}
	s.state.Proposals[p.ID] = p
	bundle := PreparedBundle{Plan: "Preserve the exact accepted ruling.", SynthesisID: p.ID, SynthesisRevision: 1, SynthesisHash: "request-hash", Sources: p.SourceBindings, Specification: &PreparedSpecification{Change: "Implement the reviewed experience", Scope: []string{"src"}, Tracker: project, Build: BuildSpec{Command: []string{"build"}, Checks: [][]string{{"check"}}, Binary: "app"}, BudgetTokens: 1000, Experience: []string{"choice then correction"}, Checklist: []string{"inspect resulting plan"}}}
	b, _ := json.Marshal(bundle)
	bundleHash := digestBytes(b)
	routeFor := func(role, model, producer string) json.RawMessage {
		raw, _ := json.Marshal([]any{map[string]any{"policy_hash": "policy", "selected_model": model, "context_json": map[string]any{"terminal": true, "role": role, "producer_identity": producer, "execution": map[string]string{"model": model}, "result": map[string]any{"exit_code": 0, "failure_class": "success"}, "resolved_route": map[string]any{"frontier_required": true, "policy_hash": "policy"}}}})
		return raw
	}
	plannerRoute := routeFor("planning", "fixture-planner", "")
	route := routeFor("plan-review", "fixture-reviewer", "fixture-planner")
	reviewHash := digestBytes([]byte("fixture review"))
	final := digestBytes([]byte(bundleHash + "\n" + reviewHash + "\n" + digestBytes(route)))
	raw, _ := json.Marshal(map[string]any{"status": "reviewed", "hash": "request-hash", "policy_hash": "policy", "sources": p.SourceBindings, "bundle_digest": final, "ratification": map[string]any{"status": "persisted", "commit": "fixture-commit", "files": map[string]any{"GUIDANCE.md": map[string]any{"new": p.SourceBindings["GUIDANCE.md"]}}}, "attempts": []any{map[string]any{"bundle": json.RawMessage(b), "bundle_digest": bundleHash, "verdict": "pass", "planner": map[string]any{"complete": true, "usage_complete": true, "model": "fixture-planner", "route": plannerRoute}, "reviewer": map[string]any{"complete": true, "usage_complete": true, "model": "fixture-reviewer", "route": route, "digest": reviewHash}}}})
	prep := Preparation{ID: preparationKey(p), Project: project, ProposalID: p.ID, ProposalRevision: 1, Status: "reviewed", Receipt: raw}
	s.state.Preparations[prep.ID] = prep
	return s, p, final
}

func TestExecutionStartRequiresDisplayedBundleAndIsIdempotent(t *testing.T) {
	s, p, digest := preparedVisitFixture(t)
	req := Request{Version: 1, ID: NewID(), Method: "execution.start", Project: p.Project, Target: p.ID, Revision: 1, Text: digest, Actor: "fixture-human"}
	wrong := req
	wrong.ID = NewID()
	wrong.Text = "unreviewed"
	if s.Apply(wrong).Error == "" {
		t.Fatal("unreviewed plan started")
	}
	wrong = req
	wrong.ID = NewID()
	wrong.Actor = ""
	if s.Apply(wrong).Error == "" {
		t.Fatal("no human approval actor required")
	}
	first := s.Apply(req)
	if first.Error != "" {
		t.Fatal(first.Error)
	}
	req.ID = NewID()
	second := s.Apply(req)
	if second.Error != "" || second.ID != first.ID {
		t.Fatal("duplicate start", second)
	}
	state := s.Snapshot()
	if len(state.Executions) != 1 || len(state.Proposals) != 2 {
		t.Fatal("start did not atomically create one pair")
	}
	implementation := state.Proposals[first.ID]
	if len(implementation.Guidance) != 0 || implementation.Kind != "implementation" || implementation.BudgetTokens != 1000 {
		t.Fatal("implementation specification drift", implementation)
	}
	reopened, err := Open(s.Dir())
	if err != nil || len(reopened.Snapshot().Executions) != 1 {
		t.Fatal("restart lost authorization", err)
	}
}

func TestExecutionStartBlocksChangedGuidanceAndReviewBindings(t *testing.T) {
	s, p, digest := preparedVisitFixture(t)
	os.WriteFile(filepath.Join(p.Project, "GUIDANCE.md"), []byte("Changed ruling"), 0600)
	req := Request{Version: 1, ID: NewID(), Method: "execution.start", Project: p.Project, Target: p.ID, Revision: 1, Text: digest, Actor: "fixture-human"}
	if s.Apply(req).Error == "" {
		t.Fatal("changed guidance started execution")
	}
	if len(s.Snapshot().Executions) != 0 {
		t.Fatal("failed start partially materialized")
	}
	s, p, _ = preparedVisitFixture(t)
	prep := s.state.Preparations[preparationKey(p)]
	prep.Receipt = json.RawMessage(strings.ReplaceAll(string(prep.Receipt), "fixture-reviewer", "fixture-planner"))
	if _, _, err := ReviewedPreparation(prep, p); err == nil {
		t.Fatal("same reviewer accepted")
	}
	if s.Apply(Request{Version: 1, ID: NewID(), Method: "preparation.save", Project: p.Project}).Error == "" {
		t.Fatal("agent can confer reviewed status")
	}
}
