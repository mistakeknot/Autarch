package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/internal/coldwine/epics"
	"github.com/mistakeknot/autarch/internal/tui"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// EpicReviewView displays proposed epics for review and editing.
// Uses the unified shell layout with chat for Q&A during review (no sidebar).
type EpicReviewView struct {
	proposals []epics.EpicProposal
	selected  int
	expanded  map[int]bool
	width     int
	height    int
	editing   bool // In inline edit mode
	editField string

	// Shell layout for unified 3-pane layout (chat only, no sidebar)
	shell *pkgtui.ShellLayout
	// Model selector shown under chat pane
	agentSelector *pkgtui.AgentSelector

	// Callbacks
	onAccept     func(proposals []epics.EpicProposal) tea.Cmd
	onRegenerate func() tea.Cmd
	onBack       func() tea.Cmd
}

// NewEpicReviewView creates a new epic review view.
func NewEpicReviewView(proposals []epics.EpicProposal) *EpicReviewView {
	return &EpicReviewView{
		proposals: proposals,
		expanded:  make(map[int]bool),
		shell:     pkgtui.NewShellLayout(),
	}
}

// SetAgentSelector sets the shared agent selector.
func (v *EpicReviewView) SetAgentSelector(selector *pkgtui.AgentSelector) {
	v.agentSelector = selector
}

// SetCallbacks sets the action callbacks.
func (v *EpicReviewView) SetCallbacks(
	onAccept func([]epics.EpicProposal) tea.Cmd,
	onRegenerate func() tea.Cmd,
	onBack func() tea.Cmd,
) {
	v.onAccept = onAccept
	v.onRegenerate = onRegenerate
	v.onBack = onBack
}

// Init implements View
func (v *EpicReviewView) Init() tea.Cmd {
	return nil
}

// Update implements View
func (v *EpicReviewView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height - 4
		v.shell.SetSize(v.width, v.height)
		return v, nil

	case tea.KeyMsg:
		if v.agentSelector != nil {
			selectorMsg, selectorCmd := v.agentSelector.Update(msg)
			if selectorMsg != nil {
				return v, tea.Batch(selectorCmd, func() tea.Msg { return selectorMsg })
			}
			if v.agentSelector.Open || msg.Type == tea.KeyF2 {
				return v, selectorCmd
			}
		}

		// Let shell handle global keys first (Tab for focus cycling)
		v.shell, cmd = v.shell.Update(msg)
		if cmd != nil {
			return v, cmd
		}

		switch {
		case key.Matches(msg, commonKeys.Select) || key.Matches(msg, commonKeys.Toggle):
			// Toggle expand selected (same as space for consistency)
			v.expanded[v.selected] = !v.expanded[v.selected]
			return v, nil

		case msg.String() == "A":
			// Accept ALL proposals (uppercase A for intentional action)
			if v.onAccept != nil {
				return v, v.onAccept(v.proposals)
			}
			return v, nil

		case msg.String() == "R":
			// Regenerate (uppercase R to differentiate from refresh)
			// Warning: destructive if user has edits
			if v.hasEdits() {
				// Show warning - for now just regenerate
				// TODO: Add confirmation dialog
			}
			if v.onRegenerate != nil {
				return v, v.onRegenerate()
			}
			return v, nil

		case msg.String() == "e":
			// Edit selected
			v.editing = true
			return v, nil

		case msg.String() == "d":
			// Delete selected
			if v.selected >= 0 && v.selected < len(v.proposals) {
				v.proposals = append(v.proposals[:v.selected], v.proposals[v.selected+1:]...)
				if v.selected >= len(v.proposals) {
					v.selected = len(v.proposals) - 1
				}
			}
			return v, nil

		case key.Matches(msg, commonKeys.NavDown):
			if v.selected < len(v.proposals)-1 {
				v.selected++
			}
			return v, nil

		case key.Matches(msg, commonKeys.NavUp):
			if v.selected > 0 {
				v.selected--
			}
			return v, nil

		case key.Matches(msg, commonKeys.Back):
			if v.editing {
				v.editing = false
			} else if v.onBack != nil {
				return v, v.onBack()
			}
			return v, nil

		case msg.String() == "backspace" || msg.String() == "b":
			if !v.editing && v.onBack != nil {
				return v, v.onBack()
			}
			return v, nil

		case msg.String() == "+":
			// Increase priority
			if v.selected >= 0 && v.selected < len(v.proposals) {
				v.proposals[v.selected].Edited = true
				switch v.proposals[v.selected].Priority {
				case epics.PriorityP3:
					v.proposals[v.selected].Priority = epics.PriorityP2
				case epics.PriorityP2:
					v.proposals[v.selected].Priority = epics.PriorityP1
				case epics.PriorityP1:
					v.proposals[v.selected].Priority = epics.PriorityP0
				}
			}
			return v, nil

		case msg.String() == "-":
			// Decrease priority
			if v.selected >= 0 && v.selected < len(v.proposals) {
				v.proposals[v.selected].Edited = true
				switch v.proposals[v.selected].Priority {
				case epics.PriorityP0:
					v.proposals[v.selected].Priority = epics.PriorityP1
				case epics.PriorityP1:
					v.proposals[v.selected].Priority = epics.PriorityP2
				case epics.PriorityP2:
					v.proposals[v.selected].Priority = epics.PriorityP3
				}
			}
			return v, nil
		}
	}

	return v, nil
}

