package review

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mistakeknot/autarch/pkg/agenttransport"
)

func handoffRequest(t *testing.T) Request {
	t.Helper()
	h := &ExternalHandoff{
		Task:   TaskIdentity{Tracker: "workspace-beads", TrackerUUID: "tracker-uuid", BeadID: "Sylveste-fuwn", Assignee: "recorded-seat", Qualification: "qualified"},
		Target: agenttransport.Target{Socket: "/tmp/isolated.sock", ServerPID: 42, ServerStarted: 1700000000, SessionID: "$1", WindowID: "@2", PaneID: "%3", PanePID: 100, Command: "codex"},
		Draft:  "approved\nmessage", Kind: "investigation", Form: "full", Readiness: "ready",
		Context: HandoffContext{Outcome: "safe delivery", Scope: "bounded slice", Decisions: []HandoffDecision{{Literal: "never replay uncertain delivery", Source: "docs/decision.md"}}, AcceptanceChecks: []string{"exactly one send"}},
	}
	h.Message = h.RenderedMessage()
	return Request{Version: Version, ID: "approved-request", Method: "handoff.approve", Project: t.TempDir(), Actor: "human", Handoff: h}
}

type handoffTransport struct {
	mu                sync.Mutex
	sends, interrupts int
	opens             int
	store             *Store
	id                string
	fail              bool
}

func (f *handoffTransport) List(context.Context) ([]agenttransport.Pane, error) { return nil, nil }
func (f *handoffTransport) Observe(context.Context, agenttransport.Target) (string, error) {
	return "", nil
}
func (f *handoffTransport) Open(context.Context, agenttransport.Target) error {
	f.opens++
	return nil
}
func (f *handoffTransport) Send(_ context.Context, _ agenttransport.Target, _ string) agenttransport.Result {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sends++
	h := f.store.Snapshot().ExternalHandoffs[f.id]
	if h.Delivery != agenttransport.InFlight {
		panic("transport invoked before durable intent")
	}
	return agenttransport.Result{State: agenttransport.Delivered}
}
func (f *handoffTransport) Interrupt(context.Context, agenttransport.Target) agenttransport.Result {
	f.interrupts++
	if f.fail {
		return agenttransport.Result{State: agenttransport.Uncertain, Err: errors.New("lost connection")}
	}
	return agenttransport.Result{State: agenttransport.Delivered}
}

func TestHandoffDurableIntentDuplicateAndReload(t *testing.T) {
	dir := t.TempDir()
	s, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	req := handoffRequest(t)
	r := s.Apply(req)
	if r.Error != "" {
		t.Fatal(r.Error)
	}
	tr := &handoffTransport{store: s, id: r.ID}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); s.DeliverHandoff(context.Background(), r.ID, tr) }()
	}
	wg.Wait()
	if tr.sends != 1 {
		t.Fatalf("sent %d times", tr.sends)
	}
	s, e = Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	if replay := s.Apply(req); !replay.Replayed || replay.ID != r.ID {
		t.Fatalf("retry=%+v", replay)
	}
	s.DeliverHandoff(context.Background(), r.ID, tr)
	if tr.sends != 1 {
		t.Fatal("restart redelivered")
	}
	req.Handoff.Message = "changed"
	if s.Apply(req).Error == "" {
		t.Fatal("rebound durable request")
	}
	if len(s.Snapshot().Executions) != 0 {
		t.Fatal("external handoff became execution")
	}
}

func TestHandoffRejectsNewApprovalForExactDeliveredMessage(t *testing.T) {
	s, _ := Open(t.TempDir())
	req := handoffRequest(t)
	r := s.Apply(req)
	tr := &handoffTransport{store: s, id: r.ID}
	if result := s.DeliverHandoff(context.Background(), r.ID, tr); result.State != agenttransport.Delivered {
		t.Fatalf("delivery: %+v", result)
	}
	req.ID = "second-approval"
	if response := s.Apply(req); response.Error == "" {
		t.Fatal("exact delivered message received a new unchained approval")
	}
}

func TestHandoffRestartDuringDeliveryIsUncertain(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	r := s.Apply(handoffRequest(t))
	if _, claimed, e := s.beginHandoff(r.ID, false); e != nil || !claimed {
		t.Fatalf("claim=%v err=%v", claimed, e)
	}
	reloaded, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	h := reloaded.Snapshot().ExternalHandoffs[r.ID]
	if h.Delivery != agenttransport.Uncertain {
		t.Fatalf("restart: %+v", h)
	}
	tr := &handoffTransport{store: reloaded, id: r.ID}
	reloaded.DeliverHandoff(context.Background(), r.ID, tr)
	if tr.sends != 0 {
		t.Fatal("replayed uncertain delivery")
	}
}

