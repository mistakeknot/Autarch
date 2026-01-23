package epics

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func WriteEpics(dir string, epics []Epic, opts WriteOptions) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir epics dir: %w", err)
	}
	for _, epic := range epics {
		path := filepath.Join(dir, fmt.Sprintf("%s.yaml", epic.ID))
		if _, err := os.Stat(path); err == nil && opts.Existing == ExistingSkip {
			continue
		}
		data, err := yaml.Marshal(epic)
		if err != nil {
			return fmt.Errorf("marshal epic %s: %w", epic.ID, err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("write epic %s: %w", epic.ID, err)
		}
	}
	return nil
}
