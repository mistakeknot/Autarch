package door

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/mistakeknot/autarch/pkg/agenttransport"
	"github.com/mistakeknot/autarch/pkg/review"
)

type workbenchTransport struct {
	panes       []agenttransport.Pane
	preview     string
	observed    int
	opened      int
	openTargets []agenttransport.Target
}

func (f *workbenchTransport) List(context.Context) ([]agenttransport.Pane, error) {
	return append([]agenttransport.Pane(nil), f.panes...), nil
}
func (f *workbenchTransport) Observe(context.Context, agenttransport.Target) (string, error) {
	f.observed++
	return f.preview, nil
}
func (f *workbenchTransport) Open(_ context.Context, target agenttransport.Target) error {
	f.opened++
	f.openTargets = append(f.openTargets, target)
	return nil
}
func (*workbenchTransport) Send(context.Context, agenttransport.Target, string) agenttransport.Result {
	panic("UI must deliver through the review controller")
}
func (*workbenchTransport) Interrupt(context.Context, agenttransport.Target) agenttransport.Result {
	panic("UI must interrupt through the review controller")
}

type workbenchClient struct {
	calls     []review.Request
	delivery  agenttransport.Delivery
	replayed  bool
	handoffs  map[string]review.ExternalHandoff
	errMethod string
}

func (f *workbenchClient) Call(_ context.Context, r review.Request) (review.Response, error) {
	f.calls = append(f.calls, r)
	if r.Method == f.errMethod {
		return review.Response{}, errors.New("controller acknowledgement lost")
	}
	if r.Method == "handoff.approve" {
		return review.Response{Version: review.Version, ID: "handoff-1", Replayed: f.replayed}, nil
	}
	if r.Method == "handoff.deliver" || r.Method == "handoff.interrupt" {
		return review.Response{Version: review.Version, ID: r.Target, Delivery: f.delivery}, nil
	}
	if r.Method == "state" {
		return review.Response{Version: review.Version, State: &review.State{ExternalHandoffs: f.handoffs}}, nil
	}
	return review.Response{Version: review.Version, ID: r.Target}, nil
}

func workbenchTarget() agenttransport.Target {
	return agenttransport.Target{Socket: "/tmp/workbench.sock", ServerPID: 42, ServerStarted: 1700000000, SessionID: "$1", WindowID: "@2", PaneID: "%3", PanePID: 100, Command: "codex"}
}

func workbenchModelFixture(t *testing.T) Model {
	t.Helper()
	m := productModelFixture(t)
	target := workbenchTarget()
	m.workbench.transport = &workbenchTransport{panes: []agenttransport.Pane{{Target: target, SessionName: "autarch", WindowName: "agent", Path: m.productRoot, Command: "codex"}}, preview: "recent\noutput"}
	m.workbench.client = &workbenchClient{delivery: agenttransport.Delivered}
	m.workbench.panes = []agenttransport.Pane{{Target: target, SessionName: "autarch", WindowName: "agent", Path: m.productRoot, Command: "codex"}}
	m.workbench.target = target
	m.workbench.readiness = "ready"
	m.workbench.outcome = "Ship a safe direct-agent workbench"
	m.workbench.task = review.TaskIdentity{Tracker: "workspace-beads", TrackerUUID: "tracker-uuid", BeadID: "Sylveste-fuwn", Assignee: "codex-seat", Qualification: "qualified"}
	m.workbench.taskTitle = "Direct agent workbench"
	m.workbench.kind = "investigation"
	m.workbench.preview = "recent\noutput"
	m.product.Journeys = []ProductJourney{{ID: "reader-01", Success: "Find the next passage in a minute", Source: ProductSource{Path: "docs/cujs/reader-01.json", State: "read"}}}
	m.product.Decisions = []ProductSource{{Path: "docs/decisions/layout.md", State: "read", Content: "Keep the document alongside its references."}}
	return m
}

