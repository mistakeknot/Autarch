package specs

import (
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Summary struct {
	ID      string
	Title   string
	Summary string
	Path    string
}

func LoadSpec(path string) (Spec, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, err
	}
	var doc Spec
	if err := yaml.Unmarshal(raw, &doc); err != nil {
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
		raw, err := os.ReadFile(path)
		if err != nil {
			warnings = append(warnings, "read failed: "+path)
			continue
		}
		var doc struct {
			ID      string `yaml:"id"`
			Title   string `yaml:"title"`
			Summary string `yaml:"summary"`
		}
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			warnings = append(warnings, "parse failed: "+path)
			continue
		}
		out = append(out, Summary{ID: doc.ID, Title: doc.Title, Summary: doc.Summary, Path: path})
	}
	return out, warnings
}
