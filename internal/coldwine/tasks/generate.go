// Package tasks provides task generation from epics.
package tasks

import (
	"fmt"
	"strings"

	"github.com/mistakeknot/autarch/internal/coldwine/epics"
)

// TaskProposal represents a proposed task within an epic.
type TaskProposal struct {
	ID           string         `yaml:"id"`
	EpicID       string         `yaml:"epic_id"`
	StoryID      string         `yaml:"story_id,omitempty"`
	Title        string         `yaml:"title"`
	Description  string         `yaml:"description"`
	Type         TaskType       `yaml:"type"`
	Priority     epics.Priority `yaml:"priority"`
	Dependencies []string       `yaml:"dependencies,omitempty"`
	Ready        bool           `yaml:"-"` // Computed: no blockers
	Edited       bool           `yaml:"-"` // User has modified
}

// TaskType categorizes tasks.
type TaskType string

const (
	TaskTypeImplementation TaskType = "implementation"
	TaskTypeTest           TaskType = "test"
	TaskTypeDocumentation  TaskType = "documentation"
	TaskTypeReview         TaskType = "review"
	TaskTypeSetup          TaskType = "setup"
	TaskTypeResearch       TaskType = "research"
)

// Generator creates task proposals from epics.
type Generator struct{}

// NewGenerator creates a new task generator.
func NewGenerator() *Generator {
	return &Generator{}
}

// GenerateFromEpics creates tasks from a list of epic proposals.
func (g *Generator) GenerateFromEpics(epicProposals []epics.EpicProposal) ([]TaskProposal, error) {
	var allTasks []TaskProposal
	taskNum := 1

	for _, ep := range epicProposals {
		tasks := g.generateForEpic(ep, &taskNum)
		allTasks = append(allTasks, tasks...)
	}

	// Compute readiness
	g.computeReadiness(allTasks)

	return allTasks, nil
}

// generateForEpic creates tasks for a single epic.
func (g *Generator) generateForEpic(ep epics.EpicProposal, taskNum *int) []TaskProposal {
	var tasks []TaskProposal

	// First, add setup task if this is a foundational epic
	if g.isFoundational(ep) {
		setupTask := TaskProposal{
			ID:          fmt.Sprintf("TASK-%03d", *taskNum),
			EpicID:      ep.ID,
			Title:       fmt.Sprintf("Setup for %s", ep.Title),
			Description: fmt.Sprintf("Initial setup and configuration for %s", ep.Title),
			Type:        TaskTypeSetup,
			Priority:    ep.Priority,
		}
		tasks = append(tasks, setupTask)
		*taskNum++
	}

	// Generate tasks from stories
	if len(ep.Stories) > 0 {
		for _, story := range ep.Stories {
			storyTasks := g.generateForStory(ep, story, taskNum)
			tasks = append(tasks, storyTasks...)
		}
	} else {
		// No stories - generate tasks directly from epic
		implTask := TaskProposal{
			ID:          fmt.Sprintf("TASK-%03d", *taskNum),
			EpicID:      ep.ID,
			Title:       fmt.Sprintf("Implement %s", ep.Title),
			Description: ep.Description,
			Type:        TaskTypeImplementation,
			Priority:    ep.Priority,
		}
		tasks = append(tasks, implTask)
		*taskNum++
	}

	// Add test task for the epic
	testTask := TaskProposal{
		ID:          fmt.Sprintf("TASK-%03d", *taskNum),
		EpicID:      ep.ID,
		Title:       fmt.Sprintf("Test %s", ep.Title),
		Description: fmt.Sprintf("Write tests for %s functionality", ep.Title),
		Type:        TaskTypeTest,
		Priority:    ep.Priority,
	}
	// Test depends on implementation tasks
	for _, t := range tasks {
		if t.Type == TaskTypeImplementation {
			testTask.Dependencies = append(testTask.Dependencies, t.ID)
		}
	}
	tasks = append(tasks, testTask)
	*taskNum++

	// Add dependencies from epic dependencies
	if len(ep.Dependencies) > 0 {
		// First implementation task in this epic depends on other epics being done
		for i := range tasks {
			if tasks[i].Type == TaskTypeImplementation || tasks[i].Type == TaskTypeSetup {
				for _, depEpic := range ep.Dependencies {
					// Add a placeholder dependency - will be resolved later
					tasks[i].Dependencies = append(tasks[i].Dependencies, fmt.Sprintf("%s:*", depEpic))
				}
				break
			}
		}
	}

	return tasks
}

