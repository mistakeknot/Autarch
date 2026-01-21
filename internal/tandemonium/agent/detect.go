package agent

import "strings"

func DetectState(line string) string {
    l := strings.ToLower(line)
    if strings.Contains(l, "done") || strings.Contains(l, "complete") {
        return "done"
    }
    if strings.Contains(l, "blocked") || strings.Contains(l, "waiting") {
        return "blocked"
    }
    return "working"
}
