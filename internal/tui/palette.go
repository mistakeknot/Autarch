package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
	"github.com/sahilm/fuzzy"
)

// Palette is a command palette with fuzzy search.
// Supports a 3-phase broadcast confirmation flow: Command → Target → Confirm → Execute.
type Palette struct {
	input    textinput.Model
	commands []Command
	matches  []fuzzy.Match
	selected int
	width    int
	height   int
	visible  bool

	// Broadcast confirmation phases
	phase         Phase
	target        Target
	paneCounts    PaneCounts
	pendingCmd    Command // Value copy of the broadcast command waiting for confirmation
	hasPendingCmd bool    // True when pendingCmd is valid

	// fetchPaneCounts is set by the parent to provide async pane count fetching.
	// Called as a tea.Cmd when entering PhaseTarget.
	fetchPaneCounts func() tea.Msg
}

// NewPalette creates a new command palette
func NewPalette() *Palette {
	input := textinput.New()
	input.Placeholder = "Type a command..."
	input.Prompt = "> "
	input.CharLimit = 64

	return &Palette{
		input: input,
	}
}

// SetCommands sets the available commands
func (p *Palette) SetCommands(cmds []Command) {
	p.commands = cmds
	p.updateMatches()
}

// SetSize sets the palette dimensions
func (p *Palette) SetSize(width, height int) {
	p.width = width
	p.height = height
	p.input.Width = width - 6
}

// Show shows the palette and focuses input
func (p *Palette) Show() tea.Cmd {
	p.visible = true
	p.phase = PhaseCommand
	p.target = TargetAll
	p.hasPendingCmd = false
	p.input.Reset()
	p.selected = 0
	p.updateMatches()
	return p.input.Focus()
}

// Hide hides the palette
func (p *Palette) Hide() {
	p.visible = false
	p.phase = PhaseCommand
	p.hasPendingCmd = false
}

// SetPaneCountFetcher sets the async pane count fetch function.
func (p *Palette) SetPaneCountFetcher(fn func() tea.Msg) {
	p.fetchPaneCounts = fn
}

// Visible returns whether the palette is visible
func (p *Palette) Visible() bool {
	return p.visible
}

// Selected returns the currently selected command, if any
func (p *Palette) Selected() *Command {
	if len(p.matches) == 0 {
		return nil
	}
	if p.selected >= len(p.matches) {
		return nil
	}
	idx := p.matches[p.selected].Index
	if idx >= len(p.commands) {
		return nil
	}
	return &p.commands[idx]
}

// Update handles input with phase-aware dispatch.
func (p *Palette) Update(msg tea.Msg) (*Palette, tea.Cmd) {
	if !p.visible {
		return p, nil
	}

	switch msg := msg.(type) {
	case PaneCountMsg:
		if msg.Err == nil {
			p.paneCounts = msg.Counts
		}
		return p, nil

	case tea.KeyMsg:
		// ctrl+c closes from any phase
		if msg.String() == "ctrl+c" {
			p.Hide()
			return p, nil
		}

		switch p.phase {
		case PhaseCommand:
			return p.updateCommandPhase(msg)
		case PhaseTarget:
			return p.updateTargetPhase(msg)
		case PhaseConfirm:
			return p.updateConfirmPhase(msg)
		}
	}

	// Non-key messages pass through to text input (command phase only)
	if p.phase == PhaseCommand {
		var cmd tea.Cmd
		p.input, cmd = p.input.Update(msg)
		p.updateMatches()
		p.selected = 0
		return p, cmd
	}

	return p, nil
}

func (p *Palette) updateCommandPhase(msg tea.KeyMsg) (*Palette, tea.Cmd) {
	switch msg.String() {
	case "esc":
		p.Hide()
		return p, nil

	case "enter":
		cmd := p.Selected()
		if cmd == nil {
			return p, nil
		}
		if cmd.Broadcast {
			p.pendingCmd = *cmd // Value copy — safe even if SetCommands replaces the slice
			p.hasPendingCmd = true
			p.phase = PhaseTarget
			if p.fetchPaneCounts != nil {
				return p, p.fetchPaneCounts
			}
			return p, nil
		}
		p.Hide()
		return p, cmd.Action()

	case "up", "ctrl+p":
		if p.selected > 0 {
			p.selected--
		}
		return p, nil

	case "down", "ctrl+n":
		if p.selected < len(p.matches)-1 {
			p.selected++
		}
		return p, nil
	}

	// Text input for fuzzy search
	var cmd tea.Cmd
	p.input, cmd = p.input.Update(msg)
	p.updateMatches()
	p.selected = 0
	return p, cmd
}

func (p *Palette) updateTargetPhase(msg tea.KeyMsg) (*Palette, tea.Cmd) {
	switch msg.String() {
	case "esc":
		p.phase = PhaseCommand
		return p, nil
	case "1":
		p.target = TargetAll
		p.phase = PhaseConfirm
		return p, nil
	case "2":
		p.target = TargetClaude
		p.phase = PhaseConfirm
		return p, nil
	case "3":
		p.target = TargetCodex
		p.phase = PhaseConfirm
		return p, nil
	case "4":
		p.target = TargetGemini
		p.phase = PhaseConfirm
		return p, nil
	}
	return p, nil
}

