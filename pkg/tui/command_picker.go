package tui

import (
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SlashCommandDef defines a slash command with its metadata.
type SlashCommandDef struct {
	Command     string // Primary command name (without /)
	Aliases     []string
	Description string
	Category    string // "global", "kickoff", "sprint", etc.
}

// CommandPicker is a fuzzy-finding picker for slash commands.
// It appears when the user types "/" in the chat composer.
type CommandPicker struct {
	commands  []SlashCommandDef
	filtered  []SlashCommandDef
	query     string // Current filter (text after /)
	selected  int
	visible   bool
	maxHeight int
	width     int
}

// NewCommandPicker creates a new command picker with the given commands.
func NewCommandPicker(commands []SlashCommandDef) *CommandPicker {
	return &CommandPicker{
		commands:  commands,
		filtered:  commands,
		maxHeight: 10,
		width:     50,
	}
}

// SetCommands updates the available commands.
func (p *CommandPicker) SetCommands(commands []SlashCommandDef) {
	p.commands = commands
	p.filter()
}

// AddCommands appends additional commands (for view-specific commands).
func (p *CommandPicker) AddCommands(commands []SlashCommandDef) {
	p.commands = append(p.commands, commands...)
	p.filter()
}

// Show opens the picker with the given query (text after /).
func (p *CommandPicker) Show(query string) {
	p.visible = true
	p.query = query
	p.selected = 0
	p.filter()
}

// Hide closes the picker.
func (p *CommandPicker) Hide() {
	p.visible = false
	p.query = ""
	p.selected = 0
}

// Visible returns whether the picker is showing.
func (p *CommandPicker) Visible() bool {
	return p.visible
}

// SetSize sets the picker dimensions.
func (p *CommandPicker) SetSize(width, maxHeight int) {
	p.width = width
	p.maxHeight = maxHeight
}

// Update handles key events for the picker.
// Returns the selected command if Enter was pressed, or "" otherwise.
func (p *CommandPicker) Update(msg tea.KeyMsg) (selectedCommand string, consumed bool) {
	if !p.visible {
		return "", false
	}

	switch msg.Type {
	case tea.KeyUp:
		if p.selected > 0 {
			p.selected--
		}
		return "", true
	case tea.KeyDown:
		if p.selected < len(p.filtered)-1 {
			p.selected++
		}
		return "", true
	case tea.KeyEnter, tea.KeyTab:
		if len(p.filtered) > 0 && p.selected < len(p.filtered) {
			cmd := p.filtered[p.selected].Command
			p.Hide()
			return cmd, true
		}
		return "", true
	case tea.KeyEsc:
		p.Hide()
		return "", true
	}

	return "", false
}

// UpdateQuery updates the filter query (called when composer text changes).
func (p *CommandPicker) UpdateQuery(query string) {
	p.query = query
	p.filter()
	// Reset selection if it's out of bounds
	if p.selected >= len(p.filtered) {
		p.selected = 0
	}
}

// filter updates the filtered list based on the current query.
func (p *CommandPicker) filter() {
	if p.query == "" {
		p.filtered = p.commands
		return
	}

	query := strings.ToLower(p.query)
	var matches []SlashCommandDef

	for _, cmd := range p.commands {
		// Check command name
		if fuzzyMatch(cmd.Command, query) {
			matches = append(matches, cmd)
			continue
		}
		// Check aliases
		for _, alias := range cmd.Aliases {
			if fuzzyMatch(alias, query) {
				matches = append(matches, cmd)
				break
			}
		}
		// Check description
		if fuzzyMatch(cmd.Description, query) {
			matches = append(matches, cmd)
		}
	}

	// Sort by relevance (exact prefix match first, then by name)
	sort.Slice(matches, func(i, j int) bool {
		iPrefix := strings.HasPrefix(strings.ToLower(matches[i].Command), query)
		jPrefix := strings.HasPrefix(strings.ToLower(matches[j].Command), query)
		if iPrefix != jPrefix {
			return iPrefix
		}
		return matches[i].Command < matches[j].Command
	})

	p.filtered = matches
}

// fuzzyMatch checks if the target contains all characters of the query in order.
func fuzzyMatch(target, query string) bool {
	target = strings.ToLower(target)
	query = strings.ToLower(query)

	if strings.Contains(target, query) {
		return true
	}

	// Fuzzy matching: all query chars must appear in order
	ti := 0
	for _, qc := range query {
		found := false
		for ti < len(target) {
			if rune(target[ti]) == qc {
				found = true
				ti++
				break
			}
			ti++
		}
		if !found {
			return false
		}
	}
	return true
}

// View renders the command picker.
func (p *CommandPicker) View() string {
	if !p.visible || len(p.filtered) == 0 {
		return ""
	}

	// Styles
	containerStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorPrimary).
		Padding(0, 1).
		Width(p.width - 2)

	selectedStyle := lipgloss.NewStyle().
		Background(ColorPrimary).
		Foreground(ColorBg).
		Bold(true).
		Width(p.width - 6)

	normalStyle := lipgloss.NewStyle().
		Foreground(ColorFg).
		Width(p.width - 6)

	aliasStyle := lipgloss.NewStyle().
		Foreground(ColorMuted)

	descStyle := lipgloss.NewStyle().
		Foreground(ColorFgDim)

	categoryStyle := lipgloss.NewStyle().
		Foreground(ColorSecondary).
		Italic(true)

	// Header
	header := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Italic(true).
		Render("Commands")

	// Build list
	var lines []string
	lines = append(lines, header)

	displayCount := p.maxHeight - 2 // Account for header and border
	if displayCount > len(p.filtered) {
		displayCount = len(p.filtered)
	}

	// Adjust scroll window to keep selected visible
	startIdx := 0
	if p.selected >= displayCount {
		startIdx = p.selected - displayCount + 1
	}
	endIdx := startIdx + displayCount
	if endIdx > len(p.filtered) {
		endIdx = len(p.filtered)
		startIdx = endIdx - displayCount
		if startIdx < 0 {
			startIdx = 0
		}
	}

	for i := startIdx; i < endIdx; i++ {
		cmd := p.filtered[i]

		// Format: /command (alias) - description [category]
		text := "/" + cmd.Command
		if len(cmd.Aliases) > 0 {
			text += " " + aliasStyle.Render("("+strings.Join(prefixSlash(cmd.Aliases), ", ")+")")
		}
		if cmd.Description != "" {
			text += " " + descStyle.Render("— "+cmd.Description)
		}
		if cmd.Category != "" && cmd.Category != "global" {
			text += " " + categoryStyle.Render("["+cmd.Category+"]")
		}

		if i == p.selected {
			lines = append(lines, selectedStyle.Render(text))
		} else {
			lines = append(lines, normalStyle.Render(text))
		}
	}

	// Scroll indicator
	if len(p.filtered) > displayCount {
		more := len(p.filtered) - displayCount
		moreText := lipgloss.NewStyle().
			Foreground(ColorMuted).
			Italic(true).
			Render("↓ " + strings.Repeat(".", 3) + " and more")
		if endIdx < len(p.filtered) {
			lines = append(lines, moreText)
		}
		_ = more // suppress unused warning
	}

	return containerStyle.Render(strings.Join(lines, "\n"))
}

