// Command archviz regenerates data sections in docs/architecture-visualizer.html
// from live Go source code. Hand-maintained HTML (layout, CSS, SVG, narrative)
// is preserved — only content between <!-- AUTOGEN:xxx --> markers is replaced.
//
// Usage:
//
//	go run ./cmd/archviz          # regenerate in place
//	go run ./cmd/archviz --check  # exit 1 if content would change (for CI)
package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func main() {
	check := len(os.Args) > 1 && os.Args[1] == "--check"

	root := findRepoRoot()
	htmlPath := filepath.Join(root, "docs", "architecture-visualizer.html")

	original, err := os.ReadFile(htmlPath)
	if err != nil {
		fatal("read HTML: %v", err)
	}

	sections := map[string]string{
		"phases":   generatePhases(root),
		"signals":  generateSignals(root),
		"messages": generateMessages(root),
	}

	result := string(original)
	for name, content := range sections {
		result, err = replaceSection(result, name, content)
		if err != nil {
			fatal("replace section %q: %v", name, err)
		}
	}

	if result == string(original) {
		fmt.Println("archviz: HTML is up to date")
		return
	}

	if check {
		fmt.Println("archviz: HTML is out of date (run `go run ./cmd/archviz` to update)")
		os.Exit(1)
	}

	if err := os.WriteFile(htmlPath, []byte(result), 0644); err != nil {
		fatal("write HTML: %v", err)
	}
	fmt.Println("archviz: updated docs/architecture-visualizer.html")
}

// replaceSection replaces content between <!-- AUTOGEN:name --> and <!-- /AUTOGEN:name -->
func replaceSection(html, name, content string) (string, error) {
	open := fmt.Sprintf("<!-- AUTOGEN:%s -->", name)
	close := fmt.Sprintf("<!-- /AUTOGEN:%s -->", name)

	start := strings.Index(html, open)
	if start == -1 {
		return "", fmt.Errorf("marker %q not found", open)
	}
	end := strings.Index(html, close)
	if end == -1 {
		return "", fmt.Errorf("marker %q not found", close)
	}

	start += len(open)
	return html[:start] + "\n" + content + "\n" + html[end:], nil
}

// generatePhases extracts phase names from internal/gurgeh/arbiter/types.go
func generatePhases(root string) string {
	src := filepath.Join(root, "internal", "gurgeh", "arbiter", "types.go")
	content, err := os.ReadFile(src)
	if err != nil {
		fatal("read types.go: %v", err)
	}

	// Extract the phase name strings from the String() method's slice literal
	re := regexp.MustCompile(`"([^"]+)"`)
	// Find the names slice inside String()
	block := extractBlock(string(content), `names := []string{`, `}`)
	if block == "" {
		fatal("could not find phase names in types.go")
	}

	matches := re.FindAllStringSubmatch(block, -1)
	if len(matches) == 0 {
		fatal("no phase names found in types.go")
	}

	var names []string
	for _, m := range matches {
		names = append(names, m[1])
	}

	// Generate: "Arbiter sprint: N sections (First → Last)"
	return fmt.Sprintf(
		`        'Arbiter sprint: %d sections (%s → %s)',`,
		len(names), names[0], names[len(names)-1],
	)
}

// generateSignals extracts signal types from pkg/signals/signal.go
func generateSignals(root string) string {
	src := filepath.Join(root, "pkg", "signals", "signal.go")
	content, err := os.ReadFile(src)
	if err != nil {
		fatal("read signal.go: %v", err)
	}

	type sigInfo struct {
		name     string
		severity string
		source   string
	}

	// Parse signal constants: SignalXxx SignalType = "xxx"
	sigRe := regexp.MustCompile(`Signal\w+\s+SignalType\s*=\s*"([^"]+)"`)
	sigMatches := sigRe.FindAllStringSubmatch(string(content), -1)

	// Known signal metadata (source + severity) — these are documented in the codebase
	meta := map[string]sigInfo{
		"competitor_shipped":    {severity: "warning", source: "Pollard"},
		"research_invalidation": {severity: "critical", source: "Pollard"},
		"assumption_decayed":    {severity: "warning", source: "Gurgeh"},
		"hypothesis_stale":      {severity: "info", source: "Gurgeh"},
		"spec_health_low":       {severity: "critical", source: "Gurgeh"},
		"execution_drift":       {severity: "warning", source: "Coldwine"},
		"vision_drift":          {severity: "critical", source: "Gurgeh"},
	}

	// Known descriptions
	descs := map[string]string{
		"competitor_shipped":    "Competitor product launch detected",
		"research_invalidation": "Research assumptions proven wrong",
		"assumption_decayed":    "Assumptions lost validity over time",
		"hypothesis_stale":      "Hypothesis no longer relevant",
		"spec_health_low":       "Spec quality declining",
		"execution_drift":       "Implementation drifting from spec",
		"vision_drift":          "Product vision shifting",
	}

	sevColors := map[string]string{
		"info":     "var(--primary)",
		"warning":  "var(--warning)",
		"critical": "var(--error)",
	}

	sevClass := map[string]string{
		"info":     "sev-info",
		"warning":  "sev-warning",
		"critical": "sev-critical",
	}

	var chips []string
	for _, m := range sigMatches {
		name := m[1]
		info, ok := meta[name]
		if !ok {
			info = sigInfo{severity: "info", source: "Unknown"}
		}
		desc := descs[name]
		if desc == "" {
			desc = strings.ReplaceAll(name, "_", " ")
		}
		color := sevColors[info.severity]
		cls := sevClass[info.severity]

		chip := fmt.Sprintf(`    <div class="signal-chip" style="border-color:%s">
      <div class="name">%s</div>
      <div class="desc">%s</div>
      <div class="sev %s">Source: %s</div>
    </div>`, color, name, desc, cls, info.source)
		chips = append(chips, chip)
	}

	return strings.Join(chips, "\n")
}

// generateMessages extracts Msg types from internal/tui/messages.go
func generateMessages(root string) string {
	src := filepath.Join(root, "internal", "tui", "messages.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, src, nil, 0)
	if err != nil {
		fatal("parse messages.go: %v", err)
	}

	var msgTypes []string
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			name := ts.Name.Name
			if strings.HasSuffix(name, "Msg") {
				msgTypes = append(msgTypes, name)
			}
		}
	}

	sort.Strings(msgTypes)

	// Generate the gurgeh card detail item showing message count and sample
	return fmt.Sprintf(
		`        '%d TUI message types (e.g. %s, %s, %s)',`,
		len(msgTypes), msgTypes[0], msgTypes[len(msgTypes)/2], msgTypes[len(msgTypes)-1],
	)
}

// extractBlock returns content between open and close delimiters (first match)
func extractBlock(src, open, close string) string {
	start := strings.Index(src, open)
	if start == -1 {
		return ""
	}
	start += len(open)
	// Find the matching close brace, accounting for nesting
	depth := 1
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start:i]
			}
		}
	}
	return ""
}

func findRepoRoot() string {
	// Walk up from cwd looking for go.mod
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			fatal("could not find repo root (no go.mod)")
		}
		dir = parent
	}
}

func fatal(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "archviz: "+format+"\n", args...)
	os.Exit(1)
}
