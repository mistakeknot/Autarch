package review

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/pkg/agenttransport"
)

// TaskIdentity records the observed tracker, qualified task and assignee. It
// does not claim a Clavain dispatch occurred or confer authority on an Execution.
type TaskIdentity struct {
	Tracker       string `json:"tracker"`
	TrackerUUID   string `json:"tracker_uuid,omitempty"`
	BeadID        string `json:"bead_id,omitempty"`
	ID            string `json:"id,omitempty"` // legacy display ID
	Assignee      string `json:"assignee"`
	Qualification string `json:"qualification"` // qualified | unqualified
}
type HandoffDecision struct {
	Literal string `json:"literal"`
	Source  string `json:"source"`
}
type HandoffContext struct {
	Outcome                  string            `json:"outcome"`
	Decisions                []HandoffDecision `json:"decisions"`
	Scope                    string            `json:"scope"`
	AcceptanceChecks         []string          `json:"acceptance_checks"`
	ImplementationAuthorized bool              `json:"implementation_authorized"`
}
type HandoffResults struct {
	Commit        string   `json:"commit,omitempty"`
	Files         []string `json:"files,omitempty"`
	Checks        []string `json:"checks,omitempty"`
	RunnableBuild string   `json:"runnable_build,omitempty"`
}
type HandoffCheck struct {
	Name     string `json:"name"`
	Result   string `json:"result"`
	Evidence string `json:"evidence"`
}
type HandoffVerdict struct {
	Result   string    `json:"result"`
	Actor    string    `json:"actor"`
	Evidence string    `json:"evidence"`
	At       time.Time `json:"at"`
}
type ExternalHandoff struct {
	ID                      string                  `json:"id"`
	Project                 string                  `json:"project"`
	Task                    TaskIdentity            `json:"task"`
	Target                  agenttransport.Target   `json:"target"`
	Message                 string                  `json:"message"`
	Draft                   string                  `json:"draft"`
	Context                 HandoffContext          `json:"context"`
	Kind                    string                  `json:"kind"` // investigation | implementation
	Form                    string                  `json:"form"` // full | delta
	ParentID                string                  `json:"parent_id,omitempty"`
	ApprovedActor           string                  `json:"approved_actor"`
	ApprovedAt              time.Time               `json:"approved_at"`
	IntentAt                time.Time               `json:"intent_at"`
	IntentState             string                  `json:"intent_state"`
	Delivery                agenttransport.Delivery `json:"delivery"`
	Readiness               string                  `json:"readiness"` // ready is an explicit caller observation
	Interruption            agenttransport.Delivery `json:"interruption,omitempty"`
	OpenRequired            bool                    `json:"open_required"`
	OpenedAt                time.Time               `json:"opened_at,omitempty"`
	DeliveryError           string                  `json:"delivery_error,omitempty"`
	Results                 HandoffResults          `json:"results"`
	AgentReportedCompletion string                  `json:"agent_reported_completion,omitempty"`
	VerifiedChecks          []HandoffCheck          `json:"verified_checks,omitempty"`
	HumanVerdict            *HandoffVerdict         `json:"human_verdict,omitempty"`
}

func (h ExternalHandoff) Clone() ExternalHandoff {
	b, _ := json.Marshal(h)
	var c ExternalHandoff
	_ = json.Unmarshal(b, &c)
	return c
}

