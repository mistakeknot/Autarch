package door

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mistakeknot/autarch/pkg/tui/theme"
)

// resultMsg carries one finished check. Rows are matched by Root, not index:
// the list re-ranks as results stream in, so positions are not identities.
type resultMsg struct{ p Project }

// statusMsg is a transient footer note (Zed launched, pin saved, errors).
type statusMsg string

type checksDoneMsg struct{}

var (
	palette = theme.Current().Semantic()

	styleTitle    = lipgloss.NewStyle().Bold(true).Foreground(palette.Interactive)
	styleCoverage = lipgloss.NewStyle().Foreground(palette.FgSecondary)
	styleFooter   = lipgloss.NewStyle().Foreground(palette.FgTertiary)
	styleErr      = lipgloss.NewStyle().Foreground(palette.StatusError)
	styleSelected = lipgloss.NewStyle().Background(palette.BgSelected).Bold(true)
	styleFunded   = lipgloss.NewStyle().Foreground(palette.StatusWarning)
	stylePinned   = lipgloss.NewStyle().Foreground(palette.StatusInfo)
	styleReason   = lipgloss.NewStyle().Foreground(palette.FgTertiary)

	// The four checker verdicts plus unchecked, each visibly distinct --
	// a DONE WHEN clause, not a styling nicety.
	verdictStyles = map[Verdict]lipgloss.Style{
		VerdictConfirmed:   lipgloss.NewStyle().Foreground(palette.StatusSuccess),
		VerdictProvisional: lipgloss.NewStyle().Foreground(palette.StatusWarning),
		VerdictInvalid:     lipgloss.NewStyle().Foreground(palette.StatusError).Bold(true),
		VerdictAbsent:      lipgloss.NewStyle().Foreground(palette.FgTertiary),
		VerdictUnchecked:   lipgloss.NewStyle().Foreground(palette.StatusError).Reverse(true),
	}
)

// Model is the door: the ranked estate, one project per row.
type Model struct {
	projects    []Project
	ranking     Ranking
	rankingPath string
	rankingErr  error
	checker     string
	checkerErr  error

	results   chan resultMsg
	remaining int // checks still in flight; 0 means the estate is fully reported

	selRoot string
	moved   bool // user has navigated; until then selection tracks the top row
	offset  int
	width   int
	height  int
	status  string
}

// NewModel builds the door over an already-discovered estate. checkerErr
// records why the checker is unavailable; the model then shows every row as
// unchecked rather than pretending the estate has no cards.
func NewModel(projects []Project, ranking Ranking, rankingPath string, rankingErr error, checker string, checkerErr error) Model {
	ranking.Apply(projects)
	Rank(projects)
	m := Model{
		projects:    projects,
		ranking:     ranking,
		rankingPath: rankingPath,
		rankingErr:  rankingErr,
		checker:     checker,
		checkerErr:  checkerErr,
		results:     make(chan resultMsg, len(projects)),
	}
	if len(projects) > 0 {
		m.selRoot = projects[0].Root
	}
	return m
}

func (m Model) Init() tea.Cmd {
	if m.checkerErr != nil || len(m.projects) == 0 {
		return nil
	}
	return tea.Batch(m.startChecks(), m.waitForResult())
}

// startChecks fans the checker out over the estate in the background; each
// finished project lands on the results channel in completion order.
func (m Model) startChecks() tea.Cmd {
	projects := make([]Project, len(m.projects))
	copy(projects, m.projects)
	checker := m.checker
	results := m.results
	return func() tea.Msg {
		go CheckAll(context.Background(), checker, projects, func(_ int, p Project) {
			results <- resultMsg{p: p}
		})
		return checksStartedMsg{count: len(projects)}
	}
}

type checksStartedMsg struct{ count int }