func TestHandoffRestartDuringInterruptNeverReplaysRedirect(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	req := handoffRequest(t)
	req.Handoff.Readiness = "busy"
	r := s.Apply(req)
	if _, claimed, err := s.beginHandoff(r.ID, true); err != nil || !claimed {
		t.Fatalf("claim=%v err=%v", claimed, err)
	}
	reloaded, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	h := reloaded.Snapshot().ExternalHandoffs[r.ID]
	if h.Interruption != agenttransport.Uncertain || h.Delivery != agenttransport.Uncertain || !h.OpenRequired {
		t.Fatalf("restart did not fail closed: %+v", h)
	}
	tr := &handoffTransport{store: reloaded, id: r.ID}
	reloaded.InterruptHandoff(context.Background(), r.ID, tr)
	reloaded.DeliverHandoff(context.Background(), r.ID, tr)
	if tr.interrupts != 0 || tr.sends != 0 {
		t.Fatal("restart replayed interrupt or replacement")
	}
}

func TestHandoffInterruptRetainsDraftAndRequiresOpen(t *testing.T) {
	for _, state := range []string{"busy", "uncertain"} {
		t.Run(state, func(t *testing.T) {
			s, _ := Open(t.TempDir())
			req := handoffRequest(t)
			req.Handoff.Readiness = state
			r := s.Apply(req)
			if r.Error != "" {
				t.Fatal(r.Error)
			}
			tr := &handoffTransport{store: s, id: r.ID, fail: true}
			s.DeliverHandoff(context.Background(), r.ID, tr)
			if tr.sends != 0 {
				t.Fatal("sent into busy/unknown pane")
			}
			s.InterruptHandoff(context.Background(), r.ID, tr)
			h := s.Snapshot().ExternalHandoffs[r.ID]
			if h.Message != req.Handoff.Message || !h.OpenRequired || h.Interruption != agenttransport.Uncertain {
				t.Fatalf("lost draft or open requirement: %+v", h)
			}
			s.InterruptHandoff(context.Background(), r.ID, tr)
			s.DeliverHandoff(context.Background(), r.ID, tr)
			if tr.sends != 0 || tr.interrupts != 1 {
				t.Fatal("automatically repeated input")
			}
		})
	}
}

func TestHandoffInterruptRedirectsOnceAfterObservedReadiness(t *testing.T) {
	s, _ := Open(t.TempDir())
	req := handoffRequest(t)
	req.Handoff.Readiness = "busy"
	r := s.Apply(req)
	tr := &handoffTransport{store: s, id: r.ID}

	result := s.InterruptHandoff(context.Background(), r.ID, tr)
	h := s.Snapshot().ExternalHandoffs[r.ID]
	if result.State != agenttransport.Delivered || tr.interrupts != 1 || tr.sends != 1 || h.Interruption != agenttransport.Delivered || h.Delivery != agenttransport.Delivered || h.OpenRequired {
		t.Fatalf("redirect did not complete exactly once: result=%+v handoff=%+v interrupts=%d sends=%d", result, h, tr.interrupts, tr.sends)
	}
	s.InterruptHandoff(context.Background(), r.ID, tr)
	if tr.interrupts != 1 || tr.sends != 1 {
		t.Fatal("duplicate interrupt replayed terminal input")
	}
}

func TestHandoffDeltaCannotSwitchSessionOrTask(t *testing.T) {
	s, _ := Open(t.TempDir())
	req := handoffRequest(t)
	r := s.Apply(req)
	tr := &handoffTransport{store: s, id: r.ID}
	if result := s.DeliverHandoff(context.Background(), r.ID, tr); result.State != agenttransport.Delivered {
		t.Fatalf("parent: %+v", result)
	}
	next := req
	h := *req.Handoff
	next.ID = "delta"
	next.Handoff = &h
	h.ParentID = r.ID
	h.Form = "delta"
	h.Draft = "follow up"
	h.Message = h.RenderedMessage()
	if r := s.Apply(next); r.Error != "" {
		t.Fatal(r.Error)
	}
	next.ID = "wrong-pane"
	h.Target.PaneID = "%4"
	if s.Apply(next).Error == "" {
		t.Fatal("delta allowed session switch")
	}
	h.Form = "full"
	h.Message = h.RenderedMessage()
	if r := s.Apply(next); r.Error != "" {
		t.Fatal(r.Error)
	}
	next.ID = "wrong-task"
	h.Task.ID = "another"
	if s.Apply(next).Error == "" {
		t.Fatal("follow up allowed task switch")
	}
}