func (p *Palette) updateConfirmPhase(msg tea.KeyMsg) (*Palette, tea.Cmd) {
	switch msg.String() {
	case "esc":
		p.phase = PhaseTarget
		return p, nil
	case "enter":
		if p.hasPendingCmd {
			// Snapshot action BEFORE Hide() modifies state.
			// Action closures must not read palette fields — they run on a
			// separate goroutine (tea.Cmd), creating a data race.
			action := p.pendingCmd.Action
			p.Hide()
			return p, action()
		}
		p.Hide()
		return p, nil
	}
	return p, nil
}

func (p *Palette) updateMatches() {
	query := strings.TrimSpace(p.input.Value())
	if query == "" {
		// Show all commands when query is empty
		p.matches = make([]fuzzy.Match, len(p.commands))
		for i := range p.commands {
			p.matches[i] = fuzzy.Match{Index: i}
		}
		return
	}

	// Build searchable list
	names := make([]string, len(p.commands))
	for i, cmd := range p.commands {
		names[i] = cmd.Name
	}

	p.matches = fuzzy.Find(query, names)
}

// View renders the palette with phase-aware content.
func (p *Palette) View() string {
	if !p.visible {
		return ""
	}

	width := p.width
	if width > 60 {
		width = 60
	}

	var content string
	switch p.phase {
	case PhaseTarget:
		content = p.viewTargetPhase(width)
	case PhaseConfirm:
		content = p.viewConfirmPhase(width)
	default:
		content = p.viewCommandPhase(width)
	}

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(pkgtui.ColorPrimary).
		Padding(1, 2).
		Width(width)

	return style.Render(content)
}

func (p *Palette) viewCommandPhase(width int) string {
	var b strings.Builder

	title := pkgtui.TitleStyle.Render("Command Palette")
	b.WriteString(title + "\n")
	b.WriteString(p.input.View() + "\n")
	b.WriteString(strings.Repeat("─", width-4) + "\n")

	maxResults := 8
	if p.height > 0 {
		maxResults = min(maxResults, p.height-6)
	}

	for i, match := range p.matches {
		if i >= maxResults {
			break
		}

		cmd := p.commands[match.Index]
		name := cmd.Name
		desc := cmd.Description

		if i == p.selected {
			name = pkgtui.SelectedStyle.Render(name)
		} else {
			name = pkgtui.UnselectedStyle.Render(name)
		}

		desc = pkgtui.LabelStyle.Render(desc)

		line := "  " + name
		if desc != "" {
			line += "  " + desc
		}
		b.WriteString(line + "\n")
	}

	if len(p.matches) == 0 {
		b.WriteString(pkgtui.LabelStyle.Render("  No matching commands\n"))
	}

	return b.String()
}

func (p *Palette) viewTargetPhase(width int) string {
	var b strings.Builder

	cmdName := ""
	if p.hasPendingCmd {
		cmdName = p.pendingCmd.Name
	}
	title := pkgtui.TitleStyle.Render("Select Target: " + cmdName)
	b.WriteString(title + "\n")
	b.WriteString(strings.Repeat("─", width-4) + "\n")

	targets := []struct {
		key    string
		target Target
	}{
		{"1", TargetAll},
		{"2", TargetClaude},
		{"3", TargetCodex},
		{"4", TargetGemini},
	}

	for _, t := range targets {
		count := p.paneCounts.ForTarget(t.target)
		label := fmt.Sprintf("  %s. %s (%d)", t.key, t.target.Label(), count)
		b.WriteString(label + "\n")
	}

	b.WriteString("\n")
	b.WriteString(pkgtui.LabelStyle.Render("  esc back"))

	return b.String()
}

func (p *Palette) viewConfirmPhase(width int) string {
	var b strings.Builder

	title := pkgtui.TitleStyle.Render("Confirm Broadcast")
	b.WriteString(title + "\n")
	b.WriteString(strings.Repeat("─", width-4) + "\n")

	cmdName := ""
	if p.hasPendingCmd {
		cmdName = p.pendingCmd.Name
	}

	count := p.paneCounts.ForTarget(p.target)
	b.WriteString(fmt.Sprintf("  Command: %s\n", cmdName))
	b.WriteString(fmt.Sprintf("  Target:  %s (%d)\n", p.target.Label(), count))

	if p.target == TargetAll && p.paneCounts.Total() > 0 {
		b.WriteString(fmt.Sprintf("           Claude(%d) Codex(%d) Gemini(%d)\n",
			p.paneCounts.Claude, p.paneCounts.Codex, p.paneCounts.Gemini))
	}

	b.WriteString("\n")
	b.WriteString(pkgtui.LabelStyle.Render("  enter confirm  esc back"))

	return b.String()
}
