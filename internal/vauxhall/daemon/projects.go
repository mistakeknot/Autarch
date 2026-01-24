package daemon

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	praudeSpecs "github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
)

// ProjectManager manages discovered projects
type ProjectManager struct {
	dirs     []string
	projects map[string]*Project
	mu       sync.RWMutex
}

// NewProjectManager creates a new project manager
func NewProjectManager(dirs []string) *ProjectManager {
	m := &ProjectManager{
		dirs:     dirs,
		projects: make(map[string]*Project),
	}
	m.Discover()
	return m
}

// List returns all discovered projects
func (m *ProjectManager) List() []*Project {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*Project, 0, len(m.projects))
	for _, p := range m.projects {
		result = append(result, p)
	}
	return result
}

// Get returns a project by path
func (m *ProjectManager) Get(path string) (*Project, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.projects[path]
	return p, ok
}

// Count returns the number of projects
func (m *ProjectManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.projects)
}

// GetTasks returns tasks for a project from its .tandemonium directory
func (m *ProjectManager) GetTasks(path string) ([]map[string]interface{}, error) {
	m.mu.RLock()
	project, ok := m.projects[path]
	m.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("project %q not found", path)
	}

	if !project.HasTandemonium {
		return []map[string]interface{}{}, nil
	}

	// TODO: Load tasks from .tandemonium/state.db
	return []map[string]interface{}{}, nil
}

// Discover scans directories for projects
func (m *ProjectManager) Discover() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, dir := range m.dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if entry.Name()[0] == '.' {
				continue // Skip hidden directories
			}

			projectPath := filepath.Join(dir, entry.Name())
			project := m.scanProject(projectPath)
			m.projects[projectPath] = project
		}
	}
}

// scanProject scans a directory for project metadata
func (m *ProjectManager) scanProject(path string) *Project {
	project := &Project{
		Path: path,
		Name: filepath.Base(path),
	}

	// Check for .praude directory
	if _, err := os.Stat(filepath.Join(path, ".praude")); err == nil {
		project.HasPraude = true
		project.PraudeStats = m.loadPraudeStats(path)
	}

	// Check for .tandemonium directory
	if _, err := os.Stat(filepath.Join(path, ".tandemonium")); err == nil {
		project.HasTandemonium = true
		project.TaskStats = m.loadTaskStats(path)
	}

	// Check for .pollard directory
	if _, err := os.Stat(filepath.Join(path, ".pollard")); err == nil {
		project.HasPollard = true
		project.PollardStats = m.loadPollardStats(path)
	}

	return project
}

// loadTaskStats loads task statistics from .tandemonium
func (m *ProjectManager) loadTaskStats(path string) *TaskStats {
	// TODO: Query .tandemonium/state.db for actual stats
	return &TaskStats{
		Todo:       0,
		InProgress: 0,
		Done:       0,
	}
}

// loadPraudeStats loads PRD statistics from .praude/specs
func (m *ProjectManager) loadPraudeStats(path string) *PraudeStats {
	praudeDir := filepath.Join(path, ".praude", "specs")
	summaries, _ := praudeSpecs.LoadSummaries(praudeDir)

	stats := &PraudeStats{}
	for _, s := range summaries {
		stats.Total++
		switch strings.ToLower(s.Status) {
		case "draft", "":
			stats.Draft++
		case "active", "in_progress", "approved":
			stats.Active++
		case "done", "complete":
			stats.Done++
		default:
			stats.Draft++
		}
	}
	return stats
}

// loadPollardStats loads research statistics from .pollard
func (m *ProjectManager) loadPollardStats(path string) *PollardStats {
	pollardPath := filepath.Join(path, ".pollard")

	// Count sources
	sourcesDir := filepath.Join(pollardPath, "sources")
	sourceCount := countYAMLFilesRecursive(sourcesDir)

	// Count insights
	insightsDir := filepath.Join(pollardPath, "insights")
	insightCount := countYAMLFilesRecursive(insightsDir)

	// Count reports and find latest
	reportsDir := filepath.Join(pollardPath, "reports")
	reportCount, lastReport := countReportsAndFindLatest(reportsDir)

	return &PollardStats{
		Sources:    sourceCount,
		Insights:   insightCount,
		Reports:    reportCount,
		LastReport: lastReport,
	}
}

// countYAMLFilesRecursive counts YAML files recursively in a directory
func countYAMLFilesRecursive(dir string) int {
	count := 0
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && (strings.HasSuffix(d.Name(), ".yaml") || strings.HasSuffix(d.Name(), ".yml")) {
			count++
		}
		return nil
	})
	return count
}

// countReportsAndFindLatest counts report files and finds the most recent
func countReportsAndFindLatest(dir string) (int, string) {
	count := 0
	var latestPath string
	var latestTime time.Time

	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			count++
			info, err := d.Info()
			if err == nil && info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
				latestPath = path
			}
		}
		return nil
	})

	if latestPath != "" {
		return count, filepath.Base(latestPath)
	}
	return count, ""
}

// Refresh rescans all project directories
func (m *ProjectManager) Refresh() {
	m.Discover()
}
