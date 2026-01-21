package discovery

import (
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/mistakeknot/vauxpraudemonium/internal/vauxhall/config"
)

// Project represents a discovered project with tooling
type Project struct {
	Path           string     `json:"path"`
	Name           string     `json:"name"`
	HasPraude      bool       `json:"has_praude"`
	HasTandemonium bool       `json:"has_tandemonium"`
	HasAgentMail   bool       `json:"has_agent_mail"`
	TaskStats      *TaskStats `json:"task_stats,omitempty"`
}

// TaskStats holds task count statistics for a project
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
	if s == nil || s.Total == 0 {
		return 0
	}
	return (s.Done * 100) / s.Total
}

// ActiveCount returns the number of tasks being worked on
func (s *TaskStats) ActiveCount() int {
	if s == nil {
		return 0
	}
	return s.InProgress + s.Review
}

// Scanner discovers projects in configured roots
type Scanner struct {
	cfg config.DiscoveryConfig
}

// NewScanner creates a new project scanner
func NewScanner(cfg config.DiscoveryConfig) *Scanner {
	return &Scanner{cfg: cfg}
}

// Scan finds all projects with Praude or Tandemonium tooling
func (s *Scanner) Scan() ([]Project, error) {
	var projects []Project
	seen := make(map[string]bool)

	for _, root := range s.cfg.ScanRoots {
		root = expandHome(root)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			slog.Warn("scan root does not exist", "path", root)
			continue
		}

		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // skip errors
			}

			// Skip excluded patterns
			for _, pattern := range s.cfg.ExcludePatterns {
				if d.IsDir() && d.Name() == pattern {
					return filepath.SkipDir
				}
			}

			// Check for tooling directories
			if d.IsDir() && (d.Name() == ".praude" || d.Name() == ".tandemonium" || d.Name() == ".agent_mail") {
				projectPath := filepath.Dir(path)
				if seen[projectPath] {
					return nil
				}
				seen[projectPath] = true

				project := s.examineProject(projectPath)
				if project.HasPraude || project.HasTandemonium {
					projects = append(projects, project)
					slog.Debug("discovered project", "path", projectPath, "praude", project.HasPraude, "tandemonium", project.HasTandemonium)
				}
				return filepath.SkipDir
			}

			// Do not descend too deep
			depth := strings.Count(strings.TrimPrefix(path, root), string(os.PathSeparator))
			if d.IsDir() && depth > 3 {
				return filepath.SkipDir
			}

			return nil
		})

		if err != nil {
			slog.Error("scan error", "root", root, "error", err)
		}
	}

	return projects, nil
}

// examineProject checks what tooling a project has
func (s *Scanner) examineProject(path string) Project {
	name := filepath.Base(path)

	project := Project{
		Path: path,
		Name: name,
	}

	// Check for .praude/
	if info, err := os.Stat(filepath.Join(path, ".praude")); err == nil && info.IsDir() {
		project.HasPraude = true
	}

	// Check for .tandemonium/
	if info, err := os.Stat(filepath.Join(path, ".tandemonium")); err == nil && info.IsDir() {
		project.HasTandemonium = true
	}

	// Check for .agent_mail/
	if info, err := os.Stat(filepath.Join(path, ".agent_mail")); err == nil && info.IsDir() {
		project.HasAgentMail = true
	}

	return project
}

func expandHome(path string) string {
	if len(path) > 0 && path[0] == '~' {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[1:])
	}
	return path
}
