package shell

import (
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/mistakeknot/vauxpraudemonium/pkg/tui"
)

// Project represents a discovered project
type Project struct {
	Path           string
	Name           string
	HasPraude      bool
	HasTandemonium bool
	HasPollard     bool
	TaskStats      *TaskStats
}

// TaskStats holds task statistics for a project
type TaskStats struct {
	Todo       int
	InProgress int
	Done       int
}

// projectItem implements list.Item for the projects list
type projectItem struct {
	project Project
}

func (i projectItem) Title() string { return i.project.Name }
func (i projectItem) Description() string {
	if i.project.TaskStats != nil {
		return fmt.Sprintf("📋 %d/%d/%d",
			i.project.TaskStats.Todo,
			i.project.TaskStats.InProgress,
			i.project.TaskStats.Done)
	}
	return ""
}
func (i projectItem) FilterValue() string { return i.project.Name + " " + i.project.Path }

// ProjectsPane manages the projects list
type ProjectsPane struct {
	list     list.Model
	projects []Project
	focused  bool
}

// NewProjectsPane creates a new projects pane
func NewProjectsPane() *ProjectsPane {
	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = tui.SelectedStyle
	delegate.Styles.NormalTitle = tui.UnselectedStyle

	l := list.New([]list.Item{}, delegate, 0, 0)
	l.Title = "Projects"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)

	return &ProjectsPane{
		list: l,
	}
}

// SetSize updates the list dimensions
func (p *ProjectsPane) SetSize(width, height int) {
	p.list.SetSize(width, height)
}

// SetFocused sets the focus state
func (p *ProjectsPane) SetFocused(focused bool) {
	p.focused = focused
}

// IsFocused returns the focus state
func (p *ProjectsPane) IsFocused() bool {
	return p.focused
}

// SetProjects updates the projects list
func (p *ProjectsPane) SetProjects(projects []Project) {
	p.projects = projects

	// Keep current selection if possible
	currentPath := ""
	if item, ok := p.list.SelectedItem().(projectItem); ok {
		currentPath = item.project.Path
	}

	items := make([]list.Item, 0, len(projects)+1)
	// Add "All Projects" option
	items = append(items, projectItem{
		project: Project{Path: "", Name: "All Projects"},
	})
	for _, project := range projects {
		items = append(items, projectItem{project: project})
	}
	p.list.SetItems(items)

	// Restore selection
	if currentPath != "" {
		p.SelectPath(currentPath)
	}
}

// SelectPath selects the project with the given path
func (p *ProjectsPane) SelectPath(path string) {
	items := p.list.Items()
	for i, item := range items {
		if pi, ok := item.(projectItem); ok && pi.project.Path == path {
			p.list.Select(i)
			return
		}
	}
}

// SelectedProject returns the currently selected project
func (p *ProjectsPane) SelectedProject() (Project, bool) {
	item, ok := p.list.SelectedItem().(projectItem)
	if !ok {
		return Project{}, false
	}
	return item.project, true
}

// Update handles messages
func (p *ProjectsPane) Update(msg tea.Msg) (*ProjectsPane, tea.Cmd) {
	var cmd tea.Cmd
	p.list, cmd = p.list.Update(msg)
	return p, cmd
}

// View renders the projects pane
func (p *ProjectsPane) View() string {
	return p.list.View()
}

// DiscoverProjects finds projects in the given directories
func DiscoverProjects(paths []string) []Project {
	var projects []Project
	for _, path := range paths {
		project := Project{
			Path: path,
			Name: filepath.Base(path),
		}
		// TODO: Check for .praude, .tandemonium, .pollard directories
		projects = append(projects, project)
	}
	return projects
}