// Selected returns the currently selected command, or "" if none.
func (p *CommandPicker) Selected() string {
	if len(p.filtered) == 0 || p.selected >= len(p.filtered) {
		return ""
	}
	return p.filtered[p.selected].Command
}

// prefixSlash adds / prefix to each alias for display.
func prefixSlash(aliases []string) []string {
	result := make([]string, len(aliases))
	for i, a := range aliases {
		result[i] = "/" + a
	}
	return result
}

// GlobalCommands returns the standard global slash commands.
func GlobalCommands() []SlashCommandDef {
	return []SlashCommandDef{
		{Command: "help", Aliases: []string{"h"}, Description: "Show help overlay", Category: "global"},
		{Command: "quit", Aliases: []string{"q", "exit"}, Description: "Quit application", Category: "global"},
		{Command: "settings", Aliases: []string{"config"}, Description: "Open chat settings", Category: "global"},
		{Command: "model", Aliases: []string{"m"}, Description: "Toggle model selector", Category: "global"},
		{Command: "palette", Aliases: []string{"p"}, Description: "Command palette", Category: "global"},
		{Command: "refresh", Aliases: []string{"r"}, Description: "Refresh current view", Category: "global"},
		{Command: "back", Aliases: []string{"b"}, Description: "Go back / cancel", Category: "global"},
		// Tool-switching commands
		{Command: "bigend", Aliases: []string{"big"}, Description: "Switch to Bigend", Category: "navigation"},
		{Command: "gurgeh", Aliases: []string{"gur"}, Description: "Switch to Gurgeh", Category: "navigation"},
		{Command: "coldwine", Aliases: []string{"cold"}, Description: "Switch to Coldwine", Category: "navigation"},
		{Command: "pollard", Aliases: []string{"pol"}, Description: "Switch to Pollard", Category: "navigation"},
		{Command: "signals", Aliases: []string{"sig"}, Description: "Toggle Signals overlay", Category: "navigation"},
		{Command: "logs", Aliases: []string{"log", "l"}, Description: "Toggle log pane", Category: "global"},
	}
}