func TestHandoffStaleStoreCannotClaimOrOverwrite(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	req := handoffRequest(t)
	r := s.Apply(req)
	stale, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	tr := &handoffTransport{store: s, id: r.ID}
	s.DeliverHandoff(context.Background(), r.ID, tr)
	stale.DeliverHandoff(context.Background(), r.ID, tr)
	if tr.sends != 1 {
		t.Fatal("stale store redelivered")
	}
	current, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	if current.Snapshot().ExternalHandoffs[r.ID].Delivery != agenttransport.Delivered {
		t.Fatal("stale store overwrote delivery")
	}
}

func TestHandoffRequiresOriginalOpenBeforeFollowup(t *testing.T) {
	s, _ := Open(t.TempDir())
	req := handoffRequest(t)
	req.Handoff.Readiness = "busy"
	r := s.Apply(req)
	tr := &handoffTransport{store: s, id: r.ID, fail: true}
	s.InterruptHandoff(context.Background(), r.ID, tr)
	next := req
	h := *req.Handoff
	next.Handoff = &h
	next.ID = "follow-up"
	h.ParentID = r.ID
	h.Readiness = "ready"
	if s.Apply(next).Error == "" {
		t.Fatal("follow up bypassed original pane open")
	}
	if e := tr.Open(context.Background(), req.Handoff.Target); e != nil {
		t.Fatal(e)
	}
	if response := s.Apply(Request{Version: Version, ID: "human-open", Method: "handoff.opened", Project: req.Project, Target: r.ID, Actor: "human", Handoff: &ExternalHandoff{Target: req.Handoff.Target}}); response.Error != "" {
		t.Fatal(response.Error)
	}
	s.DeliverHandoff(context.Background(), r.ID, tr)
	if tr.sends != 0 {
		t.Fatal("opening the pane replayed the interrupted approval")
	}
	if r := s.Apply(next); r.Error != "" {
		t.Fatal(r.Error)
	}
}

func TestHandoffOpenedObservationRequiresHumanAndSettledInput(t *testing.T) {
	s, _ := Open(t.TempDir())
	req := handoffRequest(t)
	req.Handoff.Readiness = "busy"
	r := s.Apply(req)
	observed := &ExternalHandoff{Target: req.Handoff.Target}
	wrong := req.Handoff.Target
	wrong.PaneID, wrong.PanePID = "%9", 999
	if response := s.Apply(Request{Version: Version, ID: "opened-wrong-pane", Method: "handoff.opened", Project: req.Project, Target: r.ID, Actor: "human", Handoff: &ExternalHandoff{Target: wrong}}); response.Error == "" {
		t.Fatal("different pane satisfied the original-pane inspection gate")
	}
	if response := s.Apply(Request{Version: Version, ID: "opened-without-human", Method: "handoff.opened", Project: req.Project, Target: r.ID, Handoff: observed}); response.Error == "" {
		t.Fatal("non-human open observation was accepted")
	}
	if response := s.Apply(Request{Version: Version, ID: "opened-settled-delivery", Method: "handoff.opened", Project: req.Project, Target: r.ID, Actor: "human", Handoff: observed}); response.Error != "" {
		t.Fatalf("exact human observation could not settle a lost acknowledgement: %s", response.Error)
	}
	if _, claimed, err := s.beginHandoff(r.ID, true); err != nil || !claimed {
		t.Fatalf("begin interrupt: claimed=%v err=%v", claimed, err)
	}
	if response := s.Apply(Request{Version: Version, ID: "opened-in-flight", Method: "handoff.opened", Project: req.Project, Target: r.ID, Actor: "human", Handoff: observed}); response.Error == "" {
		t.Fatal("in-flight input was marked inspected")
	}
	s.finishInterruptHandoff(r.ID, agenttransport.Result{State: agenttransport.Uncertain, Err: errors.New("readiness unknown")}, agenttransport.Result{State: agenttransport.NotSent})
	if response := s.Apply(Request{Version: Version, ID: "opened-wrong-pane-settled", Method: "handoff.opened", Project: req.Project, Target: r.ID, Actor: "human", Handoff: &ExternalHandoff{Target: wrong}}); response.Error == "" {
		t.Fatal("settled handoff accepted inspection of a different pane")
	}
	if response := s.Apply(Request{Version: Version, ID: "opened-after-inspection", Method: "handoff.opened", Project: req.Project, Target: r.ID, Actor: "human", Handoff: observed}); response.Error != "" {
		t.Fatal(response.Error)
	}
	if h := s.Snapshot().ExternalHandoffs[r.ID]; h.OpenRequired || h.OpenedAt.IsZero() {
		t.Fatalf("human inspection was not recorded: %+v", h)
	}
}

