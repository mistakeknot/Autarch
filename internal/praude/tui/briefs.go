package tui

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/vauxpraudemonium/internal/praude/brief"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/project"
	"github.com/mistakeknot/vauxpraudemonium/internal/praude/specs"
)

func writeResearchBrief(root, id, researchPath string, now time.Time) (string, error) {
	briefsDir := project.BriefsDir(root)
	if err := os.MkdirAll(briefsDir, 0o755); err != nil {
		return "", err
	}
	stamp := now.UTC().Format("20060102-150405")
	briefPath := filepath.Join(briefsDir, id+"-"+stamp+".md")
	specPath := filepath.Join(project.SpecsDir(root), id+".yaml")
	spec, err := specs.LoadSpec(specPath)
	if err != nil {
		return "", err
	}
	acceptance := []string{}
	for _, item := range spec.Acceptance {
		if strings.TrimSpace(item.Description) != "" {
			acceptance = append(acceptance, item.Description)
		}
	}
	content := buildResearchBrief(spec, researchPath, acceptance)
	if err := os.WriteFile(briefPath, []byte(content), 0o644); err != nil {
		return "", err
	}
	return briefPath, nil
}

func writeSuggestionBrief(root, id, suggPath string, now time.Time) (string, error) {
	briefsDir := project.BriefsDir(root)
	if err := os.MkdirAll(briefsDir, 0o755); err != nil {
		return "", err
	}
	stamp := now.UTC().Format("20060102-150405")
	briefPath := filepath.Join(briefsDir, id+"-"+stamp+".md")
	specPath := filepath.Join(project.SpecsDir(root), id+".yaml")
	spec, err := specs.LoadSpec(specPath)
	if err != nil {
		return "", err
	}
	content := buildSuggestionBrief(spec, suggPath)
	if err := os.WriteFile(briefPath, []byte(content), 0o644); err != nil {
		return "", err
	}
	return briefPath, nil
}

func buildResearchBrief(spec specs.Spec, researchPath string, acceptance []string) string {
	base := brief.Compose(brief.Input{
		ID:            spec.ID,
		Title:         spec.Title,
		Summary:       spec.Summary,
		Requirements:  spec.Requirements,
		Acceptance:    acceptance,
		ResearchFiles: spec.Research,
	})
	instructions := "\n\nInstructions:\n" +
		"- Fill in market research and competitive landscape sections.\n" +
		"- Include an OSS project scan with evidence refs.\n" +
		"- Use evidence refs for all claims.\n" +
		"- Write results into the research template at:\n  " + researchPath + "\n"
	return base + instructions
}

func buildSuggestionBrief(spec specs.Spec, suggPath string) string {
	acceptance := []string{}
	for _, item := range spec.Acceptance {
		if strings.TrimSpace(item.Description) != "" {
			acceptance = append(acceptance, item.Description)
		}
	}
	base := brief.Compose(brief.Input{
		ID:            spec.ID,
		Title:         spec.Title,
		Summary:       spec.Summary,
		Requirements:  spec.Requirements,
		Acceptance:    acceptance,
		ResearchFiles: spec.Research,
	})
	instructions := "\n\nInstructions:\n" +
		"- Create per-section suggestions for Summary, Requirements, CUJs, Market Research, Competitive Landscape.\n" +
		"- Set status: ready for any section you complete (leave pending otherwise).\n" +
		"- Use evidence refs for all research claims.\n" +
		"- Write results into the suggestions template at:\n  " + suggPath + "\n"
	return base + instructions
}
