package specs

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mistakeknot/autarch/pkg/yamlsafe"
	"gopkg.in/yaml.v3"
)

type Summary struct {
	ID      string
	Title   string
	Summary string
	Status  string
	Type    string
	Path    string
}

func LoadSpec(path string) (Spec, error) {
	var doc Spec
	if _, err := yamlsafe.UnmarshalFile(path, &doc); err != nil {
		return Spec{}, err
	}
	return doc, nil
}

func LoadSummaries(dir string) ([]Summary, []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []Summary{}, []string{}
	}
	var out []Summary
	var warnings []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !(strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml")) {
			continue
		}
		path := filepath.Join(dir, name)
		var doc struct {
			ID      string `yaml:"id"`
			Title   string `yaml:"title"`
			Summary string `yaml:"summary"`
			Status  string `yaml:"status"`
			Type    string `yaml:"type"`
		}
		if _, err := yamlsafe.UnmarshalFile(path, &doc); err != nil {
			warnings = append(warnings, "read failed: "+path)
			continue
		}
		status := strings.TrimSpace(doc.Status)
		if status == "" {
			status = "draft"
		}
		out = append(out, Summary{ID: doc.ID, Title: doc.Title, Summary: doc.Summary, Status: status, Type: doc.Type, Path: path})
	}
	return out, warnings
}

func LoadSummariesWithArchived(activeDir, archivedDir string, includeArchived bool) ([]Summary, []string) {
	list, warnings := LoadSummaries(activeDir)
	if !includeArchived {
		return list, warnings
	}
	archived, archivedWarnings := LoadSummaries(archivedDir)
	list = append(list, archived...)
	warnings = append(warnings, archivedWarnings...)
	return list, warnings
}

func UpdateStatus(path string, status string) error {
	spec, err := LoadSpec(path)
	if err != nil {
		return err
	}
	spec.Status = status
	out, err := yaml.Marshal(&spec)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}
