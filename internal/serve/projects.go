package serve

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mistakeknot/autarch/internal/bigend/daemon"
)

// projectTTL is how long a project listing is reused before it is re-read [A-19].
const projectTTL = 30 * time.Second

// ProjectInfo is one resolved project. Dev and Ino identify the resolved root
// directory so a later writer can detect that the path now points elsewhere.
type ProjectInfo struct {
	Name string `json:"name"`
	Root string `json:"root"`
	Dev  uint64 `json:"dev"`
	Ino  uint64 `json:"ino"`
}

type projectEntry struct {
	name string
	root string
}

// Resolver maps project base names to resolved roots. Roots come from the
// Bigend project list, go through EvalSymlinks, and must stay inside one of the
// configured project directories.
type Resolver struct {
	rawDirs      []string
	resolvedDirs []string
	ttl          time.Duration
	now          func() time.Time

	mu      sync.Mutex
	loaded  time.Time
	entries []projectEntry
}

// NewResolver builds a resolver over the given project directories (scan roots).
func NewResolver(dirs []string) *Resolver {
	r := &Resolver{ttl: projectTTL, now: time.Now}
	for _, d := range dirs {
		abs, err := filepath.Abs(d)
		if err != nil {
			continue
		}
		res, err := filepath.EvalSymlinks(abs)
		if err != nil {
			continue
		}
		r.rawDirs = append(r.rawDirs, abs)
		r.resolvedDirs = append(r.resolvedDirs, res)
	}
	return r
}

func (r *Resolver) inside(root string) bool {
	for _, d := range r.resolvedDirs {
		rel, err := filepath.Rel(d, root)
		if err != nil {
			continue
		}
		if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			continue
		}
		return true
	}
	return false
}

// load returns the cached entries, re-reading them when older than the TTL.
func (r *Resolver) load() []projectEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entries != nil && r.now().Sub(r.loaded) < r.ttl {
		return r.entries
	}
	entries := []projectEntry{}
	seen := map[string]bool{}
	for _, p := range daemon.NewProjectManager(r.rawDirs).Paths() {
		root, err := filepath.EvalSymlinks(p)
		if err != nil || !r.inside(root) {
			continue
		}
		e := projectEntry{name: filepath.Base(p), root: root}
		key := e.name + "\x00" + e.root
		if seen[key] {
			continue
		}
		seen[key] = true
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].name != entries[j].name {
			return entries[i].name < entries[j].name
		}
		return entries[i].root < entries[j].root
	})
	r.entries = entries
	r.loaded = r.now()
	return r.entries
}

// stillContained reports whether root still resolves to itself inside one of
// the configured project directories. It narrows, but cannot close, a race
// with a concurrent retarget: the writer needs write access to the scan dir.
func (r *Resolver) stillContained(root string) bool {
	cur, err := filepath.EvalSymlinks(root)
	return err == nil && cur == root && r.inside(cur)
}

// Resolve maps a project base name to exactly one resolved root.
func (r *Resolver) Resolve(name string) (string, error) {
	var roots []string
	for _, e := range r.load() {
		if e.name == name {
			roots = append(roots, e.root)
		}
	}
	switch len(roots) {
	case 0:
		return "", fmt.Errorf("unknown project %q", name)
	case 1:
		// The listing is cached; re-check the root at use time so a directory
		// swapped for an escaping symlink within the TTL is refused.
		if !r.stillContained(roots[0]) {
			return "", fmt.Errorf("project %q no longer resolves inside the project directories", name)
		}
		return roots[0], nil
	}
	return "", fmt.Errorf("ambiguous project %q: %s", name, strings.Join(roots, ", "))
}

// Projects lists every project with the dev and ino of its resolved root,
// stat-ed at call time.
func (r *Resolver) Projects() []ProjectInfo {
	out := []ProjectInfo{}
	for _, e := range r.load() {
		info := ProjectInfo{Name: e.name, Root: e.root}
		if !r.stillContained(e.root) {
			continue
		}
		if fi, err := os.Stat(e.root); err == nil {
			if st, ok := fi.Sys().(*syscall.Stat_t); ok {
				info.Dev = uint64(st.Dev) //nolint:unconvert // width differs by platform
				info.Ino = uint64(st.Ino)
			}
		}
		out = append(out, info)
	}
	return out
}