// RenderedMessage is the exact transport payload. A delta is deliberately
// small; only the durable parent establishes its context.
func (h ExternalHandoff) RenderedMessage() string {
	if h.Form == "delta" {
		return h.Draft
	}
	var out strings.Builder
	fmt.Fprintf(&out, "# External agent handoff\n\nOutcome: %s\n", h.Context.Outcome)
	if h.Task.Qualification == "qualified" {
		fmt.Fprintf(&out, "Task: tracker %s · bead %s\n", h.Task.TrackerUUID, h.Task.BeadID)
	} else {
		fmt.Fprintf(&out, "Task: UNQUALIFIED · %s · %s\n", h.Task.Tracker, firstNonempty(h.Task.BeadID, h.Task.ID))
	}
	fmt.Fprintf(&out, "Recorded assignee: %s\nInstruction kind: %s\nImplementation authority: %t\n\n", h.Task.Assignee, h.Kind, h.Context.ImplementationAuthorized)
	out.WriteString("Accepted decisions (literal wording):\n")
	for _, d := range h.Context.Decisions {
		fmt.Fprintf(&out, "- %s\n  Source: %s\n", d.Literal, d.Source)
	}
	fmt.Fprintf(&out, "\nScope:\n%s\n\nAcceptance checks:\n", h.Context.Scope)
	for _, check := range h.Context.AcceptanceChecks {
		fmt.Fprintf(&out, "- %s\n", check)
	}
	fmt.Fprintf(&out, "\nDirection:\n%s", h.Draft)
	return out.String()
}

func firstNonempty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "unknown"
}

func validateTaskIdentity(task TaskIdentity) error {
	if strings.TrimSpace(task.Tracker) == "" || strings.TrimSpace(task.Assignee) == "" {
		return errors.New("task tracker and recorded assignee required")
	}
	switch task.Qualification {
	case "qualified":
		if strings.TrimSpace(task.TrackerUUID) == "" || strings.TrimSpace(task.BeadID) == "" {
			return errors.New("qualified task requires tracker UUID and Bead ID")
		}
	case "unqualified":
		if strings.TrimSpace(task.BeadID) == "" && strings.TrimSpace(task.ID) == "" {
			return errors.New("unqualified task needs an explicit source ID")
		}
	default:
		return errors.New("task qualification must be explicit")
	}
	return nil
}

func validateFullContext(h ExternalHandoff) error {
	if h.Form != "full" {
		return nil
	}
	if strings.TrimSpace(h.Context.Outcome) == "" || strings.TrimSpace(h.Context.Scope) == "" || len(h.Context.AcceptanceChecks) == 0 {
		return errors.New("full handoff requires outcome, scope and acceptance checks")
	}
	for _, d := range h.Context.Decisions {
		if strings.TrimSpace(d.Literal) == "" || strings.TrimSpace(d.Source) == "" {
			return errors.New("accepted decisions require literal wording and source paths")
		}
	}
	for _, check := range h.Context.AcceptanceChecks {
		if strings.TrimSpace(check) == "" {
			return errors.New("acceptance checks cannot be empty")
		}
	}
	if h.Kind == "implementation" && !h.Context.ImplementationAuthorized {
		return errors.New("implementation authority must be explicit")
	}
	return nil
}

