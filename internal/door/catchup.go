package door

import (
	"context"
	"fmt"
	"os/exec"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

// CaptureThreadPane reads the current pane only. It never sends input.
func CaptureThreadPane(th Thread) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), listSessionsTimeout)
	defer cancel()
	out, err := exec.CommandContext(ctx, "tmux", "capture-pane", "-p", "-t", "="+th.Session+":").Output()
	return cleanEvidence(string(out)), err
}

// QuestionOnScreen confirms text, not a generic prompt glyph. A transcript
// question alone cannot establish that an agent is currently waiting.
func QuestionOnScreen(question, pane string) bool {
	first := strings.Split(question, "\n")[0]
	normalize := func(s string) string { return strings.Join(strings.Fields(cleanEvidence(s)), " ") }
	first = normalize(first)
	if len(first) < 12 {
		return false
	}
	return strings.Contains(normalize(pane), first)
}

func questionState(th Thread) string {
	if th.Conversation.Reply != "" && !th.QuestionVisible {
		return "later user reply · resolution unverified"
	}
	if th.Runtime == RuntimeShell {
		return "saved question · agent stopped"
	}
	if th.Runtime != th.Conversation.Provider {
		return "saved question · different process in seat"
	}
	if th.QuestionVisible {
		return "question on screen"
	}
	if th.PaneErr != nil {
		return "open question · pane unreadable"
	}
	return "open question · current wait unverified"
}

func (m Model) questions() []Thread {
	var out []Thread
	for _, th := range m.threads.Threads {
		if th.Conversation.Question != "" && (th.Conversation.Reply == "" || th.QuestionVisible) {
			out = append(out, th)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].QuestionVisible != out[j].QuestionVisible {
			return out[i].QuestionVisible
		}
		a, b := out[i].Conversation.QuestionAt, out[j].Conversation.QuestionAt
		if !a.Equal(b) {
			return a.After(b)
		}
		return out[i].Session < out[j].Session
	})
	return out
}

// A read in flight or a failed inventory cannot establish an empty question
// set. Share this state across the opening header, list and coverage footer.
func (m Model) conversationReadStatus() string {
	if !m.sessionsLoaded {
		return "Reading conversations…"
	}
	if m.threads.Err != nil {
		return "Conversations unreadable: " + m.threads.Err.Error()
	}
	if !m.threadsLoaded {
		return "Reading conversations…"
	}
	return ""
}

func (m Model) evidenceCoverage() string {
	if status := m.conversationReadStatus(); status != "" {
		return status
	}
	read, unread, unnamed := 0, 0, 0
	for _, th := range m.threads.Threads {
		switch {
		case th.Seat.ResumeID == "":
			unnamed++
		case th.Err != nil || th.Conversation.Updated.IsZero():
			unread++
		default:
			read++
		}
	}
	return fmt.Sprintf("Evidence: %d/%d seats read · %d unread · %d without conversation IDs", read, len(m.threads.Threads), unread, unnamed)
}

func (m Model) questionSummary() string {
	if status := m.conversationReadStatus(); status != "" {
		return "Questions: " + status
	}
	qs := m.questions()
	visible, saved := 0, 0
	for _, th := range qs {
		if th.QuestionVisible {
			visible++
		}
		if th.Runtime != th.Conversation.Provider {
			saved++
		}
	}
	return fmt.Sprintf("Questions: %d on screen · %d other open · %d saved · a to review", visible, len(qs)-visible-saved, saved)
}

