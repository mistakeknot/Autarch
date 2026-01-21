package git

import "strings"

func DiffUnified(r Runner, base, branch, path string) ([]string, error) {
	out, err := r.Run("git", "diff", "--unified=3", base+".."+branch, "--", path)
	if err != nil {
		return nil, err
	}
	return parseLines(out), nil
}

func parseLines(output string) []string {
	lines := strings.Split(output, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