func (m Model) waitForResult() tea.Cmd {
	results := m.results
	return func() tea.Msg { return <-results }
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampScroll()
		return m, nil

	case checksStartedMsg:
		m.remaining = msg.count
		return m, nil

	case resultMsg:
		for i := range m.projects {
			if m.projects[i].Root == msg.p.Root {
				m.projects[i] = msg.p
				break
			}
		}
		Rank(m.projects)
		// Until the user navigates, selection stays on the top row -- the
		// weakest card, which is what the door opens to show. Following a
		// project identity through the re-rank before any keypress would
		// scroll the view to wherever that project happened to land.
		if !m.moved && len(m.projects) > 0 {
			m.selRoot = m.projects[0].Root
		}
		m.clampScroll()
		if m.remaining > 0 {
			m.remaining--
		}
		if m.remaining > 0 {
			return m, m.waitForResult()
		}
		return m, nil

	case statusMsg:
		m.status = string(msg)
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if s := msg.String(); s == "q" || s == "ctrl+c" || s == "esc" {
		return m, tea.Quit
	}
	// Any other key is the user taking the wheel: selection stops tracking
	// the top row and starts following the selected project through re-ranks.
	m.moved = true

	sel := m.selIndex()
	switch msg.String() {
	case "up", "k":
		if sel > 0 {
			m.selRoot = m.projects[sel-1].Root
		}
	case "down", "j":
		if sel >= 0 && sel < len(m.projects)-1 {
			m.selRoot = m.projects[sel+1].Root
		}
	case "g", "home":
		if len(m.projects) > 0 {
			m.selRoot = m.projects[0].Root
		}
	case "G", "end":
		if len(m.projects) > 0 {
			m.selRoot = m.projects[len(m.projects)-1].Root
		}
	case "enter", "z":
		if sel >= 0 {
			return m, openInZed(m.projects[sel])
		}
	case "p":
		if sel >= 0 {
			return m.togglePin(m.projects[sel])
		}
	case "r":
		if m.checkerErr == nil && m.remaining == 0 && len(m.projects) > 0 {
			for i := range m.projects {
				m.projects[i].Verdict = VerdictUnchecked
				m.projects[i].Err = nil
			}
			m.status = "re-checking the estate"
			return m, tea.Batch(m.startChecks(), m.waitForResult())
		}
	}
	m.clampScroll()
	return m, nil
}

// openInZed hands the card path to the zed CLI. An absent card opens too --
// that is ruling 12's backfill-on-first-touch made literal: the door opens
// the empty spot where the card goes.
func openInZed(p Project) tea.Cmd {
	path := p.CardPath
	name := p.Name
	return func() tea.Msg {
		cmd := exec.Command("zed", path)
		if err := cmd.Start(); err != nil {
			return statusMsg(fmt.Sprintf("zed failed: %v", err))
		}
		go func() { _ = cmd.Wait() }() // reap; the door does not babysit Zed
		return statusMsg(fmt.Sprintf("opened %s/docs/why.md in Zed", name))
	}
}

func (m Model) togglePin(p Project) (tea.Model, tea.Cmd) {
	if p.Funded {
		m.status = p.Name + " is funded -- funded outranks pins, nothing to toggle"
		return m, nil
	}
	pinned := m.ranking.TogglePin(p.Name)
	if err := SaveRanking(m.rankingPath, m.ranking); err != nil {
		m.status = fmt.Sprintf("pin not saved: %v", err)
		return m, nil
	}
	m.ranking.Apply(m.projects)
	Rank(m.projects)
	m.clampScroll()
	if pinned {
		m.status = "pinned " + p.Name
	} else {
		m.status = "unpinned " + p.Name
	}
	return m, nil
}

func (m Model) selIndex() int {
	for i := range m.projects {
		if m.projects[i].Root == m.selRoot {
			return i
		}
	}
	return -1
}

// chromeHeight is the door's own frame: title, coverage, blank, footer.
const chromeHeight = 4

func (m *Model) visibleRows() int {
	rows := m.height - chromeHeight
	if rows < 1 {
		rows = 1
	}
	return rows
}

func (m *Model) clampScroll() {
	sel := m.selIndex()
	if sel < 0 {
		m.offset = 0
		return
	}
	rows := m.visibleRows()
	if sel < m.offset {
		m.offset = sel
	}
	if sel >= m.offset+rows {
		m.offset = sel - rows + 1
	}
}

