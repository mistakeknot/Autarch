package reviewagent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mistakeknot/autarch/internal/door"
)

// FoundationContext extends the existing foundation inventory with bounded,
// attributable source content and explicit history coverage. It is regenerated
// for every attempt, so accepted document changes influence subsequent work.
func FoundationContext(ctx context.Context, root string) string {
	brief := door.ReadProductBrief(ctx, root, nil)
	var out strings.Builder
	out.WriteString("Project foundation. Files are source evidence; inferred exceptions are not human rulings. Recover established decisions only with a source and revision. Inherit shared guidance unless a cited project exception applies. Draft missing guidance provisionally and ask only consequential questions, one at a time.\n\n")
	out.WriteString(door.BuildOnboardingBrief(brief))
	seen := map[string]bool{}
	read := func(path string) {
		if seen[path] {
			return
		}
		seen[path] = true
		data, err := os.ReadFile(path)
		if err != nil {
			return
		}
		sum := sha256.Sum256(data)
		if len(data) > 24000 {
			data = data[:24000]
		}
		fmt.Fprintf(&out, "\nSOURCE %s sha256:%x (bounded to 24000 bytes)\n%s\n", path, sum, data)
	}
	read(filepath.Join(root, "AGENTS.md"))
	read(filepath.Join(root, "PHILOSOPHY.md"))
	read(filepath.Join(root, "docs", "why.md"))
	for _, area := range brief.Foundation {
		for _, source := range area.Sources {
			if out.Len() > 90000 {
				out.WriteString("\nSource coverage partial: context size bound reached.\n")
				break
			}
			if source.State == "read" {
				path := source.Path
				if !filepath.IsAbs(path) {
					path = filepath.Join(root, path)
				}
				read(path)
			}
		}
	}
	for parent, depth := filepath.Dir(root), 0; parent != "/" && depth < 4; parent, depth = filepath.Dir(parent), depth+1 {
		for _, name := range []string{"AGENTS.md", "PHILOSOPHY.md"} {
			read(filepath.Join(parent, name))
		}
	}
	for _, command := range []struct {
		name  string
		args  []string
		label string
	}{{"git", []string{"log", "-12", "--format=%H %aI %s", "--", "AGENTS.md", "PHILOSOPHY.md", "docs"}, "Git: most recent 12 foundation/document commits"}, {"alwe", []string{"context", filepath.Join(root, "AGENTS.md")}, "Alwe: sessions referencing project guidance"}} {
		bounded, cancel := context.WithTimeout(ctx, 8*time.Second)
		cmd := exec.CommandContext(bounded, command.name, command.args...)
		cmd.Dir = root
		data, err := cmd.CombinedOutput()
		cancel()
		fmt.Fprintf(&out, "\nHistory coverage — %s: ", command.label)
		if err != nil {
			out.WriteString("unavailable; do not infer absence of past decisions.\n")
		} else {
			if len(data) > 10000 {
				data = data[:10000]
				out.WriteString("partial (10000-byte bound).\n")
			} else {
				out.WriteString("bounded results, not complete history.\n")
			}
			out.Write(data)
		}
	}
	return out.String()
}
