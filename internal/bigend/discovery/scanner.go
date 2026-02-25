package discovery

import (
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/mistakeknot/autarch/internal/bigend/config"
)

// Project represents a discovered project with tooling
type Project struct {
	Path            string             `json:"path"`
	Name            string             `json:"name"`
	HasGurgeh       bool               `json:"has_gurgeh"`
	HasColdwine     bool               `json:"has_coldwine"`
	HasPollard      bool               `json:"has_pollard"`
	HasAgentMail    bool               `json:"has_agent_mail"`
	HasIntercore    bool               `json:"has_intercore"`
	HasInterspect   bool               `json:"has_interspect"`
	TaskStats       *TaskStats         `json:"task_stats,omitempty"`
	PollardStats    *PollardStats      `json:"pollard_stats,omitempty"`
	GurgStats       *GurgStats         `json:"gurg_stats,omitempty"`
	InterspectStats *InterspectStats   `json:"interspect_stats,omitempty"`
}

// InterspectStats holds profiler evidence statistics
type InterspectStats struct {
	TotalEvents int `json:"total_events"`
	Sessions    int `json:"sessions"`
	Dispatches  int `json:"dispatches"`
	Advances    int `json:"advances"`
	Blocks      int `json:"blocks"`
}

// PollardStats holds research data statistics
type PollardStats struct {
	Sources    int `json:"sources"`
	Insights   int `json:"insights"`
	Reports    int `json:"reports"`
	LastReport string `json:"last_report,omitempty"`
}

// GurgStats holds PRD statistics
type GurgStats struct {
	Total  int `json:"total"`
	Draft  int `json:"draft"`
	Active int `json:"active"`
	Done   int `json:"done"`
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

// Scan finds all projects with Gurgeh or Coldwine tooling
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

			// Check for tooling directories (including legacy .praude/.tandemonium)
			if d.IsDir() && (d.Name() == ".gurgeh" || d.Name() == ".praude" || d.Name() == ".coldwine" || d.Name() == ".tandemonium" || d.Name() == ".pollard" || d.Name() == ".agent_mail" || d.Name() == ".clavain") {
				projectPath := filepath.Dir(path)

				// Resolve symlinks to dedup projects
				if resolved, err := filepath.EvalSymlinks(projectPath); err == nil {
					projectPath = resolved
				}

				if seen[projectPath] {
					return nil
				}
				seen[projectPath] = true

				project := s.examineProject(projectPath)
				if project.HasGurgeh || project.HasColdwine || project.HasPollard || project.HasIntercore {
					projects = append(projects, project)
					slog.Debug("discovered project", "path", projectPath, "gurgeh", project.HasGurgeh, "coldwine", project.HasColdwine, "pollard", project.HasPollard, "intercore", project.HasIntercore)
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

	// Check for .gurgeh/ or legacy .praude/
	if info, err := os.Stat(filepath.Join(path, ".gurgeh")); err == nil && info.IsDir() {
		project.HasGurgeh = true
	} else if info, err := os.Stat(filepath.Join(path, ".praude")); err == nil && info.IsDir() {
		project.HasGurgeh = true // legacy path
	}

	// Check for .coldwine/ or legacy .tandemonium/
	if info, err := os.Stat(filepath.Join(path, ".coldwine")); err == nil && info.IsDir() {
		project.HasColdwine = true
	} else if info, err := os.Stat(filepath.Join(path, ".tandemonium")); err == nil && info.IsDir() {
		project.HasColdwine = true // legacy path
	}

	// Check for .pollard/
	if info, err := os.Stat(filepath.Join(path, ".pollard")); err == nil && info.IsDir() {
		project.HasPollard = true
	}

	// Check for .agent_mail/
	if info, err := os.Stat(filepath.Join(path, ".agent_mail")); err == nil && info.IsDir() {
		project.HasAgentMail = true
	}

	// Check for .clavain/intercore.db (Intercore kernel)
	if _, err := os.Stat(filepath.Join(path, ".clavain", "intercore.db")); err == nil {
		project.HasIntercore = true
	}

	// Check for .clavain/interspect/interspect.db (Interspect profiler)
	if _, err := os.Stat(filepath.Join(path, ".clavain", "interspect", "interspect.db")); err == nil {
		project.HasInterspect = true
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
