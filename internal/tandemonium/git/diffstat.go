package git

import (
	"strconv"
	"strings"
)

type DiffStat struct {
	Path    string
	Added   int
	Deleted int
}

func ParseNumstat(output string) []DiffStat {
	var stats []DiffStat
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		added, _ := strconv.Atoi(parts[0])
		deleted, _ := strconv.Atoi(parts[1])
		stats = append(stats, DiffStat{
			Path:    parts[2],
			Added:   added,
			Deleted: deleted,
		})
	}
	return stats
}

func DiffNumstat(r Runner, base, branch string) ([]DiffStat, error) {
	out, err := r.Run("git", "diff", "--numstat", base+".."+branch)
	if err != nil {
		return nil, err
	}
	return ParseNumstat(out), nil
}
