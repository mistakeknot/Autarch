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

// ColdwineView displays epics, stories, and tasks with the unified shell layout.
type ColdwineView struct {
	client   *autarch.Client
	epics    []autarch.Epic
	stories  []autarch.Story
	tasks    []autarch.Task
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

// NewColdwineView creates a new Coldwine view
func NewColdwineView(client *autarch.Client) *ColdwineView {
	return &ColdwineView{
		client: client,
		shell:  pkgtui.NewShellLayout(),
	}
}

// SetAgentSelector sets the shared agent selector.
func (v *ColdwineView) SetAgentSelector(selector *pkgtui.AgentSelector) {
	v.agentSelector = selector
}

// Compile-time interface assertion for SidebarProvider
var _ pkgtui.SidebarProvider = (*ColdwineView)(nil)

type epicsLoadedMsg struct {
	epics   []autarch.Epic
	stories []autarch.Story
	tasks   []autarch.Task
	err     error
}

// Init implements View
func (v *ColdwineView) Init() tea.Cmd {
	return v.loadData()
}

func (v *ColdwineView) loadData() tea.Cmd {
	return func() tea.Msg {
		epics, err := v.client.ListEpics("")
		if err != nil {
			return epicsLoadedMsg{err: err}
		}
		stories, err := v.client.ListStories("")
		if err != nil {
			return epicsLoadedMsg{err: err}
		}
		tasks, err := v.client.ListTasks("", "")
		if err != nil {
			return epicsLoadedMsg{err: err}
		}
		return epicsLoadedMsg{epics: epics, stories: stories, tasks: tasks}
	}
}

// Update implements View
func (v *ColdwineView) Update(msg tea.Msg) (tui.View, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height - 4
		v.shell.SetSize(v.width, v.height)
		return v, nil

	case epicsLoadedMsg:
		v.loading = false
		if msg.err != nil {
			v.err = msg.err
		} else {
			v.epics = msg.epics
			v.stories = msg.stories
			v.tasks = msg.tasks
		}
		return v, nil

	case pkgtui.SidebarSelectMsg:
		// Find epic by ID and select it
		for i, epic := range v.epics {
			if epic.ID == msg.ItemID {
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
				if v.selected < len(v.epics)-1 {
					v.selected++
				}
			case key.Matches(msg, commonKeys.NavUp):
				if v.selected > 0 {
					v.selected--
				}
			case key.Matches(msg, commonKeys.Refresh):
				v.loading = true
				return v, v.loadData()
			}
		case pkgtui.FocusChat:
			// Chat input handled by chat panel (future)
		}
	}

	return v, nil
}

// View implements View
func (v *ColdwineView) View() string {
	if v.loading {
		return pkgtui.LabelStyle.Render("Loading epics...")
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
func (v *ColdwineView) SidebarItems() []pkgtui.SidebarItem {
	if len(v.epics) == 0 {
		return nil
	}

	items := make([]pkgtui.SidebarItem, len(v.epics))
	for i, epic := range v.epics {
		title := epic.Title
		if title == "" && len(epic.ID) >= 8 {
			title = epic.ID[:8]
		}

		items[i] = pkgtui.SidebarItem{
			ID:    epic.ID,
			Label: title,
			Icon:  epicStatusIcon(epic.Status),
		}
	}
	return items
}

// epicStatusIcon returns a plain icon for the epic status (no styling).
func epicStatusIcon(status autarch.EpicStatus) string {
	switch status {
	case autarch.EpicStatusOpen:
		return "○"
	case autarch.EpicStatusInProgress:
		return "●"
	case autarch.EpicStatusDone:
		return "✓"
	default:
		return "?"
	}
}

// renderDocument renders the main document pane (epic details with stories).
func (v *ColdwineView) renderDocument() string {
	var lines []string

	lines = append(lines, pkgtui.TitleStyle.Render("Epic Details"))
	lines = append(lines, "")

	if len(v.epics) == 0 {
		lines = append(lines, pkgtui.LabelStyle.Render("No epics found"))
		lines = append(lines, "")
		lines = append(lines, pkgtui.LabelStyle.Render("Create an epic to break down a spec into implementable work."))
		return strings.Join(lines, "\n")
	}

	if v.selected >= len(v.epics) {
		lines = append(lines, pkgtui.LabelStyle.Render("No epic selected"))
		return strings.Join(lines, "\n")
	}

	e := v.epics[v.selected]

	lines = append(lines, fmt.Sprintf("Title: %s", e.Title))
	lines = append(lines, fmt.Sprintf("Status: %s", e.Status))
	lines = append(lines, "")

	// Show stories for this epic
	lines = append(lines, pkgtui.SubtitleStyle.Render("Stories"))

	foundStories := false
	for _, s := range v.stories {
		if s.EpicID == e.ID {
			foundStories = true
			icon := v.storyStatusIcon(s.Status)
			lines = append(lines, fmt.Sprintf("  %s %s", icon, s.Title))
		}
	}

	if !foundStories {
		lines = append(lines, pkgtui.LabelStyle.Render("  No stories yet"))
	}

	return strings.Join(lines, "\n")
}

// renderChat renders the chat pane.
func (v *ColdwineView) renderChat() string {
	var lines []string

	chatTitle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorPrimary).
		Bold(true)

	lines = append(lines, chatTitle.Render("Task Chat"))
	lines = append(lines, "")

	mutedStyle := lipgloss.NewStyle().
		Foreground(pkgtui.ColorMuted).
		Italic(true)

	lines = append(lines, mutedStyle.Render("Ask questions about this epic..."))
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

func (v *ColdwineView) storyStatusIcon(status autarch.StoryStatus) string {
	switch status {
	case autarch.StoryStatusTodo:
		return pkgtui.StatusIdle.Render("○")
	case autarch.StoryStatusInProgress:
		return pkgtui.StatusRunning.Render("●")
	case autarch.StoryStatusReview:
		return pkgtui.StatusWaiting.Render("◐")
	case autarch.StoryStatusDone:
		return pkgtui.StatusRunning.Render("✓")
	default:
		return pkgtui.StatusIdle.Render("?")
	}
}

// Focus implements View
func (v *ColdwineView) Focus() tea.Cmd {
	return v.loadData()
}

// Blur implements View
func (v *ColdwineView) Blur() {}

// Name implements View
func (v *ColdwineView) Name() string {
	return "Coldwine"
}

// ShortHelp implements View
func (v *ColdwineView) ShortHelp() string {
	return "j/k navigate  r refresh  F2 model  Tab focus  Ctrl+B sidebar"
}

// Commands implements CommandProvider
func (v *ColdwineView) Commands() []tui.Command {
	return []tui.Command{
		{
			Name:        "New Epic",
			Description: "Create a new epic",
			Action: func() tea.Cmd {
				return nil
			},
		},
		{
			Name:        "New Story",
			Description: "Create a new story",
			Action: func() tea.Cmd {
				return nil
			},
		},
		{
			Name:        "New Task",
			Description: "Create a new task",
			Action: func() tea.Cmd {
				return nil
			},
		},
	}
}
