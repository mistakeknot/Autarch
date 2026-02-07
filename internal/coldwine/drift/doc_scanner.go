package drift

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ActionItem represents one potentially actionable item extracted from docs.
type ActionItem struct {
	ID         string
	SourcePath string
	Kind       string
	Text       string
	Confidence int
	Line       int
}

var (
	headingRx       = regexp.MustCompile(`^#{2,6}\s+(.+)$`)
	checkboxRx      = regexp.MustCompile(`^- \[ \]\s+(.+)$`)
	bulletRx        = regexp.MustCompile(`^[-*]\s+(.+)$`)
	researchFlagRx  = regexp.MustCompile(`\b(F\d+|RACE)\b`)
	emptyResolvedRx = regexp.MustCompile(`(?m)^date_resolved:\s*$`)
)

// ScanDocActionItems extracts potential action items from docs/**/*.md.
func ScanDocActionItems(root string) ([]ActionItem, error) {
	docsDir := filepath.Join(root, "docs")
	if st, err := os.Stat(docsDir); err != nil || !st.IsDir() {
		return []ActionItem{}, nil
	}

	items := make([]ActionItem, 0)
	err := filepath.WalkDir(docsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".md") {
			return nil
		}
		fileItems, scanErr := scanMarkdownFile(root, path)
		if scanErr != nil {
			return nil
		}
		items = append(items, fileItems...)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return dedupeItems(items), nil
}

func scanMarkdownFile(root, path string) ([]ActionItem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	items := make([]ActionItem, 0)
	rel := strings.TrimPrefix(path, root+string(filepath.Separator))
	lowerRel := strings.ToLower(rel)

	content := string(data)
	if strings.Contains(lowerRel, string(filepath.Separator)+"solutions"+string(filepath.Separator)) {
		if strings.HasPrefix(content, "---\n") && emptyResolvedRx.MatchString(content) {
			items = append(items, newItem(rel, "solution_unresolved", 95, 1, "Documented solution has no date_resolved and may still require follow-up."))
		}
	}

	head := ""
	for idx, raw := range strings.Split(content, "\n") {
		lineNo := idx + 1
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		if m := headingRx.FindStringSubmatch(line); len(m) == 2 {
			head = strings.ToLower(strings.TrimSpace(m[1]))
			continue
		}

		if m := checkboxRx.FindStringSubmatch(line); len(m) == 2 {
			items = append(items, newItem(rel, "checkbox", 70, lineNo, m[1]))
			continue
		}

		if strings.Contains(head, "open question") {
			if m := bulletRx.FindStringSubmatch(line); len(m) == 2 {
				items = append(items, newItem(rel, "open_question", 85, lineNo, m[1]))
			}
		}

		if strings.Contains(head, "critical") {
			if m := bulletRx.FindStringSubmatch(line); len(m) == 2 {
				items = append(items, newItem(rel, "critical_section", 80, lineNo, m[1]))
			}
		}

		if strings.Contains(lowerRel, string(filepath.Separator)+"research"+string(filepath.Separator)) && researchFlagRx.MatchString(line) {
			if strings.Contains(strings.ToLower(line), "priority") || strings.Contains(strings.ToLower(line), "critical") || strings.Contains(strings.ToLower(line), "high") {
				items = append(items, newItem(rel, "research_flag", 75, lineNo, line))
			}
		}
	}

	return items, nil
}

func newItem(sourcePath, kind string, confidence, line int, text string) ActionItem {
	clean := strings.TrimSpace(text)
	hash := sha1.Sum([]byte(fmt.Sprintf("%s|%s|%d|%s", sourcePath, kind, line, clean)))
	return ActionItem{
		ID:         hex.EncodeToString(hash[:]),
		SourcePath: sourcePath,
		Kind:       kind,
		Text:       clean,
		Confidence: confidence,
		Line:       line,
	}
}

func dedupeItems(items []ActionItem) []ActionItem {
	seen := make(map[string]struct{}, len(items))
	out := make([]ActionItem, 0, len(items))
	for _, it := range items {
		key := it.SourcePath + "|" + it.Kind + "|" + strings.ToLower(it.Text)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, it)
	}
	return out
}