func TestWorkbenchIsDefaultAndDetailsRemainNumbered(t *testing.T) {
	m := workbenchModelFixture(t)
	for _, size := range [][2]int{{80, 24}, {42, 18}} {
		x, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		m = x.(Model)
		view := m.View()
		for _, want := range []string{"OUTCOME", "CURRENT WORK", "AGENT", "NEXT ACTION"} {
			if !strings.Contains(view, want) {
				t.Fatalf("%dx%d missing %q:\n%s", size[0], size[1], want, view)
			}
		}
		for _, line := range strings.Split(view, "\n") {
			if ansi.StringWidth(line) > size[0] {
				t.Fatalf("%dx%d overflow: %q", size[0], size[1], line)
			}
		}
		if size[0] == 42 {
			for _, tc := range []struct {
				name, command, label, want string
				missing, commandMismatch   bool
			}{
				{name: "selected second", command: "codex", label: "autarch", want: "AGENT · Codex · %3 · autarch"},
				{name: "long label", command: "codex", label: strings.Repeat("a", 40), want: "AGENT · Codex · %3 · " + strings.Repeat("a", 16) + "…"},
				{name: "wide label", command: "claude", label: strings.Repeat("界", 20), want: "AGENT · Claude Code · %3 · " + strings.Repeat("界", 5) + "…"},
				{name: "single line", command: "codex", label: "autarch\nreview", want: "AGENT · Codex · %3 · autarch review"},
				{name: "empty label", command: "codex", want: "AGENT · Codex · %3"},
				{name: "missing target", command: "codex", missing: true, want: "AGENT · Codex · %3"},
				{name: "command mismatch", command: "codex", label: "wrong session", commandMismatch: true, want: "AGENT · Codex · %3"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					m := m
					m.workbench.target.Command = tc.command
					other := m.workbench.target
					other.Socket = "/tmp/other-workbench.sock"
					m.workbench.panes = []agenttransport.Pane{{Target: other, SessionName: "wrong session"}}
					if !tc.missing {
						candidate := m.workbench.target
						if tc.commandMismatch {
							candidate.Command = "claude"
						}
						m.workbench.panes = append(m.workbench.panes, agenttransport.Pane{Target: candidate, SessionName: tc.label})
					}
					if got := m.workbenchLines()[2]; got != tc.want {
						t.Fatalf("agent line = %q, want %q", got, tc.want)
					}
					view := ansi.Strip(m.View())
					if !strings.Contains(view, tc.want) || !strings.Contains(view, "NEXT ACTION") {
						t.Fatalf("selected agent or next action is not visible:\n%s", view)
					}
					for _, line := range strings.Split(view, "\n") {
						if ansi.StringWidth(line) > 42 {
							t.Fatalf("42-column overflow: %q", line)
						}
					}
					lines := m.productLines()
					for i, line := range lines {
						if strings.HasPrefix(ansi.Strip(line), "AGENT") && (i+1 >= len(lines) || !strings.HasPrefix(ansi.Strip(lines[i+1]), "NEXT ACTION")) {
							t.Fatalf("agent label wrapped onto another row:\n%s", view)
						}
					}
				})
			}
		}
	}
	m, _ = press(m, "7")
	if m.productSection != 6 || !strings.Contains(m.View(), "PROJECT FOUNDATION") {
		t.Fatal("foundation detail section was not preserved", m.View())
	}
	m, _ = press(m, "tab")
	if m.productSection != 0 {
		t.Fatal("seven-section navigation did not wrap")
	}
}

