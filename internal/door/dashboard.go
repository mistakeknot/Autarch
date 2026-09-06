package door

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type dashboardButton struct {
	label, key  string
	x, y, width int
}

func (m Model) dashboardButtons() []dashboardButton {
	labels := []string{"1 Catch-up", "2 Questions", "3 Projects", "4 Threads"}
	keys := []string{"1", "2", "3", "4"}
	if m.screen == screenProduct {
		labels = []string{"1 Brief", "2 Roadmap", "3 Backlog", "4 Journeys", "5 Decisions", "6 Foundation"}
		keys = []string{"1", "2", "3", "4", "5", "6"}
	}
	if m.lineWidth() < 65 && m.screen != screenProduct {
		labels = []string{"1 Home", "2 Ask", "3 Projects", "4 Threads"}
	}
	var out []dashboardButton
	x := 1
	for i, label := range labels {
		w := ansi.StringWidth(label) + 2
		out = append(out, dashboardButton{label: label, key: keys[i], x: x, y: 1, width: w})
		x += w + 1
	}
	out = append(out, dashboardButton{label: "View: " + m.density.String() + " [d/v]", key: "v", x: 1, y: 2, width: 23})
	if m.briefingOn() && m.screen != screenProduct {
		out = append(out, dashboardButton{label: "Time range [w]", key: "w", x: 26, y: 2, width: 14})
	}
	return out
}

func (m Model) dashboardRoom() int {
	chrome := 9
	if m.reviewAttention != "" {
		chrome++
	}
	return max(1, m.height-chrome)
}
func (m Model) dashboardPadding() int {
	if m.density == DensityCozy && m.lineWidth() >= 60 {
		return 2
	}
	return 1
}
func (m Model) dashboardContentWidth() int { return max(1, m.lineWidth()-2-2*m.dashboardPadding()) }

func (m Model) dashboardFrame(title string, body []string, keys string) string {
	w := m.lineWidth()
	selected := 0
	switch m.screen {
	case screenQuestions, screenQuestion:
		selected = 1
	case screenRows:
		selected = 2
	case screenThreads:
		selected = 3
	case screenProduct:
		selected = m.productSection
	}
	brand := styleTitle.Render(" AUTARCH") + styleCoverage.Render("  /  "+title)
	var nav strings.Builder
	nav.WriteString(" ")
	for i, b := range m.dashboardButtons() {
		if b.y != 1 {
			continue
		}
		st := lipgloss.NewStyle().Foreground(palette.FgSecondary)
		if i == selected {
			st = st.Bold(true).Foreground(palette.Interactive).Background(palette.BgSelected)
		}
		nav.WriteString(st.Render(" "+b.label+" ") + " ")
	}
	control := " View: " + m.density.String() + " [d/v]"
	if m.briefingOn() && m.screen != screenProduct {
		control += strings.Repeat(" ", max(1, 26-ansi.StringWidth(control))) + "Time range [w]"
		if w >= 75 {
			control += "  · " + m.displayStatus()
		}
	} else {
		control += "   o Source files"
	}
	if w >= 95 {
		control += "   ? Help"
	}
	border := lipgloss.NewStyle().Foreground(palette.BorderDefault)
	inner := max(0, w-2)
	lines := []string{brand, nav.String(), styleCoverage.Render(control), border.Render("╭" + strings.Repeat("─", inner) + "╮")}
	pad := strings.Repeat(" ", m.dashboardPadding())
	for i := 0; i < m.dashboardRoom(); i++ {
		line := ""
		if i < len(body) {
			line = body[i]
		}
		line = ansi.Truncate(line, m.dashboardContentWidth(), "…")
		fill := strings.Repeat(" ", max(0, inner-2*len(pad)-ansi.StringWidth(line)))
		lines = append(lines, border.Render("│")+pad+line+fill+pad+border.Render("│"))
	}
	lines = append(lines, border.Render("╰"+strings.Repeat("─", inner)+"╯"))
	coverage, detail := m.evidenceCoverage(), "Saved questions are history · Enter opens evidence before a session"
	if m.screen == screenRows || m.screen == screenThreads {
		footer := m
		footer.reviewAttention = "" // The dashboard reserves its own attention row.
		old := strings.Split(footer.renderFooter(), "\n")
		if len(old) > 1 {
			coverage, detail = old[0], old[1]
		}
	}
	if m.screen == screenProduct {
		coverage, detail = "Product context · source declarations, not measured outcomes", m.productRoot
	}
	if m.reviewAttention != "" {
		lines = append(lines, styleCoverage.Render(" "+m.reviewAttention))
	}
	lines = append(lines, styleCoverage.Render(" "+coverage), styleCoverage.Render(" "+detail), styleFooter.Render(" "+keys), styleCoverage.Render(" "+oneLine(m.status)))
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], w, "…")
	}
	if m.height > 0 && len(lines) > m.height {
		lines = lines[:m.height]
	}
	return strings.Join(lines, "\n")
}