func (m Model) View() string {
	var b strings.Builder

	cov := Cover(m.projects)
	b.WriteString(styleTitle.Render("AUTARCH DOOR"))
	b.WriteString(styleCoverage.Render(fmt.Sprintf("  %d projects", cov.Total)))
	b.WriteString("\n")
	b.WriteString(styleCoverage.Render(coverageLine(cov)))
	b.WriteString("\n")

	rows := m.visibleRows()
	end := m.offset + rows
	if end > len(m.projects) {
		end = len(m.projects)
	}
	sel := m.selIndex()
	for i := m.offset; i < end; i++ {
		b.WriteString(m.renderRow(m.projects[i], i == sel))
		b.WriteString("\n")
	}
	for i := end - m.offset; i < rows; i++ {
		b.WriteString("\n")
	}

	b.WriteString(m.renderFooter())
	return b.String()
}

// coverageLine states the cards axis as a fraction of the whole estate.
// Unchecked is named whenever nonzero: a dead checker must read as a dead
// checker, not as an estate with no cards.
func coverageLine(c Coverage) string {
	s := fmt.Sprintf("cards: %d confirmed · %d provisional · %d invalid · %d absent",
		c.Confirmed, c.Provisional, c.Invalid, c.Absent)
	if c.Unchecked > 0 {
		s += fmt.Sprintf(" · %d UNCHECKED", c.Unchecked)
	}
	return s
}

// renderRow lays the row out as plain fixed-width cells first and styles each
// cell afterwards, so no styled string is ever truncated (the []rune-on-ANSI
// bug class this repo's rules name).
func (m Model) renderRow(p Project, selected bool) string {
	nameWidth := m.width - 26 // marker 2 + verdict 13 + score 7 + padding
	if nameWidth < 12 {
		nameWidth = 12
	}
	if nameWidth > 32 {
		nameWidth = 32
	}

	marker := "  "
	markerStyle := lipgloss.NewStyle()
	if p.Funded {
		marker, markerStyle = "★ ", styleFunded
	} else if p.Pinned {
		marker, markerStyle = "⚑ ", stylePinned
	}

	score := "  — "
	if p.Verdict != VerdictUnchecked {
		score = fmt.Sprintf("%d/%d ", p.Strength.Score, p.Strength.Of)
	}

	name := pad(p.Name, nameWidth)
	verdict := pad(strings.ToUpper(string(p.Verdict)), 12)

	note := ""
	if p.Verdict == VerdictInvalid && p.Reason != "" {
		note = truncate(p.Reason, m.width-nameWidth-26)
	}
	if p.Verdict == VerdictUnchecked && p.Err != nil {
		note = truncate(p.Err.Error(), m.width-nameWidth-26)
	}

	line := markerStyle.Render(marker) +
		lipgloss.NewStyle().Render(name) + " " +
		verdictStyles[p.Verdict].Render(verdict) +
		fmt.Sprintf("%5s", score) + " " +
		styleReason.Render(note)
	if selected {
		return styleSelected.Render("> ") + line
	}
	return "  " + line
}

func (m Model) renderFooter() string {
	parts := []string{"↑/↓ move", "enter open in Zed", "p pin", "r re-check", "q quit"}
	line := styleFooter.Render(strings.Join(parts, " · "))
	if m.remaining > 0 {
		line += styleCoverage.Render(fmt.Sprintf("  checking %d…", m.remaining))
	}
	if m.checkerErr != nil {
		line += "\n" + styleErr.Render("checker unavailable: "+m.checkerErr.Error())
		return line
	}
	if m.rankingErr != nil {
		line += "\n" + styleErr.Render("ranking file broken (order degraded to weakest-first): "+m.rankingErr.Error())
		return line
	}
	if m.status != "" {
		line += "\n" + styleCoverage.Render(m.status)
	} else {
		line += "\n"
	}
	return line
}

// pad fixes a plain (unstyled) string to width, truncating with an ellipsis.
func pad(s string, width int) string {
	r := []rune(s)
	if len(r) > width {
		return string(r[:width-1]) + "…"
	}
	return s + strings.Repeat(" ", width-len(r))
}

// truncate shortens a plain (unstyled) string to width.
func truncate(s string, width int) string {
	if width <= 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:width-1]) + "…"
}
