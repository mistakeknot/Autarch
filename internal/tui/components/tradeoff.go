package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/internal/pollard/research"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// Tradeoff displays a suggestion with pros/cons for interview questions.
// It allows users to adopt suggestions via keyboard shortcuts.
type Tradeoff struct {
	options   []research.TradeoffOption
	selected  int
	width     int
	collapsed bool // Show only header when collapsed
	keys      pkgtui.CommonKeys
}

// NewTradeoff creates a new tradeoff component.
func NewTradeoff() *Tradeoff {
	return &Tradeoff{
		selected: -1, // No selection by default
		keys:     pkgtui.NewCommonKeys(),
	}
}

// SetOptions updates the available tradeoff options.
func (t *Tradeoff) SetOptions(options []research.TradeoffOption) {
	t.options = options
	t.selected = -1
}

// AddOption adds a single option.
func (t *Tradeoff) AddOption(opt research.TradeoffOption) {
	t.options = append(t.options, opt)
}

// SetSize updates the component width.
func (t *Tradeoff) SetSize(width int) {
	t.width = width
}

// HasOptions returns true if there are options to display.
func (t *Tradeoff) HasOptions() bool {
	return len(t.options) > 0
}

// OptionsCount returns the number of available options.
func (t *Tradeoff) OptionsCount() int {
	return len(t.options)
}

// Select sets the selected option index.
func (t *Tradeoff) Select(idx int) {
	if idx >= 0 && idx < len(t.options) {
		t.selected = idx
	}
}

// GetSelected returns the currently selected option, if any.
func (t *Tradeoff) GetSelected() *research.TradeoffOption {
	if t.selected >= 0 && t.selected < len(t.options) {
		return &t.options[t.selected]
	}
	return nil
}

// GetOption returns an option by index.
func (t *Tradeoff) GetOption(idx int) *research.TradeoffOption {
	if idx >= 0 && idx < len(t.options) {
		return &t.options[idx]
	}
	return nil
}

// Toggle expands or collapses the view.
func (t *Tradeoff) Toggle() {
	t.collapsed = !t.collapsed
}

// TradeoffSelectedMsg is sent when a user adopts a suggestion.
type TradeoffSelectedMsg struct {
	Option    research.TradeoffOption
	Index     int
	InsightID string
}

// Update handles key events for the tradeoff component.
func (t *Tradeoff) Update(msg tea.Msg) (*Tradeoff, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		maxOptions := min(3, len(t.options))
		switch {
		case key.Matches(msg, t.keys.NavDown):
			if maxOptions == 0 {
				return t, nil
			}
			if t.selected < 0 {
				t.selected = 0
				return t, nil
			}
			if t.selected < maxOptions-1 {
				t.selected++
			}
			return t, nil
		case key.Matches(msg, t.keys.NavUp):
			if maxOptions == 0 {
				return t, nil
			}
			if t.selected < 0 {
				t.selected = maxOptions - 1
				return t, nil
			}
			if t.selected > 0 {
				t.selected--
			}
			return t, nil
		case key.Matches(msg, t.keys.Select):
			if t.selected >= 0 && t.selected < maxOptions {
				return t, t.createSelectedCmd(t.selected)
			}
			return t, nil
		}
	}
	return t, nil
}

func (t *Tradeoff) createSelectedCmd(idx int) tea.Cmd {
	opt := t.options[idx]
	return func() tea.Msg {
		return TradeoffSelectedMsg{
			Option:    opt,
			Index:     idx,
			InsightID: opt.InsightID,
		}
	}
}

// View renders the tradeoff component.
func (t *Tradeoff) View() string {
	if len(t.options) == 0 {
		return ""
	}

	width := t.width
	if width <= 0 {
		width = 70
	}

	var sections []string

	// Header
	headerStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorSecondary).
		Bold(true)

	header := headerStyle.Render("Research suggests:")
	sections = append(sections, header)

	if t.collapsed {
		// Just show count
		countStyle := pkgtui.LabelStyle
		sections = append(sections, countStyle.Render(
			fmt.Sprintf("%d options available (expand to see)", len(t.options))))
		return lipgloss.JoinVertical(lipgloss.Left, sections...)
	}

	// Render each option
	for i, opt := range t.options {
		if i >= 3 {
			break // Limit to 3 options
		}
		optView := t.renderOption(opt, i, width-2)
		sections = append(sections, optView)
	}

	// Help text
	helpStyle := pkgtui.LabelStyle
	help := "Use ↑/↓ and Enter to adopt a suggestion"
	sections = append(sections, helpStyle.Render(help))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

