package tmux

import (
    "os/exec"
    "strings"
)

func ListSessions(prefix string) ([]string, error) {
    out, err := exec.Command("tmux", "ls").CombinedOutput()
    if err != nil {
        // tmux ls exits non-zero when no server; return empty slice
        return []string{}, nil
    }
    return ParseSessions(string(out), prefix), nil
}

func ParseSessions(output, prefix string) []string {
    lines := strings.Split(output, "\n")
    var sessions []string
    for _, line := range lines {
        line = strings.TrimSpace(line)
        if line == "" {
            continue
        }
        parts := strings.SplitN(line, ":", 2)
        name := strings.TrimSpace(parts[0])
        if strings.HasPrefix(name, prefix) {
            sessions = append(sessions, name)
        }
    }
    return sessions
}
