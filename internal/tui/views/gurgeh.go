package views

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mistakeknot/autarch/internal/tui"
	"github.com/mistakeknot/autarch/pkg/autarch"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// GurgehView displays specs (PRDs) with the unified shell layout.
type GurgehView struct {
	client   *autarch.Client
	specs    []autarch.Spec
	selected int
	width    int
	height   int
	loading  bool
	err      error

	// Shell layout for unified 3-pane layout
	shell *pkgtui.ShellLayout
	// Model selector shown under chat pane
	agentSelector *pkgtui.AgentSelector
}

// NewGurgehView creates a new Gurgeh view
func NewGurgehView(client *autarch.Client) *GurgehView {
	return &GurgehView{
		client: client,
		shell:  pkgtui.NewShellLayout(),
	}
}

// SetAgentSelector sets the shared agent selector.
func (v *GurgehView) SetAgentSelector(selector *pkgtui.AgentSelector) {
	v.agentSelector = selector
}

// Compile-time interface assertion for SidebarProvider
var _ pkgtui.SidebarProvider = (*GurgehView)(nil)

type specsLoadedMsg struct {
	specs []autarch.Spec
	err   error
}

// Init implements View
func (v *GurgehView) Init() tea.Cmd {
	return v.loadSpecs()
}

func (v *GurgehView) loadSpecs() tea.Cmd {
	return func() tea.Msg {
		specs, err := v.client.ListSpecs("")
		return specsLoadedMsg{specs: specs, err: err}
	}
}

// Update implements View
func (v *GurgehView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height - 4
		v.shell.SetSize(v.width, v.height)
		return v, nil

	case specsLoadedMsg:
		v.loading = false
		if msg.err != nil {
			v.err = msg.err
		} else {
			v.specs = msg.specs
		}
		return v, nil

	case pkgtui.SidebarSelectMsg:
		// Find spec by ID and select it
		for i, s := range v.specs {
			if s.ID == msg.ItemID {
				v.selected = i
				break
			}
		}
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

		// Let shell handle global keys first
		v.shell, cmd = v.shell.Update(msg)
		if cmd != nil {
			return v, cmd
		}

		// Handle view-specific keys based on focus
		switch v.shell.Focus() {
		case pkgtui.FocusSidebar:
			// Navigation handled by shell/sidebar
		case pkgtui.FocusDocument:
			switch {
			case key.Matches(msg, commonKeys.NavDown):
				if v.selected < len(v.specs)-1 {
					v.selected++
				}
			case key.Matches(msg, commonKeys.NavUp):
				if v.selected > 0 {
					v.selected--
				}
			case key.Matches(msg, commonKeys.Refresh):
				v.loading = true
				return v, v.loadSpecs()
			}
		case pkgtui.FocusChat:
			// Chat input handled by chat panel (future)
		}
	}

	return v, nil
}

// View implements View
func (v *GurgehView) View() string {
	if v.loading {
		return pkgtui.LabelStyle.Render("Loading specs...")
	}

	if v.err != nil {
		return tui.ErrorView(v.err)
	}

	// Render using shell layout
	sidebarItems := v.SidebarItems()
	document := v.renderDocument()
	chat := v.renderChat()

	return v.shell.Render(sidebarItems, document, chat)
}

// SidebarItems implements SidebarProvider.
func (v *GurgehView) SidebarItems() []pkgtui.SidebarItem {
	if len(v.specs) == 0 {
		return nil
	}

	items := make([]pkgtui.SidebarItem, len(v.specs))
	for i, s := range v.specs {
		title := s.Title
		if title == "" && len(s.ID) >= 8 {
			title = s.ID[:8]
		}

		items[i] = pkgtui.SidebarItem{
			ID:    s.ID,
			Label: title,
			Icon:  statusIcon(s.Status),
		}
	}
	return items
}

// statusIcon returns an icon for the spec status.
func statusIcon(status autarch.SpecStatus) string {
	switch status {
	case autarch.SpecStatusDraft:
		return "◐"
	case autarch.SpecStatusResearch:
		return "◑"
	case autarch.SpecStatusValidated:
		return "✓"
	case autarch.SpecStatusArchived:
		return "○"
	default:
		return "•"
	}
}

// renderDocument renders the main document pane (spec details).
func (v *GurgehView) renderDocument() string {
	width := v.shell.LeftWidth()
	if width <= 0 {
		width = v.width / 2
	}

	var lines []string

	lines = append(lines, pkgtui.TitleStyle.Render("Spec Details"))
	lines = append(lines, "")

	if len(v.specs) == 0 {
		lines = append(lines, pkgtui.LabelStyle.Render("No specs found"))
		lines = append(lines, "")
		lines = append(lines, pkgtui.LabelStyle.Render("Create a new spec with 'n' or use the command palette."))
		return strings.Join(lines, "\n")
	}

	if v.selected >= len(v.specs) {
		lines = append(lines, pkgtui.LabelStyle.Render("No spec selected"))
		return strings.Join(lines, "\n")
	}

	s := v.specs[v.selected]

	lines = append(lines, fmt.Sprintf("Title: %s", s.Title))
	lines = append(lines, fmt.Sprintf("Status: %s", s.Status))
	lines = append(lines, fmt.Sprintf("Project: %s", s.Project))
	lines = append(lines, "")

	if s.Vision != "" {
		lines = append(lines, pkgtui.SubtitleStyle.Render("Vision"))
		lines = append(lines, s.Vision)
		lines = append(lines, "")
	}

	if s.Problem != "" {
		lines = append(lines, pkgtui.SubtitleStyle.Render("Problem"))
		lines = append(lines, s.Problem)
		lines = append(lines, "")
	}

	if s.Users != "" {
		lines = append(lines, pkgtui.SubtitleStyle.Render("Users"))
		lines = append(lines, s.Users)
	}

	return strings.Join(lines, "\n")
}

// renderChat renders the chat pane (placeholder for now).
func (v *GurgehView) renderChat() string {
	var lines []string

	chatTitle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorPrimary).
		Bold(true)

	lines = append(lines, chatTitle.Render("Chat"))
	lines = append(lines, "")

	mutedStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorMuted).
		Italic(true)

	lines = append(lines, mutedStyle.Render("Ask questions about this spec..."))
	lines = append(lines, "")

	hintStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorMuted)

	lines = append(lines, hintStyle.Render("Tab to focus • Ctrl+B toggle sidebar"))

	if v.agentSelector != nil {
		lines = append(lines, "")
		lines = append(lines, v.agentSelector.View())
	}

	return strings.Join(lines, "\n")
}

// Focus implements View
func (v *GurgehView) Focus() tea.Cmd {
	return v.loadSpecs()
}

// Blur implements View
func (v *GurgehView) Blur() {}

// Name implements View
func (v *GurgehView) Name() string {
	return "Gurgeh"
}

// ShortHelp implements View
func (v *GurgehView) ShortHelp() string {
	return "j/k navigate  r refresh  F2 model  Tab focus  Ctrl+B sidebar"
}

// Commands implements CommandProvider
func (v *GurgehView) Commands() []tui.Command {
	return []tui.Command{
		{
			Name:        "New Spec",
			Description: "Create a new specification",
			Action: func() tea.Cmd {
				// TODO: implement
				return nil
			},
		},
		{
			Name:        "Refresh Specs",
			Description: "Reload spec list",
			Action: func() tea.Cmd {
				return v.loadSpecs()
			},
		},
	}
}
