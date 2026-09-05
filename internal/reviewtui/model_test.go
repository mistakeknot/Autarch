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
