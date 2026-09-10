package review

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Preparation struct {
	ID               string          `json:"id"`
	Project          string          `json:"project"`
	ProposalID       string          `json:"proposal_id"`
	ProposalRevision int             `json:"proposal_revision"`
	Actor            string          `json:"actor"`
	BudgetTokens     int             `json:"budget_tokens"`
	Status           string          `json:"status"`
	Reason           string          `json:"reason,omitempty"`
	Request          json.RawMessage `json:"request"`
	Receipt          json.RawMessage `json:"receipt,omitempty"`
	UpdatedAt        time.Time       `json:"updated_at"`
}
type PreparedSpecification struct {
	Change       string    `json:"change"`
	Scope        []string  `json:"scope"`
	Tracker      string    `json:"tracker"`
	Dependencies []string  `json:"dependencies"`
	Build        BuildSpec `json:"build"`
	BudgetTokens int       `json:"budget_tokens"`
	Experience   []string  `json:"experience"`
	Checklist    []string  `json:"checklist"`
}
type PreparedBundle struct {
	Plan              string                 `json:"plan"`
	Specification     *PreparedSpecification `json:"specification"`
	SynthesisID       string                 `json:"synthesis_id"`
	SynthesisRevision int                    `json:"synthesis_revision"`
	SynthesisHash     string                 `json:"synthesis_hash"`
	Sources           map[string]string      `json:"sources"`
}
type PreparedReceipt struct {
	Status       string            `json:"status"`
	Reason       string            `json:"reason"`
	Hash         string            `json:"hash"`
	Request      json.RawMessage   `json:"request"`
	PolicyHash   string            `json:"policy_hash"`
	Sources      map[string]string `json:"sources"`
	BundleDigest string            `json:"bundle_digest"`
	Ratification struct {
		Status string `json:"status"`
		Commit string `json:"commit"`
		Files  map[string]struct {
			New string `json:"new"`
		} `json:"files"`
	} `json:"ratification"`
	Attempts []struct {
		Bundle       json.RawMessage `json:"bundle"`
		BundleDigest string          `json:"bundle_digest"`
		Verdict      string          `json:"verdict"`
		Planner      struct {
			Complete      bool            `json:"complete"`
			UsageComplete bool            `json:"usage_complete"`
			Model         string          `json:"model"`
			Route         json.RawMessage `json:"route"`
		} `json:"planner"`
		Reviewer struct {
			Complete      bool            `json:"complete"`
			UsageComplete bool            `json:"usage_complete"`
			Model         string          `json:"model"`
			Route         json.RawMessage `json:"route"`
			Digest        string          `json:"digest"`
		} `json:"reviewer"`
	} `json:"attempts"`
}

func digestBytes(data []byte) string { sum := sha256.Sum256(data); return hex.EncodeToString(sum[:]) }
func compactJSON(data []byte) []byte {
	var b bytes.Buffer
	if json.Compact(&b, data) != nil {
		return nil
	}
	return b.Bytes()
}
func preparationKey(p Proposal) string {
	return digestBytes([]byte(p.Project + "\n" + p.ID + ":" + fmt.Sprint(p.Revision)))
}

func requestPreparation(st *State, r Request, project string, now time.Time) (string, error) {
	p, ok := st.Proposals[r.Target]
	if !ok || p.Project != project || p.Status != "accepted" || p.Revision != r.Revision || p.AcceptedAt == nil || p.AcceptedActor == "" || r.Actor == "" {
		return "", errors.New("displayed, attributed accepted synthesis required")
	}
	id := preparationKey(p)
	if old, ok := st.Preparations[id]; ok {
		if old.BudgetTokens != r.BudgetTokens || old.Actor != r.Actor {
			return "", errors.New("preparation budget/approval changed; retain the prior preparation")
		}
		if old.Status == "reviewed" {
			return id, nil
		}
		old.Status = "requested"
		old.UpdatedAt = now
		st.Preparations[id] = old
		return id, nil
	}
	if r.BudgetTokens <= 0 {
		return "", errors.New("explicit positive preparation token budget required")
	}
	if len(p.SourceBindings) == 0 {
		return "", errors.New("synthesis must display canonical source bindings")
	}
	request := map[string]any{"version": 1, "key": fmt.Sprintf("%s:%d", p.ID, p.Revision), "project": project, "actor": p.AcceptedActor, "transcriber": "Autarch", "budget_tokens": r.BudgetTokens, "sources": p.SourceBindings, "coverage_gaps": p.Uncertainties, "proposal": p}
	raw, err := json.Marshal(request)
	if err != nil {
		return "", err
	}
	st.Preparations[id] = Preparation{ID: id, Project: project, ProposalID: p.ID, ProposalRevision: p.Revision, Actor: r.Actor, BudgetTokens: r.BudgetTokens, Status: "requested", Request: raw, UpdatedAt: now}
	if v, ok := st.ProjectVisit(project); ok {
		v.PreparationID = id
		v.Status = "preparing"
		v.UpdatedAt = now
		st.Visits[v.ID] = v
	}
	return id, nil
}

