package storage

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mistakeknot/vauxpraudemonium/internal/tandemonium/project"
	"gopkg.in/yaml.v3"
)

type specTask struct {
	ID     string `yaml:"id"`
	Title  string `yaml:"title"`
	Status string `yaml:"status"`
}

func RebuildFromSpecs(root string) error {
	specDir := project.SpecsDir(root)
	entries, err := os.ReadDir(specDir)
	if err != nil {
		return nil
	}
	db, err := OpenShared(project.StateDBPath(root))
	if err != nil {
		return err
	}
	if err := Migrate(db); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(specDir, name))
		if err != nil {
			continue
		}
		var spec specTask
		if err := yaml.Unmarshal(data, &spec); err != nil {
			continue
		}
		if spec.ID == "" {
			continue
		}
		if spec.Status == "" {
			spec.Status = "todo"
		}
		if spec.Title == "" {
			spec.Title = spec.ID
		}
		if _, err := GetTask(db, spec.ID); err == nil {
			continue
		}
		_ = InsertTask(db, Task{ID: spec.ID, Title: spec.Title, Status: spec.Status})
	}
	return nil
}
