package reviewagent

import (
	"context"
	"crypto/sha256"
	"fmt"
	"github.com/mistakeknot/autarch/internal/door"
	"os"
	"path/filepath"
	"strings"
)

// Canonical sources are never silently shortened in a runtime handoff. The
// separate history/excerpt context may still be bounded as supporting evidence.
func RequiredGuidance(ctx context.Context, root string, maxBytes int) (string, error) {
	brief := door.ReadProductBrief(ctx, root, nil)
	paths := []string{filepath.Join(root, "AGENTS.md"), filepath.Join(root, "PHILOSOPHY.md"), filepath.Join(root, "docs", "why.md")}
	for _, area := range brief.Foundation {
		for _, source := range area.Sources {
			if source.State == "read" {
				path := source.Path
				if !filepath.IsAbs(path) {
					path = filepath.Join(root, path)
				}
				paths = append(paths, path)
			}
		}
	}
	for parent, depth := filepath.Dir(root), 0; parent != "/" && depth < 4; parent, depth = filepath.Dir(parent), depth+1 {
		for _, name := range []string{"AGENTS.md", "PHILOSOPHY.md"} {
			paths = append(paths, filepath.Join(parent, name))
		}
	}
	var b strings.Builder
	seen := map[string]bool{}
	for _, path := range paths {
		if seen[path] {
			continue
		}
		seen[path] = true
		if real, err := filepath.EvalSymlinks(path); err == nil && real != path && !strings.HasPrefix(real, root+string(filepath.Separator)) {
			fmt.Fprintf(&b, "\nSource unavailable: %s (escaping symlink)\n", path)
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(&b, "\nSource unavailable: %s\n", path)
			continue
		}
		sum := sha256.Sum256(data)
		if b.Len()+len(data)+len(path)+100 > maxBytes {
			return "", fmt.Errorf("insufficient context budget: complete canonical guidance exceeds %d bytes at %s", maxBytes, path)
		}
		fmt.Fprintf(&b, "\nCANONICAL SOURCE %s sha256:%x\n%s\n", path, sum, data)
	}
	return b.String(), nil
}
