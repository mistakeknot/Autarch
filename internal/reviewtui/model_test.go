package reviewtui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/pkg/review"
	"strings"
	"testing"
)

func TestWorkbenchShowsApprovedScopeAndKeepsTypingSafe(t *testing.T) {
	m := New("/project", "Cozy", review.Client{})
	m.width, m.height = 120, 40
	m.state.Proposals = map[string]review.Proposal{"p": {ID: "p", Project: "/project", Revision: 3, Outcome: "Readable review", Change: "Separate outcomes", Scope: []string{"internal/reviewtui"}, Rationale: "Taste ruling", Guidance: []review.Guidance{{Path: "docs/design.md", Text: "Keep original observations", Scope: "Review", Rationale: "Preserve context"}}, Status: "proposed"}}
	m.tab = 1
	m.selection = 0
	m.detail = true
	view := m.View()
	for _, want := range []string{"Readable review", "Separate outcomes", "internal/reviewtui", "Keep original observations", "Revision 3"} {
		if !strings.Contains(view, want) {
			t.Errorf("missing %s in %s", want, view)
		}
	}
	m.mode = "note"
	m.input.Focus()
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("qa")})
	if m.closed || m.input.Value() != "qa" {
		t.Fatal("typing activated a workbench action")
	}
}

func TestFailedSaveKeepsDraft(t *testing.T) {
	m := New("/project", "Compact", review.Client{})
	m.mode = "note"
	m.input.SetValue("an observation")
	m.Update(resultMsg{err: "disk full"})
	if m.input.Value() != "an observation" || m.mode != "note" {
		t.Fatal("failed write discarded draft")
	}
}

func TestRefreshCannotChangeReviewedProposalOrRevision(t *testing.T) {
	m := New("/project", "Cozy", review.Client{})
	m.tab = 1
	m.state.Proposals = map[string]review.Proposal{"b": {ID: "b", Project: "/project", Revision: 1, Change: "Reviewed"}}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next := m.state.Clone()
	next.Proposals["a"] = review.Proposal{ID: "a", Project: "/project", Revision: 1, Change: "Different"}
	p := next.Proposals["b"]
	p.Revision = 2
	p.Change = "Unseen revision"
	next.Proposals["b"] = p
	m.Update(resultMsg{response: review.Response{State: &next}})
	reviewed, _ := m.selectedProposal()
	if reviewed.ID != "b" || reviewed.Revision != 1 || reviewed.Change != "Reviewed" {
		t.Fatalf("approval drifted: %+v", reviewed)
	}
}

func TestSaveFreezesDraftAndRetriesSameRequest(t *testing.T) {
	m := New("/project", "Cozy", review.Client{})
	m.mode = "note"
	m.input.Focus()
	m.input.SetValue("Keep this")
	m.saveInput()
	id := m.pendingSave.ID
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("extra")})
	if m.input.Value() != "Keep this" {
		t.Fatal("allowed unsaved edits during acknowledgement")
	}
	m.Update(resultMsg{err: "connection closed"})
	m.saveInput()
	if m.pendingSave.ID != id {
		t.Fatal("retry would create duplicate feedback")
	}
}

func TestAnswerRemainsBoundToQuestionShownInComposer(t *testing.T) {
	m := New("/project", "Compact", review.Client{})
	m.state.Questions = []review.Question{{ID: "first", Project: "/project", RuntimeSession: "session", Status: "pending"}}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlQ})
	m.input.SetValue("Answer to first")
	m.state.Questions[0].Status = "cancelled"
	m.state.Questions = append(m.state.Questions, review.Question{ID: "second", Project: "/project", RuntimeSession: "session", Status: "pending"})
	if cmd := m.saveInput(); cmd != nil || m.busy || m.input.Value() != "Answer to first" {
		t.Fatal("stale answer was redirected or discarded")
	}
}

func TestPendingQuestionVisibleAfterLongConversation(t *testing.T) {
	m := New("/project", "Cozy", review.Client{})
	m.state.Turns = []review.Turn{{Project: "/project", Text: strings.Repeat("Long response\n", 100)}}
	m.state.Questions = []review.Question{{ID: "q", Project: "/project", Title: "Which outcome?", Status: "pending"}}
	if view := m.chatView(40, 15); !strings.Contains(view, "Which outcome?") {
		t.Fatalf("question hidden: %s", view)
	}
}

func TestTraceCoverageWarningsRemainVisible(t *testing.T) {
	m := New("/project", "Compact", review.Client{})
	m.Update(resultMsg{response: review.Response{Trace: []byte(`{"metadata":{"warnings":["Trace truncated at node limit","Beads source unavailable"]}}`)}})
	if !strings.Contains(m.trace, "Trace truncated") || !strings.Contains(m.trace, "Beads source unavailable") {
		t.Fatal(m.trace)
	}
}