// Only the controller's Clavain adapter may record receipts. There is no
// preparation.save IPC operation through which an agent can fabricate review.
func (s *Store) RecordPreparation(id string, raw []byte, callErr error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.state.Clone()
	p, ok := st.Preparations[id]
	if !ok {
		return errors.New("preparation missing")
	}
	if callErr != nil {
		p.Status = "unavailable"
		p.Reason = callErr.Error()
	} else {
		var receipt PreparedReceipt
		if err := json.Unmarshal(raw, &receipt); err != nil {
			return err
		}
		var request struct {
			Project      string   `json:"project"`
			BudgetTokens int      `json:"budget_tokens"`
			Actor        string   `json:"actor"`
			Proposal     Proposal `json:"proposal"`
		}
		if err := json.Unmarshal(receipt.Request, &request); err != nil {
			return err
		}
		accepted := st.Proposals[p.ProposalID]
		if request.Project != p.Project || request.Proposal.ID != p.ProposalID || request.Proposal.Revision != p.ProposalRevision || request.BudgetTokens != p.BudgetTokens || request.Actor != accepted.AcceptedActor {
			return errors.New("unrelated Clavain preparation receipt")
		}
		if receipt.Hash != digestBytes(compactJSON(receipt.Request)) {
			return errors.New("Clavain preparation request hash mismatch")
		}
		p.Status, p.Reason, p.Receipt = receipt.Status, receipt.Reason, json.RawMessage(raw)
		if p.Status == "reviewed" {
			if _, _, err := ReviewedPreparation(p, accepted); err != nil {
				return err
			}
			if v, ok := st.ProjectVisit(p.Project); ok {
				v.Status = "reviewed"
				v.PreparationID = id
				v.UpdatedAt = time.Now().UTC()
				st.Visits[v.ID] = v
			}
		}
	}
	p.UpdatedAt = time.Now().UTC()
	st.Preparations[id] = p
	return s.commit(st)
}
func ReviewedPreparation(p Preparation, synthesis Proposal) (PreparedBundle, PreparedReceipt, error) {
	var r PreparedReceipt
	var b PreparedBundle
	fail := func(message string) (PreparedBundle, PreparedReceipt, error) { return b, r, errors.New(message) }
	if err := json.Unmarshal(p.Receipt, &r); err != nil {
		return b, r, err
	}
	if r.Status != "reviewed" || r.Ratification.Status != "persisted" || r.Ratification.Commit == "" || len(r.Attempts) == 0 {
		return fail("persisted guidance and independent plan review required")
	}
	a := r.Attempts[len(r.Attempts)-1]
	if err := json.Unmarshal(a.Bundle, &b); err != nil {
		return b, r, err
	}
	if b.SynthesisID != synthesis.ID || b.SynthesisRevision != synthesis.Revision || b.SynthesisHash != r.Hash || digestBytes(compactJSON(a.Bundle)) != a.BundleDigest {
		return fail("reviewed plan/synthesis digest mismatch")
	}
	if a.Verdict != "pass" || !a.Planner.Complete || !a.Reviewer.Complete || !a.Planner.UsageComplete || !a.Reviewer.UsageComplete || a.Planner.Model == "" || a.Reviewer.Model == "" || a.Planner.Model == a.Reviewer.Model {
		return fail("complete independently attributable preparation required")
	}
	// Route records are retained as JSON, with their original compact bytes in
	// the bundle digest. Provider normalization was enforced by governed dispatch.
	for index, raw := range []json.RawMessage{a.Planner.Route, a.Reviewer.Route} {
		var routes []struct {
			Policy  string          `json:"policy_hash"`
			Model   string          `json:"selected_model"`
			Context json.RawMessage `json:"context_json"`
		}
		if json.Unmarshal(raw, &routes) != nil || len(routes) != 1 || routes[0].Policy != r.PolicyHash || r.PolicyHash == "" {
			return fail("reviewed routing policy binding mismatch")
		}
		data := routes[0].Context
		var encoded string
		if json.Unmarshal(data, &encoded) == nil {
			data = []byte(encoded)
		}
		var context struct {
			Terminal  bool   `json:"terminal"`
			Role      string `json:"role"`
			Producer  string `json:"producer_identity"`
			Execution struct {
				Model string `json:"model"`
			} `json:"execution"`
			Result struct {
				Exit    int    `json:"exit_code"`
				Failure string `json:"failure_class"`
			} `json:"result"`
			Route struct {
				Frontier bool   `json:"frontier_required"`
				Hash     string `json:"policy_hash"`
			} `json:"resolved_route"`
		}
		if json.Unmarshal(data, &context) != nil {
			return fail("malformed actual routing context")
		}
		role, model, producer := "planning", a.Planner.Model, ""
		if index == 1 {
			role, model, producer = "plan-review", a.Reviewer.Model, a.Planner.Model
		}
		if !context.Terminal || context.Role != role || context.Producer != producer || routes[0].Model != model || context.Execution.Model != model || context.Result.Exit != 0 || context.Result.Failure != "success" || !context.Route.Frontier || context.Route.Hash != r.PolicyHash {
			return fail("actual planner/reviewer role, identity or successful result mismatch")
		}
	}
	if r.BundleDigest != digestBytes([]byte(a.BundleDigest+"\n"+a.Reviewer.Digest+"\n"+digestBytes(compactJSON(a.Reviewer.Route)))) {
		return fail("reviewed bundle binding mismatch")
	}
	return b, r, nil
}
func VerifyGuidanceHashes(project string, hashes map[string]string) error {
	for relative, want := range hashes {
		clean := filepath.Clean(relative)
		if relative == "" || filepath.IsAbs(relative) || clean == ".." || strings.HasPrefix(clean, "../") || clean == ".git" || strings.HasPrefix(clean, ".git/") {
			return errors.New("unsafe guidance binding")
		}
		path := filepath.Join(project, relative)
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return err
		}
		if resolved != path {
			return errors.New("guidance binding follows a symlink")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if digestBytes(data) != want {
			return fmt.Errorf("committed guidance changed: %s", relative)
		}
	}
	return nil
}
func startPreparedExecution(st *State, r Request, project string, now time.Time) (string, error) {
	synthesis, ok := st.Proposals[r.Target]
	if !ok || synthesis.Project != project || synthesis.Status != "accepted" || synthesis.Revision != r.Revision || strings.TrimSpace(r.Actor) == "" {
		return "", errors.New("explicit human approval of the displayed accepted revision required")
	}
	preparation, ok := st.Preparations[preparationKey(synthesis)]
	if !ok {
		return "", errors.New("reviewed preparation required")
	}
	bundle, receipt, err := ReviewedPreparation(preparation, synthesis)
	if err != nil {
		return "", err
	}
	if r.Text != receipt.BundleDigest {
		return "", errors.New("displayed plan bundle changed")
	}
	spec := bundle.Specification
	if spec == nil || spec.Change == "" || spec.BudgetTokens <= 0 || len(spec.Scope) == 0 || len(spec.Checklist) == 0 || len(spec.Experience) == 0 || len(spec.Build.Command) == 0 || len(spec.Build.Checks) == 0 || spec.Build.Binary == "" || spec.Tracker == "" {
		return "", errors.New("reviewed runnable implementation specification unavailable")
	}
	for _, path := range append(append([]string{}, spec.Scope...), spec.Build.Binary) {
		clean := filepath.Clean(path)
		if path == "" || filepath.IsAbs(path) || clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || clean == ".git" || strings.HasPrefix(clean, ".git/") {
			return "", errors.New("reviewed implementation has invalid scope/binary")
		}
	}
	for _, check := range spec.Build.Checks {
		if len(check) == 0 {
			return "", errors.New("reviewed implementation has an empty check")
		}
	}
	for path, hash := range receipt.Sources {
		if hash == "missing" {
			if _, err := os.Stat(filepath.Join(project, path)); !os.IsNotExist(err) {
				return "", fmt.Errorf("reviewed missing source changed: %s", path)
			}
		} else if err = VerifyGuidanceHashes(project, map[string]string{path: hash}); err != nil {
			return "", err
		}
	}
	hashes := map[string]string{}
	for path, f := range receipt.Ratification.Files {
		hashes[path] = f.New
	}
	if err = VerifyGuidanceHashes(project, hashes); err != nil {
		return "", err
	}
	id := digestBytes([]byte(project + "\n" + synthesis.ID + ":" + fmt.Sprint(synthesis.Revision) + "\n" + r.Text))
	if old, ok := st.Executions[id]; ok {
		if old.ApprovedActor != r.Actor || old.PlanBundleDigest != r.Text {
			return "", errors.New("execution identity has different approval")
		}
		return id, nil
	}
	p := Proposal{Kind: "implementation", ID: id, Project: project, Revision: 1, Status: "accepted", AcceptedAt: &now, AcceptedActor: r.Actor, SynthesisID: synthesis.ID, PlanBundleDigest: r.Text, Outcome: synthesis.Outcome, Change: spec.Change, Scope: spec.Scope, Tracker: spec.Tracker, Dependencies: spec.Dependencies, Build: spec.Build, BudgetTokens: spec.BudgetTokens, Checklist: spec.Checklist, Priority: synthesis.Priority, At: now}
	st.Proposals[id] = p
	st.Executions[id] = Execution{ID: id, Project: project, ProposalID: id, ProposalRevision: 1, Status: "queued", GuidanceHashes: hashes, PlanBundleDigest: r.Text, ApprovedActor: r.Actor, ApprovedAt: &now, UpdatedAt: now}
	return id, nil
}