// Dirty files have no timestamp; they cannot establish movement in a window.
func (m Model) catchupMovements() []Movement {
	byRoot := make(map[string]Movement)
	for root, mv := range m.movements {
		if mv.Err == nil && (len(mv.Commits) > 0 || mv.Sessions > 0) {
			byRoot[root] = mv
		}
	}
	for _, p := range m.projects {
		for _, th := range m.threads.ByRoot[p.Root] {
			if th.Conversation.Updated.After(m.window) {
				mv, ok := byRoot[p.Root]
				if !ok {
					mv = m.movements[p.Root]
					mv.Root, mv.Name = p.Root, p.Name
				}
				if th.Conversation.Updated.After(mv.Latest) {
					mv.Latest = th.Conversation.Updated
				}
				byRoot[p.Root] = mv
			}
		}
	}
	var out []Movement
	for _, mv := range byRoot {
		out = append(out, mv)
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].Latest.Equal(out[j].Latest) {
			return out[i].Latest.After(out[j].Latest)
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func oneLine(s string) string { return strings.Join(strings.Fields(cleanEvidence(s)), " ") }

func (m Model) catchupLines(room int) []string {
	lines := []string{
		"Here’s what moved while you were away.",
		m.questionSummary(), "",
	}
	if room < 12 {
		lines = []string{m.questionSummary()}
	}
	if m.briefing.SinceErr != nil {
		lines = append(lines, "Visit window: "+m.briefing.SinceErr.Error())
	}
	moved := m.catchupMovements()
	if m.moveRemaining > 0 || len(m.movements) < len(m.projects) {
		lines = append(lines, "Reading project changes…")
	}
	if len(moved) == 0 && m.moveRemaining == 0 && len(m.movements) == len(m.projects) {
		if m.conversationReadStatus() != "" {
			lines = append(lines, "No recent commits found in the readable projects.")
		} else {
			lines = append(lines, "No recent commits or conversation activity found in the readable sources.")
		}
	}
	start := max(0, min(m.catchupOffset, len(moved)-1))
	shown := 0
	for i := start; i < len(moved); i++ {
		mv := moved[i]
		card := m.movementCard(mv, room)
		if len(lines)+len(card) > room-2 {
			break
		}
		lines = append(lines, card...)
		shown++
	}
	if len(moved) > 0 {
		lines = append(lines, fmt.Sprintf("Changes %d–%d of %d · ↑/↓ scroll", min(start+1, start+shown), start+shown, len(moved)))
	}
	dirty, unread := 0, 0
	for _, mv := range m.movements {
		if mv.Err != nil {
			unread++
		} else {
			dirty += mv.Dirty
		}
	}
	lines = append(lines, fmt.Sprintf("Working tree now: %d uncommitted files (not dated) · %d projects unread", dirty, unread))
	if m.sessionsErr != nil {
		lines = append(lines, "Claude activity index unreadable; coverage is partial")
	}
	return lines
}

func questionIndex(list []Thread, selected string) int {
	for i, th := range list {
		if th.Session == selected {
			return i
		}
	}
	return 0
}

func (m Model) questionsLines(room int) []string {
	qs := m.questions()
	lines := []string{"Questions · select one to read its evidence", "Saved questions are history; they do not establish a current wait.", ""}
	if status := m.conversationReadStatus(); status != "" {
		lines = append(lines, status)
		if len(qs) == 0 {
			return lines
		}
	}
	if len(qs) == 0 {
		return append(lines, "No question without a later reply found in the readable tails.", "Unnamed seats and unreadable conversations are not covered.", "t → i shows history and replies for any named seat.")
	}
	sel := questionIndex(qs, m.questionSel)
	count := max(1, (room-len(lines)-1)/3)
	start := max(0, min(m.questionOffset, sel))
	if sel >= start+count {
		start = sel - count + 1
	}
	for i := start; i < len(qs) && i < start+count; i++ {
		th := qs[i]
		mark := "  "
		if i == sel {
			mark = "> "
		}
		lines = append(lines, mark+th.Seat.Topic+" · "+questionState(th), "  "+oneLine(strings.Split(th.Conversation.Question, "\n")[0]), "  "+string(th.Conversation.Provider)+" · "+th.Conversation.QuestionAt.Local().Format("Jan 2 15:04"))
	}
	return append(lines, fmt.Sprintf("Question %d of %d", sel+1, len(qs)))
}

func (m Model) detailThread() (Thread, bool) {
	for _, th := range m.threads.Threads {
		if th.Session == m.detailSession {
			return th, true
		}
	}
	return Thread{}, false
}

func (m Model) questionDetailLines() []string {
	th, ok := m.detailThread()
	if !ok {
		return []string{"This seat is no longer in the current snapshot. Esc returns to the list."}
	}
	c := th.Conversation
	lines := []string{th.Seat.Topic + " · " + questionState(th), ""}
	if c.Question != "" {
		lines = append(lines, c.Question)
	} else {
		lines = append(lines, "No unanswered question found in this transcript tail.")
	}
	if c.Context != "" {
		lines = append(lines, "", "Supporting context · agent's words", c.Context)
	}
	if c.Request != "" {
		lines = append(lines, "", "Last user request", c.Request)
	}
	if c.Reply != "" {
		lines = append(lines, "", "Later user reply · may or may not resolve this question", c.Reply)
	}
	if c.Report != "" && c.Report != c.Context && c.Report != c.Question {
		lines = append(lines, "", "Latest agent report · not independently verified", c.Report)
	}
	lines = append(lines, "", "Source: "+c.Source, fmt.Sprintf("%s · %s · byte %d", c.Provider, c.QuestionAt.Local().Format("2006-01-02 15:04:05 MST"), c.QuestionOffset), "Conversation: "+th.Seat.ResumeID, "Seat: "+th.Session)
	if c.QuestionLine > 0 {
		lines = append(lines, fmt.Sprintf("Transcript line: %d", c.QuestionLine))
	}
	if th.Err != nil {
		lines = append(lines, "Read error: "+th.Err.Error())
	}
	if th.Runtime == c.Provider {
		lines = append(lines, "", "Enter opens this seat. Answer in the original agent session.")
	} else {
		lines = append(lines, "", "Enter opens the seat as it is now; the saved conversation is not running there.", "s resumes the saved conversation here. No answer is sent by Autarch.")
	}
	var wrapped []string
	for _, line := range lines {
		wrapped = append(wrapped, strings.Split(ansi.Wrap(cleanEvidence(line), max(1, m.lineWidth()-2), ""), "\n")...)
	}
	return wrapped
}

func (m Model) handleQuestionKey(key string) (tea.Model, tea.Cmd) {
	if key == "t" {
		m.prevScreen = m.screen
		m.screen = screenThreads
		m.clampThreadScroll()
		return m, nil
	}
	if m.screen == screenQuestion {
		switch key {
		case "esc", "backspace":
			m.screen = m.detailFrom
		case "j", "down":
			m.detailOffset++
		case "k", "up":
			m.detailOffset--
		case "pgdown", "space":
			m.detailOffset += m.dashboardRoom()
		case "pgup":
			m.detailOffset -= m.dashboardRoom()
		case "home", "g":
			m.detailOffset = 0
		case "end", "G":
			m.detailOffset = len(m.dashboardDetailLines())
		case "enter":
			if th, ok := m.detailThread(); ok {
				return m, switchToThread(th)
			}
		case "s":
			if th, ok := m.detailThread(); ok && th.Runtime != th.Conversation.Provider {
				return m, resumeConversation(th)
			}
		}
		m.detailOffset = max(0, min(m.detailOffset, max(0, len(m.dashboardDetailLines())-m.dashboardRoom())))
		return m, nil
	}
	qs := m.questions()
	sel := questionIndex(qs, m.questionSel)
	switch key {
	case "esc", "a", "tab":
		m.screen = screenBriefing
	case "j", "down":
		sel++
	case "k", "up":
		sel--
	case "home", "g":
		sel = 0
	case "end", "G":
		sel = len(qs) - 1
	case "enter":
		if len(qs) > 0 {
			m.detailSession = qs[sel].Session
			m.detailOffset = 0
			m.detailFrom = screenQuestions
			m.screen = screenQuestion
		}
	}
	if len(qs) > 0 {
		sel = max(0, min(sel, len(qs)-1))
		m.questionSel = qs[sel].Session
	}
	return m, nil
}

// Resume is an explicit user action and uses argv, never shell interpolation.
// It opens the saved agent conversation without supplying a new prompt.
func resumeConversation(th Thread) tea.Cmd {
	id := th.Seat.ResumeID
	if !conversationID.MatchString(id) {
		return func() tea.Msg { return statusMsg("Cannot resume: invalid conversation ID") }
	}
	var cmd *exec.Cmd
	switch th.Conversation.Provider {
	case RuntimeClaude:
		cmd = exec.Command("claude", "--resume", id)
	case RuntimeCodex:
		cmd = exec.Command("codex", "resume", id)
	default:
		return func() tea.Msg { return statusMsg("This provider cannot be resumed from Autarch") }
	}
	cmd.Dir = th.Path
	if th.Conversation.WorkDir != "" {
		cmd.Dir = th.Conversation.WorkDir
	}
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return statusMsg("Resume failed: " + err.Error())
		}
		return statusMsg("Returned from the conversation · r refreshes the evidence")
	})
}

