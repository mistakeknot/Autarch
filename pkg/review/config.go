package review

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BuildSpec is copied into the proposal BEFORE it is displayed for acceptance.
type BuildSpec struct {
	Command []string   `json:"command"`
	Checks  [][]string `json:"checks"`
	Binary  string     `json:"binary"`
}
type ProjectConfig struct {
	Version  int       `json:"version"`
	Tracker  string    `json:"tracker"`
	Build    BuildSpec `json:"build"`
	Provider string    `json:"provider,omitempty"`
	Model    string    `json:"model,omitempty"`
}

func LoadProjectConfig(project string) (ProjectConfig, error) {
	var c ProjectConfig
	data, err := os.ReadFile(filepath.Join(project, ".autarch", "review.json"))
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(data, &c); err != nil {
		return c, err
	}
	if c.Version != Version {
		return c, fmt.Errorf("unsupported project review configuration")
	}
	if !filepath.IsAbs(c.Tracker) {
		c.Tracker = filepath.Join(project, c.Tracker)
	}
	c.Tracker, err = projectPath(c.Tracker)
	if err != nil {
		return c, err
	}
	if rel, e := filepath.Rel(c.Tracker, project); e != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return c, fmt.Errorf("tracker is outside project workspace")
	}
	return c, nil
}
