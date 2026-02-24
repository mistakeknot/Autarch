## Goal
Add glamour-based markdown rendering to ChatPanel's message history in `pkg/tui/chatpanel.go`. Agent messages should render their content as styled terminal markdown instead of plain text.

## Phase 1: Explore
Before making any changes, investigate:
- Read `pkg/tui/chatpanel.go` — focus on `renderHistory()` (line 217) and the `ChatMessage` struct (line 26)
- Read `internal/gurgeh/tui/markdown.go` and `internal/coldwine/tui/markdown.go` for existing glamour usage patterns
- Check `go.mod` for glamour version: should be `github.com/charmbracelet/glamour v0.10.0`
- Read `pkg/tui/colors.go` for the Tokyo Night color constants
- Print a brief exploration summary before proceeding

## Phase 2: Implement

### Modify `pkg/tui/chatpanel.go`

1. **Add glamour import**:
```go
import (
    // existing imports...
    "github.com/charmbracelet/glamour"
)
```

2. **Add a markdown renderer cache to ChatPanel struct** (add field after `scroll`):
```go
mdRenderer *glamour.TermRenderer  // Cached markdown renderer
mdWidth    int                     // Width the renderer was created for
```

3. **Add a method to get-or-create the renderer**:
```go
// markdownRenderer returns a cached glamour renderer sized to the current width.
func (p *ChatPanel) markdownRenderer(width int) *glamour.TermRenderer {
    if p.mdRenderer != nil && p.mdWidth == width {
        return p.mdRenderer
    }
    r, err := glamour.NewTermRenderer(
        glamour.WithWordWrap(width),
        glamour.WithStandardStyle("dark"),
    )
    if err != nil {
        return nil
    }
    p.mdRenderer = r
    p.mdWidth = width
    return r
}
```

4. **Modify `renderHistory()`** — change only the content rendering section. Currently (around lines 247-260):
```go
// Content with indent
contentStyle := lipgloss.NewStyle().
    Foreground(ColorFg).
    PaddingLeft(2)

// Wrap content to fit width
contentWidth := p.width - 4
if contentWidth < 10 {
    contentWidth = 10
}
wrapped := wrapText(msg.Content, contentWidth)
for _, line := range strings.Split(wrapped, "\n") {
    lines = append(lines, contentStyle.Render(line))
}
```

Replace with:
```go
// Content rendering — agent messages get markdown, others get plain text
contentWidth := p.width - 4
if contentWidth < 10 {
    contentWidth = 10
}

if strings.ToLower(msg.Role) == "agent" {
    // Render agent messages as markdown via glamour
    if r := p.markdownRenderer(contentWidth); r != nil {
        rendered, err := r.Render(msg.Content)
        if err == nil {
            rendered = strings.TrimSpace(rendered)
            // Indent rendered markdown
            contentStyle := lipgloss.NewStyle().PaddingLeft(2)
            lines = append(lines, contentStyle.Render(rendered))
        } else {
            // Fallback to plain text on render error
            contentStyle := lipgloss.NewStyle().
                Foreground(ColorFg).
                PaddingLeft(2)
            wrapped := wrapText(msg.Content, contentWidth)
            for _, line := range strings.Split(wrapped, "\n") {
                lines = append(lines, contentStyle.Render(line))
            }
        }
    } else {
        // Fallback if renderer creation fails
        contentStyle := lipgloss.NewStyle().
            Foreground(ColorFg).
            PaddingLeft(2)
        wrapped := wrapText(msg.Content, contentWidth)
        for _, line := range strings.Split(wrapped, "\n") {
            lines = append(lines, contentStyle.Render(line))
        }
    }
} else {
    // User and system messages: plain text with word wrap
    contentStyle := lipgloss.NewStyle().
        Foreground(ColorFg).
        PaddingLeft(2)
    wrapped := wrapText(msg.Content, contentWidth)
    for _, line := range strings.Split(wrapped, "\n") {
        lines = append(lines, contentStyle.Render(line))
    }
}
```

**IMPORTANT**: Do NOT modify anything else in chatpanel.go. Only change:
1. The import block (add glamour)
2. The ChatPanel struct (add mdRenderer, mdWidth fields)
3. Add the markdownRenderer() method
4. The content rendering section inside renderHistory()

## Phase 3: Verify
After implementing, run ALL of these and report results:
1. Build: `GOCACHE=/tmp/go-build-cache go build ./pkg/tui/...`
2. Build all: `GOCACHE=/tmp/go-build-cache go build ./cmd/...`
3. Vet: `GOCACHE=/tmp/go-build-cache go vet ./pkg/tui/...`
4. Tests: `GOCACHE=/tmp/go-build-cache go test ./pkg/tui/... -v -short -count=1`
5. Diff: `git diff --stat` (ensure only pkg/tui/chatpanel.go changed)
6. If build or tests fail: fix the issue and re-verify (up to 2 self-retries)

## Final Report
At the end, print a structured verdict:
```
EXPLORATION: [1-2 sentence summary of what you found]
CHANGES: [list files modified/created with brief description]
BUILD: PASS | FAIL
TESTS: PASS | FAIL [N passed, M failed]
VERDICT: CLEAN | NEEDS_ATTENTION [reason]
```

## Constraints
- Only modify `pkg/tui/chatpanel.go`
- Do not reformat, realign, or adjust whitespace in code you didn't functionally change
- Do not add comments, docstrings, or type annotations to unchanged code
- Do not refactor or rename anything not directly related to the task
- Keep the change minimal
- Do NOT commit or push any changes
- If GOCACHE permission errors occur, use: GOCACHE=/tmp/go-build-cache
- Module path is `github.com/mistakeknot/autarch`
