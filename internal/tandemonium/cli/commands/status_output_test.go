package commands

import "testing"

func TestFormatStatusLines(t *testing.T) {
    lines := formatStatusLines(statusSummary{
        ProjectRoot: "/tmp/project",
        Initialized: true,
    })
    if len(lines) == 0 {
        t.Fatal("expected status lines")
    }
}
