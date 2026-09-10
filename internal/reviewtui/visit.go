package reviewtui

import (
	"encoding/json"
	"fmt"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/pkg/review"
	"sort"
	"strconv"
	"strings"
)

func (m *Model) visitPreferences() tea.Cmd {
	v, ok := m.state.ProjectVisit(m.project)
	if !ok {
		return nil
	}
	v.Density = m.density
	return m.call(review.Request{Method: "visit.view", VisitID: v.ID, Visit: &v}, false)
}
func (m *Model) visitItems() []string {
	ids := []string{}
	seen := map[string]bool{}
	for _, p := range m.state.Proposals {
		if p.Project == m.project && p.Kind != "implementation" {
			ids = append(ids, p.ID)
			seen[p.ID] = true
		}
	}
	sort.Strings(ids)
	var result struct {
		Entities []struct {
			ID         string         `json:"canonical_id"`
			Properties map[string]any `json:"properties"`
		} `json:"entities"`
	}
	_ = json.Unmarshal(m.mapRaw, &result)
	for _, e := range result.Entities {
		id := e.ID
		if native, ok := e.Properties["id"].(string); ok {
			id = native
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}
	return ids
}
func (m *Model) visitKey(key string) (tea.Cmd, bool) {
	if key == "alt+w" {
		m.workbench = true
		m.detail = false
		m.trace = ""
		return nil, true
	}
	if !m.workbench {
		return nil, false
	}
	v, ok := m.state.ProjectVisit(m.project)
	if !ok {
		return nil, false
	}
	switch key {
	case "m", "o":
		v.View = "map"
		if key == "o" {
			v.View = "outcome"
		}
		m.state.Visits[v.ID] = v
		m.scroll = 0
		if v.View == "map" {
			return tea.Sequence(m.visitPreferences(), m.call(review.Request{Method: "project.map"}, false)), true
		}
		return m.visitPreferences(), true
	case "l":
		if v.Layout == "purpose" {
			v.Layout = "theme"
		} else {
			v.Layout = "purpose"
		}
		m.state.Visits[v.ID] = v
		return m.visitPreferences(), true
	case "p":
		choices := []string{"all", "fast", "steady", "slow", "unknown"}
		for i, p := range choices {
			if p == v.Pace {
				v.Pace = choices[(i+1)%len(choices)]
				break
			}
		}
		m.state.Visits[v.ID] = v
		return m.visitPreferences(), true
	case "tab":
		m.chatFocus = !m.chatFocus
		return nil, true
	case "down", "j":
		if m.chatFocus {
			m.chatScroll++
		} else {
			items := m.visitItems()
			index := -1
			for i, id := range items {
				if id == v.Selection {
					index = i
				}
			}
			if len(items) > 0 {
				v.Selection = items[min(index+1, len(items)-1)]
				m.state.Visits[v.ID] = v
				return m.visitPreferences(), true
			}
		}
		return nil, true
	case "up", "k":
		if m.chatFocus {
			m.chatScroll = max(0, m.chatScroll-1)
		} else {
			items := m.visitItems()
			index := 0
			for i, id := range items {
				if id == v.Selection {
					index = i
				}
			}
			if len(items) > 0 {
				v.Selection = items[max(0, index-1)]
				m.state.Visits[v.ID] = v
				return m.visitPreferences(), true
			}
		}
		return nil, true
	case "alt+c":
		superseded := map[string]bool{}
		for _, a := range v.Answers {
			if a.Supersedes != "" {
				superseded[a.Supersedes] = true
			}
		}
		for i := len(v.Answers) - 1; i >= 0; i-- {
			a := v.Answers[i]
			if !superseded[a.ID] {
				m.correctionID = a.ID
				m.mode = "correction"
				m.input.SetValue(a.Text)
				m.input.Placeholder = "Correct this answer; original wording stays in the visit"
				m.input.Focus()
				return textarea.Blink, true
			}
		}
		m.status = "No answer to correct yet"
		return nil, true
	case "alt+o":
		m.mode = "outcome"
		m.input.SetValue(v.Outcome)
		m.input.Placeholder = "One meaningful outcome for this visit"
		m.input.Focus()
		return textarea.Blink, true
	case "alt+r":
		return m.call(review.Request{Method: "project.rebuild"}, false), true
	case "alt+b":
		p, ok := m.visitSynthesis()
		if !ok || p.Status != "accepted" {
			m.status = "Inspect and accept a synthesis before preparation"
			return nil, true
		}
		m.preparing = &p
		m.mode = "preparation budget"
		m.input.Reset()
		m.input.Placeholder = "Explicit preparation token budget. This commits the displayed accepted guidance on main and starts planning/review."
		m.input.Focus()
		return textarea.Blink, true
	case "alt+x":
		p, ok := m.visitSynthesis()
		if !ok {
			return nil, true
		}
		for _, prep := range m.state.Preparations {
			if prep.ProposalID == p.ID && prep.ProposalRevision == p.Revision {
				bundle, r, err := review.ReviewedPreparation(prep, p)
				if err != nil {
					m.status = err.Error()
					return nil, true
				}
				if bundle.Specification == nil {
					m.status = "Reviewed plan has no runnable implementation specification"
					return nil, true
				}
				m.preparing = &p
				m.startDigest = r.BundleDigest
				m.trace = "EXPLICIT IMPLEMENTATION APPROVAL\n\nAccepted synthesis: " + p.ID + fmt.Sprintf(" revision %d\nPlan bundle: %s\n", p.Revision, r.BundleDigest)
				spec, _ := json.MarshalIndent(bundle.Specification, "", "  ")
				m.trace += string(spec) + "\n\nType START and Ctrl+S to approve this implementation budget and specification."
				m.workbench = false
				m.mode = "start implementation"
				m.input.Reset()
				m.input.Focus()
				return textarea.Blink, true
			}
		}
		m.status = "Independent reviewed preparation required"
		return nil, true
	case "enter":
		p, ok := m.visitSynthesis()
		if ok {
			m.workbench = false
			m.tab = 1
			m.detail = true
			m.reviewed = &p
			for i, id := range m.items() {
				if id == p.ID {
					m.selection = i
				}
			}
			return nil, true
		}
	}
	return nil, false
}
func (m *Model) visitSynthesis() (review.Proposal, bool) {
	if v, ok := m.state.ProjectVisit(m.project); ok {
		if p, ok := m.state.Proposals[v.Selection]; ok && p.Project == m.project && p.Kind != "implementation" {
			return p, true
		}
	}
	var latest review.Proposal
	for _, p := range m.state.Proposals {
		if p.Project == m.project && p.Kind != "implementation" && (latest.ID == "" || p.At.After(latest.At) || (p.At.Equal(latest.At) && p.ID > latest.ID)) {
			latest = p
		}
	}
	return latest, latest.ID != ""
}
func (m *Model) visitBody() string {
	v, ok := m.state.ProjectVisit(m.project)
	if !ok {
		return "Opening this project visit…"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "PROJECT VISIT · %s\nOutcome: %s\n%s · %s layout · pace %s\nSelected: %s\n\n", v.Status, v.Outcome, v.View, v.Layout, v.Pace, v.Selection)
	b.WriteString("o outcome · m map · l layout · p pace · Tab focus\nAlt+O outcome · Alt+C correct answer · Enter inspect synthesis\nAlt+B prepare reviewed plan · Alt+R rebuild projection\n\n")
	if v.View == "map" {
		if m.projectMap == "" {
			b.WriteString("Project map unavailable. Alt+R explicitly rebuilds from retained source records.\n")
		} else {
			b.WriteString(m.projectMap)
		}
	} else {
		p, exists := m.visitSynthesis()
		if exists {
			fmt.Fprintf(&b, "Next outcome: %s\nWhy: %s\nSynthesis: %s · revision %d\n", p.Outcome, p.Rationale, p.Status, p.Revision)
			for _, g := range p.Guidance {
				fmt.Fprintf(&b, "\n%s\n%s\nWhy: %s\n", g.Path, g.Text, g.Rationale)
			}
		} else {
			b.WriteString("Next: choose one outcome with Flere, then inspect a synthesis. Accepted project guidance stays available in the conversation handoff.\n")
		}
		superseded := map[string]bool{}
		for _, a := range v.Answers {
			if a.Supersedes != "" {
				superseded[a.Supersedes] = true
			}
		}
		for _, a := range v.Answers {
			label := "current answer"
			if superseded[a.ID] {
				label = "superseded answer"
			}
			fmt.Fprintf(&b, "\n%s: %s\n", label, a.Text)
		}
		if m.foundation != "" {
			b.WriteString("\nSOURCED PROJECT OVERVIEW\n" + m.foundation)
		}
	}
	if _, ok := m.state.Preparations[v.PreparationID]; !ok {
		if p, exists := m.visitSynthesis(); exists && p.Status == "accepted" {
			b.WriteString("\nGuidance: accepted, pending persistence\n")
		}
	}
	if prep, ok := m.state.Preparations[v.PreparationID]; ok {
		var persisted review.PreparedReceipt
		if json.Unmarshal(prep.Receipt, &persisted) == nil && persisted.Ratification.Status == "persisted" {
			fmt.Fprintf(&b, "\nGuidance: persisted at %s\n", persisted.Ratification.Commit)
		} else {
			b.WriteString("\nGuidance: accepted, pending persistence\n")
		}
		fmt.Fprintf(&b, "\nPreparation: %s · budget %d tokens\n%s\n", prep.Status, prep.BudgetTokens, prep.Reason)
		if prep.Status == "reviewed" {
			p := m.state.Proposals[prep.ProposalID]
			bundle, receipt, err := review.ReviewedPreparation(prep, p)
			if err == nil {
				fmt.Fprintf(&b, "\nREVIEWED PLAN\n%s\nBundle: %s\nLeave and resume freely. Alt+X separately previews implementation approval.\n", bundle.Plan, receipt.BundleDigest)
			}
		}
	}
	return b.String()
}
func (m *Model) renderProjectMap(raw []byte) error {
	var result struct {
		Entities []struct {
			ID         string         `json:"canonical_id"`
			Kind       string         `json:"entity_type"`
			Properties map[string]any `json:"properties"`
		} `json:"entities"`
		Relationships []struct {
			Source   string         `json:"source"`
			Target   string         `json:"target"`
			Type     string         `json:"type"`
			Metadata map[string]any `json:"metadata"`
		} `json:"relationships"`
		Metadata struct {
			Warnings  []string          `json:"staleness_warnings"`
			Status    map[string]string `json:"subsystem_status"`
			Freshness map[string]string `json:"data_freshness"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return err
	}
	v, _ := m.state.ProjectVisit(m.project)
	var b strings.Builder
	fmt.Fprintf(&b, "Project: %s · projection revision %s\n", result.Metadata.Status["project"], result.Metadata.Freshness["source_revision"])
	for _, w := range result.Metadata.Warnings {
		fmt.Fprintf(&b, "Coverage: %s\n", w)
	}
	sort.SliceStable(result.Entities, func(i, j int) bool {
		a, c := result.Entities[i], result.Entities[j]
		if v.Layout == "theme" {
			return fmt.Sprint(a.Properties["theme"])+a.ID < fmt.Sprint(c.Properties["theme"])+c.ID
		}
		return a.Kind+a.ID < c.Kind+c.ID
	})
	for _, e := range result.Entities {
		pace, _ := e.Properties["pace"].(string)
		if pace == "" {
			pace = "unknown"
		}
		if v.Pace != "all" && v.Pace != pace {
			continue
		}
		mark := ""
		if v.Selection == e.ID || v.Selection == fmt.Sprint(e.Properties["id"]) {
			mark = "› "
		}
		fmt.Fprintf(&b, "\n%s%s · %v · pace %s\n", mark, e.Kind, e.Properties["ruling_state"], pace)
		if v.Layout == "theme" {
			theme, _ := e.Properties["theme"].(string)
			if theme == "" {
				theme = "Unclassified"
			}
			fmt.Fprintf(&b, "Theme: %s\n", theme)
		}
		for _, key := range []string{"outcome", "text", "path", "status", "source_record"} {
			if value, ok := e.Properties[key]; ok {
				fmt.Fprintf(&b, "%s: %v\n", key, value)
			}
		}
	}
	if len(result.Relationships) > 0 {
		b.WriteString("\nSourced relationships\n")
		for _, edge := range result.Relationships {
			fmt.Fprintf(&b, "%s\n  %s → %s [%v]\n", edge.Source, edge.Type, edge.Target, edge.Metadata["classification"])
		}
	}
	m.projectMap = b.String()
	return nil
}
func (m *Model) visitInput(text string) (review.Request, bool) {
	v, _ := m.state.ProjectVisit(m.project)
	switch m.mode {
	case "correction":
		return review.Request{Method: "visit.correct", VisitID: v.ID, Target: m.correctionID, Text: m.input.Value()}, true
	case "outcome":
		return review.Request{Method: "visit.outcome", VisitID: v.ID, Text: m.input.Value()}, true
	case "preparation budget":
		budget, err := strconv.Atoi(text)
		if err != nil || budget <= 0 || m.preparing == nil {
			m.status = "Enter an explicit positive token allowance"
			return review.Request{}, true
		}
		return review.Request{Method: "prepare.submit", Target: m.preparing.ID, Revision: m.preparing.Revision, BudgetTokens: budget}, true
	case "start implementation":
		if text != "START" || m.preparing == nil {
			m.status = "Type START to approve the displayed specification and implementation budget"
			return review.Request{}, true
		}
		return review.Request{Method: "execution.start", Target: m.preparing.ID, Revision: m.preparing.Revision, Text: m.startDigest}, true
	}
	return review.Request{}, false
}