func applyHandoff(st *State, r Request, project, id string, now time.Time) (string, error) {
	if project == "" {
		return "", errors.New("handoff project required")
	}
	if st.ExternalHandoffs == nil {
		st.ExternalHandoffs = map[string]ExternalHandoff{}
	}
	if r.Method == "handoff.approve" {
		if r.Handoff == nil || strings.TrimSpace(r.Actor) == "" {
			return "", errors.New("approved message and human actor required")
		}
		h := r.Handoff.Clone()
		if err := validateTaskIdentity(h.Task); err != nil {
			return "", err
		}
		if strings.TrimSpace(h.Draft) == "" {
			return "", errors.New("draft required")
		}
		if h.Kind != "investigation" && h.Kind != "implementation" {
			return "", errors.New("unknown handoff kind")
		}
		if h.Form != "full" && h.Form != "delta" {
			return "", errors.New("handoff must be full or delta")
		}
		if err := validateFullContext(h); err != nil {
			return "", err
		}
		if h.Message != h.RenderedMessage() {
			return "", errors.New("handoff message must equal the reviewed rendered payload")
		}
		if len(h.Message) > 1<<20 || strings.ContainsAny(h.Message, "\x00\x1b\r") {
			return "", errors.New("handoff message is too large or contains terminal control bytes")
		}
		if e := h.Target.Validate(); e != nil {
			return "", e
		}
		if h.ID != "" || h.Project != "" || h.IntentState != "" || h.Delivery != "" || h.Interruption != "" || h.OpenRequired || h.HumanVerdict != nil || len(h.VerifiedChecks) > 0 || h.AgentReportedCompletion != "" || h.Results.Commit != "" || len(h.Results.Files) > 0 || len(h.Results.Checks) > 0 || h.Results.RunnableBuild != "" {
			return "", errors.New("approval cannot import delivery or result authority")
		}
		if h.Readiness == "" {
			h.Readiness = "uncertain"
		}
		if h.Readiness != "ready" && h.Readiness != "busy" && h.Readiness != "uncertain" {
			return "", errors.New("unknown pane readiness")
		}
		if _, exists := st.ExternalHandoffs[id]; exists {
			return "", errors.New("handoff ID already exists")
		}
		if h.ParentID == "" {
			for _, old := range st.ExternalHandoffs {
				if old.Project == project && old.Task == h.Task && old.Target == h.Target && old.Message == h.Message && old.Delivery == agenttransport.Delivered {
					return "", errors.New("this exact message already has a delivered handoff; use a follow up or inspect its result")
				}
			}
		}
		if h.ParentID != "" {
			old, ok := st.ExternalHandoffs[h.ParentID]
			if !ok || old.Project != project || old.Task != h.Task {
				return "", errors.New("follow up requires the same project, task and recorded assignee")
			}
			if old.OpenRequired {
				return "", errors.New("open the original pane before approving a follow up")
			}
			if h.Form == "delta" && (old.Target != h.Target || old.Kind != h.Kind) {
				return "", errors.New("changed session or scope requires a fresh full handoff")
			}
			if h.Form == "delta" && old.Delivery != agenttransport.Delivered {
				return "", errors.New("delta requires delivered parent context; approve a full handoff instead")
			}
		} else if h.Form == "delta" {
			return "", errors.New("delta requires parent handoff")
		}
		h.ID = id
		h.Project = project
		h.ApprovedActor = r.Actor
		h.ApprovedAt = now
		h.IntentState = "approved"
		h.IntentAt = time.Time{}
		h.Delivery = agenttransport.NotSent
		h.OpenedAt = time.Time{}
		h.DeliveryError = ""
		st.ExternalHandoffs[id] = h
		return id, nil
	}
	h, ok := st.ExternalHandoffs[r.Target]
	if !ok || h.Project != project {
		return "", errors.New("handoff project/identity mismatch")
	}
	switch r.Method {
	case "handoff.opened":
		if r.Actor != "human" || r.Handoff == nil || r.Handoff.Target != h.Target {
			return "", errors.New("explicit human open observation required")
		}
		if h.Delivery == agenttransport.InFlight || h.Interruption == agenttransport.InFlight {
			return "", errors.New("input still in flight; open original pane after it settles")
		}
		h.OpenedAt = now
		h.OpenRequired = false
		if h.Interruption != "" {
			h.IntentState = "interrupted"
		}
	case "handoff.report":
		if r.Handoff == nil {
			return "", errors.New("result references required")
		}
		h.Results = r.Handoff.Results
		h.AgentReportedCompletion = r.Handoff.AgentReportedCompletion
		// Revised agent claims invalidate previous check/verdict associations.
		h.VerifiedChecks = nil
		h.HumanVerdict = nil
	case "handoff.verify":
		if r.Handoff == nil || len(r.Handoff.VerifiedChecks) == 0 || r.Actor == "" {
			return "", errors.New("verifier and check evidence required")
		}
		for _, c := range r.Handoff.VerifiedChecks {
			if c.Name == "" || c.Evidence == "" || !verdictValue(c.Result) {
				return "", errors.New("check name, result and evidence required")
			}
		}
		h.VerifiedChecks = r.Handoff.VerifiedChecks
		h.HumanVerdict = nil
	case "handoff.verdict":
		if r.Actor == "" || r.Handoff == nil || r.Handoff.HumanVerdict == nil {
			return "", errors.New("explicit human verdict required")
		}
		v := *r.Handoff.HumanVerdict
		if !verdictValue(v.Result) || v.Evidence == "" {
			return "", errors.New("verdict needs pass/fail/inconclusive and evidence")
		}
		v.Actor = r.Actor
		v.At = now
		h.HumanVerdict = &v
	default:
		return "", errors.New("unknown handoff method")
	}
	st.ExternalHandoffs[h.ID] = h
	return h.ID, nil
}
func verdictValue(s string) bool { return s == "pass" || s == "fail" || s == "inconclusive" }
func (s *Store) recoverHandoffs() error {
	if s.state.ExternalHandoffs == nil {
		s.state.ExternalHandoffs = map[string]ExternalHandoff{}
		return nil
	}
	next := s.state.Clone()
	changed := false
	for id, h := range next.ExternalHandoffs {
		if h.Delivery == agenttransport.InFlight {
			h.Delivery = agenttransport.Uncertain
			h.IntentState = "uncertain"
			h.OpenRequired = true
			changed = true
		}
		if h.Interruption == agenttransport.InFlight {
			h.Interruption = agenttransport.Uncertain
			h.OpenRequired = true
			changed = true
		}
		next.ExternalHandoffs[id] = h
	}
	if changed {
		return s.commit(next)
	}
	return nil
}