// KickoffCommands returns slash commands for the kickoff view.
func KickoffCommands() []SlashCommandDef {
	return []SlashCommandDef{
		{Command: "scan", Aliases: []string{"s"}, Description: "Scan current directory", Category: "kickoff"},
		{Command: "new", Aliases: []string{"n"}, Description: "New project", Category: "kickoff"},
		{Command: "delete", Aliases: []string{"d"}, Description: "Delete selected project", Category: "kickoff"},
	}
}

// SprintCommands returns slash commands for sprint/arbiter views.
func SprintCommands() []SlashCommandDef {
	return []SlashCommandDef{
		{Command: "accept", Aliases: []string{"a"}, Description: "Accept current draft", Category: "sprint"},
		{Command: "1", Description: "Select option 1", Category: "sprint"},
		{Command: "2", Description: "Select option 2", Category: "sprint"},
		{Command: "3", Description: "Select option 3", Category: "sprint"},
		// Phase navigation
		{Command: "vision", Aliases: []string{"vis"}, Description: "Jump to Vision", Category: "sprint"},
		{Command: "problem", Aliases: []string{"prob"}, Description: "Jump to Problem", Category: "sprint"},
		{Command: "users", Aliases: []string{"usr"}, Description: "Jump to Users", Category: "sprint"},
		{Command: "features", Aliases: []string{"feat"}, Description: "Jump to Features + Goals", Category: "sprint"},
		{Command: "cujs", Aliases: []string{"cuj"}, Description: "Jump to Critical User Journeys", Category: "sprint"},
		{Command: "reqs", Aliases: []string{"req"}, Description: "Jump to Requirements", Category: "sprint"},
		{Command: "scope", Aliases: []string{"scp"}, Description: "Jump to Scope + Assumptions", Category: "sprint"},
		{Command: "acceptance", Aliases: []string{"ac"}, Description: "Jump to Acceptance Criteria", Category: "sprint"},
	}
}

// EpicReviewCommands returns slash commands for epic review.
func EpicReviewCommands() []SlashCommandDef {
	return []SlashCommandDef{
		{Command: "accept", Aliases: []string{"a"}, Description: "Accept all epics", Category: "epics"},
		{Command: "edit", Aliases: []string{"e"}, Description: "Edit selected epic", Category: "epics"},
		{Command: "delete", Aliases: []string{"d"}, Description: "Delete selected epic", Category: "epics"},
		{Command: "regen", Aliases: []string{"regenerate"}, Description: "Regenerate proposals", Category: "epics"},
	}
}

// TaskReviewCommands returns slash commands for task review.
func TaskReviewCommands() []SlashCommandDef {
	return []SlashCommandDef{
		{Command: "accept", Aliases: []string{"a"}, Description: "Accept all tasks", Category: "tasks"},
		{Command: "delete", Aliases: []string{"d"}, Description: "Delete selected task", Category: "tasks"},
		{Command: "group", Aliases: []string{"g"}, Description: "Toggle grouped view", Category: "tasks"},
		{Command: "type", Aliases: []string{"t"}, Description: "Cycle task type", Category: "tasks"},
	}
}
