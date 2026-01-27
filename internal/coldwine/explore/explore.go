package explore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Options struct {
	EmitProgress func(string)
	Depth        int
}

type Output struct {
	SummaryPath string
	Summary     string
}

func Run(root, planDir string, opts Options) (Output, error) {
	emit := opts.EmitProgress
	if emit == nil {
		emit = func(string) {}
	}

	emit("Scanning docs")
	docs := scanDocs(root)
	emit("Scanning code")
	code := scanCode(root)
	emit("Scanning tests")
	tests := scanTests(root)

	summary := buildSummary(docs, code, tests, opts.Depth)
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		return Output{}, fmt.Errorf("mkdir plan: %w", err)
	}
	path := filepath.Join(planDir, "exploration.md")
	if err := os.WriteFile(path, []byte(summary), 0o644); err != nil {
		return Output{}, fmt.Errorf("write summary: %w", err)
	}
	return Output{SummaryPath: path, Summary: summary}, nil
}

func buildSummary(docs, code, tests []string, depth int) string {
	if depth <= 0 {
		depth = 2
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Exploration Summary\n\n")
	fmt.Fprintf(&b, "Depth: %d\n\n", depth)
	fmt.Fprintf(&b, "## Statistics\n\n")
	fmt.Fprintf(&b, "- Docs files: %d\n", len(docs))
	fmt.Fprintf(&b, "- Code files: %d\n", len(code))
	fmt.Fprintf(&b, "- Test files: %d\n\n", len(tests))

	// Include directory structure overview
	if len(code) > 0 {
		fmt.Fprintf(&b, "## Directory Structure\n\n")
		dirs := extractDirs(code, depth)
		for _, d := range dirs {
			fmt.Fprintf(&b, "- %s\n", d)
		}
		b.WriteString("\n")
	}

	// Include key doc snippets (README, AGENTS, CLAUDE)
	if len(docs) > 0 {
		fmt.Fprintf(&b, "## Key Documents\n\n")
		for _, doc := range docs {
			name := filepath.Base(doc)
			nameLower := strings.ToLower(name)
			if nameLower == "readme.md" || nameLower == "agents.md" || nameLower == "claude.md" {
				snippet := readSnippet(doc, 30)
				if snippet != "" {
					fmt.Fprintf(&b, "### %s\n\n%s\n\n", name, snippet)
				}
			}
		}
	}

	return b.String()
}

// extractDirs returns unique directory paths from file list, trimmed to depth.
func extractDirs(files []string, depth int) []string {
	seen := map[string]bool{}
	var dirs []string
	for _, f := range files {
		dir := filepath.Dir(f)
		parts := strings.Split(dir, string(filepath.Separator))
		// Limit depth
		if len(parts) > depth+2 {
			parts = parts[:depth+2]
		}
		key := strings.Join(parts, string(filepath.Separator))
		if !seen[key] {
			seen[key] = true
			dirs = append(dirs, key)
		}
	}
	return dirs
}

// readSnippet reads up to n lines from a file.
func readSnippet(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > n {
		lines = lines[:n]
	}
	return strings.Join(lines, "\n")
}
