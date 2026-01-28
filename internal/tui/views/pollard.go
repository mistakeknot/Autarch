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

// PollardView displays research insights with the unified shell layout.
type PollardView struct {
	client   *autarch.Client
	insights []autarch.Insight
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

// NewPollardView creates a new Pollard view
func NewPollardView(client *autarch.Client) *PollardView {
	return &PollardView{
		client: client,
		shell:  pkgtui.NewShellLayout(),
	}
}

// SetAgentSelector sets the shared agent selector.
func (v *PollardView) SetAgentSelector(selector *pkgtui.AgentSelector) {
	v.agentSelector = selector
}

// Compile-time interface assertion for SidebarProvider
var _ pkgtui.SidebarProvider = (*PollardView)(nil)

type insightsLoadedMsg struct {
	insights []autarch.Insight
	err      error
}

// Init implements View
func (v *PollardView) Init() tea.Cmd {
	return v.loadInsights()
}

func (v *PollardView) loadInsights() tea.Cmd {
	return func() tea.Msg {
		insights, err := v.client.ListInsights("", "")
		return insightsLoadedMsg{insights: insights, err: err}
	}
}

// Update implements View
func (v *PollardView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height - 4
		v.shell.SetSize(v.width, v.height)
		return v, nil

	case insightsLoadedMsg:
		v.loading = false
		if msg.err != nil {
			v.err = msg.err
		} else {
			v.insights = msg.insights
		}
		return v, nil

	case pkgtui.SidebarSelectMsg:
		// Find insight by ID and select it
		for i, insight := range v.insights {
			if insight.ID == msg.ItemID {
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
				if v.selected < len(v.insights)-1 {
					v.selected++
				}
			case key.Matches(msg, commonKeys.NavUp):
				if v.selected > 0 {
					v.selected--
				}
			case key.Matches(msg, commonKeys.Refresh):
				v.loading = true
				return v, v.loadInsights()
			}
		case pkgtui.FocusChat:
			// Chat input handled by chat panel (future)
		}
	}

	return v, nil
}

// View implements View
func (v *PollardView) View() string {
	if v.loading {
		return pkgtui.LabelStyle.Render("Loading insights...")
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
func (v *PollardView) SidebarItems() []pkgtui.SidebarItem {
	if len(v.insights) == 0 {
		return nil
	}

	items := make([]pkgtui.SidebarItem, len(v.insights))
	for i, insight := range v.insights {
		title := insight.Title
		if title == "" && len(insight.ID) >= 8 {
			title = insight.ID[:8]
		}

		items[i] = pkgtui.SidebarItem{
			ID:    insight.ID,
			Label: title,
			Icon:  categoryIcon(insight.Category),
		}
	}
	return items
}

// categoryIcon returns an icon for the insight category.
func categoryIcon(category string) string {
	switch category {
	case "competitor":
		return "⚔"
	case "technology":
		return "⚙"
	case "market":
		return "📊"
	case "user":
		return "👤"
	default:
		return "•"
	}
}

// renderDocument renders the main document pane (insight details).
func (v *PollardView) renderDocument() string {
	width := v.shell.LeftWidth()
	if width <= 0 {
		width = v.width / 2
	}

	var lines []string

	lines = append(lines, pkgtui.TitleStyle.Render("Insight Details"))
	lines = append(lines, "")

	if len(v.insights) == 0 {
		lines = append(lines, pkgtui.LabelStyle.Render("No insights found"))
		lines = append(lines, "")
		lines = append(lines, pkgtui.LabelStyle.Render("Run Pollard hunters to gather research insights."))
		return strings.Join(lines, "\n")
	}

	if v.selected >= len(v.insights) {
		lines = append(lines, pkgtui.LabelStyle.Render("No insight selected"))
		return strings.Join(lines, "\n")
	}

	i := v.insights[v.selected]

	lines = append(lines, fmt.Sprintf("Title: %s", i.Title))
	lines = append(lines, fmt.Sprintf("Category: %s", i.Category))
	lines = append(lines, fmt.Sprintf("Source: %s", i.Source))
	lines = append(lines, fmt.Sprintf("Score: %.2f", i.Score))
	lines = append(lines, "")

	if i.Body != "" {
		lines = append(lines, pkgtui.SubtitleStyle.Render("Summary"))
		wrapped := wordWrap(i.Body, width-4)
		lines = append(lines, wrapped...)
		lines = append(lines, "")
	}

	if i.URL != "" {
		lines = append(lines, fmt.Sprintf("URL: %s", i.URL))
	}

	if i.SpecID != "" {
		lines = append(lines, fmt.Sprintf("Linked Spec: %s", i.SpecID))
	}

	return strings.Join(lines, "\n")
}

// renderChat renders the chat pane.
func (v *PollardView) renderChat() string {
	var lines []string

	chatTitle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorPrimary).
		Bold(true)

	lines = append(lines, chatTitle.Render("Research Chat"))
	lines = append(lines, "")

	mutedStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorMuted).
		Italic(true)

	lines = append(lines, mutedStyle.Render("Ask questions about this insight..."))
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

func wordWrap(text string, width int) []string {
	if width <= 0 {
		return []string{text}
	}

	words := strings.Fields(text)
	var lines []string
	var current strings.Builder

	for _, word := range words {
		if current.Len()+len(word)+1 > width {
			if current.Len() > 0 {
				lines = append(lines, current.String())
				current.Reset()
			}
		}
		if current.Len() > 0 {
			current.WriteString(" ")
		}
		current.WriteString(word)
	}

	if current.Len() > 0 {
		lines = append(lines, current.String())
	}

	return lines
}

// Focus implements View
func (v *PollardView) Focus() tea.Cmd {
	return v.loadInsights()
}

// Blur implements View
func (v *PollardView) Blur() {}

// Name implements View
func (v *PollardView) Name() string {
	return "Pollard"
}

// ShortHelp implements View
func (v *PollardView) ShortHelp() string {
	return "j/k navigate  r refresh  F2 model  Tab focus  Ctrl+B sidebar"
}

// Commands implements CommandProvider
func (v *PollardView) Commands() []tui.Command {
	return []tui.Command{
		{
			Name:        "Run Research",
			Description: "Execute Pollard hunters",
			Action: func() tea.Cmd {
				return nil
			},
		},
		{
			Name:        "Link Insight",
			Description: "Link insight to a spec",
			Action: func() tea.Cmd {
				return nil
			},
		},
	}
}
