package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
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

	// Shell layout for unified 3-pane layout
	shell *pkgtui.ShellLayout
	// Chat panel for interactive input
	chatPanel *pkgtui.ChatPanel
	// Chat handler for Coldwine-specific context
	chatHandler *ColdwineChatHandler
}

// NewColdwineView creates a new Coldwine view
func NewColdwineView(client *autarch.Client) *ColdwineView {
	chatPanel := pkgtui.NewChatPanel()
	chatPanel.SetComposerPlaceholder("Ask questions about this epic...")
	chatPanel.SetComposerHint("enter send  tab focus  ctrl+b sidebar")
	chatHandler := NewColdwineChatHandler()
	chatPanel.SetHandler(chatHandler)

	return &ColdwineView{
		client:      client,
		shell:       pkgtui.NewShellLayout(),
		chatPanel:   chatPanel,
		chatHandler: chatHandler,
	}
}

// SetAgentSelector sets the shared agent selector.
func (v *ColdwineView) SetAgentSelector(selector *pkgtui.AgentSelector) {
	v.chatPanel.SetAgentSelector(selector)
}

// SetAgentName sets the selected agent name (satisfies interface).
func (v *ColdwineView) SetAgentName(name string) {}

// SetChatSettings sets chat settings on the chat panel.
func (v *ColdwineView) SetChatSettings(settings pkgtui.ChatSettings) {
	v.chatPanel.SetSettings(settings)
}

// ClearInput clears the chat composer (for ctrl+c soft cancel).
func (v *ColdwineView) ClearInput() {
	v.chatPanel.ClearComposer()
}

// Compile-time interface assertion for SidebarProvider
var _ pkgtui.SidebarProvider = (*ColdwineView)(nil)

type epicsLoadedMsg struct {
	epics   []autarch.Epic
	stories []autarch.Story
	tasks   []autarch.Task
	err     error
}

type epicCreatedMsg struct {
	epic autarch.Epic
	err  error
}

type storyCreatedMsg struct {
	story autarch.Story
	err   error
}

type taskCreatedMsg struct {
	task autarch.Task
	err  error
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
	if _, isKey := msg.(tea.KeyMsg); !isKey {
		v.chatPanel, cmd = v.chatPanel.Update(msg)
		if cmd != nil {
			return v, cmd
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		v.width = msg.Width - 6
		v.height = msg.Height - 4 - 2
		v.shell.SetSize(v.width, v.height)
		v.chatPanel.SetSize(v.shell.RightWidth(), v.shell.Height())
		return v, nil

	case epicsLoadedMsg:
		v.loading = false
		if msg.err != nil {
			// Don't block the whole view on data fetch failure —
			// show degraded view with empty data instead.
			v.epics = nil
			v.stories = nil
			v.tasks = nil
		} else {
			v.epics = msg.epics
			v.stories = msg.stories
			v.tasks = msg.tasks
		}
		return v, nil

	case epicCreatedMsg:
		if msg.err != nil {
			v.chatPanel.AddMessage("system", fmt.Sprintf("Failed to create epic: %v", msg.err))
			return v, nil
		}
		v.chatPanel.AddMessage("system", fmt.Sprintf("Created epic: %s", msg.epic.Title))
		return v, v.loadData()

	case storyCreatedMsg:
		if msg.err != nil {
			v.chatPanel.AddMessage("system", fmt.Sprintf("Failed to create story: %v", msg.err))
			return v, nil
		}
		v.chatPanel.AddMessage("system", fmt.Sprintf("Created story: %s", msg.story.Title))
		return v, v.loadData()

	case taskCreatedMsg:
		if msg.err != nil {
			v.chatPanel.AddMessage("system", fmt.Sprintf("Failed to create task: %v", msg.err))
			return v, nil
		}
		v.chatPanel.AddMessage("system", fmt.Sprintf("Created task: %s", msg.task.Title))
		return v, v.loadData()

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
		// Let shell handle global keys first (Tab, Shift-Tab, Ctrl+B)
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
			if msg.Type == tea.KeyEnter {
				if slashCmd := v.chatPanel.SubmitInput(); slashCmd != nil {
					return v, slashCmd
				}
				return v, nil
			}
			v.chatPanel, cmd = v.chatPanel.Update(msg)
			return v, cmd
		}
	}

	return v, nil
}

// View implements View
func (v *ColdwineView) View() string {
	if v.loading {
		return pkgtui.LabelStyle.Render("Loading epics...")
	}

	// Render using shell layout
	sidebarItems := v.SidebarItems()
	document := v.renderDocument()
	chat := v.chatPanel.View()

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
	v.shell.SetFocus(pkgtui.FocusChat)
	return tea.Batch(v.chatPanel.Focus(), v.loadData())
}

// Blur implements View
func (v *ColdwineView) Blur() {
	v.chatPanel.CancelStream()
	v.chatPanel.Blur()
}

// Name implements View
func (v *ColdwineView) Name() string {
	return "Coldwine"
}

// ShortHelp implements View
func (v *ColdwineView) ShortHelp() string {
	return "↑/↓ navigate  ctrl+r refresh  ctrl+g model  tab focus  ctrl+b sidebar"
}

// Commands implements CommandProvider
func (v *ColdwineView) Commands() []tui.Command {
	return []tui.Command{
		{
			Name:        "New Epic",
			Description: "Create a new epic",
			Action: func() tea.Cmd {
				client := v.client
				return func() tea.Msg {
					title := fmt.Sprintf("Untitled Epic — %s", time.Now().Format("Jan 2 15:04"))
					e, err := client.CreateEpic(autarch.Epic{Title: title})
					return epicCreatedMsg{epic: e, err: err}
				}
			},
		},
		{
			Name:        "New Story",
			Description: "Create a new story under selected epic",
			Action: func() tea.Cmd {
				client := v.client
				var epicID string
				if v.selected >= 0 && v.selected < len(v.epics) {
					epicID = v.epics[v.selected].ID
				}
				return func() tea.Msg {
					title := fmt.Sprintf("Untitled Story — %s", time.Now().Format("Jan 2 15:04"))
					s, err := client.CreateStory(autarch.Story{Title: title, EpicID: epicID})
					return storyCreatedMsg{story: s, err: err}
				}
			},
		},
		{
			Name:        "New Task",
			Description: "Create a new task",
			Action: func() tea.Cmd {
				client := v.client
				return func() tea.Msg {
					title := fmt.Sprintf("Untitled Task — %s", time.Now().Format("Jan 2 15:04"))
					t, err := client.CreateTask(autarch.Task{Title: title})
					return taskCreatedMsg{task: t, err: err}
				}
			},
		},
	}
}
