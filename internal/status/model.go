package status

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/pkg/tui"
)

const (
	pollInterval = 3 * time.Second
	eventLimit   = 20
)

// Model is the main tea.Model for the status TUI.
type Model struct {
	runs       *RunsPane
	dispatches *DispatchPane
	events     *EventsPane

	projectDir string
	width      int
	height     int

	// State
	tokens   TokenSummary
	fetching bool
	err      error
	lastRun  string // track selected run to detect changes
}

// New creates a new status Model.
func New(projectDir string) Model {
	return Model{
		runs:       NewRunsPane(),
		dispatches: NewDispatchPane(),
		events:     NewEventsPane(),
		projectDir: projectDir,
	}
}

// --- Messages ---

type tickMsg struct{}
type dataMsg struct {
	Runs       []Run
	Dispatches []Dispatch
	Events     []Event
	Tokens     TokenSummary
	Err        error
}

// --- tea.Model interface ---

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.fetchData(),
		tickCmd(),
	)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layoutPanes()
		return m, nil

	case tickMsg:
		if !m.fetching {
			m.fetching = true
			return m, tea.Batch(m.fetchData(), tickCmd())
		}
		return m, tickCmd()

	case dataMsg:
		m.fetching = false
		if msg.Err != nil {
			m.err = msg.Err
			return m, nil
		}
		m.err = nil
		m.runs.SetRuns(msg.Runs)
		m.dispatches.SetDispatches(selectedRunID(m.runs), msg.Dispatches)
		m.events.SetEvents(msg.Events)
		m.tokens = msg.Tokens
		m.lastRun = selectedRunID(m.runs)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "up", "k":
			m.runs.CursorUp()
			if selectedRunID(m.runs) != m.lastRun {
				m.lastRun = selectedRunID(m.runs)
				return m, m.fetchData()
			}
		case "down", "j":
			m.runs.CursorDown()
			if selectedRunID(m.runs) != m.lastRun {
				m.lastRun = selectedRunID(m.runs)
				return m, m.fetchData()
			}
		case "r":
			if !m.fetching {
				m.fetching = true
				return m, m.fetchData()
			}
		}
	}
	return m, nil
}

func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	var sections []string

	// Header
	header := m.renderHeader()
	sections = append(sections, header)

	// Error banner (if any)
	if m.err != nil {
		errStyle := lipgloss.NewStyle().
			Foreground(tui.ColorError).
			Padding(0, 2)
		sections = append(sections, errStyle.Render(fmt.Sprintf("Error: %v", m.err)))
	}

	// Panes
	sections = append(sections, m.runs.View())
	sections = append(sections, "")
	sections = append(sections, m.dispatches.View())
	sections = append(sections, "")
	sections = append(sections, m.events.View())

	// Footer
	footer := m.renderFooter()
	sections = append(sections, footer)

	content := strings.Join(sections, "\n")

	// Ensure content fills the screen height
	contentLines := strings.Count(content, "\n") + 1
	if contentLines < m.height {
		content += strings.Repeat("\n", m.height-contentLines)
	}

	return lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Background(tui.ColorBg).
		Render(content)
}

// --- Layout ---

func (m *Model) layoutPanes() {
	w := m.width
	// Distribute height: header(2) + runs(30%) + dispatches(30%) + events(30%) + footer(2) + gaps(3)
	available := m.height - 7 // header + footer + gaps
	if available < 9 {
		available = 9
	}

	runsH := available * 30 / 100
	dispH := available * 30 / 100
	eventsH := available - runsH - dispH

	if runsH < 3 {
		runsH = 3
	}
	if dispH < 3 {
		dispH = 3
	}
	if eventsH < 3 {
		eventsH = 3
	}

	m.runs.SetSize(w, runsH)
	m.dispatches.SetSize(w, dispH)
	m.events.SetSize(w, eventsH)
}

// --- Header / Footer ---

func (m Model) renderHeader() string {
	title := lipgloss.NewStyle().
		Foreground(tui.ColorPrimary).
		Bold(true).
		Render("Autarch Status")

	dir := lipgloss.NewStyle().
		Foreground(tui.ColorMuted).
		Render(m.projectDir)

	fetchIndicator := ""
	if m.fetching {
		fetchIndicator = lipgloss.NewStyle().
			Foreground(tui.ColorWarning).
			Render(" ⟳")
	}

	return fmt.Sprintf(" %s  %s%s", title, dir, fetchIndicator)
}

func (m Model) renderFooter() string {
	// Token summary
	tokenStr := ""
	if m.tokens.TotalTokens > 0 {
		tokenStr = fmt.Sprintf("Tokens: %s in / %s out",
			formatNumber(m.tokens.InputTokens),
			formatNumber(m.tokens.OutputTokens),
		)
	}

	tokStyle := lipgloss.NewStyle().Foreground(tui.ColorFgDim)
	helpStyle := lipgloss.NewStyle().Foreground(tui.ColorMuted)

	help := "↑↓:navigate  r:refresh  q:quit"

	left := tokStyle.Render(tokenStr)
	right := helpStyle.Render(help)

	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 4
	if gap < 1 {
		gap = 1
	}

	return fmt.Sprintf("  %s%s%s", left, strings.Repeat(" ", gap), right)
}

// --- Data fetching ---

func (m Model) fetchData() tea.Cmd {
	projectDir := m.projectDir
	runID := selectedRunID(m.runs)
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		runs, err := FetchRuns(ctx, projectDir)
		if err != nil {
			return dataMsg{Err: err}
		}

		// If no run selected yet, use first active run
		if runID == "" && len(runs) > 0 {
			runID = runs[0].ID
		}

		var dispatches []Dispatch
		var events []Event
		var tokens TokenSummary

		if runID != "" {
			dispatches, _ = FetchDispatches(ctx, projectDir, true)
			events, _ = FetchEvents(ctx, projectDir, runID, eventLimit)
			tokens, _ = FetchTokens(ctx, projectDir, runID)
		}

		// Filter dispatches to the selected run's scope
		if runID != "" {
			var filtered []Dispatch
			for _, d := range dispatches {
				if d.ScopeID != nil && *d.ScopeID == runScopeID(runs, runID) {
					filtered = append(filtered, d)
				}
			}
			// If no scope match, show all active dispatches (they may not have scope)
			if len(filtered) == 0 {
				filtered = dispatches
			}
			dispatches = filtered
		}

		return dataMsg{
			Runs:       runs,
			Dispatches: dispatches,
			Events:     events,
			Tokens:     tokens,
		}
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// --- Helpers ---

func selectedRunID(p *RunsPane) string {
	r := p.SelectedRun()
	if r == nil {
		return ""
	}
	return r.ID
}

func runScopeID(runs []Run, runID string) string {
	for _, r := range runs {
		if r.ID == runID {
			return r.ScopeID
		}
	}
	return ""
}

func formatNumber(n int64) string {
	if n < 0 {
		return "-" + formatNumber(-n)
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1_000_000 {
		return fmt.Sprintf("%d,%03d", n/1000, n%1000)
	}
	if n < 1_000_000_000 {
		return fmt.Sprintf("%d,%03d,%03d", n/1_000_000, (n%1_000_000)/1000, n%1000)
	}
	return fmt.Sprintf("%d,%03d,%03d,%03d", n/1_000_000_000, (n%1_000_000_000)/1_000_000, (n%1_000_000)/1000, n%1000)
}
