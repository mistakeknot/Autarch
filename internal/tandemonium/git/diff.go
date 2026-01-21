package git

import "strings"

func ParseNameOnly(output string) []string {
	lines := strings.Split(output, "\n")
	var files []string
	for _, l := range lines {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		files = append(files, l)
	}
	return files
}