func (m Model) catchupView() string {
	width, room := m.lineWidth(), max(1, m.height-6)
	var lines []string
	keys := "↑/↓ scroll · a questions · t threads · tab projects · r refresh · q quit"
	switch m.screen {
	case screenQuestions:
		lines = m.questionsLines(room)
		keys = "↑/↓ select · enter evidence · esc back · r refresh · q quit"
	case screenQuestion:
		all := m.questionDetailLines()
		start := max(0, min(m.detailOffset, max(0, len(all)-room)))
		lines = all[start:min(len(all), start+room)]
		keys = "↑/↓ scroll · enter seat · s resume saved · esc back · r refresh · q quit"
	default:
		lines = m.catchupLines(room)
	}
	var b strings.Builder
	b.WriteString(styleTitle.Render("AUTARCH"))
	b.WriteString("\n")
	for i := 0; i < room; i++ {
		if i < len(lines) {
			b.WriteString(ansi.Truncate(lines[i], width, "…"))
		}
		b.WriteString("\n")
	}
	b.WriteString(styleCoverage.Render(ansi.Truncate(m.evidenceCoverage(), width, "…")))
	b.WriteString("\n")
	b.WriteString(styleFooter.Render(ansi.Truncate("Snapshot · r refreshes · t shows every seat, including unread evidence", width, "…")))
	b.WriteString("\n")
	b.WriteString(styleFooter.Render(ansi.Truncate(keys, width, "…")))
	b.WriteString("\n")
	b.WriteString(styleCoverage.Render(ansi.Truncate(m.status, width, "…")))
	return b.String()
}
