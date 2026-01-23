package explore

import (
	"io/fs"
	"path/filepath"
	"strings"
)

func scanDocs(root string) []string {
	return findByExt(root, []string{".md", ".mdx", ".rst"})
}

func scanCode(root string) []string {
	return findByExt(root, []string{".go", ".ts", ".tsx", ".js"})
}

func scanTests(root string) []string {
	return findBySuffix(root, []string{"_test.go", ".spec.ts", ".test.ts"})
}

func findByExt(root string, exts []string) []string {
	return findFiles(root, func(path string) bool {
		for _, ext := range exts {
			if strings.HasSuffix(path, ext) {
				return true
			}
		}
		return false
	})
}

func findBySuffix(root string, suffixes []string) []string {
	return findFiles(root, func(path string) bool {
		for _, suffix := range suffixes {
			if strings.HasSuffix(path, suffix) {
				return true
			}
		}
		return false
	})
}

func findFiles(root string, match func(string) bool) []string {
	var matches []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if match(path) {
			matches = append(matches, path)
		}
		return nil
	})
	return matches
}

func shouldSkipDir(name string) bool {
	if name == ".git" || name == "node_modules" {
		return true
	}
	if strings.HasPrefix(name, ".") {
		return true
	}
	return false
}
