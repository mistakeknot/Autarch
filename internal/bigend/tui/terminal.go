package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/mistakeknot/autarch/internal/bigend/tmux"
	shared "github.com/mistakeknot/autarch/pkg/tui"
)

// TerminalPane displays live terminal output from a tmux session.
type TerminalPane struct {
	viewport    viewport.Model
	sessionName string
	content     string
	width       int
	height      int
	focused     bool
	lastUpdate  time.Time
	tmuxClient  *tmux.Client
}

// NewTerminalPane creates a new terminal preview pane.
func NewTerminalPane(tmuxClient *tmux.Client) *TerminalPane {
	vp := viewport.New(80, 20)
	vp.Style = lipgloss.NewStyle().
		Background(shared.ColorBg).
		Foreground(shared.ColorFg)

	return &TerminalPane{
		viewport:   vp,
		tmuxClient: tmuxClient,
	}
}

// SetSession changes the session being previewed.
func (t *TerminalPane) SetSession(name string) tea.Cmd {
	t.sessionName = name
	t.content = ""
	t.viewport.SetContent("")

	if name == "" {
		return nil
	}

	// Return a command to fetch initial content
	return t.fetchContent
}

// SetSize updates the pane dimensions.
func (t *TerminalPane) SetSize(width, height int) {
	t.width = width
	t.height = height
	t.viewport.Width = width
	t.viewport.Height = height
}

// SetFocused updates focus state.
func (t *TerminalPane) SetFocused(focused bool) {
	t.focused = focused
}

// Session returns the current session name.
func (t *TerminalPane) Session() string {
	return t.sessionName
}

// terminalContentMsg carries fetched terminal content.
type terminalContentMsg struct {
	session string
	content string
	err     error
}

// fetchContent fetches terminal content from tmux.
func (t *TerminalPane) fetchContent() tea.Msg {
	if t.sessionName == "" || t.tmuxClient == nil {
		return nil
	}

	content, err := t.tmuxClient.CapturePane(t.sessionName, 50)
	return terminalContentMsg{
		session: t.sessionName,
		content: content,
		err:     err,
	}
}

// TickTerminal returns a command to periodically refresh terminal content.
func TickTerminal(interval time.Duration) tea.Cmd {
	return tea.Tick(interval, func(t time.Time) tea.Msg {
		return terminalTickMsg{at: t}
	})
}

type terminalTickMsg struct {
	at time.Time
}

// Update handles messages for the terminal pane.
func (t *TerminalPane) Update(msg tea.Msg) (*TerminalPane, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case terminalContentMsg:
		if msg.session == t.sessionName && msg.err == nil {
			// Only update if content changed
			if msg.content != t.content {
				t.content = msg.content
				t.viewport.SetContent(t.formatContent(msg.content))
				// Auto-scroll to bottom
				t.viewport.GotoBottom()
			}
			t.lastUpdate = time.Now()
		}

	case terminalTickMsg:
		if t.sessionName != "" {
			cmds = append(cmds, t.fetchContent)
		}

	case tea.KeyMsg:
		if t.focused {
			var cmd tea.Cmd
			t.viewport, cmd = t.viewport.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return t, tea.Batch(cmds...)
}

// formatContent applies styling to terminal content.
func (t *TerminalPane) formatContent(raw string) string {
	lines := strings.Split(raw, "\n")

	// Apply subtle styling while preserving terminal colors
	var styled []string
	for _, line := range lines {
		styled = append(styled, line)
	}

	return strings.Join(styled, "\n")
}

// View renders the terminal pane.
func (t *TerminalPane) View() string {
	if t.sessionName == "" {
		return t.renderEmpty()
	}

	// Header with session info
	header := t.renderHeader()

	// Terminal content
	content := t.viewport.View()

	// Footer with scroll position
	footer := t.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left, header, content, footer)
}

func (t *TerminalPane) renderEmpty() string {
	emptyStyle := lipgloss.NewStyle().
		Foreground(shared.ColorMuted).
		Italic(true).
		Align(lipgloss.Center).
		Width(t.width).
		Height(t.height)

	return emptyStyle.Render("Select a session to preview terminal output\n\nPress 'p' to toggle preview pane")
}

func (t *TerminalPane) renderHeader() string {
	titleStyle := lipgloss.NewStyle().
		Foreground(shared.ColorPrimary).
		Bold(true)

	sessionStyle := lipgloss.NewStyle().
		Foreground(shared.ColorFg)

	updateStyle := lipgloss.NewStyle().
		Foreground(shared.ColorMuted).
		Align(lipgloss.Right)

	title := titleStyle.Render("Terminal Preview")
	session := sessionStyle.Render(" • " + t.sessionName)

	var updateText string
	if !t.lastUpdate.IsZero() {
		elapsed := time.Since(t.lastUpdate)
		if elapsed < time.Second {
			updateText = "just now"
		} else {
			updateText = elapsed.Round(time.Second).String() + " ago"
		}
	}
	update := updateStyle.Width(t.width - lipgloss.Width(title) - lipgloss.Width(session) - 2).
		Render(updateText)

	return lipgloss.JoinHorizontal(lipgloss.Left, title, session, update)
}

func (t *TerminalPane) renderFooter() string {
	scrollStyle := lipgloss.NewStyle().
		Foreground(shared.ColorMuted)

	percent := t.viewport.ScrollPercent() * 100
	scrollText := ""
	if percent < 100 {
		scrollText = scrollStyle.Render("↑↓ scroll")
	}

	return scrollText
}
