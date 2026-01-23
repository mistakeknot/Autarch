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
	fmt.Fprintf(&b, "Docs files: %d\n", len(docs))
	fmt.Fprintf(&b, "Code files: %d\n", len(code))
	fmt.Fprintf(&b, "Test files: %d\n", len(tests))
	return b.String()
}