func TestWorkbenchDraftAndSendRequireSeparateConfirmation(t *testing.T) {
	m := workbenchModelFixture(t)
	client := m.workbench.client.(*workbenchClient)
	m, _ = press(m, "e")
	for _, key := range []string{"h", "i", "enter", "t", "h", "e", "r", "e", "ctrl+s"} {
		m, _ = press(m, key)
	}
	if m.workbench.draft != "hi\nthere" || len(client.calls) != 0 {
		t.Fatalf("draft=%q calls=%d", m.workbench.draft, len(client.calls))
	}
	m, cmd := press(m, "s")
	if cmd != nil || m.workbench.mode != workbenchConfirmSend || len(client.calls) != 0 {
		t.Fatal("send input occurred before confirmation")
	}
	confirm := strings.Join(m.confirmationLines(false), "\n")
	for _, want := range []string{m.productRoot, "tracker-uuid", "Sylveste-fuwn", "codex-seat", "/tmp/workbench.sock", "$1", "@2", "%3", "investigation", "hi\nthere"} {
		if !strings.Contains(confirm, want) {
			t.Fatalf("confirmation missing %q:\n%s", want, confirm)
		}
	}
	m, cmd = press(m, "y")
	if cmd == nil || len(client.calls) != 0 || m.workbench.mode != workbenchSending {
		t.Fatal("confirmation did not stage exactly one async delivery")
	}
	_, duplicate := press(m, "y")
	if duplicate != nil {
		t.Fatal("duplicate confirmation scheduled another delivery")
	}
	msg := cmd()
	if len(client.calls) != 2 || client.calls[0].Method != "handoff.approve" || client.calls[1].Method != "handoff.deliver" {
		t.Fatalf("wrong IPC order: %+v", client.calls)
	}
	if client.calls[0].Handoff == nil || client.calls[0].Handoff.Message != client.calls[0].Handoff.RenderedMessage() {
		t.Fatal("approved durable record did not contain the exact rendered message")
	}
	x, _ := m.Update(msg)
	m = x.(Model)
	if m.workbench.draft != "" || m.workbench.lastHandoffID != "handoff-1" {
		t.Fatal("delivered draft was restored as unsent or durable identity was lost")
	}
	m.workbench.readiness = "ready"
	m, duplicate = press(m, "s")
	if duplicate != nil || m.workbench.mode != workbenchBrowse || !strings.Contains(m.status, "write a direction") {
		t.Fatal("duplicate click could approve the same payload again")
	}
}

func TestWorkbenchInterruptFailureRetainsDraftAndObservesOnly(t *testing.T) {
	m := workbenchModelFixture(t)
	m.workbench.readiness = "busy"
	m.workbench.draft = "replace direction"
	client := m.workbench.client.(*workbenchClient)
	client.delivery = agenttransport.Uncertain
	tr := m.workbench.transport.(*workbenchTransport)
	m, cmd := press(m, "x")
	if cmd != nil || m.workbench.mode != workbenchConfirmInterrupt || !strings.Contains(strings.Join(m.productLines(), "\n"), "replace direction") {
		t.Fatal("interrupt confirmation did not name replacement direction")
	}
	m, cmd = press(m, "y")
	if cmd == nil || len(client.calls) != 0 {
		t.Fatal("interrupt ran before explicit confirmation")
	}
	msg := cmd()
	if len(client.calls) != 2 || client.calls[1].Method != "handoff.interrupt" || tr.observed != 1 {
		t.Fatalf("interrupt did not persist then observe: calls=%+v observed=%d", client.calls, tr.observed)
	}
	x, _ := m.Update(msg)
	m = x.(Model)
	if m.workbench.draft != "replace direction" || !m.workbench.openRequired || m.workbench.readiness != "uncertain" {
		t.Fatalf("unsafe interrupt recovery: %+v", m.workbench)
	}
}

func TestWorkbenchSuccessfulInterruptRedirectClearsSentDraft(t *testing.T) {
	m := workbenchModelFixture(t)
	m.workbench.readiness = "busy"
	m.workbench.draft = "replacement direction"
	m, _ = press(m, "x")
	m, cmd := press(m, "y")
	msg := cmd()
	x, _ := m.Update(msg)
	m = x.(Model)
	if m.workbench.draft != "" || m.workbench.openRequired || m.workbench.lastDelivery != agenttransport.Delivered {
		t.Fatalf("successful redirect was retained as unsent: %+v", m.workbench)
	}
}