func TestTraceDoesNotStartTwiceOrUnlockOnUnrelatedPollingFailure(t *testing.T) {
	m := New("/project", "Compact", review.Client{})
	m.state.Feedback = map[string]review.Feedback{"n": {ID: "n", Project: "/project", Revision: 1}}
	if m.Update(tea.KeyMsg{Type: tea.KeyCtrlL}) == nil {
		t.Fatal("first trace did not start")
	}
	if m.Update(tea.KeyMsg{Type: tea.KeyCtrlL}) != nil {
		t.Fatal("duplicate trace started")
	}
	m.Update(resultMsg{err: "unrelated state poll failed"})
	if m.Update(tea.KeyMsg{Type: tea.KeyCtrlL}) != nil {
		t.Fatal("polling error released trace ownership")
	}
	m.Update(resultMsg{method: "trace", err: "projection failed"})
	if m.Update(tea.KeyMsg{Type: tea.KeyCtrlL}) == nil {
		t.Fatal("failed trace could not be retried")
	}
}

func TestHelpEscapeReturnsToListInOneKey(t *testing.T) {
	m := New("/project", "Compact", review.Client{})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.detail || m.trace != "" || m.closed {
		t.Fatal("help did not return to its list")
	}
}

func TestDeletedSessionCannotOpenDeleteComposer(t *testing.T) {
	m := New("/project", "Compact", review.Client{})
	m.tab = 3
	m.state.Sessions = map[string]review.Session{"s": {ID: "s", Project: "/project", Revision: 3, Status: "deleted"}}
	if m.Update(tea.KeyMsg{Type: tea.KeyCtrlD}) != nil || m.mode != "" {
		t.Fatal("deleted session can be deleted again")
	}
}

func TestOtherProjectsCannotHideConversation(t *testing.T) {
	m := New("/project", "Compact", review.Client{})
	m.state.Turns = append(m.state.Turns, review.Turn{Project: "/project", Text: "Our ruling"})
	for i := 0; i < 40; i++ {
		m.state.Turns = append(m.state.Turns, review.Turn{Project: "/elsewhere", Text: "Other"})
	}
	if !strings.Contains(m.conversation(), "Our ruling") {
		t.Fatal("project conversation disappeared")
	}
}

func TestConversationShowsActualDeliveryState(t *testing.T) {
	m := New("/project", "Cozy", review.Client{})
	for _, delivery := range []string{"pending", "switching", "sending", "delivered", "superseded", "failed"} {
		m.state.Turns = []review.Turn{{Project: "/project", Kind: "model selection", Model: "provider/model", Text: "Explicit choice", Delivery: delivery}}
		if !strings.Contains(m.conversation(), delivery) {
			t.Errorf("conversation hides %s", delivery)
		}
	}
}

func TestProposalAcceptanceRequiresItsDisplayedSnapshot(t *testing.T) {
	for _, display := range []string{"list", "help-from-list", "help-after-review", "trace-after-review", "review"} {
		t.Run(display, func(t *testing.T) {
			m := New("/project", "Cozy", review.Client{})
			m.tab = 1
			m.state.Proposals = map[string]review.Proposal{"p": {ID: "p", Project: "/project", Revision: 3, Status: "proposed", Change: "Visible scope", Scope: []string{"file"}}}
			if display == "review" || strings.HasSuffix(display, "after-review") {
				m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			}
			if strings.HasPrefix(display, "help") {
				m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
			}
			if strings.HasPrefix(display, "trace") {
				m.Update(resultMsg{method: "trace", response: review.Response{Trace: []byte(`{"entities":[]}`)}})
			}
			cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
			if (cmd != nil) != (display == "review") {
				t.Fatalf("incorrect acceptance availability from %s", display)
			}
		})
	}
}

func TestProposalRejectionRequiresItsDisplayedSnapshot(t *testing.T) {
	for _, display := range []string{"list", "help-from-list", "help-after-review", "trace-after-review", "review"} {
		t.Run(display, func(t *testing.T) {
			m := New("/project", "Cozy", review.Client{})
			m.tab = 1
			m.state.Proposals = map[string]review.Proposal{"p": {ID: "p", Project: "/project", Revision: 3, Status: "proposed", Change: "Visible scope", Scope: []string{"file"}}}
			if display == "review" || strings.HasSuffix(display, "after-review") {
				m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			}
			if strings.HasPrefix(display, "help") {
				m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
			}
			if strings.HasPrefix(display, "trace") {
				m.Update(resultMsg{method: "trace", response: review.Response{Trace: []byte(`{"entities":[]}`)}})
			}
			cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlX})
			if (cmd != nil) != (display == "review") {
				t.Fatalf("incorrect rejection availability from %s", display)
			}
		})
	}
}