// generateForStory creates tasks for a single story.
func (g *Generator) generateForStory(ep epics.EpicProposal, story epics.StoryProposal, taskNum *int) []TaskProposal {
	var tasks []TaskProposal

	// Main implementation task
	implTask := TaskProposal{
		ID:          fmt.Sprintf("TASK-%03d", *taskNum),
		EpicID:      ep.ID,
		StoryID:     story.ID,
		Title:       story.Title,
		Description: story.Description,
		Type:        TaskTypeImplementation,
		Priority:    ep.Priority,
	}
	tasks = append(tasks, implTask)
	*taskNum++

	// For larger stories, add additional tasks
	if story.Size == epics.SizeLarge || story.Size == epics.SizeXLarge {
		// Research task
		researchTask := TaskProposal{
			ID:          fmt.Sprintf("TASK-%03d", *taskNum),
			EpicID:      ep.ID,
			StoryID:     story.ID,
			Title:       fmt.Sprintf("Research: %s", g.shortenTitle(story.Title)),
			Description: fmt.Sprintf("Research implementation approach for: %s", story.Title),
			Type:        TaskTypeResearch,
			Priority:    ep.Priority,
		}
		tasks = append(tasks, researchTask)
		*taskNum++

		// Make implementation depend on research
		implTask.Dependencies = append(implTask.Dependencies, researchTask.ID)
	}

	return tasks
}

// isFoundational returns true if this epic should have a setup task.
func (g *Generator) isFoundational(ep epics.EpicProposal) bool {
	titleLower := strings.ToLower(ep.Title)
	foundationalTerms := []string{"foundation", "infrastructure", "setup", "auth", "core"}
	for _, term := range foundationalTerms {
		if strings.Contains(titleLower, term) {
			return true
		}
	}
	return ep.Priority == epics.PriorityP0 || ep.Priority == epics.PriorityP1
}

// shortenTitle creates a shorter version of a title.
func (g *Generator) shortenTitle(title string) string {
	if len(title) <= 30 {
		return title
	}
	return title[:27] + "..."
}

// computeReadiness marks tasks as ready if they have no unresolved dependencies.
func (g *Generator) computeReadiness(tasks []TaskProposal) {
	taskIDs := make(map[string]bool)
	for _, t := range tasks {
		taskIDs[t.ID] = true
	}

	for i := range tasks {
		tasks[i].Ready = true
		for _, dep := range tasks[i].Dependencies {
			// Check if dependency exists
			if !taskIDs[dep] && !strings.Contains(dep, ":*") {
				tasks[i].Ready = false
				break
			}
		}
	}
}

// GroupByEpic groups tasks by their epic ID.
func GroupByEpic(tasks []TaskProposal) map[string][]TaskProposal {
	groups := make(map[string][]TaskProposal)
	for _, t := range tasks {
		groups[t.EpicID] = append(groups[t.EpicID], t)
	}
	return groups
}

// CountReady returns the number of ready tasks.
func CountReady(tasks []TaskProposal) int {
	count := 0
	for _, t := range tasks {
		if t.Ready {
			count++
		}
	}
	return count
}

// GetReadyTasks returns only the tasks that are ready to start.
func GetReadyTasks(tasks []TaskProposal) []TaskProposal {
	var ready []TaskProposal
	for _, t := range tasks {
		if t.Ready {
			ready = append(ready, t)
		}
	}
	return ready
}

// BuildDependencyGraph creates a map of task ID to tasks that depend on it.
func BuildDependencyGraph(tasks []TaskProposal) map[string][]string {
	graph := make(map[string][]string)
	for _, t := range tasks {
		for _, dep := range t.Dependencies {
			if !strings.Contains(dep, ":*") { // Skip cross-epic deps for now
				graph[dep] = append(graph[dep], t.ID)
			}
		}
	}
	return graph
}

// ResolveCrossEpicDependencies replaces "EPIC-xxx:*" placeholders with actual task IDs.
func ResolveCrossEpicDependencies(tasks []TaskProposal) {
	// Find last task of each epic
	lastTaskOfEpic := make(map[string]string)
	for _, t := range tasks {
		lastTaskOfEpic[t.EpicID] = t.ID
	}

	// Resolve placeholders
	for i := range tasks {
		var resolvedDeps []string
		for _, dep := range tasks[i].Dependencies {
			if strings.HasSuffix(dep, ":*") {
				epicID := strings.TrimSuffix(dep, ":*")
				if lastTask, ok := lastTaskOfEpic[epicID]; ok {
					resolvedDeps = append(resolvedDeps, lastTask)
				}
			} else {
				resolvedDeps = append(resolvedDeps, dep)
			}
		}
		tasks[i].Dependencies = resolvedDeps
	}

	// Recompute readiness after resolution
	taskIDs := make(map[string]bool)
	for _, t := range tasks {
		taskIDs[t.ID] = true
	}

	for i := range tasks {
		tasks[i].Ready = true
		for _, dep := range tasks[i].Dependencies {
			if !taskIDs[dep] {
				tasks[i].Ready = false
				break
			}
		}
	}
}