func TestWorkbenchOpensExactPaneLocallyThenRecordsObservation(t *testing.T) {
	m := workbenchModelFixture(t)
	m.workbench.lastHandoffID = "h1"
	m.workbench.lastTarget = m.workbench.target
	m.workbench.openRequired = true
	m.workbench.recoveryHandoffID = "h1"
	m.workbench.recoveryTarget = m.workbench.target
	client := m.workbench.client.(*workbenchClient)
	transport := m.workbench.transport.(*workbenchTransport)
	other := m.workbench.target
	other.PaneID, other.PanePID = "%9", 999
	m.workbench.target = other

	m, cmd := press(m, "t")
	if cmd == nil {
		t.Fatal("open was not scheduled")
	}
	msg := cmd()
	if transport.opened != 1 || len(transport.openTargets) != 1 || transport.openTargets[0] != m.workbench.lastTarget {
		t.Fatal("UI did not open the exact local pane")
	}
	if got := client.calls[len(client.calls)-1]; got.Method != "handoff.opened" || got.Target != "h1" || got.Actor != "human" || got.Handoff == nil || got.Handoff.Target != m.workbench.lastTarget {
		t.Fatalf("open observation was not recorded: %+v", got)
	}
	x, _ := m.Update(msg)
	m = x.(Model)
	if m.workbench.openRequired {
		t.Fatal("recorded open did not clear local recovery state")
	}
}

func TestWorkbenchOpensOlderUnresolvedHandoffBeforeLatestResult(t *testing.T) {
	m := workbenchModelFixture(t)
	oldTarget := m.workbench.target
	newTarget := oldTarget
	newTarget.PaneID, newTarget.PanePID = "%4", 101
	m.workbench.handoffs = []review.ExternalHandoff{
		{ID: "older-uncertain", Project: m.productRoot, Target: oldTarget, ApprovedAt: time.Unix(1, 0), OpenRequired: true},
		{ID: "latest-result", Project: m.productRoot, Target: newTarget, ApprovedAt: time.Unix(2, 0), Delivery: agenttransport.Delivered},
	}
	m.workbench.lastHandoffID = "latest-result"
	m.workbench.lastTarget = newTarget
	m.selectWorkbenchRecovery()
	if m.workbench.recoveryHandoffID != "older-uncertain" || m.workbench.recoveryTarget != oldTarget {
		t.Fatalf("wrong recovery handoff selected: %+v", m.workbench)
	}
	_, cmd := press(m, "t")
	_ = cmd()
	transport := m.workbench.transport.(*workbenchTransport)
	if len(transport.openTargets) != 1 || transport.openTargets[0] != oldTarget {
		t.Fatal("latest result hid the older unresolved pane")
	}
}

func TestWorkbenchTreatsLostDeliveryAcknowledgementAsUncertain(t *testing.T) {
	m := workbenchModelFixture(t)
	m.workbench.draft = "send once"
	client := m.workbench.client.(*workbenchClient)
	client.errMethod = "handoff.deliver"
	m, _ = press(m, "s")
	m, cmd := press(m, "y")
	x, _ := m.Update(cmd())
	m = x.(Model)
	if !m.workbench.openRequired || m.workbench.lastDelivery != agenttransport.Uncertain || m.workbench.draft != "send once" || m.workbench.lastHandoffID != "handoff-1" {
		t.Fatalf("lost acknowledgement did not fail closed: %+v", m.workbench)
	}
	m, retry := press(m, "s")
	if retry != nil || m.workbench.mode != workbenchBrowse || !strings.Contains(strings.ToLower(m.status), "open the original pane") {
		t.Fatal("uncertain delivery admitted another approval before inspection")
	}
	// The controller may have committed Delivered before its acknowledgement was
	// lost. Polling that state is not a substitute for inspecting the exact pane.
	client.handoffs = map[string]review.ExternalHandoff{
		"handoff-1": {ID: "handoff-1", Project: m.productRoot, Target: m.workbench.lastTarget, Delivery: agenttransport.Delivered},
	}
	x, _ = m.Update(reviewAttentionMsg{state: &review.State{ExternalHandoffs: client.handoffs}})
	m = x.(Model)
	if !m.workbench.openRequired || m.workbench.recoveryHandoffID != "handoff-1" || m.workbench.recoveryTarget != m.workbench.lastTarget {
		t.Fatalf("state polling erased the local lost-ack inspection gate: %+v", m.workbench)
	}
	reopened := NewProductModel(m.productRoot)
	if !reopened.workbench.openRequired || reopened.workbench.recoveryHandoffID != "handoff-1" || reopened.workbench.recoveryTarget != m.workbench.lastTarget {
		t.Fatalf("restart erased the local lost-ack inspection gate: %+v", reopened.workbench)
	}
}

