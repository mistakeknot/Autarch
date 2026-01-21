package tui

import (
	"strings"
)

func FindTestSummary(log string) string {
	lines := strings.Split(log, "\n")
	var last string
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "test") || strings.Contains(lower, "pass") || strings.Contains(lower, "fail") {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				last = trimmed
			}
		}
	}
	if last == "" {
		return "Tests: unknown"
	}
	return last
}
