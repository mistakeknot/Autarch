// Test harness for onboarding flow
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/autarch/internal/coldwine/epics"
	"github.com/mistakeknot/autarch/internal/coldwine/tasks"
	"github.com/mistakeknot/autarch/internal/pollard/research"
	"github.com/mistakeknot/autarch/internal/tui"
	"github.com/mistakeknot/autarch/internal/tui/views"
	"github.com/mistakeknot/autarch/pkg/autarch"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: testui <view>")
		fmt.Println()
		fmt.Println("Individual views (for debugging):")
		fmt.Println("  kickoff       - New project prompt")
		fmt.Println("  epic-review   - Epic review with mock data")
		fmt.Println("  task-review   - Task review with mock data")
		fmt.Println("  task-detail   - Task detail with mock data")
		fmt.Println()
		fmt.Println("Full flows:")
		fmt.Println("  flow          - Complete onboarding flow (kickoff → epics → tasks)")
		os.Exit(1)
	}

	var err error
	switch os.Args[1] {
	case "kickoff":
		err = runKickoff()
	case "epic-review":
		err = runEpicReview()
	case "task-review":
		err = runTaskReview()
	case "task-detail":
		err = runTaskDetail()
	case "flow":
		err = runFullFlow()
	default:
		fmt.Printf("Unknown view: %s\n", os.Args[1])
		os.Exit(1)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// viewWrapper wraps a tui.View to implement tea.Model
type viewWrapper struct {
	view   tui.View
	width  int
	height int
}

func (w *viewWrapper) Init() tea.Cmd {
	return w.view.Init()
}

func (w *viewWrapper) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		w.width = msg.Width
		w.height = msg.Height
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return w, tea.Quit
		}
	}

	newView, cmd := w.view.Update(msg)
	w.view = newView
	return w, cmd
}

func (w *viewWrapper) View() string {
	return w.view.View() + "\n\n(press ctrl+c to quit)"
}

func wrap(v tui.View) tea.Model {
	return &viewWrapper{view: v}
}