func (t *Tradeoff) renderOption(opt research.TradeoffOption, idx int, width int) string {
	var lines []string

	// Option header with number
	numStyle := lipgloss.NewStyle().
		Background(pkgtui.ColorBgLight).
		Foreground(pkgtui.ColorPrimary).
		Bold(true).
		Padding(0, 1)

	labelStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorFg).
		Bold(true)

	isSelected := idx == t.selected
	if isSelected {
		labelStyle = labelStyle.Foreground(pkgtui.ColorSuccess)
	}

	header := fmt.Sprintf("%s %s", numStyle.Render(fmt.Sprintf("%d", idx+1)), labelStyle.Render(opt.Label))
	lines = append(lines, header)

	// Popularity if available
	if opt.Popularity != "" {
		popStyle := pkgtui.LabelStyle.MarginLeft(4)
		lines = append(lines, popStyle.Render(opt.Popularity))
	}

	// Pros (✓)
	proStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorSuccess).MarginLeft(4)
	for _, pro := range opt.Pros {
		lines = append(lines, proStyle.Render("✓ "+pro))
	}

	// Cons (✗)
	conStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorError).MarginLeft(4)
	for _, con := range opt.Cons {
		lines = append(lines, conStyle.Render("✗ "+con))
	}

	// Sources
	if len(opt.Sources) > 0 {
		sourceStyle := pkgtui.LabelStyle.MarginLeft(4).Foreground(pkgtui.ColorMuted)
		sources := strings.Join(opt.Sources, ", ")
		if len(sources) > width-10 {
			sources = sources[:width-13] + "..."
		}
		lines = append(lines, sourceStyle.Render("via: "+sources))
	}

	lines = append(lines, "") // Spacer between options

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// ViewCompact renders a compact single-line summary.
func (t *Tradeoff) ViewCompact() string {
	if len(t.options) == 0 {
		return ""
	}

	style := lipgloss.NewStyle().
		Foreground(pkgtui.ColorSecondary)

	if len(t.options) == 1 {
		return style.Render(fmt.Sprintf("Suggestion: %s [1 to adopt]", t.options[0].Label))
	}

	return style.Render(fmt.Sprintf("%d suggestions available [1-%d to adopt]",
		len(t.options), min(len(t.options), 3)))
}

// ViewFYI renders a non-intrusive "FYI" version for already-touched questions.
func (t *Tradeoff) ViewFYI() string {
	if len(t.options) == 0 {
		return ""
	}

	style := pkgtui.LabelStyle.
		Foreground(pkgtui.ColorMuted).
		Italic(true)

	if len(t.options) == 1 {
		return style.Render(fmt.Sprintf("FYI: Research found \"%s\" (you've already answered)", t.options[0].Label))
	}

	return style.Render(fmt.Sprintf("FYI: Research found %d options (you've already answered)", len(t.options)))
}

// CreateFromFindings creates tradeoff options from research findings.
func CreateFromFindings(findings []research.Finding, topicKey string) []research.TradeoffOption {
	var options []research.TradeoffOption

	for _, f := range findings {
		if f.Relevance < 0.3 {
			continue // Skip low relevance findings
		}

		opt := research.TradeoffOption{
			Label:     f.Title,
			InsightID: f.ID,
			Sources:   []string{f.Source},
		}

		// Extract pros/cons from summary (simplified - real implementation would parse)
		if strings.Contains(strings.ToLower(f.Summary), "recommend") ||
			strings.Contains(strings.ToLower(f.Summary), "popular") {
			opt.Pros = append(opt.Pros, "Commonly used approach")
		}
		if strings.Contains(strings.ToLower(f.Summary), "complex") ||
			strings.Contains(strings.ToLower(f.Summary), "overhead") {
			opt.Cons = append(opt.Cons, "May add complexity")
		}

		// Add relevance-based popularity
		if f.Relevance >= 0.7 {
			opt.Popularity = "Highly relevant"
		} else if f.Relevance >= 0.5 {
			opt.Popularity = "Moderately relevant"
		}

		options = append(options, opt)
	}

	// Limit to 3 options
	if len(options) > 3 {
		options = options[:3]
	}

	return options
}
