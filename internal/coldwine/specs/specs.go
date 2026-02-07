package specs

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mistakeknot/autarch/pkg/yamlsafe"
)

type SpecSummary struct {
	ID            string
	Title         string
	Status        string
	Path          string
	FilesToModify []string
}

type specDoc struct {
	ID            string   `yaml:"id"`
	Title         string   `yaml:"title"`
	Status        string   `yaml:"status"`
	FilesToModify []string `yaml:"files_to_modify"`
}

func LoadSummaries(dir string) ([]SpecSummary, []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return []SpecSummary{}, []string{}
	}
	var summaries []SpecSummary
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
		var doc specDoc
		data, err := yamlsafe.UnmarshalFile(path, &doc)
		if err != nil {
			warnings = append(warnings, "read failed: "+path)
			continue
		}
		if err := Validate(data); err != nil {
			warnings = append(warnings, "validation failed: "+path)
		}
		id := doc.ID
		if id == "" {
			id = strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
			warnings = append(warnings, "missing id: "+path)
		}
		if doc.Title == "" {
			warnings = append(warnings, "missing title: "+path)
		}
		if doc.Status == "" {
			warnings = append(warnings, "missing status: "+path)
		}
		summaries = append(summaries, SpecSummary{
			ID:            id,
			Title:         doc.Title,
			Status:        doc.Status,
			Path:          path,
			FilesToModify: doc.FilesToModify,
		})
	}
	return summaries, warnings
}

func FindByID(list []SpecSummary, id string) (SpecSummary, bool) {
	for _, s := range list {
		if s.ID == id {
			return s, true
		}
	}
	return SpecSummary{}, false
}