func runKickoff() error {
	view := views.NewKickoffView()
	view.SetProjectStartCallback(func(project *views.Project) tea.Cmd {
		return tea.Printf("Project created: %s (%s)", project.Name, project.ID)
	})
	p := tea.NewProgram(wrap(view), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func runEpicReview() error {
	mockEpics := []epics.EpicProposal{
		{
			ID:          "EPIC-001",
			Title:       "User Authentication System",
			Description: "Implement secure user authentication with OAuth and JWT",
			Size:        epics.SizeLarge,
			Priority:    epics.PriorityP1,
			TaskCount:   5,
			Stories: []epics.StoryProposal{
				{ID: "STORY-001", Title: "Login form", Size: epics.SizeMedium},
				{ID: "STORY-002", Title: "OAuth integration", Size: epics.SizeLarge},
			},
		},
		{
			ID:           "EPIC-002",
			Title:        "Dashboard UI",
			Description:  "Build the main dashboard with widgets and charts",
			Size:         epics.SizeMedium,
			Priority:     epics.PriorityP2,
			TaskCount:    3,
			Dependencies: []string{"EPIC-001"},
		},
		{
			ID:          "EPIC-003",
			Title:       "API Documentation",
			Description: "Generate and publish API documentation",
			Size:        epics.SizeSmall,
			Priority:    epics.PriorityP3,
			TaskCount:   2,
		},
	}
	view := views.NewEpicReviewView(mockEpics)
	view.SetCallbacks(
		func(accepted []epics.EpicProposal) tea.Cmd {
			return tea.Printf("Accepted %d epics!", len(accepted))
		},
		nil,
		func() tea.Cmd {
			return tea.Println("Back pressed")
		},
	)
	p := tea.NewProgram(wrap(view), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func runTaskReview() error {
	mockTasks := []tasks.TaskProposal{
		{
			ID:          "TASK-001",
			EpicID:      "EPIC-001",
			Title:       "Setup authentication middleware",
			Description: "Create Express middleware for JWT validation",
			Type:        tasks.TaskTypeSetup,
			Priority:    epics.PriorityP1,
			Ready:       true,
		},
		{
			ID:           "TASK-002",
			EpicID:       "EPIC-001",
			StoryID:      "STORY-001",
			Title:        "Implement login form component",
			Description:  "React component with email/password fields",
			Type:         tasks.TaskTypeImplementation,
			Priority:     epics.PriorityP1,
			Dependencies: []string{"TASK-001"},
			Ready:        false,
		},
		{
			ID:           "TASK-003",
			EpicID:       "EPIC-001",
			Title:        "Write authentication tests",
			Description:  "Unit and integration tests for auth flow",
			Type:         tasks.TaskTypeTest,
			Priority:     epics.PriorityP1,
			Dependencies: []string{"TASK-002"},
			Ready:        false,
		},
		{
			ID:          "TASK-004",
			EpicID:      "EPIC-002",
			Title:       "Create dashboard layout",
			Description: "Responsive grid layout for widgets",
			Type:        tasks.TaskTypeImplementation,
			Priority:    epics.PriorityP2,
			Ready:       true,
		},
	}
	view := views.NewTaskReviewView(mockTasks)
	view.SetAcceptCallback(func(accepted []tasks.TaskProposal) tea.Cmd {
		return tea.Printf("Accepted %d tasks!", len(accepted))
	})
	view.SetBackCallback(func() tea.Cmd {
		return tea.Println("Back pressed")
	})
	p := tea.NewProgram(wrap(view), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func runTaskDetail() error {
	mockTask := tasks.TaskProposal{
		ID:           "TASK-001",
		EpicID:       "EPIC-001",
		StoryID:      "STORY-001",
		Title:        "Implement OAuth 2.0 integration",
		Description:  "Add Google and GitHub OAuth providers using Passport.js",
		Type:         tasks.TaskTypeImplementation,
		Priority:     epics.PriorityP1,
		Dependencies: []string{"TASK-000"},
		Ready:        true,
	}
	coordinator := research.NewCoordinator(nil)
	view := views.NewTaskDetailView(mockTask, coordinator)
	view.SetCallbacks(
		func(t tasks.TaskProposal, agent views.AgentType, worktree bool) tea.Cmd {
			return tea.Printf("Starting %s with agent=%s worktree=%v", t.ID, agent, worktree)
		},
		func() tea.Cmd {
			return tea.Println("Back pressed")
		},
	)
	p := tea.NewProgram(wrap(view), tea.WithAltScreen())
	_, err := p.Run()
	return err
}

func runFullFlow() error {
	// Create a mock client (won't connect to anything real)
	client := autarch.NewClient("http://localhost:7338")

	// Create unified app with onboarding flow
	app := tui.NewUnifiedApp(client)

	// Set up view factories for state transitions
	app.SetViewFactories(
		// Kickoff view factory
		func() tui.View {
			v := views.NewKickoffView()
			v.SetProjectStartCallback(func(project *views.Project) tea.Cmd {
				return func() tea.Msg {
					return tui.ProjectCreatedMsg{
						ProjectID:   project.ID,
						ProjectName: project.Name,
						Description: project.Description,
					}
				}
			})
			return v
		},
		// Spec summary view factory
		func(spec *tui.SpecSummary, coord *research.Coordinator) tui.View {
			return views.NewSpecSummaryView(spec, coord)
		},
		// Epic review view factory
		func(proposals []epics.EpicProposal) tui.View {
			v := views.NewEpicReviewView(proposals)
			v.SetCallbacks(
				func(accepted []epics.EpicProposal) tea.Cmd {
					return func() tea.Msg {
						return tui.EpicsAcceptedMsg{Epics: accepted}
					}
				},
				nil,
				func() tea.Cmd {
					return func() tea.Msg {
						return tui.NavigateBackMsg{}
					}
				},
			)
			return v
		},
		// Task review view factory
		func(taskList []tasks.TaskProposal) tui.View {
			v := views.NewTaskReviewView(taskList)
			v.SetAcceptCallback(func(accepted []tasks.TaskProposal) tea.Cmd {
				return func() tea.Msg {
					return tui.TasksAcceptedMsg{Tasks: accepted}
				}
			})
			v.SetBackCallback(func() tea.Cmd {
				return func() tea.Msg {
					return tui.NavigateBackMsg{}
				}
			})
			return v
		},
		// Task detail view factory
		func(task tasks.TaskProposal, coord *research.Coordinator) tui.View {
			v := views.NewTaskDetailView(task, coord)
			v.SetCallbacks(
				func(t tasks.TaskProposal, agent views.AgentType, worktree bool) tea.Cmd {
					return func() tea.Msg {
						return tui.StartAgentMsg{
							Task:     t,
							Agent:    string(agent),
							Worktree: worktree,
						}
					}
				},
				func() tea.Cmd {
					return func() tea.Msg {
						return tui.NavigateBackMsg{}
					}
				},
			)
			return v
		},
		// Dashboard views factory
		func(c *autarch.Client) []tui.View {
			return []tui.View{
				views.NewBigendView(c),
				views.NewGurgehView(c),
				views.NewColdwineView(c),
				views.NewPollardView(c),
			}
		},
	)

	return tui.RunUnified(client, app)
}