func TestHandoffRecordVersionAndEvidenceSeparation(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	req := handoffRequest(t)
	r := s.Apply(req)
	if s.Snapshot().Version <= 2 {
		t.Fatal("older readers could silently discard new handoff records")
	}
	report := Request{Version: Version, ID: "report", Project: req.Project, Target: r.ID, Method: "handoff.report", Handoff: &ExternalHandoff{Results: HandoffResults{Commit: "abc", Files: []string{"a.go"}, Checks: []string{"agent says tests pass"}, RunnableBuild: "/tmp/build"}, AgentReportedCompletion: "done"}}
	if v := s.Apply(report); v.Error != "" {
		t.Fatal(v.Error)
	}
	snapshot := s.Snapshot()
	h := snapshot.ExternalHandoffs[r.ID]
	if h.HumanVerdict != nil || len(h.VerifiedChecks) > 0 {
		t.Fatal("agent report became verification or human verdict")
	}
	h.Results.Files[0] = "mutated"
	if s.Snapshot().ExternalHandoffs[r.ID].Results.Files[0] != "a.go" {
		t.Fatal("snapshot aliases durable state")
	}
	// Version 2 remains readable; the next write migrates its version.
	oldDir := t.TempDir()
	os.Mkdir(filepath.Join(oldDir, "records"), 0700)
	data, _ := json.Marshal(State{Version: 2, Revision: 1})
	os.WriteFile(filepath.Join(oldDir, "records", "00000000000000000001.json"), data, 0600)
	old, e := Open(oldDir)
	if e != nil {
		t.Fatal(e)
	}
	if v := old.Apply(req); v.Error != "" {
		t.Fatal(v.Error)
	}
	if old.Snapshot().Version != RecordVersion {
		t.Fatal("old state not migrated on write")
	}
}

func TestHandoffDeltaRequiresDeliveredContext(t *testing.T) {
	s, _ := Open(t.TempDir())
	req := handoffRequest(t)
	parent := s.Apply(req)
	h := *req.Handoff
	h.ParentID = parent.ID
	h.Form = "delta"
	h.Draft = "only the changes"
	h.Message = h.RenderedMessage()
	req.Handoff = &h
	req.ID = "delta-before-context"
	if s.Apply(req).Error == "" {
		t.Fatal("delta admitted without delivered context")
	}
}

func TestHandoffDuplicateReceiptWhileNextIntentIsInFlight(t *testing.T) {
	s, _ := Open(t.TempDir())
	req := handoffRequest(t)
	first := s.Apply(req)
	tr := &handoffTransport{store: s, id: first.ID}
	s.DeliverHandoff(context.Background(), first.ID, tr)
	req.ID = "second-intent"
	req.Handoff.Draft = "a different next intent"
	req.Handoff.Message = req.Handoff.RenderedMessage()
	second := s.Apply(req)
	if _, claimed, e := s.beginHandoff(second.ID, false); !claimed || e != nil {
		t.Fatalf("second claim: %v %v", claimed, e)
	}
	if result := s.DeliverHandoff(context.Background(), first.ID, tr); result.State != agenttransport.Delivered || result.Err != nil {
		t.Fatalf("lost durable receipt: %+v", result)
	}
	if tr.sends != 1 {
		t.Fatal("duplicate sent")
	}
}

func TestHandoffLinkedSessionCannotBypassPaneExclusion(t *testing.T) {
	s, _ := Open(t.TempDir())
	req := handoffRequest(t)
	first := s.Apply(req)
	req.ID = "linked-session"
	req.Handoff.Target.SessionID = "$2"
	req.Handoff.Readiness = "busy"
	second := s.Apply(req)
	if _, claimed, e := s.beginHandoff(first.ID, false); !claimed || e != nil {
		t.Fatalf("first claim: %v %v", claimed, e)
	}
	tr := &handoffTransport{store: s, id: first.ID}
	s.InterruptHandoff(context.Background(), second.ID, tr)
	if tr.interrupts != 0 {
		t.Fatal("linked session bypassed exclusion")
	}
}

func TestHandoffCannotInterruptAnotherInFlightIntent(t *testing.T) {
	s, _ := Open(t.TempDir())
	req := handoffRequest(t)
	first := s.Apply(req)
	other := req
	h := *req.Handoff
	other.Handoff = &h
	other.ID = "busy-draft"
	h.Readiness = "busy"
	second := s.Apply(other)
	if _, claimed, e := s.beginHandoff(first.ID, false); !claimed || e != nil {
		t.Fatalf("first claim: %v %v", claimed, e)
	}
	tr := &handoffTransport{store: s, id: first.ID}
	s.InterruptHandoff(context.Background(), second.ID, tr)
	if tr.interrupts != 0 {
		t.Fatal("interrupted another handoff between paste and submission")
	}
}
