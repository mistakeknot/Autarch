package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/list"

	"github.com/mistakeknot/autarch/internal/bigend/aggregator"
	"github.com/mistakeknot/autarch/internal/bigend/mcp"
	"github.com/mistakeknot/autarch/internal/icdata"
)

// SessionItem represents a session in the list
type SessionItem struct {
	Session   aggregator.TmuxSession
	Status    icdata.UnifiedStatus
	AgentType string
}

func (i SessionItem) Title() string {
	name := i.Session.Name
	if i.Session.AgentName != "" {
		name = i.Session.AgentName
	}
	return name
}

func (i SessionItem) Description() string {
	parts := []string{}
	if i.Session.ProjectPath != "" {
		parts = append(parts, filepath.Base(i.Session.ProjectPath))
	}
	if i.Session.AgentType != "" {
		parts = append(parts, i.Session.AgentType)
	}
	parts = append(parts, i.Status.String())
	return strings.Join(parts, " • ")
}

func (i SessionItem) FilterValue() string {
	return i.Session.Name + " " + i.Session.ProjectPath
}

// ProjectItem represents a project in the list
type ProjectItem struct {
	Path         string
	Name         string
	HasColdwine  bool
	RunCount     int
	BlockedCount int
	KernelError  string
	TaskStats    *struct {
		Todo       int
		InProgress int
		Done       int
	}
}

func (i ProjectItem) Title() string {
	name := i.Name
	if i.KernelError != "" {
		name = "! " + name
	}
	if i.BlockedCount > 0 {
		name = fmt.Sprintf("%s %s", name,
			StatusError.Render(fmt.Sprintf("[%d blocked]", i.BlockedCount)))
	} else if i.RunCount > 0 {
		name = fmt.Sprintf("%s [%d]", name, i.RunCount)
	}
	return name
}
func (i ProjectItem) Description() string {
	if i.TaskStats != nil {
		return fmt.Sprintf("%d todo, %d in progress, %d done", i.TaskStats.Todo, i.TaskStats.InProgress, i.TaskStats.Done)
	}
	return i.Path
}
func (i ProjectItem) FilterValue() string { return i.Name + " " + i.Path }

// GroupHeaderItem represents a grouped header in session/agent lists.
type GroupHeaderItem struct {
	ProjectPath string
	Name        string
	Count       int
	Expanded    bool
}

func (i GroupHeaderItem) Title() string {
	if i.Count > 0 {
		return fmt.Sprintf("%s (%d)", i.Name, i.Count)
	}
	return i.Name
}

func (i GroupHeaderItem) Description() string { return "" }
func (i GroupHeaderItem) FilterValue() string { return i.Name + " " + i.ProjectPath }

// AgentItem represents an agent in the list
type AgentItem struct {
	Agent aggregator.Agent
}

func (i AgentItem) Title() string { return i.Agent.Name }
func (i AgentItem) Description() string {
	parts := []string{i.Agent.Program, i.Agent.Model}
	if i.Agent.UnreadCount > 0 {
		parts = append(parts, fmt.Sprintf("📬 %d unread", i.Agent.UnreadCount))
	}
	return strings.Join(parts, " • ")
}
func (i AgentItem) FilterValue() string { return i.Agent.Name + " " + i.Agent.Program }

// MCPItem represents an MCP component status in the list
type MCPItem struct {
	Status mcp.ComponentStatus
}

func (i MCPItem) Title() string       { return i.Status.Component }
func (i MCPItem) Description() string { return string(i.Status.Status) }
func (i MCPItem) FilterValue() string { return i.Status.Component }

// groupSessionItemsByProject groups session items under project headers.
func (m *Model) groupSessionItemsByProject(items []list.Item) []list.Item {
	if len(items) == 0 {
		return items
	}
	grouped := map[string][]SessionItem{}
	for _, item := range items {
		session, ok := item.(SessionItem)
		if !ok {
			continue
		}
		grouped[session.Session.ProjectPath] = append(grouped[session.Session.ProjectPath], session)
	}
	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]list.Item, 0, len(items)+len(keys))
	for _, key := range keys {
		name := filepath.Base(key)
		if key == "" {
			name = "Unassigned"
		}
		groupItems := grouped[key]
		expanded := m.isGroupExpanded(TabSessions, key)
		out = append(out, GroupHeaderItem{
			ProjectPath: key,
			Name:        name,
			Count:       len(groupItems),
			Expanded:    expanded,
		})
		if expanded {
			for _, session := range groupItems {
				out = append(out, session)
			}
		}
	}
	return out
}

func (m *Model) isGroupExpanded(tab Tab, projectPath string) bool {
	if m.groupExpanded == nil {
		m.groupExpanded = map[string]bool{}
	}
	key := groupKey(tab, projectPath)
	expanded, ok := m.groupExpanded[key]
	if !ok {
		return true
	}
	return expanded
}

func (m *Model) toggleGroup(tab Tab, projectPath string) {
	if m.groupExpanded == nil {
		m.groupExpanded = map[string]bool{}
	}
	key := groupKey(tab, projectPath)
	current := m.groupExpanded[key]
	if !current {
		m.groupExpanded[key] = true
		return
	}
	m.groupExpanded[key] = false
}

func groupKey(tab Tab, projectPath string) string {
	prefix := "sessions"
	if tab == TabAgents {
		prefix = "agents"
	}
	return prefix + ":" + projectPath
}

// groupAgentItemsByProject groups agent items under project headers.
func (m *Model) groupAgentItemsByProject(items []list.Item) []list.Item {
	if len(items) == 0 {
		return items
	}
	grouped := map[string][]AgentItem{}
	for _, item := range items {
		agent, ok := item.(AgentItem)
		if !ok {
			continue
		}
		grouped[agent.Agent.ProjectPath] = append(grouped[agent.Agent.ProjectPath], agent)
	}
	keys := make([]string, 0, len(grouped))
	for key := range grouped {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]list.Item, 0, len(items)+len(keys))
	for _, key := range keys {
		name := filepath.Base(key)
		if key == "" {
			name = "Unassigned"
		}
		groupItems := grouped[key]
		expanded := m.isGroupExpanded(TabAgents, key)
		out = append(out, GroupHeaderItem{
			ProjectPath: key,
			Name:        name,
			Count:       len(groupItems),
			Expanded:    expanded,
		})
		if expanded {
			for _, agent := range groupItems {
				out = append(out, agent)
			}
		}
	}
	return out
}