func (v *EpicReviewView) hasEdits() bool {
	for _, p := range v.proposals {
		if p.Edited {
			return true
		}
	}
	return false
}

// View implements View
func (v *EpicReviewView) View() string {
	// Render using shell layout (without sidebar for review views)
	document := v.renderDocument()
	chat := v.renderChat()

	return v.shell.RenderWithoutSidebar(document, chat)
}

// renderDocument renders the main document pane (epic list).
func (v *EpicReviewView) renderDocument() string {
	if len(v.proposals) == 0 {
		emptyStyle := lipgloss.NewStyle().
			Foreground(pkgtui.ColorMuted).
			Italic(true)
		return emptyStyle.Render("No epics proposed. Press b to go back and try a different description.")
	}

	var sections []string

	// Header with summary stats
	headerStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorPrimary).
		Bold(true)
	totalTasks := epics.EstimateTotalTasks(v.proposals)
	header := fmt.Sprintf("Review Epics")
	sections = append(sections, headerStyle.Render(header))

	// Stats line
	statsStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorMuted).
		MarginBottom(1)
	stats := fmt.Sprintf("%d epics  •  ~%d estimated tasks", len(v.proposals), totalTasks)
	sections = append(sections, statsStyle.Render(stats))
	sections = append(sections, "")

	// Epic list in a styled container
	var epicLines []string
	for i, p := range v.proposals {
		isSelected := i == v.selected
		isExpanded := v.expanded[i]
		epicView := v.renderEpic(p, isSelected, isExpanded)
		epicLines = append(epicLines, epicView)
	}

	epicsContent := strings.Join(epicLines, "\n")
	listWidth := v.shell.LeftWidth()
	if listWidth <= 0 {
		listWidth = v.width / 2
	}
	listStyle := lipgloss.NewStyle().
		Background(pkgtui.ColorBgLight).
		Padding(1, 2).
		Width(listWidth - 4)

	sections = append(sections, listStyle.Render(epicsContent))

	return lipgloss.JoinVertical(lipgloss.Left, sections...)
}

// renderChat renders the chat pane.
func (v *EpicReviewView) renderChat() string {
	var lines []string

	chatTitle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorPrimary).
		Bold(true)

	lines = append(lines, chatTitle.Render("Epic Review Chat"))
	lines = append(lines, "")

	mutedStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorMuted).
		Italic(true)

	lines = append(lines, mutedStyle.Render("Ask questions about the epics..."))
	lines = append(lines, "")

	hintStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorMuted)

	lines = append(lines, hintStyle.Render("Tab to focus chat"))

	if v.agentSelector != nil {
		lines = append(lines, "")
		lines = append(lines, v.agentSelector.View())
	}

	return strings.Join(lines, "\n")
}

