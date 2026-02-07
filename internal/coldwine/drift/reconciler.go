package drift

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/mistakeknot/autarch/pkg/yamlsafe"
)

// ReconciledActionItem contains tracking resolution for an action item.
type ReconciledActionItem struct {
	ActionItem
	Tracked bool
	Matched string
}

var (
	autarchIssueRx = regexp.MustCompile(`Autarch-[a-z0-9]+`)
	namedIDRx      = regexp.MustCompile(`\b(?:TASK|EPIC|PRD)-[A-Za-z0-9._-]+\b`)
	titleFieldRx   = regexp.MustCompile(`"title"\s*:\s*"([^"]+)"`)
)

// ReconcileActionItems marks doc action items as tracked/untracked by
// cross-referencing known task systems (beads, Coldwine tasks, Gurgeh sprints).
func ReconcileActionItems(root string, items []ActionItem) ([]ReconciledActionItem, error) {
	index, err := buildTrackingIndex(root)
	if err != nil {
		return nil, err
	}

	out := make([]ReconciledActionItem, 0, len(items))
	for _, item := range items {
		tracked, matched := isTracked(item.Text, index)
		out = append(out, ReconciledActionItem{ActionItem: item, Tracked: tracked, Matched: matched})
	}
	return out, nil
}

type trackingIndex struct {
	IDs     map[string]struct{}
	Phrases []string
}

func buildTrackingIndex(root string) (trackingIndex, error) {
	idx := trackingIndex{IDs: map[string]struct{}{}, Phrases: []string{}}

	// Coldwine tasks (authoritative local task store)
	tasksDir := filepath.Join(root, ".coldwine", "tasks")
	if entries, err := os.ReadDir(tasksDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || (!strings.HasSuffix(e.Name(), ".yaml") && !strings.HasSuffix(e.Name(), ".yml")) {
				continue
			}
			var task struct {
				ID    string `yaml:"id"`
				Title string `yaml:"title"`
			}
			path := filepath.Join(tasksDir, e.Name())
			_, _ = yamlsafe.UnmarshalFile(path, &task)
			if task.ID != "" {
				idx.IDs[strings.ToLower(task.ID)] = struct{}{}
			}
			if task.Title != "" {
				idx.Phrases = append(idx.Phrases, normalize(task.Title))
			}
		}
	}

	// Gurgeh sprint ids (file names + lightweight content IDs)
	sprintDir := filepath.Join(root, ".gurgeh", "sprints")
	if entries, err := os.ReadDir(sprintDir); err == nil {
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
				continue
			}
			id := strings.TrimSuffix(e.Name(), ".json")
			if id != "" {
				idx.IDs[strings.ToLower(id)] = struct{}{}
			}
		}
	}

	// Beads issue IDs and titles from .beads artifacts.
	beadsDir := filepath.Join(root, ".beads")
	_ = filepath.WalkDir(beadsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil || info.Size() > yamlsafe.DefaultMaxBytes {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		content := string(data)
		for _, id := range autarchIssueRx.FindAllString(content, -1) {
			idx.IDs[strings.ToLower(id)] = struct{}{}
		}
		for _, id := range namedIDRx.FindAllString(content, -1) {
			idx.IDs[strings.ToLower(id)] = struct{}{}
		}
		matches := titleFieldRx.FindAllStringSubmatch(content, -1)
		for _, m := range matches {
			if len(m) == 2 && strings.TrimSpace(m[1]) != "" {
				idx.Phrases = append(idx.Phrases, normalize(m[1]))
			}
		}
		return nil
	})

	idx.Phrases = dedupeStrings(idx.Phrases)
	return idx, nil
}

func isTracked(text string, idx trackingIndex) (bool, string) {
	lower := strings.ToLower(text)
	for _, id := range autarchIssueRx.FindAllString(text, -1) {
		if _, ok := idx.IDs[strings.ToLower(id)]; ok {
			return true, id
		}
	}
	for _, id := range namedIDRx.FindAllString(text, -1) {
		if _, ok := idx.IDs[strings.ToLower(id)]; ok {
			return true, id
		}
	}

	norm := normalize(lower)
	for _, phrase := range idx.Phrases {
		if phrase == "" || len(phrase) < 8 {
			continue
		}
		if strings.Contains(norm, phrase) {
			return true, phrase
		}
	}
	return false, ""
}

func normalize(s string) string {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "" {
		return ""
	}
	replacer := strings.NewReplacer(
		"\t", " ",
		"\n", " ",
		"\r", " ",
		":", " ",
		";", " ",
		",", " ",
		".", " ",
		"/", " ",
		"\\", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		"{", " ",
		"}", " ",
		"\"", " ",
		"'", " ",
	)
	return strings.Join(strings.Fields(replacer.Replace(lower)), " ")
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