// beginHandoff commits the claim before releasing the mutex or calling any
// transport. Neither duplicate requests nor another Store can claim it twice:
// immutable revision publication rejects stale writers without replacing data.
func (s *Store) beginHandoff(id string, interrupt bool) (ExternalHandoff, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h, ok := s.state.ExternalHandoffs[id]
	if !ok {
		return h, false, errors.New("handoff missing")
	}
	// A duplicate returns its immutable delivery receipt even if another draft
	// is now using the pane. Only new input claims need exclusion checks.
	if !interrupt && (h.IntentState != "approved" || h.Delivery != agenttransport.NotSent) {
		return h, false, nil
	}
	// Input operations on the same pane exclude each other, even when the UI
	// holds multiple task drafts. In particular C-c cannot split paste/submit.
	for otherID, other := range s.state.ExternalHandoffs {
		if otherID != id && other.Target.SamePane(h.Target) && (other.Delivery == agenttransport.InFlight || other.Interruption == agenttransport.InFlight || other.OpenRequired) {
			return h, false, errors.New("original pane has another in-flight or unresolved handoff; open it before input")
		}
	}
	if interrupt {
		if h.Interruption != "" || h.OpenRequired || h.Delivery != agenttransport.NotSent {
			return h, false, errors.New("open original pane; interruption cannot be replayed")
		}
		if h.Readiness != "busy" && h.Readiness != "uncertain" {
			return h, false, errors.New("interrupt requires busy or uncertain pane")
		}
		if h.Delivery == agenttransport.InFlight {
			return h, false, errors.New("delivery in progress; open original pane")
		}
		h.Interruption = agenttransport.InFlight
		h.Delivery = agenttransport.InFlight
		h.IntentAt = time.Now().UTC()
		h.IntentState = "interrupting"
		h.OpenRequired = true
	} else {
		if h.Readiness != "ready" || h.OpenRequired {
			return h, false, errors.New("open original pane and review a fresh handoff before sending")
		}
		h.IntentAt = time.Now().UTC()
		h.IntentState = "delivering"
		h.Delivery = agenttransport.InFlight
	}
	next := s.state.Clone()
	next.ExternalHandoffs[id] = h
	if e := s.commit(next); e != nil {
		return h, false, e
	}
	return h.Clone(), true, nil
}
func (s *Store) finishDeliveryHandoff(id string, result agenttransport.Result) agenttransport.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	if result.State != agenttransport.Delivered && result.State != agenttransport.NotSent && result.State != agenttransport.Uncertain {
		result = agenttransport.Result{State: agenttransport.Uncertain, Err: errors.New("invalid transport outcome")}
	}
	if result.Err != nil && result.State == agenttransport.Delivered {
		result.State = agenttransport.Uncertain
	}
	next := s.state.Clone()
	h := next.ExternalHandoffs[id]
	h.Delivery = result.State
	h.IntentState = "finished"
	if result.State == agenttransport.Uncertain {
		h.IntentState = "uncertain"
		h.OpenRequired = true
	}
	if result.Err != nil {
		h.DeliveryError = result.Err.Error()
	}
	next.ExternalHandoffs[id] = h
	if e := s.commit(next); e != nil {
		return agenttransport.Result{State: agenttransport.Uncertain, Err: e}
	}
	return result
}
func (s *Store) DeliverHandoff(ctx context.Context, id string, tr agenttransport.Transport) agenttransport.Result {
	h, claimed, e := s.beginHandoff(id, false)
	if e != nil {
		return agenttransport.Result{State: agenttransport.NotSent, Err: e}
	}
	if !claimed {
		return agenttransport.Result{State: h.Delivery}
	}
	return s.finishDeliveryHandoff(id, tr.Send(ctx, h.Target, h.Message))
}

