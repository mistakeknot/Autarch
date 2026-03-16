package tui

import (
	"fmt"
	"log/slog"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/Masaq/viewport"
)

const maxLogEntries = 500

// LogPane displays log messages in a scrollable viewport.
type LogPane struct {
	viewport viewport.Model
	entries  []LogMsg
	width    int
	height   int
}

// NewLogPane creates a new log pane for displaying logs.
func NewLogPane() *LogPane {
	return &LogPane{
		entries: make([]LogMsg, 0, maxLogEntries),
	}
}

// SetSize updates the pane dimensions and recreates the viewport.
func (p *LogPane) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.viewport = viewport.New(width-2, height-1) // Account for padding and header
	p.updateContent()
}

// Update handles messages for the log pane.
func (p *LogPane) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case LogBatchMsg:
		for _, entry := range msg.Entries {
			p.entries = append(p.entries, entry)
		}
		// Simple buffer rotation: keep newest entries
		if len(p.entries) > maxLogEntries {
			p.entries = p.entries[len(p.entries)-maxLogEntries:]
		}
		p.updateContent()
		p.viewport.ScrollToBottom()
		return nil

	case tea.KeyMsg:
		switch msg.String() {
		case "g":
			p.viewport.ScrollTo(0)
		case "G":
			p.viewport.ScrollToBottom()
		default:
			var cmd tea.Cmd
			p.viewport, cmd = p.viewport.Update(msg)
			return cmd
		}
	}
	return nil
}

// updateContent rebuilds the viewport content from log entries.
func (p *LogPane) updateContent() {
	var b strings.Builder
	for _, e := range p.entries {
		b.WriteString(p.formatEntry(e))
		b.WriteString("\n")
	}
	p.viewport.SetContent(b.String())
}

// formatEntry formats a single log entry for display.
func (p *LogPane) formatEntry(e LogMsg) string {
	ts := e.Time.Format("15:04:05")

	var levelStr string
	var levelColor lipgloss.Color
	switch {
	case e.Level >= slog.LevelError:
		levelStr, levelColor = "ERR", ColorError
	case e.Level >= slog.LevelWarn:
		levelStr, levelColor = "WRN", ColorWarning
	case e.Level >= slog.LevelInfo:
		levelStr, levelColor = "INF", ColorPrimary
	default:
		levelStr, levelColor = "DBG", ColorMuted
	}

	level := lipgloss.NewStyle().Foreground(levelColor).Render(levelStr)
	return fmt.Sprintf("%s %s %s", ts, level, e.Message)
}

// View renders the log pane with header (no border, matches other panes).
func (p *LogPane) View() string {
	// Match the padding style used by other panes (1 char on each side)
	style := lipgloss.NewStyle().
		Padding(0, 1).
		Width(p.width).
		Height(p.height)

	header := lipgloss.NewStyle().
		Foreground(ColorPrimary).
		Bold(true).
		Background(ColorBgDark).
		Padding(0, 1).
		Render("Logs")

	// Pad header to full width like SplitLayout does
	headerLine := padToWidth(header, p.width-2)

	return style.Render(
		lipgloss.JoinVertical(lipgloss.Left, headerLine, p.viewport.View()),
	)
}

// Empty returns true when no log entries have been recorded yet.
func (p *LogPane) Empty() bool {
	return len(p.entries) == 0
}

// Entries returns all log entries for scrollback dump on exit.
func (p *LogPane) Entries() []LogMsg {
	return p.entries
}