func (v *EpicReviewView) renderEpic(p epics.EpicProposal, selected, expanded bool) string {
	var lines []string

	// Epic header
	var selector string
	if selected {
		selector = ">"
	} else {
		selector = " "
	}

	// Size badge
	sizeStyle := lipgloss.NewStyle().
		Background(pkgtui.ColorBgLight).
		Foreground(pkgtui.ColorFgDim).
		Padding(0, 1)

	sizeBadge := sizeStyle.Render(string(p.Size))

	// Priority badge
	var priorityStyle lipgloss.Style
	switch p.Priority {
	case epics.PriorityP0:
		priorityStyle = lipgloss.NewStyle().Foreground(pkgtui.ColorError).Bold(true)
	case epics.PriorityP1:
		priorityStyle = lipgloss.NewStyle().Foreground(pkgtui.ColorWarning).Bold(true)
	case epics.PriorityP2:
		priorityStyle = lipgloss.NewStyle().Foreground(pkgtui.ColorPrimary)
	default:
		priorityStyle = lipgloss.NewStyle().Foreground(pkgtui.ColorMuted)
	}
	priorityBadge := priorityStyle.Render(string(p.Priority))

	// Title
	titleStyle := lipgloss.NewStyle().Foreground(pkgtui.ColorFg)
	if selected {
		titleStyle = titleStyle.Bold(true).Foreground(pkgtui.ColorPrimary)
	}

	// Edited indicator
	editedMark := ""
	if p.Edited {
		editedMark = lipgloss.NewStyle().Foreground(pkgtui.ColorWarning).Render(" (edited)")
	}

	header := fmt.Sprintf("%s %s %s  %s  %s%s",
		selector,
		p.ID,
		sizeBadge,
		priorityBadge,
		titleStyle.Render(p.Title),
		editedMark,
	)
	lines = append(lines, header)

	// Dependencies
	if len(p.Dependencies) > 0 {
		depStyle := pkgtui.LabelStyle.MarginLeft(4)
		deps := strings.Join(p.Dependencies, ", ")
		lines = append(lines, depStyle.Render(fmt.Sprintf("depends on: %s", deps)))
	}

	// Task estimate
	taskStyle := pkgtui.LabelStyle.MarginLeft(4)
	lines = append(lines, taskStyle.Render(fmt.Sprintf("~%d tasks", p.TaskCount)))

	// Expanded view
	if expanded {
		// Description
		if p.Description != "" {
			descStyle := pkgtui.LabelStyle.MarginLeft(4).Width(v.width - 10)
			lines = append(lines, "")
			lines = append(lines, descStyle.Render(p.Description))
		}

		// Stories
		if len(p.Stories) > 0 {
			lines = append(lines, "")
			storyHeaderStyle := pkgtui.SubtitleStyle.MarginLeft(4)
			lines = append(lines, storyHeaderStyle.Render("Stories:"))
			for _, s := range p.Stories {
				storySizeStyle := lipgloss.NewStyle().
					Background(pkgtui.ColorBgLight).
					Foreground(pkgtui.ColorFgDim).
					Padding(0, 1)
				storyLine := fmt.Sprintf("      %s %s - %s",
					storySizeStyle.Render(string(s.Size)),
					s.ID,
					s.Title,
				)
				lines = append(lines, storyLine)
			}
		}
	}

	lines = append(lines, "") // Spacer

	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

func (v *EpicReviewView) renderActions() string {
	var actions []string

	keyStyle := pkgtui.HelpKeyStyle
	descStyle := pkgtui.HelpDescStyle

	actions = append(actions, fmt.Sprintf("%s %s",
		keyStyle.Render("A"),
		descStyle.Render("accept all")))

	actions = append(actions, fmt.Sprintf("%s %s",
		keyStyle.Render("Enter"),
		descStyle.Render("expand")))

	actions = append(actions, fmt.Sprintf("%s %s",
		keyStyle.Render("e"),
		descStyle.Render("edit")))

	actions = append(actions, fmt.Sprintf("%s %s",
		keyStyle.Render("d"),
		descStyle.Render("delete")))

	actions = append(actions, fmt.Sprintf("%s %s",
		keyStyle.Render("+/-"),
		descStyle.Render("priority")))

	actions = append(actions, fmt.Sprintf("%s %s",
		keyStyle.Render("R"),
		descStyle.Render("regenerate")))

	actions = append(actions, fmt.Sprintf("%s %s",
		keyStyle.Render("space"),
		descStyle.Render("expand")))

	return strings.Join(actions, "  ")
}

// Focus implements View
func (v *EpicReviewView) Focus() tea.Cmd {
	return nil
}

// Blur implements View
func (v *EpicReviewView) Blur() {}

// Name implements View
func (v *EpicReviewView) Name() string {
	return "Epics"
}

// ShortHelp implements View
func (v *EpicReviewView) ShortHelp() string {
	return "A accept  b back  d delete  +/- priority  space expand  F2 model  Tab focus"
}

// FullHelp implements FullHelpProvider
func (v *EpicReviewView) FullHelp() []tui.HelpBinding {
	return []tui.HelpBinding{
		{Key: "j/k", Description: "Navigate down/up"},
		{Key: "enter", Description: "Toggle expand selected"},
		{Key: "space", Description: "Toggle expand selected"},
		{Key: "A", Description: "Accept ALL proposals"},
		{Key: "e", Description: "Edit selected epic"},
		{Key: "d", Description: "Delete selected epic"},
		{Key: "+/-", Description: "Increase/decrease priority"},
		{Key: "R", Description: "Regenerate proposals"},
		{Key: "b", Description: "Go back"},
		{Key: "esc", Description: "Cancel edit / go back"},
		{Key: "backspace", Description: "Go back"},
	}
}

// GetProposals returns the current proposals (potentially edited).
func (v *EpicReviewView) GetProposals() []epics.EpicProposal {
	return v.proposals
}
