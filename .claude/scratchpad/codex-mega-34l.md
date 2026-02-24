## Goal
Make the Composer textarea auto-expand based on content height. Currently the composer is fixed at 4 lines. It should start at 1 line and grow up to a maximum of 6 lines as the user types multi-line content.

## Phase 1: Explore
Before making any changes, investigate:
- Read `pkg/tui/composer.go` — understand the full Composer struct, NewComposer(), SetSize(), View()
- Read `pkg/tui/chatpanel.go` — understand how ChatPanel creates and sizes the Composer (lines 46, 86-93, 172)
- Check how `bubbles/textarea` handles SetHeight dynamically
- Read the textarea model in the vendor or module cache if needed: the key method is `SetHeight(int)` and `LineCount() int`
- Print a brief exploration summary before proceeding

## Phase 2: Implement

### Modify `pkg/tui/composer.go`

1. **Add min/max height constants**:
```go
const (
    composerMinLines = 1  // Start with single line
    composerMaxLines = 6  // Grow up to 6 lines
)
```

2. **Add a `minContentHeight` field to Composer struct** (replaces the fixed content height concept):
- Remove the `contentHeight` parameter from `NewComposer` — but wait, we can't change the signature because ChatPanel calls `NewComposer(4)`. Instead:
- Keep `NewComposer(contentHeight int)` signature but ignore the param internally and use auto-sizing.
- Actually, the cleanest approach: keep `NewComposer(contentHeight int)` as the max height, and add a minContentHeight field.

Actually, simplest approach:

**Change `NewComposer()`**: The `contentHeight` parameter becomes the maximum. Start the textarea at 1 line:
```go
func NewComposer(maxContentHeight int) *Composer {
    if maxContentHeight < 1 {
        maxContentHeight = 4
    }
    ta := textarea.New()
    ta.Placeholder = "Type your response..."
    ta.CharLimit = 2000
    ta.SetHeight(composerMinLines) // Start small
    ta.ShowLineNumbers = false
    // ... existing style code unchanged ...
    return &Composer{
        textarea:         ta,
        hint:             "enter: send  ctrl+j: newline",
        height:           maxContentHeight + 4,
        maxContentHeight: maxContentHeight,
    }
}
```

3. **Add `maxContentHeight` field to Composer struct**:
```go
type Composer struct {
    textarea         textarea.Model
    title            string
    hint             string
    width            int
    height           int
    focused          bool
    maxContentHeight int  // Maximum lines before scrolling
}
```

4. **Add auto-resize method**:
```go
// autoResize adjusts textarea height based on content.
func (c *Composer) autoResize() {
    lines := c.textarea.LineCount()
    if lines < composerMinLines {
        lines = composerMinLines
    }
    if lines > c.maxContentHeight {
        lines = c.maxContentHeight
    }
    c.textarea.SetHeight(lines)
}
```

5. **Call `autoResize()` in `Update()` after the textarea update**:
```go
func (c *Composer) Update(msg tea.Msg) (*Composer, tea.Cmd) {
    var cmd tea.Cmd
    if keyMsg, ok := msg.(tea.KeyMsg); ok && keyMsg.Type == tea.KeyRunes {
        if isMouseEscapeSequence(keyMsg.String()) {
            return c, nil
        }
    }
    c.textarea, cmd = c.textarea.Update(msg)
    c.autoResize()
    return c, cmd
}
```

6. **Call `autoResize()` in `Reset()`**:
```go
func (c *Composer) Reset() {
    c.textarea.Reset()
    c.autoResize()
}
```

7. **Call `autoResize()` in `SetValue()`**:
```go
func (c *Composer) SetValue(s string) {
    c.textarea.SetValue(s)
    c.autoResize()
}
```

8. **Update `SetSize()`** — the total height calculation needs to account for dynamic content:
```go
func (c *Composer) SetSize(width, height int) {
    c.width = width
    c.height = height
    textareaWidth := width - 4
    if textareaWidth < 10 {
        textareaWidth = 10
    }
    c.textarea.SetWidth(textareaWidth)
    c.autoResize() // Let auto-resize handle the height
}
```

### Modify `pkg/tui/chatpanel.go`

9. **Update ChatPanel.SetSize()** — the composer height is no longer fixed at 8:
Currently (line 91):
```go
composerHeight := 8 // 4 lines content + borders/decorations
p.composer.SetSize(width, composerHeight)
```
Change to:
```go
p.composer.SetSize(width, 0) // Height managed by auto-resize
```

Actually, we need to be careful. The Composer.SetSize currently controls the max available space. With auto-expanding, we just need to set the width and let `autoResize` handle height. Change to:
```go
p.composer.SetSize(width, 12) // Max available space for composer
```
The 12 gives room for 6 content lines + 4 decorations + 2 buffer.

10. **Update ChatPanel.View()** — the composerHeight constant needs to become dynamic:
Currently (line 172):
```go
composerHeight := 8
historyHeight := p.height - composerHeight - 1 - selectorHeight
```
Change to use actual rendered height:
```go
composerView := p.composer.View()
composerHeight := lipgloss.Height(composerView)
historyHeight := p.height - composerHeight - 1 - selectorHeight
```
And move `composerView` rendering up before historyHeight calculation, then use the pre-rendered composerView instead of calling p.composer.View() again later.

## Phase 3: Verify
After implementing, run ALL of these and report results:
1. Build: `GOCACHE=/tmp/go-build-cache go build ./pkg/tui/...`
2. Build all: `GOCACHE=/tmp/go-build-cache go build ./cmd/...`
3. Vet: `GOCACHE=/tmp/go-build-cache go vet ./pkg/tui/...`
4. Tests: `GOCACHE=/tmp/go-build-cache go test ./pkg/tui/... -v -short -count=1`
5. Diff: `git diff --stat` (ensure only pkg/tui/composer.go and pkg/tui/chatpanel.go changed)
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
- Only modify `pkg/tui/composer.go` and `pkg/tui/chatpanel.go`
- Do not reformat, realign, or adjust whitespace in code you didn't functionally change
- Do not add comments, docstrings, or type annotations to unchanged code
- Do not refactor or rename anything not directly related to the task
- Keep the change minimal
- Do NOT commit or push any changes
- If GOCACHE permission errors occur, use: GOCACHE=/tmp/go-build-cache
- Module path is `github.com/mistakeknot/autarch`
