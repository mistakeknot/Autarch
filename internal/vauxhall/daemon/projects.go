package daemon

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
	}

	// Check for .tandemonium directory
	if _, err := os.Stat(filepath.Join(path, ".tandemonium")); err == nil {
		project.HasTandemonium = true
		project.TaskStats = m.loadTaskStats(path)
	}

	// Check for .pollard directory
	if _, err := os.Stat(filepath.Join(path, ".pollard")); err == nil {
		project.HasPollard = true
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

// Refresh rescans all project directories
func (m *ProjectManager) Refresh() {
	m.Discover()
}
