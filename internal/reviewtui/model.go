package reviewtui

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/mistakeknot/autarch/pkg/review"
)

type tickMsg struct{}
type resultMsg struct {
	response review.Response
	err      string
	saved    bool
}
type readyMsg struct {
	client review.Client
	err    error
}
type ClosedMsg struct{}

type Model struct {
	project, density                      string
	client                                review.Client
	state                                 review.State
	width, height, tab, selection, scroll int
	mode, status                          string
	detail, closed, busy                  bool
	input                                 textarea.Model
	storage                               int64
	reviewed                              *review.Proposal
	pendingSave                           *review.Request
	pendingText, pendingMode              string
	answering                             *review.Question
	chatScroll                            int
}

func New(project, density string, client review.Client) *Model {
	if p, err := filepath.EvalSymlinks(project); err == nil {
		project = p
	}
	input := textarea.New()
	input.Placeholder = "Write your observation…"
	input.SetHeight(4)
	input.CharLimit = 20000
	return &Model{project: project, density: density, client: client, input: input, state: review.State{Version: review.Version}}
}
func (m *Model) Init() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		c, err := review.EnsureController(ctx)
		return readyMsg{c, err}
	}
}
func tick() tea.Cmd { return tea.Tick(time.Second, func(time.Time) tea.Msg { return tickMsg{} }) }
func (m *Model) call(r review.Request, saved bool) tea.Cmd {
	if r.Project == "" {
		r.Project = m.project
	}
	if r.ID == "" {
		r.ID = review.NewID()
	}
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		response, err := client.Call(ctx, r)
		msg := resultMsg{response: response, saved: saved}
		if err != nil {
			msg.err = err.Error()
		}
		return msg
	}
}
func (m *Model) Update(msg tea.Msg) tea.Cmd {
	switch v := msg.(type) {
	case readyMsg:
		if v.err != nil {
			m.status = v.err.Error()
			return tick()
		}
		m.client = v.client
		return m.context()
	case tickMsg:
		return m.call(review.Request{Method: "state"}, false)
	case resultMsg:
		if v.err != "" {
			m.status = v.err
			m.busy = false
			return tick()
		}
		if v.response.State != nil {
			oldItems := m.items()
			oldID := ""
			if m.selection < len(oldItems) {
				oldID = oldItems[m.selection]
			}
			m.state = *v.response.State
			for i, id := range m.items() {
				if id == oldID {
					m.selection = i
					break
				}
			}
			m.storage = v.response.StorageBytes
			return tick()
		}
		if v.saved {
			m.status = "Saved locally"
			m.input.Reset()
			m.mode = ""
			m.busy = false
			m.pendingSave = nil
		}
		return m.call(review.Request{Method: "state"}, false)
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
		m.input.SetWidth(max(10, v.Width-6))
		return nil
	case tea.KeyMsg:
		key := v.String()
		if m.mode != "" {
			if m.busy {
				return nil
			}
			if key == "esc" && !m.busy {
				m.mode = ""
				return nil
			}
			if key == "ctrl+s" && !m.busy {
				return m.saveInput()
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(v)
			return cmd
		}
		switch key {
		case "esc":
			if m.detail {
				m.detail = false
				m.scroll = 0
				return nil
			}
			m.closed = true
			return func() tea.Msg { return ClosedMsg{} }
		case "q", "ctrl+c":
			m.closed = true
			return func() tea.Msg { return ClosedMsg{} }
		case "tab":
			m.tab = (m.tab + 1) % 4
			m.selection, m.scroll = 0, 0
			m.detail = false
		case "1", "2", "3", "4":
			m.tab = int(key[0] - '1')
			m.selection, m.scroll = 0, 0
			m.detail = false
		case "d":
			if m.density == "Cozy" {
				m.density = "Compact"
			} else {
				m.density = "Cozy"
			}
			return m.context()
		case "down", "j":
			if m.detail {
				m.scroll++
			} else {
				m.selection = min(m.selection+1, max(0, len(m.items())-1))
			}
		case "up", "k":
			if m.detail {
				m.scroll = max(0, m.scroll-1)
			} else {
				m.selection = max(0, m.selection-1)
			}
		case "pgdown":
			m.scroll += max(1, m.height-10)
		case "pgup":
			m.scroll = max(0, m.scroll-max(1, m.height-10))
		case "enter":
			m.reviewed = nil
			if m.tab == 1 {
				if p, ok := m.selectedProposal(); ok {
					m.reviewed = &p
				}
			}
			m.detail = true
			m.scroll = 0
		case "ctrl+n":
			m.mode = "note"
			m.input.Placeholder = "What did you notice? Ctrl+S saves locally."
			m.input.Focus()
			return textarea.Blink
		case "ctrl+f":
			m.mode = "chat"
			m.input.Placeholder = "Ask Flere about this project…"
			m.input.Focus()
			return textarea.Blink
		case "ctrl+q":
			if q := m.pendingQuestion(); q != nil {
				m.answering = q
				m.mode = "answer"
				m.input.Placeholder = q.Title
				m.input.Focus()
				return textarea.Blink
			}
		case "ctrl+g":
			return m.call(review.Request{Method: "runtime.cancel"}, false)
		case "ctrl+up":
			m.chatScroll++
		case "ctrl+down":
			m.chatScroll = max(0, m.chatScroll-1)
		case "ctrl+a":
			if m.tab == 1 && m.detail {
				if p, ok := m.selectedProposal(); ok {
					m.status = "Recording acceptance…"
					return m.call(review.Request{Method: "proposal.accept", Target: p.ID, Revision: p.Revision}, false)
				}
			}
		case "ctrl+x":
			if m.tab == 1 && m.detail {
				if p, ok := m.selectedProposal(); ok {
					return m.call(review.Request{Method: "proposal.reject", Target: p.ID, Revision: p.Revision}, false)
				}
			}
		case "ctrl+r":
			return m.call(review.Request{Method: "capture.command", Text: "open"}, false)
		case "ctrl+p":
			command := "pause"
			for _, s := range m.state.Sessions {
				if s.Project == m.project && s.Status == "paused" {
					command = "resume"
				}
			}
			return m.call(review.Request{Method: "capture.command", Text: command}, false)
		case "ctrl+o":
			return m.call(review.Request{Method: "capture.command", Text: "stop"}, false)
		case "ctrl+v":
			return m.call(review.Request{Method: "capture.command", Text: "voice"}, false)
		case "ctrl+e":
			if source := m.selectedEvidence(); source != nil {
				return m.call(review.Request{Method: "capture.command", Text: "play", Source: source}, false)
			}
		case "ctrl+t":
			if m.tab == 2 {
				m.mode = "verdict"
				m.input.Placeholder = "pass, fail or inconclusive; then your retest notes"
				m.input.Focus()
				return textarea.Blink
			}
		}
	}
	return nil
}
func (m *Model) context() tea.Cmd {
	return m.call(review.Request{Method: "context", Context: &review.UIContext{View: "review", Project: m.project, Density: m.density, Build: review.BuildIdentity()}}, false)
}
func (m *Model) saveInput() tea.Cmd {
	text := strings.TrimSpace(m.input.Value())
	if text == "" {
		return nil
	}
	if m.pendingSave != nil && m.pendingText == text && m.pendingMode == m.mode {
		m.busy = true
		return m.call(*m.pendingSave, true)
	}
	r := review.Request{}
	switch m.mode {
	case "note":
		r.Method = "feedback.save"
		r.Feedback = &review.Feedback{Text: text, Context: review.UIContext{At: time.Now().UTC(), View: "review", Project: m.project, Density: m.density, Build: review.BuildIdentity()}}
		for _, s := range m.state.Sessions {
			if s.Project == m.project && (s.Status == "recording" || s.Status == "paused") {
				r.Feedback.SessionID = s.ID
				break
			}
		}
	case "chat":
		r.Method = "turn.save"
		r.Turn = &review.Turn{Kind: "user", Text: text}
	case "answer":
		q := m.answering
		if q == nil {
			m.status = "Question no longer pending"
			return nil
		}
		valid := false
		for _, current := range m.state.Questions {
			if current.ID == q.ID && current.RuntimeSession == q.RuntimeSession && current.Status == "pending" {
				valid = true
			}
		}
		if !valid {
			m.status = "This question is no longer pending. Your draft is retained."
			return nil
		}
		r.Method = "question.answer"
		r.Target = q.ID
		r.Text = text
	case "verdict":
		items := m.items()
		if m.selection >= len(items) {
			return nil
		}
		e := m.state.Executions[items[m.selection]]
		verdict, notes, _ := strings.Cut(text, " ")
		r.Method = "verdict.save"
		r.Verdict = &review.Verdict{ExecutionID: e.ID, Build: e.Build, Verdict: verdict, Notes: notes}
	}
	m.busy = true
	m.status = "Saving…"
	r.ID = review.NewID()
	m.pendingSave = &r
	m.pendingText, m.pendingMode = text, m.mode
	return m.call(r, true)
}
func (m *Model) items() []string {
	var ids []string
	switch m.tab {
	case 0:
		for id, v := range m.state.Feedback {
			if v.Project == m.project || v.Project == "" {
				ids = append(ids, id)
			}
		}
	case 1:
		for id, v := range m.state.Proposals {
			if v.Project == m.project {
				ids = append(ids, id)
			}
		}
	case 2:
		for id, v := range m.state.Executions {
			if v.Project == m.project {
				ids = append(ids, id)
			}
		}
	case 3:
		for id, v := range m.state.Sessions {
			if v.Project == m.project {
				ids = append(ids, id)
			}
		}
	}
	sort.Strings(ids)
	return ids
}
func (m *Model) selectedProposal() (review.Proposal, bool) {
	if m.detail && m.reviewed != nil {
		return *m.reviewed, true
	}
	items := m.items()
	if m.selection >= len(items) {
		return review.Proposal{}, false
	}
	p, ok := m.state.Proposals[items[m.selection]]
	return p, ok
}
func (m *Model) pendingQuestion() *review.Question {
	for _, q := range m.state.Questions {
		if q.Project == m.project && q.Status == "pending" {
			return &q
		}
	}
	return nil
}
func (m *Model) selectedEvidence() *review.Source {
	items := m.items()
	if m.selection >= len(items) {
		return nil
	}
	var sources []review.Source
	switch m.tab {
	case 0:
		sources = m.state.Feedback[items[m.selection]].Evidence
	case 1:
		sources = m.state.Proposals[items[m.selection]].Evidence
	case 3:
		sources = m.state.Sessions[items[m.selection]].Media
	}
	for _, s := range sources {
		if s.Status == "available" {
			return &s
		}
	}
	return nil
}
func (m *Model) body() string {
	items := m.items()
	selected := m.selection
	if m.detail && m.tab == 1 && m.reviewed != nil {
		items = []string{m.reviewed.ID}
		selected = 0
	}
	if len(items) == 0 {
		return []string{"No feedback yet. Ctrl+N adds an observation; Ctrl+R selects a review window.", "Flere's proposals appear here with evidence and enduring guidance.", "Accepted work and build-specific retest checklists appear here.", "Ctrl+R opens the companion to select a window and explicitly start recording."}[m.tab]
	}
	selected = min(selected, len(items)-1)
	var b strings.Builder
	for i, id := range items {
		if m.detail && i != selected {
			continue
		}
		mark := "  "
		if i == selected {
			mark = "› "
		}
		switch m.tab {
		case 0:
			v := m.state.Feedback[id]
			fmt.Fprintf(&b, "%s%s · %s\n%s\n", mark, v.At.Local().Format("15:04:05"), v.Analysis, v.Text)
			if m.detail {
				fmt.Fprintf(&b, "\nOriginal: %s\nSession: %s\nRevision %d\n", v.OriginalText, v.SessionID, v.Revision)
				for _, e := range v.Evidence {
					fmt.Fprintf(&b, "%s · %s · %d ms\n", e.Kind, e.Status, e.OffsetMS)
				}
			}
		case 1:
			p := m.state.Proposals[id]
			if m.detail && m.reviewed != nil {
				p = *m.reviewed
			}
			fmt.Fprintf(&b, "%s%s · %s\n", mark, p.Outcome, p.Status)
			if m.detail {
				fmt.Fprintf(&b, "Revision %d\n\nImmediate change: %s\nScope: %s\nWhy: %s\nPriority: P%d · Budget: %d tokens\nDependencies: %s\n", p.Revision, p.Change, strings.Join(p.Scope, ", "), p.Rationale, p.Priority, p.BudgetTokens, strings.Join(p.Dependencies, ", "))
				fmt.Fprintf(&b, "Tracker: %s\nBuild: %s → %s\nBudget checked on reported model turns; an active turn can exceed it.\n", p.Tracker, strings.Join(p.Build.Command, " "), p.Build.Binary)
				for _, check := range p.Build.Checks {
					fmt.Fprintf(&b, "Check: %s\n", strings.Join(check, " "))
				}
				for _, fid := range p.FeedbackIDs {
					fmt.Fprintf(&b, "\nOriginal observation: %s\n", m.state.Feedback[fid].OriginalText)
				}
				for _, g := range p.Guidance {
					fmt.Fprintf(&b, "\nEnduring guidance: %s\n%s\nScope: %s\nWhy: %s\nBase revision: %s\n", g.Path, g.Text, g.Scope, g.Rationale, g.BaseRevision)
				}
				fmt.Fprintf(&b, "\nUncertainty: %s\nChallenge: %s\nRetest: %s\n", strings.Join(p.Uncertainties, "; "), p.Pushback, strings.Join(p.Checklist, "; "))
				for _, e := range p.Evidence {
					fmt.Fprintf(&b, "Evidence: %s [%s] %s\n", e.Path, e.Status, e.Revision)
				}
				b.WriteString("\nCtrl+A accepts this change AND guidance · Ctrl+X rejects\n")
			}
		case 2:
			e := m.state.Executions[id]
			p := m.state.Proposals[e.ProposalID]
			fmt.Fprintf(&b, "%s%s · %s\n%s\n", mark, p.Outcome, e.Status, e.Reason)
			if m.detail {
				fmt.Fprintf(&b, "Work: %s · Run: %s\nModel: %s\nBuild: %s\nBinary: %s\n", e.WorkID, e.RunID, e.Model, e.Build, e.Binary)
				for _, fid := range p.FeedbackIDs {
					fmt.Fprintf(&b, "Original feedback: %s\n", m.state.Feedback[fid].OriginalText)
				}
				for _, c := range p.Checklist {
					fmt.Fprintf(&b, "□ %s\n", c)
				}
				b.WriteString("Ctrl+T records your verdict against this build.\n")
			}
		case 3:
			s := m.state.Sessions[id]
			fmt.Fprintf(&b, "%s%s · %s\n%s\n", mark, s.WindowTitle, s.Status, s.StartedAt.Local().Format(time.RFC822))
			if s.Error != "" {
				fmt.Fprintln(&b, s.Error)
			}
			if m.detail {
				for _, e := range s.Media {
					fmt.Fprintf(&b, "%s · %s · %s\n", e.Kind, e.Status, e.Path)
				}
			}
		}
		if m.density == "Cozy" {
			b.WriteString("\n")
		}
	}
	return b.String()
}
func (m *Model) conversation() string {
	var b strings.Builder
	b.WriteString("FLERE · project conversation\n\n")
	start := max(0, len(m.state.Turns)-30)
	for _, t := range m.state.Turns[start:] {
		if t.Project == m.project {
			fmt.Fprintf(&b, "%s · %s\n%s\n", t.Kind, t.Model, t.Text)
			if t.RuntimeSession != "" {
				fmt.Fprintf(&b, "Session: %s\n", t.RuntimeSession)
			}
			b.WriteString("\n")
		}
	}
	for _, t := range m.state.Streams {
		if t.Project == m.project {
			fmt.Fprintf(&b, "Flere · responding\n%s\n", t.Text)
		}
	}
	if q := m.pendingQuestion(); q != nil {
		fmt.Fprintf(&b, "DECISION: %s\n%s\nCtrl+Q to answer\n", q.Title, strings.Join(q.Options, " / "))
	}
	return b.String()
}
func fit(s string, width, height, offset int) string {
	lines := strings.Split(ansi.Wrap(s, max(1, width), ""), "\n")
	offset = min(max(0, offset), max(0, len(lines)-height))
	lines = lines[offset:min(len(lines), offset+height)]
	for len(lines) < height {
		lines = append(lines, "")
	}
	for i, line := range lines {
		lines[i] = ansi.Truncate(line, width, "")
	}
	return strings.Join(lines, "\n")
}
func (m *Model) chatView(width, height int) string {
	content := m.conversation()
	lines := strings.Split(ansi.Wrap(content, max(1, width), ""), "\n")
	return fit(content, width, height, max(0, len(lines)-height-m.chatScroll))
}
func (m *Model) View() string {
	width, height := max(30, m.width), max(10, m.height)
	title := fmt.Sprintf("AUTARCH REVIEW · %s · %s · %.1f MB local", filepath.Base(m.project), m.density, float64(m.storage)/1048576)
	tabs := []string{"1 Feedback", "2 Proposals", "3 Retest", "4 Sessions"}
	tabs[m.tab] = "[" + tabs[m.tab] + "]"
	status := m.status
	for _, s := range m.state.Sessions {
		if s.Project == m.project && (s.Status == "recording" || s.Status == "paused") {
			status = s.Status + " · " + s.WindowTitle + " | " + status
		}
	}
	room := height - 7
	if m.mode != "" {
		room -= 6
	}
	room = max(1, room)
	body := m.body()
	if width >= 100 {
		left := width * 3 / 5
		body = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(left).Render(fit(body, left-2, room, m.scroll)), "│ ", m.chatView(width-left-3, room))
	} else {
		if m.tab == 1 && !m.detail {
			body += "\n" + m.conversation()
		}
		body = fit(body, width-2, room, m.scroll)
	}
	lines := []string{ansi.Truncate(title, width, ""), strings.Join(tabs, "  "), "", body}
	if m.mode != "" {
		lines = append(lines, m.mode+" · Ctrl+S save · Esc back", m.input.View())
	}
	lines = append(lines, ansi.Truncate(status, width, ""), ansi.Truncate("Ctrl+R record · Ctrl+N note · Ctrl+V voice · Ctrl+P pause/resume · Ctrl+O stop", width, ""), ansi.Truncate("Ctrl+F Flere · Ctrl+E evidence · Enter details · d density · Esc back", width, ""))
	return strings.Join(lines, "\n")
}
