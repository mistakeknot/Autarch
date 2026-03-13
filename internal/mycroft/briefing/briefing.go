// Package briefing assembles context documents for dispatched agents.
package briefing

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/internal/mycroft"
)

// Generate creates a briefing document for an agent's dispatched bead
// and writes it to the briefings directory.
func Generate(briefingDir string, agent string, bead mycroft.BeadView) (string, error) {
	if err := os.MkdirAll(briefingDir, 0755); err != nil {
		return "", fmt.Errorf("create briefing dir: %w", err)
	}

	filename := fmt.Sprintf("%s.md", bead.ID)
	path := filepath.Join(briefingDir, filename)

	content := buildBriefing(agent, bead)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("write briefing: %w", err)
	}

	return path, nil
}

// ValidateContextPath ensures a context file path is safe:
// - Must be absolute
// - Must not contain ".."
// - Must be under the project root
func ValidateContextPath(path string, projectRoot string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("context path must be absolute: %q", path)
	}

	cleaned := filepath.Clean(path)
	if strings.Contains(cleaned, "..") {
		return fmt.Errorf("context path contains traversal: %q", path)
	}

	// Resolve symlinks.
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		// File doesn't exist yet — check parent dir.
		parentResolved, err := filepath.EvalSymlinks(filepath.Dir(cleaned))
		if err != nil {
			return fmt.Errorf("resolve context path parent: %w", err)
		}
		resolved = filepath.Join(parentResolved, filepath.Base(cleaned))
	}

	rootResolved, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return fmt.Errorf("resolve project root: %w", err)
	}

	if !strings.HasPrefix(resolved, rootResolved+"/") && resolved != rootResolved {
		return fmt.Errorf("context path %q is outside project root %q", path, projectRoot)
	}

	return nil
}

func buildBriefing(agent string, bead mycroft.BeadView) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Briefing: %s\n\n", bead.Title)
	fmt.Fprintf(&b, "**Assigned to:** %s\n", agent)
	fmt.Fprintf(&b, "**Bead:** %s\n", bead.ID)
	fmt.Fprintf(&b, "**Type:** %s\n", bead.Type)
	fmt.Fprintf(&b, "**Priority:** P%d\n", bead.Priority)
	if bead.Complexity != "" && bead.Complexity != "unknown" {
		fmt.Fprintf(&b, "**Complexity:** %s\n", bead.Complexity)
	}
	fmt.Fprintf(&b, "**Generated:** %s\n\n", time.Now().Format(time.RFC3339))

	if len(bead.Labels) > 0 {
		fmt.Fprintf(&b, "**Labels:** %s\n\n", strings.Join(bead.Labels, ", "))
	}

	if len(bead.Dependencies) > 0 {
		fmt.Fprintf(&b, "## Dependencies\n\n")
		for _, dep := range bead.Dependencies {
			fmt.Fprintf(&b, "- %s\n", dep)
		}
		fmt.Fprintln(&b)
	}

	fmt.Fprintln(&b, "## Instructions")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "1. Review the bead details: `bd show %s`\n", bead.ID)
	fmt.Fprintf(&b, "2. Claim the work: `bd update %s --status=in_progress`\n", bead.ID)
	fmt.Fprintf(&b, "3. Complete the task as described\n")
	fmt.Fprintf(&b, "4. Close when done: `bd close %s`\n", bead.ID)

	return b.String()
}