func (m Model) dashboardDetailLines() []string {
	content := m
	content.width = m.dashboardContentWidth() + 2
	return content.questionDetailLines()
}

func (m Model) dashboardView() string {
	if !m.briefingOn() && m.screen == screenBriefing {
		m.screen = screenRows
	}
	content := m
	content.width = m.dashboardContentWidth()
	room := m.dashboardRoom()
	title, keys := "Catch up", "↑↓ scroll · a Questions · Tab Projects · w Time range · d View · ? Help · q Quit"
	var lines []string
	switch m.screen {
	case screenQuestions:
		title, keys = "Questions", "↑↓ select · Enter evidence · Esc back · d View · ? Help · r Refresh · q Quit"
		lines = content.questionsLines(room)
	case screenQuestion:
		title, keys = "Question & context", "↑↓ scroll · Enter session · s Resume saved · Esc back · ? Help · q Quit"
		all := m.dashboardDetailLines()
		start := max(0, min(m.detailOffset, len(all)-room))
		lines = all[start:min(len(all), start+room)]
	case screenRows:
		title, keys = "Projects", "↑↓ move · i product · enter switch/open · z card · Tab Catch-up · d View · q Quit"
		stride := 1
		if m.density == DensityCozy && room >= 12 {
			stride = 2
		}
		content.height = room/stride + content.frameChromeHeight()
		content.clampScroll()
		sel := content.selIndex()
		for i := content.offset; i < len(content.projects) && len(lines)+stride <= room; i++ {
			lines = append(lines, content.renderRow(content.projects[i], i == sel))
			if stride == 2 {
				lines = append(lines, "")
			}
		}
	case screenThreads:
		title, keys = "Threads", "↑↓ move · Enter session · i evidence · t back · d View · r Refresh · q Quit"
		content.height = room + content.frameChromeHeight()
		content.clampThreadScroll()
		lines = content.threadsLines(room)
	default:
		lines = content.catchupLines(room)
	}
	return m.dashboardFrame(title, lines, keys)
}

func wrappedExcerpt(text string, width, limit int) []string {
	lines := strings.Split(ansi.Wrap(oneLine(text), max(1, width), ""), "\n")
	if len(lines) > limit {
		lines = lines[:limit]
		lines[limit-1] = ansi.Truncate(lines[limit-1], max(1, width-1), "") + "…"
	}
	return lines
}

func (m Model) movementCard(mv Movement, room int) []string {
	head := fmt.Sprintf("%s  ·  %s", mv.Name, humanAgo(m.now().Sub(mv.Latest)))
	lines := []string{styleTitle.Render(head)}
	summary := "Conversation activity; no recent commit found"
	if len(mv.Commits) > 0 {
		c := mv.Commits[0]
		summary = fmt.Sprintf("%s: %s (%s across local refs)", c.Hash[:min(7, len(c.Hash))], cleanEvidence(c.Subject), plural(len(mv.Commits), "commit"))
	} else if mv.Err != nil {
		summary = "Git unreadable: " + mv.Err.Error()
	}
	limit := 1
	if m.density == DensityCozy && room >= 14 {
		limit = 2
	}
	for _, line := range wrappedExcerpt(summary, m.lineWidth()-2, limit) {
		lines = append(lines, "  "+line)
	}
	report := ""
	for _, th := range m.threads.ByRoot[mv.Root] {
		if th.Conversation.Report != "" && th.Conversation.ReportAt.After(m.window) {
			report = fmt.Sprintf("%s · %s reported: %s", th.Seat.Topic, th.Conversation.Provider, oneLine(th.Conversation.Report))
			break
		}
	}
	if report == "" {
		report = "No recent agent report linked to a named seat"
	}
	for _, line := range wrappedExcerpt(report, m.lineWidth()-2, limit) {
		lines = append(lines, styleCoverage.Render("  "+line))
	}
	if m.density == DensityCozy && room >= 14 {
		lines = append(lines, "")
	}
	return lines
}
