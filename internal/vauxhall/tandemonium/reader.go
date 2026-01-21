package tandemonium

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Reader reads Tandemonium data from a project directory
type Reader struct {
	projectPath string
	tandoPath   string
}

// NewReader creates a reader for a project's .tandemonium directory
func NewReader(projectPath string) *Reader {
	return &Reader{
		projectPath: projectPath,
		tandoPath:   filepath.Join(projectPath, ".tandemonium"),
	}
}

// Exists checks if the .tandemonium directory exists
func (r *Reader) Exists() bool {
	info, err := os.Stat(r.tandoPath)
	return err == nil && info.IsDir()
}

// ReadTasks reads and parses tasks.yml
func (r *Reader) ReadTasks() ([]Task, error) {
	tasksPath := filepath.Join(r.tandoPath, "tasks.yml")

	data, err := os.ReadFile(tasksPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []Task{}, nil
		}
		return nil, fmt.Errorf("failed to read tasks.yml: %w", err)
	}

	var tasksFile TasksFile
	if err := yaml.Unmarshal(data, &tasksFile); err != nil {
		return nil, fmt.Errorf("failed to parse tasks.yml: %w", err)
	}

	slog.Debug("read tasks", "project", r.projectPath, "count", len(tasksFile.Data.Tasks))
	return tasksFile.Data.Tasks, nil
}

// ReadConfig reads and parses config.yml
func (r *Reader) ReadConfig() (*ConfigFile, error) {
	configPath := filepath.Join(r.tandoPath, "config.yml")

	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read config.yml: %w", err)
	}

	var config ConfigFile
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config.yml: %w", err)
	}

	return &config, nil
}

// GetTasksByStatus returns tasks grouped by status
func (r *Reader) GetTasksByStatus() (map[string][]Task, error) {
	tasks, err := r.ReadTasks()
	if err != nil {
		return nil, err
	}

	result := map[string][]Task{
		StatusTodo:       {},
		StatusInProgress: {},
		StatusReview:     {},
		StatusDone:       {},
		StatusBlocked:    {},
	}

	for _, task := range tasks {
		// Skip subtasks for the top-level view
		if task.ParentID != "" {
			continue
		}
		status := task.Status
		if _, ok := result[status]; !ok {
			status = StatusTodo // default unknown statuses to todo
		}
		result[status] = append(result[status], task)
	}

	return result, nil
}

// GetActiveTaskCount returns the number of in-progress and review tasks
func (r *Reader) GetActiveTaskCount() (int, error) {
	tasks, err := r.ReadTasks()
	if err != nil {
		return 0, err
	}

	count := 0
	for _, task := range tasks {
		if task.IsActive() && task.ParentID == "" {
			count++
		}
	}
	return count, nil
}

// GetTaskStats returns task statistics
func (r *Reader) GetTaskStats() (*TaskStats, error) {
	tasks, err := r.ReadTasks()
	if err != nil {
		return nil, err
	}

	stats := &TaskStats{}
	for _, task := range tasks {
		// Only count top-level tasks
		if task.ParentID != "" {
			continue
		}
		stats.Total++
		switch task.Status {
		case StatusTodo:
			stats.Todo++
		case StatusInProgress:
			stats.InProgress++
		case StatusReview:
			stats.Review++
		case StatusDone:
			stats.Done++
		case StatusBlocked:
			stats.Blocked++
		}
	}
	return stats, nil
}

// TaskStats holds task count statistics
type TaskStats struct {
	Total      int `json:"total"`
	Todo       int `json:"todo"`
	InProgress int `json:"in_progress"`
	Review     int `json:"review"`
	Done       int `json:"done"`
	Blocked    int `json:"blocked"`
}

// PercentDone returns the completion percentage
func (s *TaskStats) PercentDone() int {
	if s.Total == 0 {
		return 0
	}
	return (s.Done * 100) / s.Total
}