func TestWorkbenchResultEvidenceIsSeparate(t *testing.T) {
	m := workbenchModelFixture(t)
	now := time.Now().UTC()
	m.workbench.handoffs = []review.ExternalHandoff{{
		ID: "h1", Project: m.productRoot, Target: m.workbench.target,
		AgentReportedCompletion: "implemented", Results: review.HandoffResults{Commit: "abc123", Files: []string{"a.go"}},
		VerifiedChecks: []review.HandoffCheck{{Name: "go test", Result: "pass", Evidence: "exit 0"}},
		HumanVerdict:   &review.HandoffVerdict{Result: "inconclusive", Actor: "mk", Evidence: "needs live", At: now},
	}}
	view := strings.Join(m.productLines(), "\n")
	for _, want := range []string{"Agent reported", "implemented", "Verified checks", "go test", "Human verdict", "inconclusive", "Corrective follow-up"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %q:\n%s", want, view)
		}
	}
}

func TestWorkbenchRecordsAgentReportVerificationAndExactVerdict(t *testing.T) {
	m := workbenchModelFixture(t)
	m.workbench.handoffs = []review.ExternalHandoff{{ID: "h1", Project: m.productRoot, Target: m.workbench.target, Delivery: agenttransport.Delivered}}
	client := m.workbench.client.(*workbenchClient)

	m, _ = press(m, "R")
	m.workbench.recordInput = `{"completion":"implemented","commit":"abc123","files":["a.go"],"checks":["agent says tests pass"],"runnable_build":"/tmp/app"}`
	m, cmd := press(m, "ctrl+s")
	if cmd == nil || m.workbench.mode != workbenchRecording {
		t.Fatal("agent report was not staged")
	}
	msg := cmd()
	if got := client.calls[len(client.calls)-1]; got.Method != "handoff.report" || got.Target != "h1" || got.Handoff == nil || got.Handoff.Results.Commit != "abc123" || got.Handoff.AgentReportedCompletion != "implemented" {
		t.Fatalf("wrong agent report request: %+v", got)
	}
	x, _ := m.Update(msg)
	m = x.(Model)

	m.workbench.handoffs[0].AgentReportedCompletion = "implemented"
	m, _ = press(m, "V")
	m.workbench.recordInput = `{"name":"go test","result":"pass","evidence":"fresh exit 0"}`
	m, cmd = press(m, "ctrl+s")
	if cmd == nil {
		t.Fatal("verified check was not staged")
	}
	_ = cmd()
	if got := client.calls[len(client.calls)-1]; got.Method != "handoff.verify" || got.Handoff == nil || len(got.Handoff.VerifiedChecks) != 1 || got.Handoff.VerifiedChecks[0].Evidence != "fresh exit 0" {
		t.Fatalf("wrong verification request: %+v", got)
	}
	m.workbench.handoffs[0].Results.Commit = "abc123"
	m.workbench.handoffs[0].VerifiedChecks = []review.HandoffCheck{{Name: "go test", Result: "pass", Evidence: "fresh exit 0"}}

	m.workbench.mode = workbenchBrowse
	m, cmd = press(m, "P")
	if cmd != nil || m.workbench.mode != workbenchConfirmVerdict || m.workbench.pendingVerdict != "pass" {
		t.Fatal("pass did not open an exact-result confirmation")
	}
	verdictPreview := strings.Join(m.workbenchVerdictLines(), "\n")
	if !strings.Contains(verdictPreview, "h1") || !strings.Contains(verdictPreview, "1 verified check") || !strings.Contains(verdictPreview, "abc123") {
		t.Fatal("verdict confirmation omitted exact result evidence")
	}
	m, cmd = press(m, "y")
	if cmd == nil {
		t.Fatal("confirmed verdict was not staged")
	}
	_ = cmd()
	if got := client.calls[len(client.calls)-1]; got.Method != "handoff.verdict" || got.Handoff == nil || got.Handoff.HumanVerdict == nil || got.Handoff.HumanVerdict.Result != "pass" || got.Target != "h1" {
		t.Fatalf("wrong verdict request: %+v", got)
	}
}

