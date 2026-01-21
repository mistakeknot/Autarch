package scan

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type Options struct {
	MaxBytesTotal   int64
	MaxBytesPerFile int64
}

type Entry struct {
	Path    string
	Size    int64
	Content string
}

type Result struct {
	Entries    []Entry
	Skipped    []string
	TotalBytes int64
}

type ignoreRule struct {
	pattern  string
	dirOnly  bool
	anchored bool
}

func ScanRepo(root string, opts Options) (Result, error) {
	res := Result{}
	if opts.MaxBytesTotal <= 0 {
		opts.MaxBytesTotal = 2 * 1024 * 1024
	}
	if opts.MaxBytesPerFile <= 0 {
		opts.MaxBytesPerFile = 256 * 1024
	}
	rules, _ := loadGitignore(root)
	walkErr := filepath.WalkDir(root, func(full string, d fs.DirEntry, err error) error {
		if err != nil {
			res.Skipped = append(res.Skipped, "walk error: "+full)
			return nil
		}
		if full == root {
			return nil
		}
		rel, err := filepath.Rel(root, full)
		if err != nil {
			res.Skipped = append(res.Skipped, "rel error: "+full)
			return nil
		}
		rel = filepath.ToSlash(rel)
		if isDefaultIgnored(rel) || matchesIgnore(rules, rel, d.IsDir()) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			res.Skipped = append(res.Skipped, "stat error: "+rel)
			return nil
		}
		if info.Size() > opts.MaxBytesPerFile {
			res.Skipped = append(res.Skipped, rel+": exceeds max bytes per file")
			return nil
		}
		if res.TotalBytes+info.Size() > opts.MaxBytesTotal {
			res.Skipped = append(res.Skipped, rel+": exceeds total bytes")
			return nil
		}
		data, err := os.ReadFile(full)
		if err != nil {
			res.Skipped = append(res.Skipped, "read error: "+rel)
			return nil
		}
		if isBinary(data) {
			res.Skipped = append(res.Skipped, rel+": binary file")
			return nil
		}
		res.Entries = append(res.Entries, Entry{Path: rel, Size: info.Size(), Content: string(data)})
		res.TotalBytes += info.Size()
		return nil
	})
	return res, walkErr
}

func loadGitignore(root string) ([]ignoreRule, error) {
	path := filepath.Join(root, ".gitignore")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(raw), "\n")
	var rules []ignoreRule
	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		anchored := strings.HasPrefix(trim, "/")
		if anchored {
			trim = strings.TrimPrefix(trim, "/")
		}
		dirOnly := strings.HasSuffix(trim, "/")
		if dirOnly {
			trim = strings.TrimSuffix(trim, "/")
		}
		rules = append(rules, ignoreRule{pattern: trim, dirOnly: dirOnly, anchored: anchored})
	}
	return rules, nil
}

func matchesIgnore(rules []ignoreRule, rel string, isDir bool) bool {
	base := path.Base(rel)
	for _, rule := range rules {
		if rule.dirOnly && !isDir {
			continue
		}
		if rule.anchored {
			if matchPath(rule.pattern, rel) {
				return true
			}
			continue
		}
		if strings.Contains(rule.pattern, "/") {
			if matchPath(rule.pattern, rel) {
				return true
			}
			if rule.dirOnly && strings.HasPrefix(rel, rule.pattern+"/") {
				return true
			}
			continue
		}
		if matchPath(rule.pattern, base) {
			return true
		}
	}
	return false
}

func matchPath(pattern, target string) bool {
	if pattern == target {
		return true
	}
	matched, err := path.Match(pattern, target)
	if err != nil {
		return false
	}
	return matched
}

func isBinary(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return true
		}
	}
	return false
}

func isDefaultIgnored(rel string) bool {
	if rel == ".git" || strings.HasPrefix(rel, ".git/") {
		return true
	}
	if rel == ".praude" || strings.HasPrefix(rel, ".praude/") {
		return true
	}
	return false
}
