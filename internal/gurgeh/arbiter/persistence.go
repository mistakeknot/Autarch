package arbiter

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const sprintsDir = ".gurgeh/sprints"

// SaveSprintState persists a sprint to disk
func SaveSprintState(state *SprintState) error {
	if state == nil {
		return fmt.Errorf("state cannot be nil")
	}
	if state.ID == "" || filepath.Base(state.ID) != state.ID {
		return fmt.Errorf("invalid sprint ID: %q", state.ID)
	}

	dir := filepath.Join(state.ProjectPath, sprintsDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create sprints dir: %w", err)
	}

	data, err := yaml.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}

	path := filepath.Join(dir, state.ID+".yaml")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write state: %w", err)
	}

	return nil
}

// LoadSprintState reads a sprint from disk
func LoadSprintState(projectPath, id string) (*SprintState, error) {
	if id == "" || filepath.Base(id) != id {
		return nil, fmt.Errorf("invalid sprint ID: %q", id)
	}

	path := filepath.Join(projectPath, sprintsDir, id+".yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}

	var state SprintState
	if err := yaml.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}

	return &state, nil
}

// ListSprints returns all sprint IDs in a project
func ListSprints(projectPath string) ([]string, error) {
	dir := filepath.Join(projectPath, sprintsDir)

	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var ids []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".yaml" {
			ids = append(ids, e.Name()[:len(e.Name())-5])
		}
	}

	return ids, nil
}
