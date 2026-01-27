package compound

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// SolutionsDir is the relative path to the solutions directory from project root.
const SolutionsDir = "docs/solutions"

// Capture writes a solution document to docs/solutions/{module}/.
// The filename is generated from the component and date.
// Returns the path to the created file.
func Capture(projectPath string, sol Solution, body string) (string, error) {
	if err := validateSolution(sol); err != nil {
		return "", fmt.Errorf("invalid solution: %w", err)
	}

	// Ensure date is set
	if sol.Date == "" {
		sol.Date = time.Now().Format("2006-01-02")
	}

	// Build target directory
	targetDir := filepath.Join(projectPath, SolutionsDir, sol.Module)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("create directory: %w", err)
	}

	// Generate filename: {component}-{date}.md
	filename := generateFilename(sol)
	targetPath := filepath.Join(targetDir, filename)

	// Build file content with YAML frontmatter
	content, err := buildContent(sol, body)
	if err != nil {
		return "", fmt.Errorf("build content: %w", err)
	}

	// Write file
	if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write file: %w", err)
	}

	return targetPath, nil
}

// validateSolution checks that required fields are present and valid.
func validateSolution(sol Solution) error {
	if sol.Module == "" {
		return fmt.Errorf("module is required")
	}
	if !contains(ValidModules, sol.Module) {
		return fmt.Errorf("invalid module %q, must be one of: %v", sol.Module, ValidModules)
	}

	if sol.ProblemType == "" {
		return fmt.Errorf("problem_type is required")
	}
	if !contains(ValidProblemTypes, sol.ProblemType) {
		return fmt.Errorf("invalid problem_type %q, must be one of: %v", sol.ProblemType, ValidProblemTypes)
	}

	if sol.Component == "" {
		return fmt.Errorf("component is required")
	}

	if sol.Severity == "" {
		return fmt.Errorf("severity is required")
	}
	if !contains(ValidSeverities, sol.Severity) {
		return fmt.Errorf("invalid severity %q, must be one of: %v", sol.Severity, ValidSeverities)
	}

	if len(sol.Symptoms) == 0 {
		return fmt.Errorf("at least one symptom is required")
	}

	if sol.RootCause == "" {
		return fmt.Errorf("root_cause is required")
	}

	return nil
}

// generateFilename creates a kebab-case filename from solution metadata.
func generateFilename(sol Solution) string {
	// Sanitize component for filename
	component := strings.ToLower(sol.Component)
	component = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(component, "-")
	component = strings.Trim(component, "-")

	return fmt.Sprintf("%s-%s.md", component, sol.Date)
}

// buildContent creates the markdown file content with YAML frontmatter.
func buildContent(sol Solution, body string) (string, error) {
	var buf bytes.Buffer

	// Write YAML frontmatter
	buf.WriteString("---\n")
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(sol); err != nil {
		return "", fmt.Errorf("encode yaml: %w", err)
	}
	encoder.Close()
	buf.WriteString("---\n\n")

	// Write body
	buf.WriteString(body)

	return buf.String(), nil
}

// contains checks if a slice contains a value.
func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
