## Goal
Add a thinking/streaming status indicator to ChatPanel. When the agent is thinking or streaming a response, show an animated spinner with a status label (e.g., "Thinking...", "Responding...") below the last message in the history area.

## Phase 1: Explore
Before making any changes, investigate:
- Read `pkg/tui/chatpanel.go` — understand the ChatPanel struct, its fields, the `renderHistory()` method, and the View() layout
- Read `pkg/tui/colors.go` for Tokyo Night color constants
- Check if `github.com/charmbracelet/bubbles/spinner` is available in go.mod (it should be in `bubbles v0.21.0`)
- Read the bubbles spinner docs or source: understand `spinner.New()`, `spinner.Model`, `spinner.Tick`, `spinner.Update`, `spinner.View`
- Check existing spinner/loading patterns in the codebase: `grep -r "spinner\|Spinner\|loading" --include="*.go" pkg/tui/`
- Print a brief exploration summary before proceeding

## Phase 2: Implement

### Modify `pkg/tui/chatpanel.go`

1. **Add imports**:
```go
import (
    "github.com/charmbracelet/bubbles/spinner"
    // ... existing imports
)
```

2. **Add status fields to ChatPanel struct**:
```go
type ChatPanel struct {
    // ... existing fields ...
    spinner   spinner.Model
    status    string // Current status text ("Thinking...", "Responding...", "")
    streaming bool   // Whether the agent is currently streaming
}
```

3. **Initialize spinner in NewChatPanel()**:
```go
func NewChatPanel() *ChatPanel {
    composer := NewComposer(4)
    picker := NewCommandPicker(GlobalCommands())

    s := spinner.New()
    s.Spinner = spinner.Dot
    s.Style = lipgloss.NewStyle().Foreground(ColorPrimary)

    return &ChatPanel{
        messages:      []ChatMessage{},
        composer:      composer,
        commandPicker: picker,
        settings:      DefaultChatSettings(),
        spinner:       s,
    }
}
```

4. **Add public methods to control status**:
```go
// SetStatus sets the streaming status label. Pass empty string to clear.
func (p *ChatPanel) SetStatus(status string) {
    p.status = status
    p.streaming = status != ""
}

// IsStreaming returns whether the agent is currently streaming.
func (p *ChatPanel) IsStreaming() bool {
    return p.streaming
}

// SpinnerTick returns the spinner tick command. Call this from the parent view's Init or when streaming starts.
func (p *ChatPanel) SpinnerTick() tea.Cmd {
    return p.spinner.Tick
}
```

5. **Handle spinner messages in Update()**:
Add at the start of Update() (before the keyMsg check):
```go
// Handle spinner animation
if msg, ok := msg.(spinner.TickMsg); ok && p.streaming {
    var cmd tea.Cmd
    p.spinner, cmd = p.spinner.Update(msg)
    return p, cmd
}
```

6. **Render status indicator in renderHistory()**:
At the end of `renderHistory()`, after building the message lines but before the scrolling section, add:
```go
// Add streaming status indicator
if p.streaming && p.status != "" {
    statusStyle := lipgloss.NewStyle().
        Foreground(ColorPrimary).
        PaddingLeft(2)
    lines = append(lines, statusStyle.Render(p.spinner.View()+" "+p.status))
    lines = append(lines, "") // Blank line after status
}
```

This should be inserted just before the comment `// Apply scrolling - show most recent messages that fit` (which is around line 264 in the current file, though line numbers may have shifted from prior changes).

**IMPORTANT**:
- Do NOT modify any other methods in chatpanel.go besides the ones listed above
- Do NOT touch composer.go — that file is being modified by another agent
- The spinner needs to work with Bubble Tea's message passing. The parent view must call `SpinnerTick()` to start animation and forward `spinner.TickMsg` through `ChatPanel.Update()`.
- Keep the change minimal — this is just the indicator, not the streaming state machine.

## Phase 3: Verify
After implementing, run ALL of these and report results:
1. Build: `GOCACHE=/tmp/go-build-cache go build ./pkg/tui/...`
2. Build all: `GOCACHE=/tmp/go-build-cache go build ./cmd/...`
3. Vet: `GOCACHE=/tmp/go-build-cache go vet ./pkg/tui/...`
4. Tests: `GOCACHE=/tmp/go-build-cache go test ./pkg/tui/... -v -short -count=1`
5. Diff: `git diff --stat` (ensure only expected files changed)
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
- Do NOT modify `pkg/tui/composer.go` (being edited separately)
- Do not reformat, realign, or adjust whitespace in code you didn't functionally change
- Do not add comments, docstrings, or type annotations to unchanged code
- Do not refactor or rename anything not directly related to the task
- Keep the change minimal
- Do NOT commit or push any changes
- If GOCACHE permission errors occur, use: GOCACHE=/tmp/go-build-cache
- Module path is `github.com/mistakeknot/autarch`