func TestRecommendationOrderPolicyContinuityObservationThenPreference(t *testing.T) {
	base := workbenchTarget()
	codex := agenttransport.Pane{Target: base}
	claudeA := base
	claudeA.Command, claudeA.PaneID, claudeA.PanePID = "claude", "%4", 101
	claudeB := base
	claudeB.Command, claudeB.PaneID, claudeB.PanePID = "claude", "%5", 102
	panes := []agenttransport.Pane{codex, {Target: claudeA}, {Target: claudeB}}
	ranked := RecommendPanes(panes, RecommendationContext{
		RequiredProvider: "Claude Code",
		ContinuityTarget: claudeA,
		Readiness:        map[string]string{claudeB.Key(): "ready"},
		Capacity:         map[string]CapacityObservation{claudeB.Key(): {State: "available"}}, // no observation timestamp: unknown
		PreferenceTarget: claudeB,
	})
	if len(ranked) != 3 || ranked[0].Target != claudeA || ranked[1].Target != claudeB || ranked[2].Target != base {
		t.Fatalf("wrong policy/continuity/observation/preference order: %+v", ranked)
	}
}

func TestWorkbenchDeltaOnlyForSameTaskAndExactPane(t *testing.T) {
	m := workbenchModelFixture(t)
	m.workbench.draft = "correction"
	m.workbench.followup = true
	m.workbench.lastHandoffID = "parent"
	m.workbench.lastTarget = m.workbench.target
	m.workbench.lastTask = m.workbench.task
	h, err := m.buildPendingHandoff(false)
	if err != nil || h.Form != "delta" || h.ParentID != "parent" || h.Message != "correction" {
		t.Fatalf("same-pane delta: %+v %v", h, err)
	}
	m.workbench.target.PaneID, m.workbench.target.PanePID = "%9", 999
	h, err = m.buildPendingHandoff(false)
	if err != nil || h.Form != "full" || h.ParentID != "" || !strings.Contains(h.Message, "Accepted decisions (literal wording)") || !strings.Contains(h.Message, "Keep annotations with their sources") {
		t.Fatalf("changed pane did not force full context: %+v %v", h, err)
	}
}

func TestWorkbenchFollowupCanReturnToFreshFullContext(t *testing.T) {
	m := workbenchModelFixture(t)
	m.workbench.draft = "same-session correction"
	m.workbench.lastHandoffID = "parent"
	m.workbench.lastTarget = m.workbench.target
	m.workbench.lastTask = m.workbench.task

	m, _ = press(m, "f")
	if !m.workbench.followup {
		t.Fatal("follow-up was not selected")
	}
	m, _ = press(m, "f")
	if m.workbench.followup || !strings.Contains(m.status, "fresh full context") {
		t.Fatalf("follow-up could not be toggled off: %+v", m.workbench)
	}
	handoff, err := m.buildPendingHandoff(false)
	if err != nil || handoff.Form != "full" || handoff.ParentID != "" || !strings.Contains(handoff.Message, "# External agent handoff") {
		t.Fatalf("fresh full context was not restored: %+v err=%v", handoff, err)
	}
}

func TestWorkbenchOnlyLabelsConfirmedDecisionSourcesAsAccepted(t *testing.T) {
	m := workbenchModelFixture(t)
	m.product.Card.Status = "draft"
	m.workbench.draft = "investigate only"

	handoff, err := m.buildPendingHandoff(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(handoff.Context.Decisions) != 0 || strings.Contains(handoff.Message, "Keep the document alongside its references.") {
		t.Fatalf("draft decision source was presented as accepted: %+v", handoff.Context.Decisions)
	}
}