// Interrupt is an explicit human operation, never part of Send or a retry.
// The transport reports success only after observing provider readiness; only
// then is the already-approved replacement sent. Any uncertainty retains the
// durable draft and requires opening the original pane.
func (s *Store) InterruptHandoff(ctx context.Context, id string, tr agenttransport.Transport) agenttransport.Result {
	h, claimed, e := s.beginHandoff(id, true)
	if e != nil {
		return agenttransport.Result{State: agenttransport.NotSent, Err: e}
	}
	if !claimed {
		return agenttransport.Result{State: h.Interruption}
	}
	interrupted := tr.Interrupt(ctx, h.Target)
	redirected := agenttransport.Result{State: agenttransport.NotSent}
	if interrupted.State == agenttransport.Delivered && interrupted.Err == nil {
		redirected = tr.Send(ctx, h.Target, h.Message)
	}
	return s.finishInterruptHandoff(id, interrupted, redirected)
}

func (s *Store) finishInterruptHandoff(id string, interrupted, redirected agenttransport.Result) agenttransport.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	normalize := func(result agenttransport.Result) agenttransport.Result {
		if result.State != agenttransport.Delivered && result.State != agenttransport.NotSent && result.State != agenttransport.Uncertain {
			return agenttransport.Result{State: agenttransport.Uncertain, Err: errors.New("invalid transport outcome")}
		}
		if result.Err != nil && result.State == agenttransport.Delivered {
			result.State = agenttransport.Uncertain
		}
		return result
	}
	interrupted, redirected = normalize(interrupted), normalize(redirected)
	if interrupted.State != agenttransport.Delivered {
		redirected = agenttransport.Result{State: agenttransport.NotSent}
	}
	next := s.state.Clone()
	h := next.ExternalHandoffs[id]
	h.Interruption = interrupted.State
	h.Delivery = redirected.State
	h.OpenRequired = interrupted.State != agenttransport.Delivered || redirected.State != agenttransport.Delivered
	h.IntentState = "finished"
	if h.OpenRequired {
		h.IntentState = "uncertain"
	}
	var deliveryErrors []string
	if interrupted.Err != nil {
		deliveryErrors = append(deliveryErrors, "interrupt: "+interrupted.Err.Error())
	}
	if redirected.Err != nil {
		deliveryErrors = append(deliveryErrors, "redirect: "+redirected.Err.Error())
	}
	h.DeliveryError = strings.Join(deliveryErrors, "; ")
	next.ExternalHandoffs[id] = h
	if e := s.commit(next); e != nil {
		return agenttransport.Result{State: agenttransport.Uncertain, Err: e}
	}
	if interrupted.State != agenttransport.Delivered {
		return interrupted
	}
	return redirected
}
