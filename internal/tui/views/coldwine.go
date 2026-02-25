package views

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/tui"
	"github.com/mistakeknot/autarch/pkg/autarch"
	"github.com/mistakeknot/autarch/pkg/intercore"
	pkgtui "github.com/mistakeknot/autarch/pkg/tui"
)

// ColdwineView displays epics, stories, and tasks with the unified shell layout.
type ColdwineView struct {
	client   *autarch.Client
	iclient  *intercore.Client // optional — nil when ic unavailable
	epics    []autarch.Epic
	stories  []autarch.Story
	tasks    []autarch.Task
	selected     int
	selectedTask int // index into filtered tasks for selected epic (-1 = none)
	width        int
	height       int
	loading      bool

	// Sprint data cached from Intercore (loaded async).
	epicRuns map[string]*intercore.Run // epicID → Run (nil if no sprint)

	// Task→dispatch mapping loaded from Intercore state.
	// Key: taskID, Value: dispatchID. Populated on data load.
	taskDispatches map[string]string

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

// SetIntercore sets the Intercore client for sprint operations.
// Pass nil if ic is unavailable — sprint commands will be hidden.
func (v *ColdwineView) SetIntercore(ic *intercore.Client) {
	v.iclient = ic
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

type sprintCreatedMsg struct {
	runID  string
	epicID string
	goal   string
	err    error
}

type taskDispatchedMsg struct {
	taskID     string
	dispatchID string
	err        error
}

// epicRunsLoadedMsg carries cached sprint data for all epics.
type epicRunsLoadedMsg struct {
	runs map[string]*intercore.Run // epicID → Run
}

// taskDispatchMapMsg carries the task→dispatch mapping loaded from Intercore state.
type taskDispatchMapMsg struct {
	dispatches map[string]string // taskID → dispatchID
}

// taskStatusPersistedMsg confirms that a task status was persisted to the backend.
type taskStatusPersistedMsg struct {
	taskID string
	status autarch.TaskStatus
	err    error
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

// loadEpicRuns fetches sprint associations for all loaded epics.
func (v *ColdwineView) loadEpicRuns() tea.Cmd {
	if v.iclient == nil || len(v.epics) == 0 {
		return nil
	}
	ic := v.iclient
	epicIDs := make([]string, len(v.epics))
	for i, e := range v.epics {
		epicIDs[i] = e.ID
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		runs := make(map[string]*intercore.Run, len(epicIDs))
		for _, eid := range epicIDs {
			runID, err := ic.StateGet(ctx, "epic.run_id", eid)
			if err != nil || runID == "" {
				continue
			}
			// StateGet returns JSON-quoted string — strip quotes.
			runID = strings.Trim(runID, `"`)
			if runID == "" {
				continue
			}
			run, err := ic.RunStatus(ctx, runID)
			if err != nil {
				continue
			}
			runs[eid] = run
		}
		return epicRunsLoadedMsg{runs: runs}
	}
}

// loadTaskDispatches loads the task→dispatch mapping from Intercore state
// for all currently-running tasks.
func (v *ColdwineView) loadTaskDispatches() tea.Cmd {
	if v.iclient == nil {
		return nil
	}
	// Collect IDs of tasks that might have dispatches.
	var taskIDs []string
	for _, t := range v.tasks {
		if t.Status == autarch.TaskStatusRunning {
			taskIDs = append(taskIDs, t.ID)
		}
	}
	if len(taskIDs) == 0 {
		return nil
	}

	ic := v.iclient
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		dispatches := make(map[string]string, len(taskIDs))
		for _, tid := range taskIDs {
			dispID, err := ic.StateGet(ctx, "task.dispatch_id", tid)
			if err != nil || dispID == "" {
				continue
			}
			dispID = strings.Trim(dispID, `"`)
			if dispID != "" {
				dispatches[tid] = dispID
			}
		}
		return taskDispatchMapMsg{dispatches: dispatches}
	}
}

// getEpicRunID returns the cached run ID for an epic, or empty string.
func (v *ColdwineView) getEpicRunID(epicID string) string {
	if run, ok := v.epicRuns[epicID]; ok && run != nil {
		return run.ID
	}
	return ""
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
		// Trigger async sprint data + task dispatch map load.
		return v, tea.Batch(v.loadEpicRuns(), v.loadTaskDispatches())

	case epicRunsLoadedMsg:
		v.epicRuns = msg.runs
		return v, nil

	case taskDispatchMapMsg:
		v.taskDispatches = msg.dispatches
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

	case sprintCreatedMsg:
		if msg.err != nil {
			v.chatPanel.AddMessage("system", fmt.Sprintf("Failed to create sprint: %v", msg.err))
			return v, nil
		}
		v.chatPanel.AddMessage("system", fmt.Sprintf("Sprint created: %s (run %s)", msg.goal, msg.runID))
		// Store the run ID in Intercore state and update local cache.
		if v.iclient != nil && msg.epicID != "" {
			if v.epicRuns == nil {
				v.epicRuns = make(map[string]*intercore.Run)
			}
			v.epicRuns[msg.epicID] = &intercore.Run{
				ID:    msg.runID,
				Goal:  msg.goal,
				Phase: "brainstorm",
			}
			ic := v.iclient
			epicID := msg.epicID
			runID := msg.runID
			return v, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = ic.StateSet(ctx, "epic.run_id", epicID, fmt.Sprintf("%q", runID))
				return nil
			}
		}
		return v, nil

	case tui.DispatchCompletedMsg:
		// A dispatch finished — find the matching task and update its status.
		d := msg.Dispatch
		taskIdx := -1
		for i := range v.tasks {
			if v.tasks[i].Status != autarch.TaskStatusRunning {
				continue
			}
			if taskMatchesDispatch(v.tasks[i], d, v.taskDispatches) {
				taskIdx = i
				break
			}
		}
		if taskIdx < 0 {
			return v, nil
		}

		// Determine new status based on dispatch outcome.
		var newStatus autarch.TaskStatus
		switch d.Status {
		case "completed":
			if d.ExitCode != nil && *d.ExitCode == 0 {
				newStatus = autarch.TaskStatusDone
			} else {
				newStatus = autarch.TaskStatusPending // non-zero exit → retry
			}
		case "failed", "cancelled":
			newStatus = autarch.TaskStatusPending
		default:
			return v, nil
		}

		// Update local state immediately for responsive UI.
		v.tasks[taskIdx].Status = newStatus
		taskTitle := v.tasks[taskIdx].Title
		v.chatPanel.AddMessage("system", fmt.Sprintf(
			"Dispatch %s %s for task %q — %s",
			d.ID, d.Status, taskTitle, d.ResultSummary()))

		// Persist: update task status in backend + store result summary in ic state.
		return v, v.persistDispatchResult(v.tasks[taskIdx], d, newStatus)

	case taskStatusPersistedMsg:
		if msg.err != nil {
			v.chatPanel.AddMessage("system", fmt.Sprintf("Failed to persist task status: %v", msg.err))
		}
		return v, nil

	case taskDispatchedMsg:
		if msg.err != nil {
			v.chatPanel.AddMessage("system", fmt.Sprintf("Dispatch failed: %v", msg.err))
			return v, nil
		}
		v.chatPanel.AddMessage("system", fmt.Sprintf("Dispatched task %s → dispatch %s", msg.taskID, msg.dispatchID))
		// Update local task status to running
		for i := range v.tasks {
			if v.tasks[i].ID == msg.taskID {
				v.tasks[i].Status = autarch.TaskStatusRunning
				break
			}
		}
		// Update local dispatch map so the watcher can match completions.
		if v.taskDispatches == nil {
			v.taskDispatches = make(map[string]string)
		}
		v.taskDispatches[msg.taskID] = msg.dispatchID
		// Store task→dispatch mapping in Intercore state
		if v.iclient != nil {
			ic := v.iclient
			taskID := msg.taskID
			dispatchID := msg.dispatchID
			return v, func() tea.Msg {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_ = ic.StateSet(ctx, "task.dispatch_id", taskID, fmt.Sprintf("%q", dispatchID))
				return nil
			}
		}
		return v, nil

	case pkgtui.SidebarSelectMsg:
		// Find epic by ID and select it
		for i, epic := range v.epics {
			if epic.ID == msg.ItemID {
				v.selected = i
				v.selectedTask = 0
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
				epicTasks := v.epicTasks()
				if v.selectedTask < len(epicTasks)-1 {
					v.selectedTask++
				} else if v.selected < len(v.epics)-1 {
					v.selected++
					v.selectedTask = 0
				}
			case key.Matches(msg, commonKeys.NavUp):
				if v.selectedTask > 0 {
					v.selectedTask--
				} else if v.selected > 0 {
					v.selected--
					v.selectedTask = 0
				}
			case key.Matches(msg, commonKeys.Refresh):
				v.loading = true
				return v, v.loadData()
			case msg.String() == "d":
				return v, v.dispatchSelectedTask()
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

	// Show sprint info if cached from async load.
	if run, ok := v.epicRuns[e.ID]; ok && run != nil {
		lines = append(lines, fmt.Sprintf("Sprint: %s  Phase: %s", run.ID, run.Phase))
	}

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

	// Show tasks for this epic
	lines = append(lines, "")
	lines = append(lines, pkgtui.SubtitleStyle.Render("Tasks"))

	epicTasks := v.epicTasks()
	if len(epicTasks) == 0 {
		lines = append(lines, pkgtui.LabelStyle.Render("  No tasks yet"))
	} else {
		for i, t := range epicTasks {
			icon := taskStatusIcon(t.Status)
			cursor := "  "
			if i == v.selectedTask {
				cursor = "▸ "
			}
			agent := ""
			if t.Agent != "" {
				agent = fmt.Sprintf(" [%s]", t.Agent)
			}
			lines = append(lines, fmt.Sprintf("%s%s %s%s", cursor, icon, t.Title, agent))
		}
		if v.iclient != nil {
			lines = append(lines, "")
			lines = append(lines, pkgtui.LabelStyle.Render("  d dispatch selected task"))
		}
	}

	return strings.Join(lines, "\n")
}

// taskMatchesDispatch checks if a task corresponds to a dispatch.
// If a dispatch ID mapping exists for the task, it's authoritative — name
// matching is only used as a fallback when no mapping is stored.
func taskMatchesDispatch(task autarch.Task, d intercore.Dispatch, taskDispatches map[string]string) bool {
	// Primary: dispatch ID from Intercore state mapping.
	if dispID, ok := taskDispatches[task.ID]; ok {
		// Mapping exists — only match if IDs agree. Don't fall through
		// to name matching, which could false-positive on title collisions.
		return dispID == d.ID
	}
	// No mapping stored — fall back to name matching.
	if d.Name != nil && *d.Name == task.Title {
		return true
	}
	// Legacy fallback: ic maps name to agent field in some responses.
	if d.Agent == task.Title {
		return true
	}
	return false
}

func taskStatusIcon(status autarch.TaskStatus) string {
	switch status {
	case autarch.TaskStatusPending:
		return pkgtui.StatusIdle.Render("○")
	case autarch.TaskStatusRunning:
		return pkgtui.StatusRunning.Render("●")
	case autarch.TaskStatusBlocked:
		return pkgtui.StatusWaiting.Render("◐")
	case autarch.TaskStatusDone:
		return pkgtui.StatusRunning.Render("✓")
	default:
		return pkgtui.StatusIdle.Render("?")
	}
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
	return "↑/↓ navigate  d dispatch  ctrl+r refresh  ctrl+g model  tab focus  ctrl+b sidebar"
}

// epicTasks returns tasks belonging to the selected epic (via story membership).
func (v *ColdwineView) epicTasks() []autarch.Task {
	if v.selected < 0 || v.selected >= len(v.epics) {
		return nil
	}
	epicID := v.epics[v.selected].ID
	// Collect story IDs for this epic
	storyIDs := make(map[string]bool)
	for _, s := range v.stories {
		if s.EpicID == epicID {
			storyIDs[s.ID] = true
		}
	}
	// Filter tasks belonging to those stories
	var tasks []autarch.Task
	for _, t := range v.tasks {
		if storyIDs[t.StoryID] {
			tasks = append(tasks, t)
		}
	}
	return tasks
}

// persistDispatchResult persists the task status change and stores the dispatch
// result summary in Intercore state. Both operations are best-effort —
// failures are reported to the chat panel but don't block the UI.
func (v *ColdwineView) persistDispatchResult(task autarch.Task, d intercore.Dispatch, newStatus autarch.TaskStatus) tea.Cmd {
	client := v.client
	ic := v.iclient
	taskID := task.ID

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Persist task status to backend (Intermute or local source).
		updated := task
		updated.Status = newStatus
		_, err := client.UpdateTask(updated)
		if err != nil {
			return taskStatusPersistedMsg{taskID: taskID, status: newStatus, err: err}
		}

		// Store dispatch result summary in Intercore state (best-effort).
		if ic != nil {
			summary := d.ResultSummary()
			_ = ic.StateSet(ctx, "task.dispatch_summary", taskID, fmt.Sprintf("%q", summary))
		}

		return taskStatusPersistedMsg{taskID: taskID, status: newStatus}
	}
}

// dispatchSelectedTask creates a dispatch for the currently selected task.
func (v *ColdwineView) dispatchSelectedTask() tea.Cmd {
	if v.iclient == nil {
		return nil
	}
	epicTasks := v.epicTasks()
	if v.selectedTask < 0 || v.selectedTask >= len(epicTasks) {
		return nil
	}
	task := epicTasks[v.selectedTask]
	if task.Status == autarch.TaskStatusRunning || task.Status == autarch.TaskStatusDone {
		v.chatPanel.AddMessage("system", fmt.Sprintf("Task %q is already %s", task.Title, task.Status))
		return nil
	}

	// Find run ID for this epic
	var epicID string
	if v.selected >= 0 && v.selected < len(v.epics) {
		epicID = v.epics[v.selected].ID
	}
	runID := v.getEpicRunID(epicID)
	if runID == "" {
		v.chatPanel.AddMessage("system", "No sprint run for this epic — create one first")
		return nil
	}

	ic := v.iclient
	taskID := task.ID
	taskTitle := task.Title
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		dispatchID, err := ic.DispatchSpawn(ctx, runID,
			intercore.WithDispatchType("task"),
			intercore.WithDispatchName(taskTitle),
		)
		return taskDispatchedMsg{taskID: taskID, dispatchID: dispatchID, err: err}
	}
}

// Commands implements CommandProvider
func (v *ColdwineView) Commands() []tui.Command {
	cmds := []tui.Command{
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

	// Intercore commands only available when ic is connected.
	if v.iclient != nil {
		cmds = append(cmds, tui.Command{
			Name:        "Dispatch Task",
			Description: "Dispatch selected task to an agent via Intercore",
			Action:      func() tea.Cmd { return v.dispatchSelectedTask() },
		})
		cmds = append(cmds, tui.Command{
			Name:        "Create Sprint",
			Description: "Create an Intercore sprint from selected epic",
			Action: func() tea.Cmd {
				ic := v.iclient
				var epicID, goal string
				if v.selected >= 0 && v.selected < len(v.epics) {
					epicID = v.epics[v.selected].ID
					goal = v.epics[v.selected].Title
				}
				if goal == "" {
					goal = "Untitled sprint"
				}
				return func() tea.Msg {
					ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
					defer cancel()
					runID, err := ic.RunCreate(ctx, ".", goal,
						intercore.WithScopeID(epicID),
					)
					return sprintCreatedMsg{runID: runID, epicID: epicID, goal: goal, err: err}
				}
			},
		})
	}

	return cmds
}
