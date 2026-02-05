// Package prd provides functionality to import Gurgeh artifacts into Coldwine.
package prd

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mistakeknot/autarch/internal/coldwine/storage"
)

// BriefImportOptions configures how Brief import behaves.
type BriefImportOptions struct {
	// Root is the project root directory.
	Root string
	// SpecID is the spec whose briefs to import (e.g., "PRD-001").
	SpecID string
	// StoryID is the story to attach tasks to. If empty, a placeholder is created.
	StoryID string
}

// BriefImportResult contains the result of a Brief import.
type BriefImportResult struct {
	Tasks    []storage.WorkTask
	StoryID  string
	Warnings []string
}

// ImportFromBriefs reads Gurgeh briefs and generates Coldwine WorkTasks.
// Briefs are markdown files in .gurgeh/briefs/<spec-id>/.
func ImportFromBriefs(opts BriefImportOptions) (*BriefImportResult, error) {
	briefsDir := filepath.Join(opts.Root, ".gurgeh", "briefs", opts.SpecID)

	if _, err := os.Stat(briefsDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("briefs directory not found: %s (run 'gurgeh export %s --format briefs' first)", briefsDir, opts.SpecID)
	}

	entries, err := os.ReadDir(briefsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read briefs directory: %w", err)
	}

	result := &BriefImportResult{
		Tasks:   make([]storage.WorkTask, 0),
		StoryID: opts.StoryID,
	}

	// If no story specified, we'll note that the caller should create one
	if result.StoryID == "" {
		result.StoryID = fmt.Sprintf("STORY-%s-BRIEFS", opts.SpecID)
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("No story specified. Tasks will reference placeholder story '%s'", result.StoryID))
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		briefPath := filepath.Join(briefsDir, entry.Name())
		task, err := parseBriefFile(briefPath, result.StoryID)
		if err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Failed to parse %s: %v", entry.Name(), err))
			continue
		}

		result.Tasks = append(result.Tasks, task)
	}

	if len(result.Tasks) == 0 {
		return nil, fmt.Errorf("no valid briefs found in %s", briefsDir)
	}

	return result, nil
}

// parseBriefFile reads a brief markdown file and extracts task fields.
// Brief format:
//
//	# Title
//
//	## Outcome
//	Description of success state
//
//	## Acceptance Criteria
//	- [ ] Criterion 1
//	- [ ] Criterion 2
func parseBriefFile(path string, storyID string) (storage.WorkTask, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return storage.WorkTask{}, err
	}

	text := string(content)

	// Extract title from first # heading
	title := extractBriefTitle(text)
	if title == "" {
		title = filepath.Base(path)
	}

	// Extract outcome section
	outcome := extractSection(text, "Outcome")

	// Extract acceptance criteria
	criteria := extractCriteria(text)

	// Build description from outcome + criteria
	description := outcome
	if len(criteria) > 0 {
		description += "\n\n## Acceptance Criteria\n"
		for _, c := range criteria {
			description += "- " + c + "\n"
		}
	}

	// Generate task ID from filename (BRIEF-001-slug.md → TASK-001)
	taskID := generateTaskID(filepath.Base(path))

	return storage.WorkTask{
		ID:          taskID,
		StoryID:     storyID,
		Title:       title,
		Description: description,
		Status:      storage.TaskStatusTodo,
		Priority:    2, // Default medium priority
	}, nil
}

// extractBriefTitle extracts the title from a markdown heading.
func extractBriefTitle(text string) string {
	re := regexp.MustCompile(`(?m)^#\s+(.+)$`)
	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// extractSection extracts content under a ## heading.
func extractSection(text, heading string) string {
	// Find the heading
	pattern := fmt.Sprintf(`(?ms)^##\s+%s\s*\n(.*?)(?:^##|\z)`, regexp.QuoteMeta(heading))
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(text)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// extractCriteria extracts acceptance criteria items.
func extractCriteria(text string) []string {
	section := extractSection(text, "Acceptance Criteria")
	if section == "" {
		return nil
	}

	var criteria []string
	// Match both checked and unchecked items
	re := regexp.MustCompile(`(?m)^-\s*\[[ x]\]\s*(.+)$`)
	matches := re.FindAllStringSubmatch(section, -1)
	for _, m := range matches {
		if len(m) > 1 {
			criteria = append(criteria, strings.TrimSpace(m[1]))
		}
	}
	return criteria
}

// generateTaskID converts a brief filename to a task ID.
// BRIEF-001-implement-auth.md → TASK-001
func generateTaskID(filename string) string {
	// Remove extension
	name := strings.TrimSuffix(filename, ".md")

	// Try to extract number from BRIEF-XXX pattern
	re := regexp.MustCompile(`BRIEF-(\d+)`)
	matches := re.FindStringSubmatch(name)
	if len(matches) > 1 {
		return fmt.Sprintf("TASK-%s", matches[1])
	}

	// Fallback: use sanitized filename
	return fmt.Sprintf("TASK-%s", sanitizeID(name))
}

// sanitizeID converts a string to a valid ID component.
func sanitizeID(s string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	return strings.ToUpper(re.ReplaceAllString(s, "-"))
}
